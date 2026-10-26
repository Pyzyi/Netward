package security

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/l2"
	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/proto"
	"github.com/Pyzyi/netzwart/internal/store"
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
