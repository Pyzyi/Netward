package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/Pyzyi/netzwart/internal/auth"
	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/store"
)

func TestRandomPassword(t *testing.T) {
	re := regexp.MustCompile(`^[a-zA-Z2-9]{4}(-[a-zA-Z2-9]{4}){3}$`)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p := RandomPassword()
		if !re.MatchString(p) || seen[p] {
			t.Fatalf("bad or repeated password %q", p)
		}
		seen[p] = true
		if err := auth.CheckPassword("admin", p); err != nil {
			t.Fatalf("generated password violates policy: %v", err)
		}
	}
}

func TestLoadSettingsRepairsAndFillsDefaults(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// An old version stored fewer fields and an invalid interval.
	_ = st.SetSetting(ctx, "settings", json.RawMessage(`{"networkName":"Zuhause","scanInterval":5}`))
	s, err := LoadSettings(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if s.NetworkName != "Zuhause" || s.ScanInterval != config.Defaults().ScanInterval || s.Validate() != nil {
		t.Fatalf("unexpected settings %+v (valid: %v)", s, s.Validate())
	}
}

func TestResetFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	env := config.Env{DataDir: dir}
	st, err := store.Open(ctx, DBPath(env))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	old, _ := auth.HashPassword(ctx, "Altes-Passwort-123")
	u, _ := st.CreateFirstUser(ctx, "admin", old)

	_ = os.WriteFile(filepath.Join(dir, "reset-password"), nil, 0o600)
	if err := handleResetFiles(ctx, env, st, log); err != nil {
		t.Fatal(err)
	}
	u2, _ := st.GetUser(ctx, u.ID)
	if u2.PasswordHash == old {
		t.Fatal("password not reset")
	}
	if _, err := os.Stat(filepath.Join(dir, "reset-password")); !os.IsNotExist(err) {
		t.Fatal("trigger file not removed")
	}

	_ = os.WriteFile(filepath.Join(dir, "factory-reset"), nil, 0o600)
	if err := handleResetFiles(ctx, env, st, log); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.UserCount(ctx); n != 0 {
		t.Fatal("factory reset kept the account")
	}
}

func TestSeedDemo(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := SeedDemo(ctx, st); err != nil {
		t.Fatal(err)
	}
	devs, _ := st.ListDevices(ctx)
	if len(devs) != len(demoDevices) {
		t.Fatalf("seeded %d devices, want %d", len(devs), len(demoDevices))
	}
	// Seeding twice must not duplicate anything.
	if err := SeedDemo(ctx, st); err != nil {
		t.Fatal(err)
	}
	devs2, _ := st.ListDevices(ctx)
	if len(devs2) != len(devs) {
		t.Fatal("demo data duplicated")
	}
}

func TestSeedDemoEnglish(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := SeedDemoLang(ctx, st, "en"); err != nil {
		t.Fatal(err)
	}
	devs, _ := st.ListDevices(ctx)
	names := map[string]string{}
	for _, d := range devs {
		names[d.Name] = d.Location
	}
	if loc, ok := names["Living room TV"]; !ok || loc != "Living room" {
		t.Fatalf("English names missing: %v", names)
	}
	if _, ok := names["Fernseher Wohnzimmer"]; ok {
		t.Fatal("German name in the English demo")
	}
	if DemoWLANLang("en").APs[1].Name != "Upstairs repeater" {
		t.Fatal("repeater not renamed")
	}
	if DemoWLAN().APs[1].Name != "Repeater OG" {
		t.Fatal("German demo changed")
	}
}
