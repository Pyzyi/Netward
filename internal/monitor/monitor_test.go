package monitor

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/l2"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/proto"
	"github.com/Pyzyi/netzwart/internal/software"
	"github.com/Pyzyi/netzwart/internal/store"
)

type recorder struct {
	mu     sync.Mutex
	events []notify.Event
}

func (r *recorder) Notify(e notify.Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}

func (r *recorder) kinds() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int{}
	for _, e := range r.events {
		out[e.Kind]++
	}
	return out
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type fakeProber struct{ alive bool }

func (p fakeProber) Probe(context.Context, store.Device) bool { return p.alive }

type env struct {
	m   *Monitor
	st  *store.Store
	rec *recorder
	clk *clock
	ctx context.Context
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return newEnvWithStore(t, st)
}

func newEnvWithStore(t *testing.T, st *store.Store) *env {
	t.Helper()
	ctx := context.Background()
	rec := &recorder{}
	clk := &clock{t: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)}
	m, err := New(ctx, st, hub.New(), rec, config.Defaults(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	m.SetClock(clk.Now)
	m.SetNetwork([]netip.Addr{netip.MustParseAddr("192.168.1.1")}, []net.HardwareAddr{mac(0xee)})
	return &env{m: m, st: st, rec: rec, clk: clk, ctx: ctx}
}

func mac(last byte) net.HardwareAddr { return net.HardwareAddr{0x3c, 0x22, 0xfb, 0, 0, last} }
func ip(last byte) netip.Addr        { return netip.AddrFrom4([4]byte{192, 168, 1, last}) }

func (e *env) sweep(entries map[byte][]byte) {
	res := l2.SweepResult{}
	for i, macs := range entries {
		for _, m := range macs {
			res[ip(i)] = append(res[ip(i)], mac(m))
		}
	}
	e.m.ApplySweep(e.ctx, "eth0", res, e.clk.Now())
}

func (e *env) events(t *testing.T, kind string) []store.Event {
	t.Helper()
	evs, err := e.st.ListEvents(e.ctx, store.EventQuery{Kinds: []string{kind}})
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestLearningPhaseThenNewDevice(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{1: {1}, 10: {10}, 20: {20}})
	if n := len(e.m.Devices()); n != 3 {
		t.Fatalf("got %d devices, want 3", n)
	}
	if len(e.rec.kinds()) != 0 || len(e.events(t, notify.KindDeviceNew)) != 0 {
		t.Fatal("first scan must not report new devices")
	}
	if len(e.events(t, KindScanBaseline)) != 1 {
		t.Fatal("baseline event missing")
	}
	for _, d := range e.m.Devices() {
		if d.New {
			t.Fatalf("baseline device flagged new: %+v", d)
		}
		if d.IP == "192.168.1.1" && (!d.IsGateway || d.Type != "router") {
			t.Fatalf("gateway not recognised: %+v", d)
		}
	}
	e.clk.Add(time.Minute)
	e.sweep(map[byte][]byte{1: {1}, 10: {10}, 20: {20}, 30: {30}})
	if e.rec.kinds()[notify.KindDeviceNew] != 1 {
		t.Fatalf("expected one new-device notification, got %v", e.rec.kinds())
	}
	var found bool
	for _, d := range e.m.Devices() {
		if d.IP == "192.168.1.30" {
			found = d.New && d.Vendor == "Apple"
		}
	}
	if !found {
		t.Fatal("new device not flagged as new with vendor")
	}
	if _, err := e.m.AcknowledgeAll(e.ctx); err != nil {
		t.Fatal(err)
	}
	for _, d := range e.m.Devices() {
		if d.New {
			t.Fatal("acknowledged device still new")
		}
	}
}

func TestDHCPStashAppliedOnARP(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{1: {1}})
	dhcp := &proto.DHCP{Op: 1, MsgType: proto.DHCPRequest, ClientMAC: mac(5), Hostname: "Annas-iPhone",
		ParamList: []byte{1, 121, 3, 6, 15, 108, 114, 119, 252}}
	e.m.Observe(e.ctx, l2.Sighting{MAC: mac(5), Iface: "eth0", Source: "dhcp", DHCP: dhcp, At: e.clk.Now()})
	if len(e.m.Devices()) != 1 {
		t.Fatal("DHCP alone must not create a device")
	}
	e.m.Observe(e.ctx, l2.Sighting{MAC: mac(5), IP: ip(5), Iface: "eth0", Source: "arp", ARP: true, At: e.clk.Now()})
	d, ok := e.m.DeviceByIP(ip(5))
	if !ok || d.Hostname != "Annas-iPhone" || d.Type != "phone" || d.OS != "iOS" {
		t.Fatalf("stashed DHCP info not applied: %+v", d)
	}
}

