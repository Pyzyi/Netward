package security

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Pyzyi/Netward/internal/config"
	"github.com/Pyzyi/Netward/internal/dnscheck/dnschecktest"
	"github.com/Pyzyi/Netward/internal/hub"
	"github.com/Pyzyi/Netward/internal/l2"
	"github.com/Pyzyi/Netward/internal/monitor"
	"github.com/Pyzyi/Netward/internal/notify"
	"github.com/Pyzyi/Netward/internal/proto"
	"github.com/Pyzyi/Netward/internal/store"
	"github.com/Pyzyi/Netward/internal/tripwire"
)

type rec struct{ evs []notify.Event }

func (r *rec) Notify(e notify.Event) { r.evs = append(r.evs, e) }

func TestDescribe(t *testing.T) {
	title, detail := Describe(CheckRiskyPort, map[string]any{"service": "telnet"}, "de")
	if title != "Riskanter Dienst erreichbar: TELNET" || detail == "" {
		t.Fatalf("%q %q", title, detail)
	}
	if title, _ := Describe(CheckUPnP, nil, "en"); title != "UPnP enabled on the router" {
		t.Fatal(title)
	}
}

func TestRunFindsUnknownDevicesAndPrunes(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hub.New()
	mon, err := monitor.New(ctx, st, h, nil, config.Defaults(), log)
	if err != nil {
		t.Fatal(err)
	}
	// TEST-NET addresses never answer, so no risky ports are found.
	for i := byte(1); i <= 6; i++ {
		mon.Observe(ctx, l2.Sighting{MAC: net.HardwareAddr{0x3c, 0, 0, 0, 0, i}, IP: netip.AddrFrom4([4]byte{192, 0, 2, i}),
			Iface: "eth0", Source: "arp", ARP: true, At: time.Now()})
	}
	r := &rec{}
	c := New(st, mon, h, r, log)
	c.run(ctx)
	fs, _ := st.ListFindings(ctx)
	if len(fs) != 1 || fs[0].CheckID != CheckUnknown || fs[0].Severity != "warning" {
		t.Fatalf("unexpected findings %+v", fs)
	}
	if len(r.evs) != 0 {
		t.Fatal("unknown devices must not trigger a notification")
	}
	if c.Status().LastRunAt == 0 {
		t.Fatal("last run not recorded")
	}
	// After acknowledging everything the finding disappears on the next run.
	if _, err := mon.AcknowledgeAll(ctx); err != nil {
		t.Fatal(err)
	}
	c.run(ctx)
	fs, _ = st.ListFindings(ctx)
	if len(fs) != 0 {
		t.Fatalf("stale finding not pruned: %+v", fs)
	}
}

type fakeProber struct {
	calls  int
	chaddr net.HardwareAddr
	answer []l2.DHCPAnswer
}

func (p *fakeProber) ProbeDHCP(_ context.Context, chaddr net.HardwareAddr, _ time.Duration) ([]l2.DHCPAnswer, bool) {
	p.calls++
	p.chaddr = chaddr
	return p.answer, true
}

func TestActiveDHCPCheck(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "s.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hub.New()
	mon, err := monitor.New(ctx, st, h, nil, config.Defaults(), log)
	if err != nil {
		t.Fatal(err)
	}
	mon.SetNetwork([]netip.Addr{netip.MustParseAddr("192.168.1.1")}, nil)
	answer := func(mac byte, server, router string) l2.DHCPAnswer {
		ip := netip.MustParseAddr(server)
		return l2.DHCPAnswer{MAC: net.HardwareAddr{0x3c, 0, 0, 0, 0, mac}, IP: ip, DHCP: proto.DHCP{Op: 2, MsgType: proto.DHCPOffer,
			ServerID: ip, Router: netip.MustParseAddr(router), DNS: []netip.Addr{netip.MustParseAddr(router)}}}
	}
	p := &fakeProber{answer: []l2.DHCPAnswer{answer(1, "192.168.1.1", "192.168.1.1"), answer(9, "192.168.1.9", "192.168.1.9")}}
	c := New(st, mon, h, &rec{}, log)
	c.SetDHCPProber(p)

	c.run(ctx) // switched off: no request is sent
	if p.calls != 0 {
		t.Fatal("probe sent although the check is off")
	}
	c.SetDHCPCheck(true)
	c.run(ctx)
	if p.calls != 1 || len(p.chaddr) != 6 || p.chaddr[0]&0x03 != 0x02 {
		t.Fatalf("probe calls %d with chaddr %v, want one with a local unicast MAC", p.calls, p.chaddr)
	}
	if n := mon.Network(); n.Probe == nil || n.Probe.Answers != 2 || len(n.DHCPServers) != 2 {
		t.Fatalf("network state %+v", n)
	}
	c.EvaluateNetwork(ctx)
	fs, _ := st.ListFindings(ctx)
	if len(fs) != 1 || fs[0].CheckID != CheckDHCPRogue || fs[0].Severity != "critical" {
		t.Fatalf("findings %+v", fs)
	}
	// The probe MAC stays the same for the installation.
	mon2, err := monitor.New(ctx, st, h, nil, config.Defaults(), log)
	if err != nil {
		t.Fatal(err)
	}
	if mon2.DHCPProbeMAC().String() != p.chaddr.String() {
		t.Fatalf("probe MAC changed: %v -> %v", p.chaddr, mon2.DHCPProbeMAC())
	}
}

