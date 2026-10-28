package monitor

import (
	"context"
	"time"

	"github.com/Pyzyi/Netward/internal/classify"
	"github.com/Pyzyi/Netward/internal/proto"
	"github.com/Pyzyi/Netward/internal/store"
)

// Behaviour baseline (docs/09, F2, deliberately small): a known device that
// suddenly asks for an address like a different operating system family is
// worth a look; another device may be using its identity. Only families are
// compared (an iPhone becoming a Mac is not news), only for devices known
// for a week that were seen twice with their usual family, and only when the
// new family shows up twice as well.

// KindIdentityChanged is the activity log entry for such a change.
const KindIdentityChanged = "identity_changed"

// KindPortNew is the activity log entry for a newly open port, confirmed
// by two port scans with the same scope.
const KindPortNew = "port_new"

const (
	keyBaseline = "baseline_dhcp_family"
	// baselineAge: younger devices are still being set up.
	baselineAge = 7 * 24 * time.Hour
	// sightingGap: DISCOVER and REQUEST of one lease are one sighting.
	sightingGap = 10 * time.Minute
)

// IdentityChange is a confirmed change of the operating system family.
type IdentityChange struct {
	From string `json:"from"`
	To   string `json:"to"`
	At   int64  `json:"at"`
}

type dhcpBase struct {
	Family    string          `json:"family"`
	Count     int             `json:"count"`
	LastAt    int64           `json:"lastAt"`
	Cand      string          `json:"cand,omitempty"`
	CandCount int             `json:"candCount,omitempty"`
	CandAt    int64           `json:"candAt,omitempty"`
	Change    *IdentityChange `json:"change,omitempty"`
}

// osFamily groups what classify.GuessOS returns.
func osFamily(os string) string {
	switch os {
	case "iOS", "macOS":
		return "apple"
	case "Windows":
		return "windows"
	case "Android":
		return "android"
	case "Linux", "Embedded Linux", "Embedded":
		return "linux"
	}
	return ""
}

func (m *Monitor) loadBaseline(ctx context.Context) {
	var b map[int64]*dhcpBase
	if err := m.st.GetSetting(ctx, keyBaseline, &b); err == nil && b != nil {
		m.baseline = b
	}
}

func (m *Monitor) saveBaseline(ctx context.Context) {
	for id := range m.baseline {
		if m.byID[id] == nil {
			delete(m.baseline, id) // forgotten or merged device
		}
	}
	_ = m.st.SetSetting(ctx, keyBaseline, m.baseline)
}

// observeFamily updates the baseline from a client DHCP message; m.mu must
// be held.
func (m *Monitor) observeFamily(ctx context.Context, e *entry, d *proto.DHCP, now int64) {
	fam := osFamily(classify.GuessOS(d.VendorClass, proto.ParamListString(d.ParamList)))
	if fam == "" {
		return
	}
	b := m.baseline[e.d.ID]
	switch {
	case b == nil:
		m.baseline[e.d.ID] = &dhcpBase{Family: fam, Count: 1, LastAt: now}
	case fam == b.Family:
		if b.Cand != "" {
			b.Cand, b.CandCount = "", 0
		}
		if now-b.LastAt < sightingGap.Milliseconds() {
			return
		}
		b.Count, b.LastAt = min(b.Count+1, 1000), now
	case b.Change != nil && b.Change.To == fam:
		return // already reported, waiting for the user
	default:
		settled := e.d.Known && b.Count >= 2 && time.Duration(now-e.d.FirstSeen)*time.Millisecond >= baselineAge
		if !settled {
			// Still learning: the newer family simply becomes the baseline,
			// at most once per sightingGap (a device alternating its
			// fingerprint per packet must not rewrite the settings each time).
			if now-b.LastAt < sightingGap.Milliseconds() {
				return
			}
			m.baseline[e.d.ID] = &dhcpBase{Family: fam, Count: 1, LastAt: now}
			break
		}
		if b.Cand != fam {
			b.Cand, b.CandCount, b.CandAt = fam, 1, now
			break
		}
		if now-b.CandAt < sightingGap.Milliseconds() {
			return
		}
		b.CandCount, b.CandAt = b.CandCount+1, now
		if b.CandCount >= 2 {
			b.Change = &IdentityChange{From: b.Family, To: fam, At: now}
			b.Cand, b.CandCount = "", 0
			snap := m.view(e)
			m.addEvent(ctx, KindIdentityChanged, "warning", &snap, map[string]any{"from": b.Change.From, "to": b.Change.To})
			m.netChanged = true
		}
	}
	m.saveBaseline(ctx)
}

// IdentityChanges returns the unresolved family changes by device.
func (m *Monitor) IdentityChanges() map[int64]IdentityChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[int64]IdentityChange{}
	for id, b := range m.baseline {
		if b.Change != nil && m.byID[id] != nil {
			out[id] = *b.Change
		}
	}
	return out
}

// ResolveIdentity answers a family change: "set up anew" makes the new
// family the baseline; "don't know it" also marks the device as not known,
// so it shows up among the devices to review.
func (m *Monitor) ResolveIdentity(ctx context.Context, id int64, setUpAnew bool) bool {
	m.mu.Lock()
	b, e := m.baseline[id], m.byID[id]
	ok := b != nil && b.Change != nil && e != nil
	if ok {
		m.baseline[id] = &dhcpBase{Family: b.Change.To, Count: 1, LastAt: m.nowMS()}
		m.netChanged = true
		m.saveBaseline(ctx)
	}
	m.mu.Unlock()
	if ok && !setUpAnew {
		known := false
		_, _ = m.PatchDevice(ctx, id, store.DevicePatch{Known: &known})
	}
	m.softwareChanged()
	return ok
}

// notePorts compares a port scan with the previous one of the same scope
// and reports ports that are open in two scans in a row but were closed
// before; m.mu must be held.
func (m *Monitor) notePorts(ctx context.Context, e *entry, ports []store.Port, scope string) {
	open := map[int]bool{}
	for _, p := range ports {
		open[p.Port] = true
	}
	if e.portScope == scope && e.portPrev != nil {
		for _, p := range ports {
			switch {
			case e.portCand[p.Port]:
				snap := m.view(e)
				m.addEvent(ctx, KindPortNew, "info", &snap, map[string]any{"port": p.Port, "service": p.Service})
				delete(e.portCand, p.Port)
			case !e.portPrev[p.Port]:
				if e.portCand == nil {
					e.portCand = map[int]bool{}
				}
				e.portCand[p.Port] = true
			}
		}
		for port := range e.portCand {
			if !open[port] {
				delete(e.portCand, port) // closed again: never mind
			}
		}
	} else {
		e.portCand = nil
	}
	e.portScope, e.portPrev = scope, open
}
