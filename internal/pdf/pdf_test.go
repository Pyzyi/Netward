package pdf

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWidthAndEncode(t *testing.T) {
	if w := Width("Hello", Regular, 10); w < 22.77 || w > 22.79 {
		t.Fatalf("Width(Hello) = %v, want 22.78", w)
	}
	if w := Width("Hello", Bold, 10); w < 24.43 || w > 24.45 { // 722+556+278+278+611
		t.Fatalf("bold width %v", w)
	}
	if got := Encode("Größe – 5 €→"); !bytes.Equal(got, []byte{'G', 'r', 0xf6, 0xdf, 'e', ' ', 0x96, ' ', '5', ' ', 0x80, '?'}) {
		t.Fatalf("Encode = % x", got)
	}
	lines := Wrap("Die Bundesnetzagentur verlangt das Messprotokoll der Breitbandmessung.", Regular, 10, 150)
	if len(lines) < 2 {
		t.Fatalf("not wrapped: %q", lines)
	}
	for _, l := range lines {
		if Width(l, Regular, 10) > 150 {
			t.Fatalf("line too wide: %q", l)
		}
	}
}

func TestDocumentIsValid(t *testing.T) {
	d := &Document{Title: "Nachweis Internetanschluss", Author: "Netzwart", Created: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)}
	p := d.AddPage()
	p.Text(50, 60, Bold, 18, Color{}, "Nachweis für Größe & Prüfung (§ 57 TKG)")
	p.TextRight(545, 80, Regular, 9, Color{0.4, 0.4, 0.4}, "Seite 1 von 2")
	p.Line(50, 90, 545, 90, 0.5, Color{0.8, 0.8, 0.8})
	p.Rect(50, 100, 100, 20, Color{0.9, 0.3, 0.2})
	p.Polyline([]float64{50, 200, 100, 180, 150, 190}, 1, Color{0, 0.5, 0})
	p.Text(50, 240, Regular, 10, Color{}, `Klammern ( ) und \ Backslash bleiben harmlos`)
	d.AddPage().Text(50, 60, Regular, 10, Color{}, "Zweite Seite")
	var buf bytes.Buffer
	if err := d.Write(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if !bytes.HasPrefix(out, []byte("%PDF-1.4")) || !bytes.HasSuffix(out, []byte("%%EOF\n")) {
		t.Fatal("not a PDF")
	}
	path := filepath.Join(t.TempDir(), "t.pdf")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	// CI installs qpdf; locally the structural check is skipped without it.
	if _, err := exec.LookPath("qpdf"); err == nil {
		if res, err := exec.Command("qpdf", "--check", path).CombinedOutput(); err != nil {
			t.Fatalf("qpdf --check: %v\n%s", err, res)
		}
	} else if os.Getenv("CI") != "" {
		t.Fatal("qpdf missing in CI")
	}
	if _, err := exec.LookPath("pdftotext"); err == nil {
		res, err := exec.Command("pdftotext", path, "-").Output()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(res), "Nachweis für Größe & Prüfung (§ 57 TKG)") || !strings.Contains(string(res), "Zweite Seite") {
			t.Fatalf("text not extractable: %q", res)
		}
	}
}
