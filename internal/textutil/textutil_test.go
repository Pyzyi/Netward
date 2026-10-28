package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClean(t *testing.T) {
	cases := map[string]string{
		"  Anna's  iPhone ":            "Anna's iPhone",
		"evil\x00name\x1b[31m":         "evilname[31m",
		"rtl\u202egnp.exe":             "rtlgnp.exe",
		"tab\tand\nnewline":            "tab and newline",
		"<script>alert(1)</script>":    "<script>alert(1)</script>", // escaping is the renderer's job
		string([]byte{0xff, 'o', 'k'}): "ok",
	}
	for in, want := range cases {
		if got := Clean(in, 0); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanTruncates(t *testing.T) {
	got := Clean(strings.Repeat("ä", 500), 0)
	if n := utf8.RuneCountInString(got); n != MaxName {
		t.Fatalf("got %d runes, want %d", n, MaxName)
	}
}

func TestHostname(t *testing.T) {
	cases := map[string]string{
		"anna-iphone.fritz.box.": "anna-iphone",
		"NAS.local":              "NAS",
		"printer.lan":            "printer",
		"server.example.com":     "server.example.com",
		".local":                 ".local",
		"":                       "",
	}
	for in, want := range cases {
		if got := Hostname(in); got != want {
			t.Errorf("Hostname(%q) = %q, want %q", in, got, want)
		}
	}
}

func FuzzClean(f *testing.F) {
	f.Add("hello world", 10)
	f.Add("\u202e\x00x", 3)
	f.Fuzz(func(t *testing.T, s string, max int) {
		if max > 1000 || max < -10 {
			return
		}
		out := Clean(s, max)
		if !utf8.ValidString(out) {
			t.Fatalf("invalid UTF-8 output %q", out)
		}
		limit := max
		if limit <= 0 {
			limit = MaxName
		}
		if utf8.RuneCountInString(out) > limit {
			t.Fatalf("output longer than %d: %q", limit, out)
		}
		if strings.TrimSpace(out) != out {
			t.Fatalf("output not trimmed: %q", out)
		}
	})
}
