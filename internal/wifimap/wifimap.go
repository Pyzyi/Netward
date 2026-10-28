// Package wifimap turns speed measurements on a floor plan into plain
// advice (docs/10, F4): which room is weak and where a repeater would help.
// The measurements come from the phone's browser: throughput and latency
// between the phone and Netzwart, so no router access is needed.
package wifimap

import (
	"math"
	"sort"

	"github.com/Pyzyi/Netward/internal/store"
)

// Hint kinds.
const (
	KindWeak     = "weak"     // much slower than the best spot
	KindUnstable = "unstable" // fast enough, but high or jumpy latency
	KindGood     = "good"     // no weak spot measured
)

// Hint is one piece of advice. The UI turns it into a sentence.
type Hint struct {
	Kind      string  `json:"kind"`
	PointID   int64   `json:"pointId,omitempty"`
	Room      string  `json:"room,omitempty"` // "" = an unnamed spot
	DownMbps  float64 `json:"downMbps"`
	LatencyMs float64 `json:"latencyMs,omitempty"`
	JitterMs  float64 `json:"jitterMs,omitempty"`
	Best      string  `json:"best,omitempty"` // room of the best spot
	BestMbps  float64 `json:"bestMbps,omitempty"`
	Via       string  `json:"via,omitempty"`    // room where a repeater would help
	Band24    bool    `json:"band24,omitempty"` // the phone used 2.4 GHz there
	Severe    bool    `json:"severe,omitempty"`
}

// Thresholds. A spot is weak when it reaches less than 40 % of the best
// spot or less than 10 Mbit/s (enough for one HD stream, not for more).
const (
	weakShare   = 0.4
	severeShare = 0.2
	weakAbs     = 10.0
	severeAbs   = 5.0
	highLatency = 50.0 // ms
	highJitter  = 30.0 // ms
	maxHints    = 5
)

// Reference returns the spot the others are compared with: the fastest,
// unless it is more than twice as fast as the runner-up. Such an outlier is
// usually a measurement over a cable and would make every room look weak.
func Reference(points []store.WifiPoint) (store.WifiPoint, bool) {
	if len(points) == 0 {
		return store.WifiPoint{}, false
	}
	sorted := append([]store.WifiPoint(nil), points...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].DownMbps > sorted[j].DownMbps })
	if len(sorted) >= 3 && sorted[0].DownMbps > outlier*sorted[1].DownMbps {
		return sorted[1], true
	}
	return sorted[0], true
}

const outlier = 2.0

// Hints evaluates the points of one plan. w and h are the plan's size in
// pixels, so distances are measured as on the drawing.
func Hints(points []store.WifiPoint, w, h int) []Hint {
	best, ok := Reference(points)
	if !ok {
		return []Hint{}
	}
	sx, sy := float64(w), float64(h)
	if sx <= 0 || sy <= 0 {
		sx, sy = 1, 1
	}
	isWeak := func(p store.WifiPoint) bool {
		return p.DownMbps < weakShare*best.DownMbps || p.DownMbps < weakAbs
	}
	// One hint per room: its worst spot. Unnamed spots stand alone.
	worst := map[string]store.WifiPoint{}
	var unnamed []store.WifiPoint
	for _, p := range points {
		if p.Room == "" {
			unnamed = append(unnamed, p)
			continue
		}
		if q, ok := worst[p.Room]; !ok || p.DownMbps < q.DownMbps {
			worst[p.Room] = p
		}
	}
	spots := unnamed
	for _, p := range worst {
		spots = append(spots, p)
	}
	sort.Slice(spots, func(i, j int) bool {
		if spots[i].DownMbps != spots[j].DownMbps {
			return spots[i].DownMbps < spots[j].DownMbps
		}
		return spots[i].ID < spots[j].ID
	})

	var out []Hint
	for _, p := range spots {
		if len(out) == maxHints {
			break
		}
		switch {
		case isWeak(p) && p.ID != best.ID && p.DownMbps < best.DownMbps:
			out = append(out, Hint{Kind: KindWeak, PointID: p.ID, Room: p.Room, DownMbps: round(p.DownMbps),
				Best: best.Room, BestMbps: round(best.DownMbps), Via: via(points, best, p, sx, sy, isWeak),
				Band24: p.Band == "2.4", Severe: p.DownMbps < severeShare*best.DownMbps || p.DownMbps < severeAbs})
		case p.LatencyMs > highLatency || p.JitterMs > highJitter:
			out = append(out, Hint{Kind: KindUnstable, PointID: p.ID, Room: p.Room, DownMbps: round(p.DownMbps),
				LatencyMs: round(p.LatencyMs), JitterMs: round(p.JitterMs)})
		}
	}
	if len(out) == 0 {
		low := spots[0]
		out = append(out, Hint{Kind: KindGood, PointID: low.ID, Room: low.Room, DownMbps: round(low.DownMbps)})
	}
	return out
}

// via names the room of a good spot about halfway between the best spot
// and the weak one: where a repeater still gets a strong signal and
// reaches the weak room. "" when no measured room lies near that line.
func via(points []store.WifiPoint, best, weak store.WifiPoint, sx, sy float64, isWeak func(store.WifiPoint) bool) string {
	mx, my := (best.X+weak.X)/2*sx, (best.Y+weak.Y)/2*sy
	span := math.Hypot((best.X-weak.X)*sx, (best.Y-weak.Y)*sy)
	name, dist := "", math.Inf(1)
	for _, p := range points {
		if p.Room == "" || p.Room == weak.Room || p.Room == best.Room || isWeak(p) {
			continue
		}
		if d := math.Hypot(p.X*sx-mx, p.Y*sy-my); d < dist && d <= span/2 {
			name, dist = p.Room, d
		}
	}
	return name
}

func round(v float64) float64 { return math.Round(v*10) / 10 }
