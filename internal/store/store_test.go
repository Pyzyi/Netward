package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenCreatesPrivateFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "x.db")
	s, err := Open(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("db mode %v, want 0600", fi.Mode().Perm())
	}
	// Re-opening must be idempotent.
	s2, err := Open(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}

func TestDeviceLifecycle(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	d := Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.5", FirstSeen: 1000, LastSeen: 1000, Online: true}
	if err := s.InsertDevice(ctx, &d); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertDevice(ctx, &Device{MAC: d.MAC, FirstSeen: 1, LastSeen: 1}); err == nil {
		t.Fatal("duplicate MAC accepted")
	}
	name, typ, known := "Küchenradio", "speaker", true
	if err := s.PatchDevice(ctx, d.ID, DevicePatch{Name: &name, TypeOverride: &typ, Known: &known}); err != nil {
		t.Fatal(err)
	}
	if err := s.PatchDevice(ctx, 9999, DevicePatch{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("patch of missing device: %v", err)
	}
	got, err := s.GetDevice(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name || got.Type != "speaker" || got.DetectedType != "unknown" || !got.Known {
		t.Fatalf("unexpected device %+v", got)
	}
	if err := s.TouchLastSeen(ctx, map[int64]int64{d.ID: 5000}); err != nil {
		t.Fatal(err)
	}
	if err := s.TouchLastSeen(ctx, map[int64]int64{d.ID: 4000}); err != nil { // older timestamps never win
		t.Fatal(err)
	}
	got, _ = s.GetDevice(ctx, d.ID)
	if got.LastSeen != 5000 {
		t.Fatalf("last_seen %d, want 5000", got.LastSeen)
	}

	if err := s.UpsertName(ctx, d.ID, "dhcp", "radio"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplacePorts(ctx, d.ID, []Port{{Port: 80, Service: "http"}, {Port: 22}}, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplacePorts(ctx, d.ID, []Port{{Port: 80, Service: "http"}}, 200); err != nil {
		t.Fatal(err)
	}
	ports, _ := s.ListPorts(ctx, d.ID)
	if len(ports[d.ID]) != 1 || ports[d.ID][0].FirstSeen != 100 || ports[d.ID][0].LastSeen != 200 {
		t.Fatalf("unexpected ports %+v", ports)
	}
	if err := s.OpenPresence(ctx, d.ID, 1000); err != nil {
		t.Fatal(err)
	}
	if err := s.OpenPresence(ctx, d.ID, 2000); err != nil { // already open: no-op
		t.Fatal(err)
	}
	if err := s.ClosePresence(ctx, d.ID, 3000); err != nil {
		t.Fatal(err)
	}
	spans, _ := s.ListPresence(ctx, d.ID, 0, 10000)
	if len(spans) != 1 || spans[0].Start != 1000 || spans[0].End != 3000 {
		t.Fatalf("unexpected presence %+v", spans)
	}

	if err := s.DeleteDevice(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	names, _ := s.ListNames(ctx, d.ID)
	if len(names) != 0 {
		t.Fatal("names not cascaded")
	}
}

func TestCreateFirstUserIsExclusive(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := s.CreateFirstUser(ctx, "admin", "hash"); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("%d concurrent setups succeeded, want exactly 1", ok)
	}
	if _, err := s.GetUserByName(ctx, "ADMIN"); err != nil {
		t.Fatal("username lookup must be case-insensitive")
	}
}

func TestEventsAndFindings(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	for i := 0; i < 5; i++ {
		kind := "device_new"
		if i%2 == 1 {
			kind = "device_offline"
		}
		if err := s.AddEvent(ctx, &Event{TS: int64(i), Kind: kind}); err != nil {
			t.Fatal(err)
		}
	}
	evs, err := s.ListEvents(ctx, EventQuery{Limit: 2, Kinds: []string{"device_new"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[0].TS != 4 || evs[1].TS != 2 {
		t.Fatalf("unexpected events %+v", evs)
	}
	next, _ := s.ListEvents(ctx, EventQuery{Before: evs[1].ID, Kinds: []string{"device_new"}})
	if len(next) != 1 || next[0].TS != 0 {
		t.Fatalf("cursor pagination broken: %+v", next)
	}

	isNew, err := s.UpsertFinding(ctx, Finding{CheckID: "upnp", Severity: "warning", LastSeen: 10})
	if err != nil || !isNew {
		t.Fatalf("first upsert: new=%v err=%v", isNew, err)
	}
	isNew, _ = s.UpsertFinding(ctx, Finding{CheckID: "upnp", Severity: "warning", LastSeen: 20})
	if isNew {
		t.Fatal("second upsert reported as new")
	}
	// Another job's prune leaves the finding alone.
	if err := s.PruneFindings(ctx, 30, nil, []string{"software_outdated"}); err != nil {
		t.Fatal(err)
	}
	if fs, _ := s.ListFindings(ctx); len(fs) != 1 {
		t.Fatal("finding pruned by a job that does not own it")
	}
	if err := s.PruneFindings(ctx, 30, nil, []string{"upnp"}); err != nil {
		t.Fatal(err)
	}
	fs, _ := s.ListFindings(ctx)
	if len(fs) != 0 {
		t.Fatal("stale finding not pruned")
	}

	// Device findings: only pruned for checked devices; dismissals survive a missed run.
	nas := Device{Ident: "aa:00:00:00:00:01", MAC: "aa:00:00:00:00:01", FirstSeen: 1, LastSeen: 1}
	if err := s.InsertDevice(ctx, &nas); err != nil {
		t.Fatal(err)
	}
	telnet := Finding{CheckID: "risky_port", DeviceID: nas.ID, Severity: "critical", Data: json.RawMessage(`{"port":23}`), LastSeen: 40}
	_, _ = s.UpsertFinding(ctx, telnet)
	fs, _ = s.ListFindings(ctx)
	_ = s.SetFindingDismissed(ctx, fs[0].ID, true)
	_ = s.PruneFindings(ctx, 50, nil, []string{"risky_port"}) // NAS asleep: not checked
	_ = s.PruneFindings(ctx, 60, []int64{nas.ID}, []string{"risky_port"})
	telnet.LastSeen = 70
	if isNew, _ := s.UpsertFinding(ctx, telnet); isNew {
		t.Fatal("dismissed finding came back as new")
	}
	if fs, _ = s.ListFindings(ctx); len(fs) != 1 || !fs[0].Dismissed {
		t.Fatalf("dismissal lost: %+v", fs)
	}
	_, _ = s.UpsertFinding(ctx, Finding{CheckID: "risky_port", DeviceID: nas.ID, Severity: "critical", Data: json.RawMessage(`{"port":21}`), LastSeen: 70})
	_ = s.PruneFindings(ctx, 80, []int64{nas.ID}, []string{"risky_port"})
	if fs, _ = s.ListFindings(ctx); len(fs) != 1 || string(fs[0].Data) != `{"port":23}` {
		t.Fatalf("undismissed stale finding of a checked device not pruned: %+v", fs)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	var v map[string]int
	if err := s.GetSetting(ctx, "x", &v); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing setting: %v", err)
	}
	if err := s.SetSetting(ctx, "x", map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.GetSetting(ctx, "x", &v); err != nil || v["a"] != 1 {
		t.Fatalf("got %v %v", v, err)
	}
}

func TestMergeDevices(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	a := Device{MAC: "da:00:00:00:00:01", FirstSeen: 100, LastSeen: 200, MACKind: "random"}
	b := Device{MAC: "da:00:00:00:00:02", FirstSeen: 300, LastSeen: 400, MACKind: "random"}
	if err := s.InsertDevice(ctx, &a); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertDevice(ctx, &b); err != nil {
		t.Fatal(err)
	}
	name, notes := "Annas iPhone", "Arbeitshandy"
	_ = s.PatchDevice(ctx, a.ID, DevicePatch{Name: &name, Notes: &notes})
	_ = s.OpenPresence(ctx, a.ID, 100)
	_ = s.ClosePresence(ctx, a.ID, 200)
	_ = s.AddEvent(ctx, &Event{TS: 150, Kind: "device_new", DeviceID: a.ID})
	if err := s.MergeDevices(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDevice(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name || got.Notes != notes || got.FirstSeen != 100 {
		t.Fatalf("merge did not carry user data: %+v", got)
	}
	if _, err := s.GetDevice(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("source device still exists")
	}
	spans, _ := s.ListPresence(ctx, b.ID, 0, 1000)
	evs, _ := s.ListEvents(ctx, EventQuery{DeviceID: b.ID})
	if len(spans) != 1 || len(evs) != 1 {
		t.Fatalf("history not moved: spans=%d events=%d", len(spans), len(evs))
	}
	if err := s.MergeDevices(ctx, b.ID, b.ID); err == nil {
		t.Fatal("self-merge accepted")
	}
}

func TestExpireRandomDevices(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	old := Device{MAC: "da:00:00:00:00:01", FirstSeen: 1, LastSeen: 10, MACKind: "random"}
	kept := Device{MAC: "da:00:00:00:00:02", FirstSeen: 1, LastSeen: 10, MACKind: "random"}
	uni := Device{MAC: "3c:00:00:00:00:03", FirstSeen: 1, LastSeen: 10}
	for _, d := range []*Device{&old, &kept, &uni} {
		if err := s.InsertDevice(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	yes := true
	_ = s.PatchDevice(ctx, kept.ID, DevicePatch{Known: &yes})
	ids, err := s.ExpireRandomDevices(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != old.ID {
		t.Fatalf("expired %v, want only %d", ids, old.ID)
	}
}

func TestBackup(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	_ = s.SetSetting(ctx, "k", 1)
	p := filepath.Join(t.TempDir(), "backup.db")
	if err := s.Backup(ctx, p); err != nil {
		t.Fatal(err)
	}
	b, err := Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var v int
	if err := b.GetSetting(ctx, "k", &v); err != nil || v != 1 {
		t.Fatalf("backup content wrong: %v %v", v, err)
	}
}

func TestSoftwareRowsAndPreUpdateBackup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "netward.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	a := Device{Ident: "aa:00:00:00:00:0a", MAC: "aa:00:00:00:00:0a", FirstSeen: 1, LastSeen: 1}
	b := Device{Ident: "aa:00:00:00:00:0b", MAC: "aa:00:00:00:00:0b", FirstSeen: 1, LastSeen: 1}
	_ = s.InsertDevice(ctx, &a)
	_ = s.InsertDevice(ctx, &b)
	_ = s.UpsertSoftware(ctx, Software{DeviceID: a.ID, Source: "smb", Product: "windows", Version: "10.0.19045", SeenAt: 10})
	_ = s.UpsertSoftware(ctx, Software{DeviceID: a.ID, Source: "smb", Product: "windows", Version: "10.0.26200", SeenAt: 20})
	_ = s.UpsertSoftware(ctx, Software{DeviceID: b.ID, Source: "smb", Product: "windows", Version: "10.0.22631", SeenAt: 15})
	_ = s.UpsertSoftware(ctx, Software{DeviceID: b.ID, Source: "manual", Product: "windows", Version: "x", SeenAt: 5})
	// Merging keeps the newer row per source and moves the others.
	if err := s.MergeDevices(ctx, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	all, err := s.ListSoftware(ctx)
	if err != nil || len(all[b.ID]) != 0 || len(all[a.ID]) != 2 {
		t.Fatalf("after merge: %+v %v", all, err)
	}
	for _, sw := range all[a.ID] {
		if sw.Source == "smb" && sw.Version != "10.0.26200" {
			t.Fatalf("older row won the merge: %+v", sw)
		}
	}
	_ = s.DeleteSoftware(ctx, a.ID, "manual")
	if all, _ = s.ListSoftware(ctx); len(all[a.ID]) != 1 {
		t.Fatalf("manual row not deleted: %+v", all)
	}

	// Pretend the database is one schema behind: reopening keeps a copy the
	// older version can still open.
	s.Close()
	withExtraMigration(t)
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	backup := filepath.Join(dir, "backup", fmt.Sprintf("pre-update-schema-%d.db", len(migrations)-1))
	old, err := sql.Open("sqlite", "file:"+backup)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var v int
	if err := old.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil || v != len(migrations)-1 {
		t.Fatalf("backup schema %d err %v", v, err)
	}
	// A factory reset removes backups: they still hold the old secrets.
	if err := s.FactoryReset(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup survived the factory reset: %v", err)
	}
}

func TestPreUpdateBackupFailureDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "netward.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	withExtraMigration(t)
	// "backup" exists as a file: the copy cannot be written.
	if err := os.WriteFile(filepath.Join(dir, "backup"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("start blocked by a failed backup: %v", err)
	}
	defer s.Close()
	if len(s.Warnings()) != 1 {
		t.Fatalf("warnings %v", s.Warnings())
	}
}

// withExtraMigration makes an up-to-date database one schema behind, the way
// it is after installing a newer version.
func withExtraMigration(t *testing.T) {
	orig := migrations
	migrations = append(orig[:len(orig):len(orig)], `CREATE TABLE test_extra (x INTEGER);`)
	t.Cleanup(func() { migrations = orig })
}

func TestDeviceLogTrimAndMerge(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := Device{MAC: "aa:00:00:00:00:01", FirstSeen: 1, LastSeen: 1}
	b := Device{MAC: "aa:00:00:00:00:02", FirstSeen: 1, LastSeen: 1}
	if err := st.InsertDevice(ctx, &a); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertDevice(ctx, &b); err != nil {
		t.Fatal(err)
	}
	for i := range deviceLogKeep + 20 {
		if err := st.AddDeviceLog(ctx, a.ID, int64(i), "outage", nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.TrimDeviceLog(ctx); err != nil {
		t.Fatal(err)
	}
	log, _ := st.DeviceLog(ctx, a.ID, 0)
	if len(log) != deviceLogKeep || log[0].TS != deviceLogKeep+19 {
		t.Fatalf("kept %d entries, newest %d", len(log), log[0].TS)
	}
	day := "2024-01-02"
	if err := st.PatchDevice(ctx, a.ID, DevicePatch{PurchaseDate: &day}); err != nil {
		t.Fatal(err)
	}
	if err := st.MergeDevices(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	merged, _ := st.GetDevice(ctx, b.ID)
	if merged.PurchaseDate != day {
		t.Fatal("purchase date lost in merge")
	}
	if log, _ := st.DeviceLog(ctx, b.ID, 0); len(log) != deviceLogKeep {
		t.Fatalf("history not moved: %d", len(log))
	}
}