// Review 8: a rogue server first seen without a reference is only "info";
// once the router's server is known it becomes critical and must alert.
func TestRogueDHCPEscalationAlerts(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hub.New()
	mon, err := monitor.New(ctx, st, h, nil, config.Defaults(), log)
	if err != nil {
		t.Fatal(err)
	}
	gw := netip.MustParseAddr("192.168.1.1")
	mon.SetNetwork([]netip.Addr{gw}, nil)
	offer := func(server, router, dns netip.Addr) {
		d := &proto.DHCP{Op: 2, MsgType: proto.DHCPOffer, ServerID: server, Router: router, DNS: []netip.Addr{dns}}
		mon.Observe(ctx, l2.Sighting{MAC: net.HardwareAddr{0x3c, 0, 0, 0, 0, server.As4()[3]}, IP: server, Iface: "eth0",
			Source: "dhcp-server", DHCP: d, At: time.Now()})
	}
	r := &rec{}
	c := New(st, mon, h, r, log)
	rogue := netip.MustParseAddr("192.168.1.66")
	offer(rogue, gw, netip.MustParseAddr("203.0.113.53"))
	offer(netip.MustParseAddr("192.168.1.67"), gw, gw)
	c.EvaluateNetwork(ctx)
	if len(r.evs) != 0 {
		t.Fatalf("alert without a reference: %+v", r.evs)
	}
	offer(gw, gw, gw)
	c.EvaluateNetwork(ctx)
	if len(r.evs) != 2 || r.evs[0].Severity == "info" {
		t.Fatalf("escalation did not alert: %+v", r.evs)
	}
	// A server naming a router that is not ours is at least a warning,
	// even when there is nothing to compare with.
	st2, _ := store.Open(ctx, filepath.Join(t.TempDir(), "s2.db"))
	defer st2.Close()
	mon2, _ := monitor.New(ctx, st2, h, nil, config.Defaults(), log)
	mon2.SetNetwork([]netip.Addr{gw}, nil)
	for _, s := range []netip.Addr{netip.MustParseAddr("192.168.1.70"), netip.MustParseAddr("192.168.1.71")} {
		d := &proto.DHCP{Op: 2, MsgType: proto.DHCPOffer, ServerID: s, Router: s}
		mon2.Observe(ctx, l2.Sighting{MAC: net.HardwareAddr{0x3c, 0, 0, 0, 1, s.As4()[3]}, IP: s, Iface: "eth0", Source: "dhcp-server", DHCP: d, At: time.Now()})
	}
	r2 := &rec{}
	New(st2, mon2, h, r2, log).EvaluateNetwork(ctx)
	if len(r2.evs) != 2 || r2.evs[0].Severity != "warning" {
		t.Fatalf("foreign router without reference: %+v", r2.evs)
	}
}