func TestOfflineAfterThresholdAndMobile(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{10: {10}})
	d, _ := e.m.DeviceByIP(ip(10))
	yes := true
	if _, err := e.m.PatchDevice(e.ctx, d.ID, store.DevicePatch{Watch: &yes}); err != nil {
		t.Fatal(err)
	}
	e.clk.Add(4 * time.Minute)
	e.m.SweepOffline(e.ctx)
	if d, _ := e.m.Device(d.ID); !d.Online {
		t.Fatal("offline too early")
	}
	e.clk.Add(2 * time.Minute)
	e.m.SweepOffline(e.ctx)
	if d, _ := e.m.Device(d.ID); d.Online {
		t.Fatal("device should be offline after 6 minutes")
	}
	if e.rec.kinds()[notify.KindDeviceOffline] != 1 {
		t.Fatal("watched device going offline must notify")
	}
	spans, _ := e.st.ListPresence(e.ctx, d.ID, 0, e.clk.Now().UnixMilli())
	if len(spans) != 1 || spans[0].End == 0 {
		t.Fatalf("presence span not closed: %+v", spans)
	}
	// Back online: new span, online notification.
	e.sweep(map[byte][]byte{10: {10}})
	if d, _ := e.m.Device(d.ID); !d.Online {
		t.Fatal("device should be online again")
	}
	if e.rec.kinds()[notify.KindDeviceOnline] != 1 {
		t.Fatal("online notification missing")
	}

	// Phones get the longer mobile threshold.
	phone := "phone"
	e.m.PatchDevice(e.ctx, d.ID, store.DevicePatch{TypeOverride: &phone})
	e.clk.Add(10 * time.Minute)
	e.m.SweepOffline(e.ctx)
	if d, _ := e.m.Device(d.ID); !d.Online {
		t.Fatal("phone went offline before the mobile threshold")
	}
}

func TestProbeKeepsDeviceOnline(t *testing.T) {
	e := newEnv(t)
	e.m.SetProber(fakeProber{alive: true})
	e.sweep(map[byte][]byte{10: {10}})
	d, _ := e.m.DeviceByIP(ip(10))
	e.clk.Add(10 * time.Minute)
	e.m.SweepOffline(e.ctx)
	// The probe runs asynchronously.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		e.m.mu.Lock()
		probing := e.m.byID[d.ID].probing
		e.m.mu.Unlock()
		if !probing {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if d, _ := e.m.Device(d.ID); !d.Online {
		t.Fatal("device answering the probe went offline")
	}
}

func TestProxyRepeater(t *testing.T) {
	e := newEnv(t)
	// MAC 50 answers for three addresses: a MAC-NAT repeater.
	e.sweep(map[byte][]byte{50: {50}, 51: {50}, 52: {50}})
	devs := e.m.Devices()
	if len(devs) != 3 {
		t.Fatalf("got %d devices, want 3 (one per IP behind the repeater)", len(devs))
	}
	proxies, via := 0, 0
	for _, d := range devs {
		if d.Proxy {
			proxies++
		}
		if d.ViaProxy {
			via++
		}
	}
	if proxies != 1 || via != 2 {
		t.Fatalf("proxy=%d via=%d, want 1 and 2", proxies, via)
	}
	// Repeated sweeps must not create more devices or IP-change events.
	e.clk.Add(time.Minute)
	e.sweep(map[byte][]byte{50: {50}, 51: {50}, 52: {50}})
	if len(e.m.Devices()) != 3 || len(e.events(t, KindIPChanged)) != 0 {
		t.Fatal("repeater caused churn")
	}
}

func TestIPConflictNeedsTwoSweeps(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{40: {40, 41}})
	if len(e.events(t, notify.KindIPConflict)) != 0 {
		t.Fatal("conflict reported after a single sweep")
	}
	e.clk.Add(time.Minute)
	e.sweep(map[byte][]byte{40: {40, 41}})
	e.clk.Add(time.Minute)
	e.sweep(map[byte][]byte{40: {40, 41}})
	if n := len(e.events(t, notify.KindIPConflict)); n != 1 {
		t.Fatalf("got %d conflict events, want exactly 1", n)
	}
}

func TestFloodProtection(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{1: {1}})
	for i := 0; i < 100; i++ {
		m := net.HardwareAddr{0x3c, 0x55, 0, 0, byte(i >> 8), byte(i)}
		e.m.Observe(e.ctx, l2.Sighting{MAC: m, IP: ip(byte(100 + i%100)), Iface: "eth0", Source: "arp", ARP: true, At: e.clk.Now()})
	}
	if n := len(e.m.Devices()); n > 1+admissionPerMinute {
		t.Fatalf("%d devices admitted, limit is %d per minute", n-1, admissionPerMinute)
	}
	if len(e.events(t, notify.KindDeviceFlood)) != 1 {
		t.Fatal("flood event missing")
	}
	// After a minute new devices are admitted again.
	e.clk.Add(time.Minute)
	e.m.Observe(e.ctx, l2.Sighting{MAC: net.HardwareAddr{0x3c, 0x66, 0, 0, 0, 1}, IP: ip(250), Iface: "eth0", Source: "arp", ARP: true, At: e.clk.Now()})
	if _, ok := e.m.DeviceByIP(ip(250)); !ok {
		t.Fatal("admission did not recover")
	}
}

