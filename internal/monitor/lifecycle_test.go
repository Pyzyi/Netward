package monitor

import (
	"testing"
	"time"

	"github.com/Pyzyi/Netward/internal/software"
	"github.com/Pyzyi/Netward/internal/store"
)

func TestHistoryRecordsOutageOfAlwaysOnDevice(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{10: {10}, 20: {20}})
	devs := e.m.Devices()
	nas, phone := devs[0], devs[1]
	nasType, phoneType := "nas", "phone"
	e.m.PatchDevice(e.ctx, nas.ID, store.DevicePatch{TypeOverride: &nasType})
	e.m.PatchDevice(e.ctx, phone.ID, store.DevicePatch{TypeOverride: &phoneType})

	// Both disappear for an hour and come back.
	e.clk.Add(time.Hour)
	e.m.SweepOffline(e.ctx)
	e.sweep(map[byte][]byte{10: {10}, 20: {20}})

	log, _ := e.st.DeviceLog(e.ctx, nas.ID, 0)
	if len(log) != 1 || log[0].Kind != KindOutage {
		t.Fatalf("NAS history = %+v, want one outage", log)
	}
	if log, _ := e.st.DeviceLog(e.ctx, phone.ID, 0); len(log) != 0 {
		t.Fatalf("a phone being away is not an outage: %+v", log)
	}

	// A short absence (10 minutes) is not recorded.
	e.clk.Add(10 * time.Minute)
	e.m.SweepOffline(e.ctx)
	e.sweep(map[byte][]byte{10: {10}})
	if log, _ := e.st.DeviceLog(e.ctx, nas.ID, 0); len(log) != 1 {
		t.Fatalf("short absence recorded: %+v", log)
	}
}

func TestHistorySkipsOutageWhileNetzwartWasDown(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{10: {10}})
	nas := e.m.Devices()[0]
	nasType := "nas"
	e.m.PatchDevice(e.ctx, nas.ID, store.DevicePatch{TypeOverride: &nasType})
	start := e.clk.Now()
	e.clk.Add(2 * time.Hour)
	e.m.SweepOffline(e.ctx)
	// Netzwart itself was not running for part of that time.
	if err := e.st.AddGap(e.ctx, start.Add(30*time.Minute).UnixMilli(), start.Add(90*time.Minute).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	e.sweep(map[byte][]byte{10: {10}})
	if log, _ := e.st.DeviceLog(e.ctx, nas.ID, 0); len(log) != 0 {
		t.Fatalf("outage overlapping a monitoring gap recorded: %+v", log)
	}
}

func TestHistoryRecordsSoftwareUpdate(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{10: {10}})
	d := e.m.Devices()[0]
	e.m.mu.Lock()
	en := e.m.byID[d.ID]
	e.m.observeSoftware(en, []software.Observation{{Source: "ssh", Product: "debian", Version: "11"}})
	e.m.observeSoftware(en, []software.Observation{{Source: "ssh", Product: "debian", Version: "11"}})
	e.m.observeSoftware(en, []software.Observation{{Source: "ssh", Product: "debian", Version: "12"}})
	e.m.mu.Unlock()
	log, _ := e.st.DeviceLog(e.ctx, d.ID, 0)
	if len(log) != 1 || log[0].Kind != KindSoftwareChanged || string(log[0].Data) != `{"from":"11","product":"debian","to":"12"}` {
		t.Fatalf("history = %+v", log)
	}
}

func TestInventoryFieldsAreValidated(t *testing.T) {
	e := newEnv(t)
	e.sweep(map[byte][]byte{10: {10}})
	d := e.m.Devices()[0]
	bad := "2026-13-01"
	if _, err := e.m.PatchDevice(e.ctx, d.ID, store.DevicePatch{PurchaseDate: &bad}); err != ErrInvalid {
		t.Fatalf("invalid date accepted: %v", err)
	}
	day, until, serial := "2024-03-15", "2027-03-15", "  SN-123\x00 "
	got, err := e.m.PatchDevice(e.ctx, d.ID, store.DevicePatch{PurchaseDate: &day, WarrantyUntil: &until, Serial: &serial})
	if err != nil {
		t.Fatal(err)
	}
	if got.PurchaseDate != day || got.WarrantyUntil != until || got.Serial != "SN-123" {
		t.Fatalf("got %+v", got)
	}
	stored, _ := e.st.GetDevice(e.ctx, d.ID)
	if stored.PurchaseDate != day || stored.Serial != "SN-123" {
		t.Fatalf("not stored: %+v", stored)
	}
	empty := ""
	if got, _ := e.m.PatchDevice(e.ctx, d.ID, store.DevicePatch{PurchaseDate: &empty}); got.PurchaseDate != "" {
		t.Fatal("date not cleared")
	}
}
