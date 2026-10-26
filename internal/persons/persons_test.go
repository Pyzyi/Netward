package persons

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Pyzyi/netzwart/internal/store"
)

type devs []store.Device

func (d devs) Devices() []store.Device { return d }

func TestStatuses(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	mk := func(mac string) int64 {
		d := store.Device{Ident: mac, MAC: mac, IP: "192.168.1.9", MACKind: "universal", FirstSeen: 1, LastSeen: 1}
		if err := st.InsertDevice(ctx, &d); err != nil {
			t.Fatal(err)
		}
		return d.ID
	}
	phone, watch, laptop := mk("aa:00:00:00:00:01"), mk("aa:00:00:00:00:02"), mk("aa:00:00:00:00:03")
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) int64 { return now.Add(d).UnixMilli() }
	// Phone: yesterday 08-20 h, today since 07:00. Watch: today 06:30-07:30.
	_ = st.OpenPresence(ctx, phone, ms(-28*time.Hour))
	_ = st.ClosePresence(ctx, phone, ms(-16*time.Hour))
	_ = st.OpenPresence(ctx, watch, ms(-5*time.Hour-30*time.Minute))
	_ = st.ClosePresence(ctx, watch, ms(-4*time.Hour-30*time.Minute))
	_ = st.OpenPresence(ctx, phone, ms(-5*time.Hour))
	_ = st.OpenPresence(ctx, laptop, ms(-48*time.Hour)) // not a presence device

	anna, _ := st.CreatePerson(ctx, "Anna")
	max, _ := st.CreatePerson(ctx, "Max")
	_ = st.SetPersonDevices(ctx, anna.ID, []store.PersonDevice{{DeviceID: phone, Presence: true}, {DeviceID: watch, Presence: true}, {DeviceID: laptop}})
	anna.TrackTime, anna.ConsentAt = true, now.UnixMilli()
	_ = st.UpdatePerson(ctx, anna)
	// The laptop moves to Max.
	_ = st.SetPersonDevices(ctx, max.ID, []store.PersonDevice{{DeviceID: laptop, Presence: true}})

	live := devs{{ID: phone, Online: true}, {ID: watch}, {ID: laptop}}
	s, err := New(ctx, st, live, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Statuses(ctx, now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	a, m := out[0], out[1]
	if len(a.Devices) != 2 || !a.Home || a.Since != ms(-5*time.Hour-30*time.Minute) {
		t.Fatalf("anna %+v (since %v)", a, time.UnixMilli(a.Since).UTC())
	}
	if len(a.Days) != 7 || a.Days[6].MS != (5*time.Hour+30*time.Minute).Milliseconds() || a.Days[5].MS != (12*time.Hour).Milliseconds() {
		t.Fatalf("days %+v", a.Days)
	}
	if m.Home || m.Days != nil || m.Since != 0 { // the laptop's span is still open but the laptop is offline now
		t.Fatalf("max %+v", m)
	}
	if hs := s.States(); len(hs) != 2 || !hs[0].Home || hs[1].Home {
		t.Fatalf("states %+v", hs)
	}
	// Without consent no time per day.
	anna.ConsentAt = 0
	_ = st.UpdatePerson(ctx, anna)
	_ = s.Changed(ctx)
	out, _ = s.Statuses(ctx, now, time.UTC)
	if out[0].Days != nil {
		t.Fatal("time per day without consent")
	}
	if err := st.DeletePerson(ctx, anna.ID); err != nil {
		t.Fatal(err)
	}
	if ps, _ := st.ListPersons(ctx); len(ps) != 1 {
		t.Fatalf("persons after delete %+v", ps)
	}
}