func TestIPChangeWaitsForOldAddress(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{60: {60}})
	d, _ := e.m.DeviceByIP(ip(60))
	// Secondary address while the old one is still active: keep primary.
	e.m.Observe(e.ctx, l2.Sighting{MAC: mac(60), IP: ip(61), Iface: "eth0", Source: "arp", ARP: true, At: e.clk.Now()})
	if d2, _ := e.m.Device(d.ID); d2.IP != "192.168.1.60" {
		t.Fatalf("primary switched too early: %s", d2.IP)
	}
	// Old address silent beyond the threshold: switch and log once.
	e.clk.Add(6 * time.Minute)
	e.m.Observe(e.ctx, l2.Sighting{MAC: mac(60), IP: ip(61), Iface: "eth0", Source: "arp", ARP: true, At: e.clk.Now()})
	if d2, _ := e.m.Device(d.ID); d2.IP != "192.168.1.61" {
		t.Fatalf("primary did not switch: %s", d2.IP)
	}
	// A DHCP ACK is authoritative and switches immediately.
	e.m.Observe(e.ctx, l2.Sighting{MAC: mac(60), IP: ip(62), Iface: "eth0", Source: "dhcp-ack", At: e.clk.Now(),
		DHCP: &proto.DHCP{Op: 2, MsgType: proto.DHCPAck, ClientMAC: mac(60), YourIP: ip(62)}})
	if d2, _ := e.m.Device(d.ID); d2.IP != "192.168.1.62" {
		t.Fatalf("DHCP ACK did not switch: %s", d2.IP)
	}
	if n := len(e.events(t, KindIPChanged)); n != 2 {
		t.Fatalf("got %d ip_changed events, want 2", n)
	}
}

func TestGapRecovery(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	e := newEnvWithStore(t, st)
	e.sweep(map[byte][]byte{10: {10}})
	d, _ := e.m.DeviceByIP(ip(10))
	yes := true
	e.m.PatchDevice(ctx, d.ID, store.DevicePatch{Watch: &yes})
	e.m.flush(ctx)
	// Simulate a heartbeat from one hour ago (process was down since).
	old := time.Now().Add(-time.Hour).UnixMilli()
	_ = st.SetSetting(ctx, "heartbeat", old)
	e2 := newEnvWithStore(t, st)
	d2, _ := e2.m.Device(d.ID)
	if d2.Online {
		t.Fatal("device still online after a monitoring gap")
	}
	gaps, _ := st.ListGaps(ctx, 0, time.Now().UnixMilli())
	if len(gaps) != 1 || gaps[0].Start != old {
		t.Fatalf("gap not recorded: %+v", gaps)
	}
	spans, _ := st.ListPresence(ctx, d.ID, 0, time.Now().UnixMilli())
	if len(spans) != 1 || spans[0].End == 0 {
		t.Fatalf("open span not closed: %+v", spans)
	}
	// Review 2: a watched device that is still there is not reported as "came online".
	e2.sweep(map[byte][]byte{10: {10}})
	if d2, _ := e2.m.Device(d.ID); !d2.Online {
		t.Fatal("device not online again")
	}
	if e2.rec.kinds()[notify.KindDeviceOnline] != 0 {
		t.Fatal("restart produced an online notification")
	}
}

// Review 2: a wall clock that jumps back must not keep devices online.
func TestClockJumpBack(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{10: {10}})
	d, _ := e.m.DeviceByIP(ip(10))
	e.clk.Add(-2 * time.Hour)
	e.m.SweepOffline(e.ctx)
	e.clk.Add(time.Hour)
	e.m.SweepOffline(e.ctx)
	if d, _ := e.m.Device(d.ID); d.Online {
		t.Fatal("device silent for an hour is still online after the clock jumped back")
	}
}

func TestSelfAndMACKinds(t *testing.T) {
	e := newEnv(t)
	e.m.ObserveSelf(e.ctx, "eth0", ip(2), mac(0xee))
	d, ok := e.m.DeviceByIP(ip(2))
	if !ok || !d.IsSelf || d.Type != "server" {
		t.Fatalf("self not recognised: %+v", d)
	}
	cases := map[string]string{"02:42:ac:11:00:02": "virtual", "52:54:00:12:34:56": "virtual",
		"da:a1:19:00:00:01": "random", "3c:22:fb:00:00:01": "universal"}
	for s, want := range cases {
		m, _ := net.ParseMAC(s)
		if got := MACKind(m); got != want {
			t.Errorf("MACKind(%s) = %s, want %s", s, got, want)
		}
	}
}

