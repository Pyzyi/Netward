package internet

import (
	"context"
	"io"
	"log/slog"
	"net/netip"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Pyzyi/Netward/internal/egress"
	"github.com/Pyzyi/Netward/internal/fritzbox"
	"github.com/Pyzyi/Netward/internal/hub"
	"github.com/Pyzyi/Netward/internal/notify"
	"github.com/Pyzyi/Netward/internal/store"
)

type rec struct {
	mu    sync.Mutex
	kinds []string
}

func (r *rec) Notify(e notify.Event) { r.mu.Lock(); r.kinds = append(r.kinds, e.Kind); r.mu.Unlock() }
func (r *rec) AddEvent(_ context.Context, kind, _ string, _ int64, _ map[string]any) {
	r.mu.Lock()
	r.kinds = append(r.kinds, "log:"+kind)
	r.mu.Unlock()
}

func TestOutageStateMachine(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := &rec{}
	m := New(st, hub.New(), r, r, nil, &egress.Policy{}, func() []netip.Addr { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.Configure(true, []string{"192.0.2.1:443"})
	now := time.Now()
	m.record(ctx, now, StateOnline, 12*time.Millisecond, "", true)
	m.record(ctx, now.Add(30*time.Second), StateOffline, -1, "", true)
	if len(r.kinds) != 0 {
		t.Fatal("a single failure must not start an outage")
	}
	m.record(ctx, now.Add(60*time.Second), StateOffline, -1, "", true)
	m.record(ctx, now.Add(90*time.Second), StateOffline, -1, "", true)
	m.record(ctx, now.Add(5*time.Minute), StateOnline, 15*time.Millisecond, "", true)
	want := []string{"log:internet_down", "internet_down", "log:internet_up", "internet_up"}
	if len(r.kinds) != len(want) {
		t.Fatalf("got %v, want %v", r.kinds, want)
	}
	for i := range want {
		if r.kinds[i] != want[i] {
			t.Fatalf("got %v, want %v", r.kinds, want)
		}
	}
	outs, _ := st.ListOutages(ctx, 0)
	if len(outs) != 1 || outs[0].End == 0 || outs[0].Start != now.Add(30*time.Second).UnixMilli() {
		t.Fatalf("unexpected outages %+v", outs)
	}
	if s := m.Status(); s.State != StateOnline || s.LatencyMS != 15 {
		t.Fatalf("unexpected status %+v", s)
	}
	if len(m.History()) != 5 {
		t.Fatal("history not recorded")
	}
}

func TestDisabled(t *testing.T) {
	m := New(nil, hub.New(), &rec{}, &rec{}, nil, &egress.Policy{}, func() []netip.Addr { return nil }, slog.Default())
	m.Configure(false, nil)
	m.Check(context.Background())
	if m.Status().State != StateDisabled {
		t.Fatal("disabled monitor must report disabled")
	}
}

// Review 2: switching the check off during an outage must close it.
func TestDisableClosesOutage(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := &rec{}
	m := New(st, hub.New(), r, r, nil, &egress.Policy{}, func() []netip.Addr { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.Configure(true, []string{"192.0.2.1:443"})
	now := time.Now()
	m.record(ctx, now, StateOffline, -1, "", true)
	m.record(ctx, now.Add(30*time.Second), StateOffline, -1, "", true)
	m.Configure(false, nil)
	if outs, _ := st.ListOutages(ctx, 0); len(outs) != 1 || outs[0].End == 0 {
		t.Fatalf("outage left open: %+v", outs)
	}
	m.Configure(true, []string{"192.0.2.1:443"})
	m.record(ctx, now.Add(time.Hour), StateOnline, 10*time.Millisecond, "", true)
	for _, k := range r.kinds {
		if k == "internet_up" {
			t.Fatal("bogus recovery notification")
		}
	}
}

func TestOutageKindsAndLatencyBuckets(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := &rec{}
	m := New(st, hub.New(), r, r, nil, &egress.Policy{}, func() []netip.Addr { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.Configure(true, []string{"192.0.2.1:443"})
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	at := func(i int) time.Time { return base.Add(time.Duration(i) * 30 * time.Second) }
	// Bucket 12:00: ten checks, one lost while the router answered.
	for i := 0; i < 10; i++ {
		ms := time.Duration(10+i) * time.Millisecond
		if i == 4 {
			ms = -1
		}
		state := StateOnline
		if ms < 0 {
			state = StateOffline
		}
		m.record(ctx, at(i), state, ms, "192.168.1.1", true)
	}
	// 12:05: the router reboots (router outage), then DNS fails for a while.
	for i := 10; i < 14; i++ {
		m.record(ctx, at(i), StateLANDown, -1, "192.168.1.1", false)
	}
	for i := 14; i < 18; i++ {
		m.record(ctx, at(i), StateDegraded, 20*time.Millisecond, "192.168.1.1", true)
	}
	m.record(ctx, at(18), StateOnline, 12*time.Millisecond, "192.168.1.1", true)
	m.record(ctx, at(30), StateOnline, 12*time.Millisecond, "192.168.1.1", true) // 12:15: flushes 12:05

	outs, _ := st.OutagesBetween(ctx, 0, base.Add(time.Hour).UnixMilli())
	if len(outs) != 2 || outs[0].Kind != store.OutageRouter || outs[1].Kind != store.OutageDNS || outs[0].End == 0 || outs[1].End == 0 {
		t.Fatalf("outages %+v", outs)
	}
	if listed, _ := st.ListOutages(ctx, 0); len(listed) != 1 {
		t.Fatalf("DNS outages must not show up as connectivity outages: %+v", listed)
	}
	for _, k := range r.kinds {
		if k == "log:internet_down" && len(r.kinds) != 4 {
			t.Fatalf("a DNS outage must not notify: %v", r.kinds)
		}
	}
	ls, _ := st.LatencyBetween(ctx, 0, base.Add(time.Hour).UnixMilli())
	if len(ls) != 2 {
		t.Fatalf("buckets %+v", ls)
	}
	if b := ls[0]; b.TS != base.UnixMilli() || b.Samples != 10 || b.Lost != 1 || b.Median != 15 || b.Max != 19 {
		t.Fatalf("first bucket %+v", b)
	}
	// Failures while the router was down are not counted against the line.
	if b := ls[1]; b.Lost != 0 || b.Samples != 5 {
		t.Fatalf("second bucket %+v", b)
	}
}

func TestApplyLineStoresChangesAndReconnects(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := New(st, hub.New(), &rec{}, &rec{}, nil, &egress.Policy{}, func() []netip.Addr { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	l := fritzbox.Line{Access: "dsl", Up: true, DownKbps: 92000, UpKbps: 38000, DownMaxKbps: 118000, UpMaxKbps: 41000, Uptime: 1000}
	for i := 0; i < 5; i++ { // unchanged for 5 minutes, max rate wanders by 1 %
		l.Uptime += 60
		l.DownMaxKbps = 118000 + i*200
		m.ApplyLine(ctx, l, base.Add(time.Duration(i)*time.Minute))
	}
	l.Uptime = 30 // reconnected, new sync rate
	l.DownKbps = 61000
	m.ApplyLine(ctx, l, base.Add(6*time.Minute))
	l.Uptime += 60
	m.ApplyLine(ctx, l, base.Add(7*time.Minute))
	m.ApplyLine(ctx, l, base.Add(68*time.Minute)) // hourly heartbeat
	ls, _ := st.LineSamplesBetween(ctx, 0, base.Add(2*time.Hour).UnixMilli())
	if len(ls) != 3 || ls[1].DownKbps != 61000 || !ls[1].Reconnect || ls[0].Reconnect || ls[2].Reconnect {
		t.Fatalf("samples %+v", ls)
	}
	// The sample before the window describes the state at its start.
	if ls, _ := st.LineSamplesBetween(ctx, base.Add(30*time.Minute).UnixMilli(), base.Add(2*time.Hour).UnixMilli()); len(ls) != 2 || ls[0].TS != base.Add(6*time.Minute).UnixMilli() {
		t.Fatalf("window %+v", ls)
	}
	if m.Line() == nil || m.Line().DownKbps != 61000 {
		t.Fatal("current line not kept")
	}
}
