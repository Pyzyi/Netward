// Package persons answers "who is at home" (docs/09, F4): a person is at
// home while at least one of their presence devices is connected.
//
// Privacy by default: only "at home since" / "away since" is shown. The
// time at home per day is computed only for persons where the user switched
// it on and confirmed that the person knows. Netzwart sees that a device is
// connected, never what happens on it.
package persons

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Pyzyi/Netward/internal/hub"
	"github.com/Pyzyi/Netward/internal/store"
)

// Devices gives the live device state.
type Devices interface {
	Devices() []store.Device
}

// Day is the time at home on one calendar day.
type Day struct {
	Date string `json:"date"` // YYYY-MM-DD
	MS   int64  `json:"ms"`
}

// Status is a person as the UI shows it.
type Status struct {
	store.Person
	Home  bool  `json:"home"`
	Since int64 `json:"since"` // at home since / away since; 0 = unknown
	Days  []Day `json:"days,omitempty"`
}

// State is what Home Assistant gets.
type State struct {
	ID   int64
	Name string
	Home bool
	HA   bool
}

// Service keeps the persons in memory for the live checks.
type Service struct {
	st   *store.Store
	devs Devices
	hub  *hub.Hub

	mu      sync.Mutex
	persons []store.Person
}

// New loads the persons.
func New(ctx context.Context, st *store.Store, devs Devices, h *hub.Hub) (*Service, error) {
	s := &Service{st: st, devs: devs, hub: h}
	return s, s.reload(ctx)
}

func (s *Service) reload(ctx context.Context) error {
	ps, err := s.st.ListPersons(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.persons = ps
	s.mu.Unlock()
	if s.hub != nil {
		s.hub.Publish("persons", nil)
	}
	return nil
}

// Changed reloads after a change through the store.
func (s *Service) Changed(ctx context.Context) error { return s.reload(ctx) }

// List returns the persons (without live state).
func (s *Service) List() []store.Person {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.Person, len(s.persons))
	copy(out, s.persons)
	return out
}

func presenceIDs(p store.Person) []int64 {
	var ids []int64
	for _, d := range p.Devices {
		if d.Presence {
			ids = append(ids, d.DeviceID)
		}
	}
	return ids
}

func (s *Service) online() map[int64]bool {
	on := map[int64]bool{}
	for _, d := range s.devs.Devices() {
		if d.Online {
			on[d.ID] = true
		}
	}
	return on
}

// States returns the live home/away state for Home Assistant.
func (s *Service) States() []State {
	on := s.online()
	var out []State
	for _, p := range s.List() {
		st := State{ID: p.ID, Name: p.Name, HA: p.HA}
		for _, id := range presenceIDs(p) {
			st.Home = st.Home || on[id]
		}
		out = append(out, st)
	}
	return out
}

type span struct{ a, b int64 }

// union merges overlapping spans; open spans end at now.
func union(spans []store.PresenceSpan, now int64) []span {
	var in []span
	for _, sp := range spans {
		end := sp.End
		if end == 0 {
			end = now
		}
		if end > sp.Start {
			in = append(in, span{sp.Start, end})
		}
	}
	sort.Slice(in, func(i, j int) bool { return in[i].a < in[j].a })
	var out []span
	for _, x := range in {
		if n := len(out); n > 0 && x.a <= out[n-1].b {
			out[n-1].b = max(out[n-1].b, x.b)
			continue
		}
		out = append(out, x)
	}
	return out
}

// Statuses computes the state of every person for the UI.
func (s *Service) Statuses(ctx context.Context, now time.Time, loc *time.Location) ([]Status, error) {
	if loc == nil {
		loc = time.UTC
	}
	nowMS := now.UnixMilli()
	from := now.Add(-30 * 24 * time.Hour).UnixMilli()
	all, err := s.st.ListAllPresence(ctx, from, nowMS)
	if err != nil {
		return nil, err
	}
	on := s.online()
	out := []Status{}
	for _, p := range s.List() {
		st := Status{Person: p}
		var spans []store.PresenceSpan
		for _, id := range presenceIDs(p) {
			st.Home = st.Home || on[id]
			spans = append(spans, all[id]...)
		}
		u := union(spans, nowMS)
		if n := len(u); n > 0 {
			last := u[n-1]
			switch {
			case st.Home && last.b >= nowMS-2*60*1000:
				st.Since = last.a
			case !st.Home && last.b < nowMS-2*60*1000:
				st.Since = last.b // a span still open while offline says nothing
			}
		}
		if p.TrackTime && p.ConsentAt > 0 {
			// Only time since the confirmation: switching it on does not
			// reveal the days before.
			st.Days = days(clip(u, p.ConsentAt), now, loc)
		}
		out = append(out, st)
	}
	return out, nil
}

// clip drops the parts of u before from.
func clip(u []span, from int64) []span {
	var out []span
	for _, x := range u {
		if x.b > from {
			out = append(out, span{max(x.a, from), x.b})
		}
	}
	return out
}

// days sums the time at home over the last 7 calendar days, today last.
func days(u []span, now time.Time, loc *time.Location) []Day {
	t := now.In(loc)
	today := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	out := make([]Day, 0, 7)
	for i := 6; i >= 0; i-- {
		start := today.AddDate(0, 0, -i)
		a, b := start.UnixMilli(), min(start.AddDate(0, 0, 1).UnixMilli(), now.UnixMilli())
		var ms int64
		for _, x := range u {
			ms += max(0, min(x.b, b)-max(x.a, a))
		}
		out = append(out, Day{Date: start.Format("2006-01-02"), MS: ms})
	}
	return out
}
