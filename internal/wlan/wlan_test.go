package wlan

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/Pyzyi/Netward/internal/fritzbox"
	"github.com/Pyzyi/Netward/internal/store"
)

type devs []store.Device

func (d devs) Devices() []store.Device { return d }

func fixture(t *testing.T) fritzbox.WLANState {
	data, err := os.ReadFile("../fritzbox/testdata/meshlist.json")
	if err != nil {
		t.Fatal(err)
	}
	st, err := fritzbox.ParseMeshList(data)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestHintsNeedThreePolls(t *testing.T) {
	tr := New(devs{{ID: 7, MAC: "AA:BB:CC:00:00:02"}})
	st := fixture(t)
	now := time.Now()
	for i := 1; i <= 3; i++ {
		tr.Apply(st, now.Add(time.Duration(i)*5*time.Minute))
		v := tr.View()
		rep := v.APs[1]
		laptop := rep.Clients[0]
		if laptop.DeviceID != 7 {
			t.Fatalf("laptop not mapped to its device: %+v", laptop)
		}
		if i < 3 && (len(laptop.Hints) != 0 || len(rep.Hints) != 0) {
			t.Fatalf("hint after %d polls: %v %v", i, laptop.Hints, rep.Hints)
		}
		if i == 3 {
			if !slices.Equal(laptop.Hints, []string{HintWeak, HintBand24, HintViaRepeater}) || !slices.Equal(rep.Hints, []string{HintBackhaulWeak}) {
				t.Fatalf("hints after 3 polls: laptop %v repeater %v", laptop.Hints, rep.Hints)
			}
			if phone := v.APs[0].Clients[0]; len(phone.Hints) != 0 {
				t.Fatalf("good phone got hints %v", phone.Hints)
			}
		}
	}
	// One good poll resets the streak.
	good := fixture(t)
	for i := range good.Clients {
		good.Clients[i].SignalDBm, good.Clients[i].Band = -55, "5"
	}
	good.APs[1].Backhaul.Rate, good.APs[1].Backhaul.SignalDBm = 400, -60
	tr.Apply(good, now.Add(time.Hour))
	tr.Apply(st, now.Add(2*time.Hour))
	if h := tr.View().APs[1].Clients[0].Hints; len(h) != 0 {
		t.Fatalf("streak not reset: %v", h)
	}
}

func TestEmptyView(t *testing.T) {
	v := New(nil).View()
	if v.At != 0 || v.APs == nil || len(v.APs) != 0 {
		t.Fatalf("%+v", v)
	}
}