func TestDNSCheckFindsManipulatedServer(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hub.New()
	mon, err := monitor.New(ctx, st, h, nil, config.Defaults(), log)
	if err != nil {
		t.Fatal(err)
	}
	mon.SetNetwork([]netip.Addr{netip.MustParseAddr("192.0.2.1")}, nil)
	bad := map[string]string{"one.one.one.one": "1.1.1.1", "dns.google": "203.0.113.9", "dns.quad9.net": "9.9.9.9"}
	srv := dnschecktest.Server(t, bad, "")
	r := &rec{}
	c := New(st, mon, h, r, log)
	c.SetDNSCheck(true)
	c.dnsTestPort = srv.Port()
	c.dnsDial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, srv.String())
	}
	c.run(ctx)
	fs, _ := st.ListFindings(ctx)
	var dns []store.Finding
	for _, f := range fs {
		if f.CheckID == CheckDNSManipulated {
			dns = append(dns, f)
		}
	}
	if len(dns) != 1 || dns[0].Severity != "critical" || string(dns[0].Data) != `{"server":"192.0.2.1"}` {
		t.Fatalf("findings %+v", fs)
	}
	if n := mon.Network().DNS; n == nil || len(n.Results) != 1 || n.Results[0].Answers[1].Got[0] != "203.0.113.9" {
		t.Fatalf("stored check %+v", n)
	}
	alerted := false
	for _, e := range r.evs {
		if title, _ := e.Data["title"].(string); strings.HasPrefix(title, "DNS-Server liefert falsche Adressen") {
			alerted = true
		}
	}
	if !alerted {
		t.Fatal("no notification for a manipulating DNS server")
	}
	// Switched off: the finding goes away on the next run.
	c.SetDNSCheck(false)
	c.run(ctx)
	fs, _ = st.ListFindings(ctx)
	for _, f := range fs {
		if f.CheckID == CheckDNSManipulated {
			t.Fatal("DNS finding kept after the check was switched off")
		}
	}
}

func TestTripwireOneFindingPerDevice(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hub.New()
	mon, err := monitor.New(ctx, st, h, nil, config.Defaults(), log)
	if err != nil {
		t.Fatal(err)
	}
	r := &rec{}
	c := New(st, mon, h, r, log)
	ip := netip.MustParseAddr("192.168.1.66")
	at := time.Now().Add(-31 * 24 * time.Hour) // a port scan a month ago
	c.Tripwire(ctx, tripwire.Hit{IP: ip, Port: 2323, Service: "telnet", At: at})
	c.Tripwire(ctx, tripwire.Hit{IP: ip, Port: 3389, Service: "rdp", At: at.Add(time.Second)})
	fs, _ := st.ListFindings(ctx)
	if len(fs) != 1 || fs[0].CheckID != CheckTripwire || fs[0].Severity != "critical" || string(fs[0].Data) != `{"ip":"192.168.1.66"}` {
		t.Fatalf("findings %+v", fs)
	}
	if len(r.evs) != 1 || !strings.Contains(r.evs[0].Data["detail"].(string), "TELNET") {
		t.Fatalf("notifications %+v", r.evs)
	}
	evs, _ := st.ListEvents(ctx, store.EventQuery{Kinds: []string{notify.KindSecurity}, Limit: 10})
	if len(evs) != 2 {
		t.Fatalf("events = %d, want one per hit", len(evs))
	}
	c.pruneTripwire(ctx)
	if fs, _ := st.ListFindings(ctx); len(fs) != 0 {
		t.Fatalf("old tripwire finding kept: %+v", fs)
	}
}

func TestTripwireUnknownSourcesAreBounded(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hub.New()
	mon, err := monitor.New(ctx, st, h, nil, config.Defaults(), log)
	if err != nil {
		t.Fatal(err)
	}
	r := &rec{}
	c := New(st, mon, h, r, log)
	for i := range 40 {
		c.Tripwire(ctx, tripwire.Hit{IP: netip.AddrFrom4([4]byte{10, 0, 0, byte(i + 1)}), Port: 2323, Service: "telnet", At: time.Now()})
	}
	// Privacy addresses of one IPv6 host are one source.
	c.Tripwire(ctx, tripwire.Hit{IP: netip.MustParseAddr("fd00:1::1"), Port: 2323, Service: "telnet", At: time.Now()})
	fs, _ := st.ListFindings(ctx)
	if len(fs) != maxUnknownTripwire+1 {
		t.Fatalf("findings = %d, want %d", len(fs), maxUnknownTripwire+1)
	}
	if len(r.evs) != maxUnknownTripwire+1 {
		t.Fatalf("notifications = %d", len(r.evs))
	}
}
