package wifimap

import (
	"testing"

	"github.com/Pyzyi/Netward/internal/store"
)

func pt(id int64, room string, x, y, down float64) store.WifiPoint {
	return store.WifiPoint{ID: id, Room: room, X: x, Y: y, DownMbps: down, UpMbps: down / 2, LatencyMs: 4, JitterMs: 1}
}

func TestWeakRoomGetsRepeaterAdvice(t *testing.T) {
	points := []store.WifiPoint{
		pt(1, "Wohnzimmer", 0.1, 0.5, 310),
		pt(2, "Flur", 0.5, 0.5, 190),
		pt(3, "Kinderzimmer", 0.9, 0.5, 40),
		pt(4, "Kinderzimmer", 0.85, 0.45, 70),
		pt(5, "Küche", 0.1, 0.1, 260),
	}
	points[2].Band = "2.4"
	hs := Hints(points, 1000, 600)
	if len(hs) != 1 {
		t.Fatalf("hints = %+v", hs)
	}
	h := hs[0]
	if h.Kind != KindWeak || h.Room != "Kinderzimmer" || h.DownMbps != 40 || h.Via != "Flur" || h.Best != "Wohnzimmer" || !h.Band24 || !h.Severe {
		t.Fatalf("hint = %+v", h)
	}
}

func TestNoViaWhenNothingInBetween(t *testing.T) {
	hs := Hints([]store.WifiPoint{pt(1, "Büro", 0.1, 0.1, 400), pt(2, "Keller", 0.9, 0.9, 3)}, 800, 800)
	if len(hs) != 1 || hs[0].Via != "" || !hs[0].Severe {
		t.Fatalf("hints = %+v", hs)
	}
}

func TestUnstableAndGood(t *testing.T) {
	a, b := pt(1, "Bad", 0.2, 0.2, 200), pt(2, "Schlafzimmer", 0.8, 0.8, 180)
	if hs := Hints([]store.WifiPoint{a, b}, 100, 100); len(hs) != 1 || hs[0].Kind != KindGood || hs[0].DownMbps != 180 {
		t.Fatalf("good: %+v", hs)
	}
	b.JitterMs = 45
	if hs := Hints([]store.WifiPoint{a, b}, 100, 100); len(hs) != 1 || hs[0].Kind != KindUnstable || hs[0].Room != "Schlafzimmer" {
		t.Fatalf("unstable: %+v", hs)
	}
	if hs := Hints(nil, 1, 1); len(hs) != 0 {
		t.Fatalf("empty: %+v", hs)
	}
}

func TestOutlierIsNotTheReference(t *testing.T) {
	points := []store.WifiPoint{pt(1, "Büro", 0.1, 0.1, 2900), pt(2, "Wohnzimmer", 0.2, 0.2, 300), pt(3, "Küche", 0.5, 0.5, 250),
		pt(4, "Bad", 0.9, 0.9, 60)}
	ref, _ := Reference(points)
	hs := Hints(points, 100, 100)
	if ref.ID != 2 || len(hs) != 1 || hs[0].Room != "Bad" || hs[0].BestMbps != 300 {
		t.Fatalf("ref %+v hints %+v", ref, hs)
	}
}
