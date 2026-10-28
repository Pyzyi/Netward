// Package pdf writes simple, valid PDF 1.4 documents: A4 pages with text in
// the standard fonts Helvetica and Helvetica-Bold, lines, rectangles and
// colours. That is all the provider report needs (docs/09, F5), and it
// keeps Netward free of a PDF dependency.
//
// Text is encoded in WinAnsi (Windows-1252), which covers German; other
// characters become "?". Strings are written as hex, so no character of
// the input can break out of a string. Coordinates are in points with the
// origin at the top left (unlike PDF itself).
package pdf

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// A4 size in points.
const (
	PageWidth  = 595.28
	PageHeight = 841.89
)

// Font selects one of the two built-in fonts.
type Font int

// Fonts.
const (
	Regular Font = iota
	Bold
)

// Color is an RGB colour with components 0..1.
type Color struct{ R, G, B float64 }

// Document collects pages.
type Document struct {
	Title, Author string
	Created       time.Time

	pages []*Page
}

// Page is one A4 page.
type Page struct {
	buf bytes.Buffer
}

// AddPage starts a new page.
func (d *Document) AddPage() *Page {
	p := &Page{}
	d.pages = append(d.pages, p)
	return p
}

// Pages returns the number of pages.
func (d *Document) Pages() int { return len(d.pages) }

// Each calls fn for every page, for example to add page numbers.
func (d *Document) Each(fn func(i int, p *Page)) {
	for i, p := range d.pages {
		fn(i, p)
	}
}

func num(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func (p *Page) printf(format string, args ...any) {
	fmt.Fprintf(&p.buf, format, args...)
}

// Text draws s with its baseline at y.
func (p *Page) Text(x, y float64, f Font, size float64, c Color, s string) {
	p.printf("BT /F%d %s Tf %s %s %s rg %s %s Td <%s> Tj ET\n", f+1, num(size), num(c.R), num(c.G), num(c.B),
		num(x), num(PageHeight-y), hex.EncodeToString(Encode(s)))
}

// TextRight draws s ending at x.
func (p *Page) TextRight(x, y float64, f Font, size float64, c Color, s string) {
	p.Text(x-Width(s, f, size), y, f, size, c, s)
}

// Line draws a straight line.
func (p *Page) Line(x1, y1, x2, y2, width float64, c Color) {
	p.printf("%s %s %s RG %s w %s %s m %s %s l S\n", num(c.R), num(c.G), num(c.B), num(width),
		num(x1), num(PageHeight-y1), num(x2), num(PageHeight-y2))
}

// Rect fills a rectangle whose top left corner is (x, y).
func (p *Page) Rect(x, y, w, h float64, c Color) {
	p.printf("%s %s %s rg %s %s %s %s re f\n", num(c.R), num(c.G), num(c.B), num(x), num(PageHeight-y-h), num(w), num(h))
}

// Polyline draws connected line segments through the points (x0, y0, x1, y1, ...).
func (p *Page) Polyline(pts []float64, width float64, c Color) {
	if len(pts) < 4 {
		return
	}
	p.printf("%s %s %s RG %s w 1 j %s %s m", num(c.R), num(c.G), num(c.B), num(width), num(pts[0]), num(PageHeight-pts[1]))
	for i := 2; i+1 < len(pts); i += 2 {
		p.printf(" %s %s l", num(pts[i]), num(PageHeight-pts[i+1]))
	}
	p.printf(" S\n")
}

// Wrap breaks s into lines no wider than width.
func Wrap(s string, f Font, size, width float64) []string {
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, w := range strings.Fields(para) {
			try := w
			if line != "" {
				try = line + " " + w
			}
			if line != "" && Width(try, f, size) > width {
				lines = append(lines, line)
				line = w
				continue
			}
			line = try
		}
		lines = append(lines, line)
	}
	return lines
}

// Width returns the width of s in points.
func Width(s string, f Font, size float64) float64 {
	widths := &helvetica
	if f == Bold {
		widths = &helveticaBold
	}
	total := 0
	for _, b := range Encode(s) {
		total += int(widths[b])
	}
	return float64(total) * size / 1000
}

// Encode converts s to WinAnsi; characters outside it become "?".
func Encode(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			out = append(out, ' ')
		case r >= 0x20 && r < 0x7f, r >= 0xa0 && r <= 0xff:
			out = append(out, byte(r))
		default:
			if b, ok := winAnsiExtra[r]; ok {
				out = append(out, b)
			} else {
				out = append(out, '?')
			}
		}
	}
	return out
}

// Characters of Windows-1252 between 0x80 and 0x9f.
var winAnsiExtra = map[rune]byte{
	'€': 0x80, '‚': 0x82, '„': 0x84, '…': 0x85, '‰': 0x89, '‹': 0x8b, '‘': 0x91, '’': 0x92,
	'“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97, '™': 0x99, '›': 0x9b,
}

// Write renders the document.
func (d *Document) Write(w io.Writer) error {
	if len(d.pages) == 0 {
		d.AddPage()
	}
	var out bytes.Buffer
	var offsets []int
	obj := func(body string) int {
		offsets = append(offsets, out.Len())
		n := len(offsets)
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", n, body)
		return n
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	// Fixed object numbers: 1 catalog, 2 pages, 3/4 fonts, 5 info, then
	// two objects (page, content) per page.
	n := len(d.pages)
	kids := make([]string, n)
	for i := range d.pages {
		kids[i] = fmt.Sprintf("%d 0 R", 6+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	created := d.Created
	if created.IsZero() {
		created = time.Now()
	}
	obj(fmt.Sprintf("<< /Title <%s> /Author <%s> /Producer (Netward) /CreationDate (D:%s) >>",
		utf16Hex(d.Title), utf16Hex(d.Author), created.UTC().Format("20060102150405")+"Z"))
	for i, p := range d.pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>",
			num(PageWidth), num(PageHeight), 7+2*i))
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		_, _ = zw.Write(p.buf.Bytes())
		_ = zw.Close()
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", len(offsets), z.Len())
		out.Write(z.Bytes())
		out.WriteString("\nendstream\nendobj\n")
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info 5 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	_, err := w.Write(out.Bytes())
	return err
}

// utf16Hex encodes a document info string (UTF-16BE with BOM).
func utf16Hex(s string) string {
	b := []byte{0xfe, 0xff}
	for _, r := range s {
		if r > 0xffff {
			r = '?'
		}
		b = append(b, byte(r>>8), byte(r))
	}
	return hex.EncodeToString(b)
}
