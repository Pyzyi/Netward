// Package wlan turns the FRITZ!Box Wi-Fi state into hints a person can act
// on (docs/09, F3): weak signal, a device stuck on 2.4 GHz although it can
// do 5 GHz, a repeater that is itself badly connected.
//
// Only the current state is kept (no history). A hint shows up after it was
// seen in three polls in a row, so a phone walking past the garden does not
// produce one. Hints never notify; they are shown where the device is.
package wlan

import (
	"strings"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/fritzbox"
	"github.com/Pyzyi/netzwart/internal/store"
)

// Hint kinds.
const (
	HintWeak         = "weak"          // signal below -75 dBm
	HintVeryWeak     = "very_weak"     // signal below -82 dBm
	HintBand24       = "band24"        // on 2.4 GHz although it can do 5 GHz
	HintBackhaulWeak = "backhaul_weak" // repeater with a weak link to the mesh
	HintViaRepeater  = "weak_repeater" // device behind such a repeater
)

const (
	weakDBm     = -75
	veryWeakDBm = -82
	// backhaulMinRate: below this a repeater cannot pass on a decent rate.
	backhaulMinRate = 100
	// streakNeeded: polls in a row before a hint is shown.
	streakNeeded = 3
)

// Devices resolves MAC addresses to known devices.
type Devices interface {
	Devices() []store.Device
}

// Client is a Wi-Fi device with its hints.
type Client struct {
	fritzbox.WLANClient
	DeviceID int64    `json:"deviceId,omitempty"`
	Hints    []string `json:"hints"`
}

// AccessPoint is the FRITZ!Box or a repeater with its devices.
type AccessPoint struct {
	fritzbox.WLANAccessPoint
	DeviceID int64    `json:"deviceId,omitempty"`
	Hints    []string `json:"hints"`
	Clients  []Client `json:"clients"`
}

// View is what the API returns.
type View struct {
	At     int64         `json:"at"` // 0 = no data yet
	Source string        `json:"source,omitempty"`
	APs    []AccessPoint `json:"aps"`
}

// Tracker keeps the latest Wi-Fi state.
type Tracker struct {
	devs Devices

	mu     sync.Mutex
	state  fritzbox.WLANState
	at     int64
	streak map[string]int // "mac|kind" -> polls in a row
}

// New creates an empty tracker.
func New(devs Devices) *Tracker {
	return &Tracker{devs: devs, streak: map[string]int{}}
}

// Apply records a new poll.
func (t *Tracker) Apply(st fritzbox.WLANState, at time.Time) {
	raw := rawHints(st)
	t.mu.Lock()
	defer t.mu.Unlock()
	next := make(map[string]int, len(raw))
	for k := range raw {
		next[k] = t.streak[k] + 1
	}
	t.streak, t.state, t.at = next, st, at.UnixMilli()
}

// SeedDemo fills the tracker with an example (demo mode), hints included.
func (t *Tracker) SeedDemo(st fritzbox.WLANState, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state, t.at, t.streak = st, at.UnixMilli(), map[string]int{}
	for k := range rawHints(st) {
		t.streak[k] = streakNeeded
	}
}

// rawHints returns the hints of one poll as "mac|kind" keys.
func rawHints(st fritzbox.WLANState) map[string]bool {
	out := map[string]bool{}
	aps := map[string]fritzbox.WLANAccessPoint{}
	weakRepeater := map[string]bool{}
	for _, a := range st.APs {
		aps[a.MAC] = a
		if b := a.Backhaul; !a.Master && b != nil && b.Type == "wlan" &&
			((b.Rate > 0 && b.Rate < backhaulMinRate) || (b.SignalDBm != 0 && b.SignalDBm < weakDBm)) {
			out[a.MAC+"|"+HintBackhaulWeak] = true
			weakRepeater[a.MAC] = true
		}
	}
	offers5 := false
	for _, a := range st.APs {
		for _, b := range a.Bands {
			offers5 = offers5 || b == "5"
		}
	}
	for _, c := range st.Clients {
		switch {
		case c.SignalDBm != 0 && c.SignalDBm < veryWeakDBm:
			out[c.MAC+"|"+HintVeryWeak] = true
		case c.SignalDBm != 0 && c.SignalDBm < weakDBm:
			out[c.MAC+"|"+HintWeak] = true
		}
		if c.Band == "2.4" && c.Can5GHz && offers5 {
			out[c.MAC+"|"+HintBand24] = true
		}
		if weakRepeater[c.APMAC] {
			out[c.MAC+"|"+HintViaRepeater] = true
		}
	}
	return out
}

func (t *Tracker) hints(mac string) []string {
	out := []string{}
	for _, k := range []string{HintVeryWeak, HintWeak, HintBand24, HintBackhaulWeak, HintViaRepeater} {
		if t.streak[mac+"|"+k] >= streakNeeded {
			out = append(out, k)
		}
	}
	return out
}

// View returns the current state grouped by access point.
func (t *Tracker) View() View {
	byMAC := map[string]int64{}
	if t.devs != nil {
		for _, d := range t.devs.Devices() {
			byMAC[strings.ToLower(d.MAC)] = d.ID
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	v := View{At: t.at, Source: t.state.Source, APs: []AccessPoint{}}
	idx := map[string]int{}
	for _, a := range t.state.APs {
		idx[a.MAC] = len(v.APs)
		v.APs = append(v.APs, AccessPoint{WLANAccessPoint: a, DeviceID: byMAC[a.MAC], Hints: t.hints(a.MAC), Clients: []Client{}})
	}
	for _, c := range t.state.Clients {
		i, ok := idx[c.APMAC]
		if !ok {
			continue
		}
		v.APs[i].Clients = append(v.APs[i].Clients, Client{WLANClient: c, DeviceID: byMAC[c.MAC], Hints: t.hints(c.MAC)})
	}
	return v
}
