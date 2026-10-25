package privdrop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckDataDir(t *testing.T) {
	dir := t.TempDir()
	if err := CheckDataDir(dir); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(dir)
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v, want 0700", fi.Mode().Perm())
	}
	if err := CheckDataDir(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing directory accepted")
	}
	file := filepath.Join(dir, "f")
	_ = os.WriteFile(file, nil, 0o600)
	if err := CheckDataDir(file); err == nil {
		t.Fatal("file accepted as directory")
	}
	// A directory the process owns but cannot write is repaired (0700).
	ro := filepath.Join(dir, "ro")
	_ = os.Mkdir(ro, 0o500)
	if err := CheckDataDir(ro); err != nil {
		t.Fatalf("own read-only directory not repaired: %v", err)
	}
	// A directory owned by someone else cannot be repaired; the error explains the fix.
	if os.Geteuid() != 0 {
		if err := CheckDataDir("/"); err == nil || !strings.Contains(err.Error(), "chown") {
			t.Fatalf("foreign directory: %v", err)
		}
	}
}

func TestCapEffReadable(t *testing.T) {
	if c := CapEff(); len(c) != 16 {
		t.Fatalf("unexpected CapEff %q", c)
	}
}

func TestDropIsNoopForUnprivileged(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("would change the test process identity")
	}
	if err := Drop(12345, 12345); err != nil {
		t.Fatal(err)
	}
}
