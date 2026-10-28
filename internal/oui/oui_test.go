package oui

import (
	"net"
	"testing"
)

func mustMAC(t *testing.T, s string) net.HardwareAddr {
	t.Helper()
	m, err := net.ParseMAC(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLookup(t *testing.T) {
	if Size() < 40000 {
		t.Fatalf("registry too small: %d entries", Size())
	}
	cases := []struct {
		mac, want string
	}{
		{"3c:22:fb:00:00:01", "Apple"},
		{"b8:27:eb:12:34:56", "Raspberry Pi"},
		{"dc:a6:32:12:34:56", "Raspberry Pi"},
		{"24:0a:c4:00:00:00", "Espressif"},
	}
	for _, c := range cases {
		v, ok := Lookup(mustMAC(t, c.mac))
		if !ok || v.Name != c.want {
			t.Errorf("Lookup(%s) = %q, %v; want %q", c.mac, v.Name, ok, c.want)
		}
	}
}

func TestLookupLongestPrefix(t *testing.T) {
	// 70:B3:D5 is an IEEE block split into MA-S assignments; the result must
	// come from the 36-bit table, never "IEEE Registration Authority".
	v, ok := Lookup(mustMAC(t, "70:b3:d5:a3:00:01"))
	if !ok {
		t.Fatal("expected MA-S match")
	}
	if v.Name == "IEEE Registration Authority" {
		t.Fatalf("got registry placeholder instead of MA-S holder")
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, ok := Lookup(net.HardwareAddr{1, 2}); ok {
		t.Fatal("short MAC must not match")
	}
}

func TestIsRandomized(t *testing.T) {
	if !IsRandomized(mustMAC(t, "da:a1:19:00:00:01")) {
		t.Error("locally administered address not detected")
	}
	if IsRandomized(mustMAC(t, "3c:22:fb:00:00:01")) {
		t.Error("universal address reported as randomized")
	}
}
