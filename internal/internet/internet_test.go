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

	"github.com/Pyzyi/netzwart/internal/egress"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/store"
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