func TestPatchValidationAndMerge(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{10: {10}, 11: {11}})
	a, _ := e.m.DeviceByIP(ip(10))
	b, _ := e.m.DeviceByIP(ip(11))
	bad := "toaster"
	if _, err := e.m.PatchDevice(e.ctx, a.ID, store.DevicePatch{TypeOverride: &bad}); err != ErrInvalid {
		t.Fatalf("invalid type accepted: %v", err)
	}
	name := "  Küche\u202e  "
	got, err := e.m.PatchDevice(e.ctx, a.ID, store.DevicePatch{Name: &name})
	if err != nil || got.Name != "Küche" {
		t.Fatalf("name not cleaned: %q %v", got.Name, err)
	}
	merged, err := e.m.MergeDevices(e.ctx, a.ID, b.ID)
	if err != nil || merged.Name != "Küche" {
		t.Fatalf("merge failed: %+v %v", merged, err)
	}
	if _, ok := e.m.Device(a.ID); ok {
		t.Fatal("merged source still present")
	}
}

func TestGatewayFlagAppliedLater(t *testing.T) {
	e := newEnv(t)
	e.m.SetNetwork(nil, nil)
	// The router is first seen passively, before the routing table was read.
	e.m.Observe(e.ctx, l2.Sighting{MAC: mac(1), IP: ip(1), Iface: "eth0", Source: "arp", ARP: true, At: e.clk.Now()})
	if d, _ := e.m.DeviceByIP(ip(1)); d.IsGateway {
		t.Fatal("gateway flag set without routing information")
	}
	e.m.SetNetwork([]netip.Addr{ip(1)}, nil)
	d, _ := e.m.DeviceByIP(ip(1))
	if !d.IsGateway || d.Type != "router" {
		t.Fatalf("gateway not updated: %+v", d)
	}
}

func TestRouterHostsNameAndWLANPresence(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{10: {10}, 11: {11}})
	phone, _ := e.m.DeviceByIP(ip(10))
	pc, _ := e.m.DeviceByIP(ip(11))
	e.clk.Add(20 * time.Minute)
	e.m.SweepOffline(e.ctx)
	if d, _ := e.m.Device(phone.ID); d.Online {
		t.Fatal("precondition: device should be offline")
	}
	now := e.clk.Now().UnixMilli()
	n := e.m.ApplyRouterHosts(e.ctx, []RouterHost{
		{MAC: mac(10).String(), IP: ip(10), Name: "Annas iPhone", Active: true, WLAN: true},
		{MAC: mac(11).String(), IP: ip(11), Name: "", Active: true, WLAN: false}, // wired: no presence evidence
		{MAC: mac(99).String(), IP: ip(99), Name: "Altgerät", Active: true, WLAN: true},
	}, now)
	if n != 2 {
		t.Fatalf("matched %d, want 2", n)
	}
	d, _ := e.m.Device(phone.ID)
	if !d.Online || d.Hostname != "Annas iPhone" || d.LastSeen != now {
		t.Fatalf("phone not updated: online=%v host=%q", d.Online, d.Hostname)
	}
	if d, _ := e.m.Device(pc.ID); d.Online {
		t.Fatal("wired host marked online from the router table")
	}
	if _, ok := e.m.DeviceByIP(ip(99)); ok {
		t.Fatal("device created from the router table")
	}
}

func TestSoftwareMergeAndPlausibility(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{20: {0x20}, 21: {0x21}})
	var a, b store.Device
	for _, d := range e.m.Devices() {
		switch d.IP {
		case ip(20).String():
			a = d
		case ip(21).String():
			b = d
		}
	}
	hooked := 0
	e.m.SetSoftwareHook(func() { hooked++ })
	if err := e.m.SetSoftwareManual(e.ctx, a.ID, software.IOS, "16"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.m.MergeDevices(e.ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	sw, _ := e.m.Software(b.ID)
	if sw.Status != software.StatusOutdated || sw.Source != software.SourceManual {
		t.Fatalf("manual entry lost in the merge: %+v", sw)
	}
	if hooked < 2 {
		t.Fatalf("hook not run after the merge (%d)", hooked)
	}
	// A FRITZ!OS announcement for a device that is neither AVM nor the gateway is ignored.
	e.m.AddSoftware(b.ID, software.FromSSDPServer("FRITZ!Box 7390 UPnP/1.0 AVM FRITZ!Box 7390 84.06.83"))
	if sw, _ := e.m.Software(b.ID); sw.Product == software.FritzOS {
		t.Fatalf("spoofed FRITZ!OS accepted: %+v", sw)
	}
}
