package report

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/Pyzyi/Netward/internal/pdf"
	"github.com/Pyzyi/Netward/internal/store"
)

// Info is what the PDF shows besides the numbers. Holder, customer number
// and address are only printed, never stored.
type Info struct {
	Holder, Customer, Address string
	Version                   string
	Lang                      string // de | en
	Loc                       *time.Location
	Created                   time.Time
	CSVName, CSVSum           string
}

const (
	margin  = 50.0
	right   = pdf.PageWidth - margin
	bottom  = pdf.PageHeight - 60
	colVal  = 255.0
	lineGap = 15.0
)

var (
	black = pdf.Color{R: 0.07, G: 0.08, B: 0.1}
	gray  = pdf.Color{R: 0.4, G: 0.42, B: 0.45}
	light = pdf.Color{R: 0.85, G: 0.86, B: 0.87}
	pale  = pdf.Color{R: 0.95, G: 0.95, B: 0.94}
	red   = pdf.Color{R: 0.75, G: 0.2, B: 0.15}
	green = pdf.Color{R: 0.1, G: 0.5, B: 0.3}
	amber = pdf.Color{R: 0.85, G: 0.55, B: 0.1}
)

type writer struct {
	doc  *pdf.Document
	page *pdf.Page
	y    float64
	tr   func(string, ...any) string
	in   Info
}

func (w *writer) newPage() {
	w.page = w.doc.AddPage()
	w.y = 60
}

// need starts a new page unless h points still fit.
func (w *writer) need(h float64) {
	if w.y+h > bottom {
		w.newPage()
	}
}

func (w *writer) heading(s string) {
	w.need(60)
	w.y += 14
	w.page.Text(margin, w.y, pdf.Bold, 13, black, s)
	w.y += 6
	w.page.Line(margin, w.y, right, w.y, 0.8, black)
	w.y += lineGap + 2
}

// row prints a label and a value that may wrap.
func (w *writer) row(label, value string) {
	lines := pdf.Wrap(value, pdf.Regular, 10, right-colVal)
	labels := pdf.Wrap(label, pdf.Regular, 10, colVal-margin-12)
	n := max(len(lines), len(labels))
	w.need(float64(n) * lineGap)
	for i := 0; i < n; i++ {
		if i < len(labels) {
			w.page.Text(margin, w.y, pdf.Regular, 10, gray, labels[i])
		}
		if i < len(lines) {
			w.page.Text(colVal, w.y, pdf.Regular, 10, black, lines[i])
		}
		w.y += lineGap
	}
}

func (w *writer) para(s string, size float64, c pdf.Color) {
	for _, l := range pdf.Wrap(s, pdf.Regular, size, right-margin) {
		w.need(size + 4)
		w.page.Text(margin, w.y, pdf.Regular, size, c, l)
		w.y += size + 4
	}
}

func (w *writer) dur(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Minute:
		return w.tr("sec", int(d.Seconds()))
	case d < time.Hour:
		return w.tr("min", int(d.Minutes()+0.5))
	case d < 48*time.Hour:
		h := int(d.Hours())
		return w.tr("hmin", h, int(d.Minutes())-60*h)
	}
	return w.tr("days", d.Hours()/24)
}

func (w *writer) when(ms int64) string {
	t := time.UnixMilli(ms).In(w.in.Loc)
	if w.in.Lang == "en" {
		return t.Format("2006-01-02 15:04")
	}
	return t.Format("02.01.2006 15:04")
}

func (w *writer) date(t time.Time) string {
	if w.in.Lang == "en" {
		return t.Format("Mon 2006-01-02")
	}
	return [...]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}[t.Weekday()] + " " + t.Format("02.01.2006")
}

// dec formats a number with the language's decimal separator.
func (w *writer) dec(f float64, prec int) string {
	s := fmt.Sprintf("%.*f", prec, f)
	if w.in.Lang != "en" {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

func (w *writer) mbit(kbps int) string { return w.dec(float64(kbps)/1000, 1) }

// PDF renders the report.
func PDF(r Report, in Info) ([]byte, error) {
	if in.Loc == nil {
		in.Loc = time.UTC
	}
	if in.Lang != "en" {
		in.Lang = "de"
	}
	texts := de
	if in.Lang == "en" {
		texts = en
	}
	w := &writer{doc: &pdf.Document{Title: texts["title"], Author: "Netzwart", Created: in.Created}, in: in}
	w.tr = func(k string, a ...any) string {
		s, ok := texts[k]
		if !ok {
			s = k
		}
		if len(a) > 0 {
			return fmt.Sprintf(s, a...)
		}
		return s
	}
	tr := w.tr
	w.newPage()

	// Title block.
	w.page.Text(margin, w.y, pdf.Bold, 20, black, tr("title"))
	w.y += 22
	w.page.Text(margin, w.y, pdf.Regular, 11, black, tr("period", w.when(r.From), w.when(r.To), in.Loc.String()))
	w.y += 15
	w.page.Text(margin, w.y, pdf.Regular, 9, gray, tr("created", w.when(in.Created.UnixMilli()), in.Version))
	w.y += 18
	if in.Holder != "" || in.Customer != "" || in.Address != "" {
		if in.Holder != "" {
			w.row(tr("holder"), in.Holder)
		}
		if in.Customer != "" {
			w.row(tr("customer"), in.Customer)
		}
		if in.Address != "" {
			w.row(tr("address"), strings.Join(strings.Fields(strings.ReplaceAll(in.Address, "\n", ", ")), " "))
		}
		w.y += 4
	}

	// Summary.
	w.heading(tr("summary"))
	if r.Availability >= 0 {
		w.row(tr("availability"), w.dec(100*r.Availability, 2)+" %")
	} else {
		w.row(tr("availability"), tr("noData"))
	}
	cover := 0.0
	if r.PeriodMS > 0 {
		cover = 100 * float64(r.MeasuredMS) / float64(r.PeriodMS)
	}
	w.row(tr("coverage"), tr("coverageVal", w.dec(cover, 1), w.dur(r.PeriodMS-r.MeasuredMS)))
	w.row(tr("outInternet"), w.tally(r.Internet))
	w.row(tr("outRouter"), w.tally(r.Router))
	w.row(tr("outDNS"), w.tally(r.DNS))
	if r.Median >= 0 {
		w.row(tr("latency"), tr("latencyVal", r.Median, r.P95))
	}
	w.row(tr("loss"), w.dec(r.LossPct, 2)+" %")

	if l := r.Line; l != nil {
		w.heading(tr("line"))
		w.row(tr("access"), tr("access."+l.Access))
		w.row(tr("syncDown"), tr("syncVal", w.mbit(l.DownMin), w.mbit(l.DownAvg), w.mbit(l.DownMax)))
		w.row(tr("syncUp"), tr("syncVal", w.mbit(l.UpMin), w.mbit(l.UpAvg), w.mbit(l.UpMax)))
		w.row(tr("reconnects"), fmt.Sprint(l.Reconnects))
		if l.DownMS > 0 {
			w.row(tr("lineDown"), w.dur(l.DownMS))
		}
		c := r.Contract
		if c.MaxDown > 0 || c.NormalDown > 0 || c.MinDown > 0 {
			w.row(tr("contractDown"), w.contract(c.MaxDown, c.NormalDown, c.MinDown))
			if c.MaxUp > 0 || c.NormalUp > 0 || c.MinUp > 0 {
				w.row(tr("contractUp"), w.contract(c.MaxUp, c.NormalUp, c.MinUp))
			}
			if c.MinDown > 0 {
				w.row(tr("belowMin"), w.dec(l.BelowMinPct, 1)+" %")
			}
			if c.NormalDown > 0 {
				w.row(tr("belowNormal"), w.dec(l.BelowNormalPct, 1)+" %")
			}
		}
	}

	// What this report is and is not.
	w.y += 8
	notes := []string{tr("noteBNetzA"), tr("noteRouter"), tr("noteSync")}
	h := 16.0
	for _, n := range notes {
		h += float64(len(pdf.Wrap(n, pdf.Regular, 9, right-margin-20)))*13 + 6
	}
	w.need(h)
	w.page.Rect(margin, w.y, right-margin, h, pale)
	w.page.Rect(margin, w.y, 3, h, amber)
	y := w.y + 18
	for _, n := range notes {
		for _, l := range pdf.Wrap(n, pdf.Regular, 9, right-margin-20) {
			w.page.Text(margin+12, y, pdf.Regular, 9, black, l)
			y += 13
		}
		y += 6
	}
	w.y += h + 10

	// Charts and the daily table.
	w.newPage()
	if l := r.Line; l != nil && l.Access == "dsl" {
		w.syncChart(r)
	}
	w.latencyChart(r)
	w.dailyTable(r)
	w.outageList(r)

	// Raw data.
	w.heading(tr("raw"))
	w.para(tr("rawText"), 9, black)
	w.y += 4
	w.row(tr("rawFile"), in.CSVName)
	w.row("SHA-256", in.CSVSum[:32])
	w.row("", in.CSVSum[32:])

	w.footers()
	var buf bytes.Buffer
	if err := w.doc.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// footers numbers the pages once the total is known.
func (w *writer) footers() {
	total := w.doc.Pages()
	w.doc.Each(func(i int, p *pdf.Page) {
		p.Line(margin, pdf.PageHeight-42, right, pdf.PageHeight-42, 0.4, light)
		p.Text(margin, pdf.PageHeight-30, pdf.Regular, 8, gray, w.tr("footer"))
		p.TextRight(right, pdf.PageHeight-30, pdf.Regular, 8, gray, w.tr("page", i+1, total))
	})
}

func (w *writer) tally(t Tally) string {
	if t.Count == 0 {
		return w.tr("none")
	}
	return w.tr("tallyVal", t.Count, w.dur(t.MS), w.dur(t.LongestMS))
}

func (w *writer) contract(max, normal, min float64) string {
	f := func(v float64) string {
		if v <= 0 {
			return "–"
		}
		return w.dec(v, 0)
	}
	return w.tr("contractVal", f(max), f(normal), f(min))
}

// chartFrame draws axes and returns the plot area.
func (w *writer) chartFrame(title string, h float64, maxV float64, unit string) (x0, y0, cw, ch float64) {
	w.need(h + 50)
	w.page.Text(margin, w.y, pdf.Bold, 11, black, title)
	w.y += 16
	// Room on the right for the labels of reference lines.
	x0, y0, cw, ch = margin+40, w.y, right-margin-40-62, h
	for i := 0; i <= 4; i++ {
		y := y0 + ch - ch*float64(i)/4
		w.page.Line(x0, y, x0+cw, y, 0.3, light)
		w.page.TextRight(x0-4, y+3, pdf.Regular, 7, gray, w.dec(maxV*float64(i)/4, 0))
	}
	w.page.Text(margin, y0-2, pdf.Regular, 7, gray, unit)
	w.y += h + 6
	return
}

func (w *writer) dayLabels(r Report, x0, cw float64) {
	n := len(r.Days)
	step := max(1, n/8)
	for i, d := range r.Days {
		if i%step != 0 {
			continue
		}
		x := x0 + cw*(float64(i)+0.5)/float64(n)
		label := d.Date.Format("02.01.")
		if w.in.Lang == "en" {
			label = d.Date.Format("01-02")
		}
		w.page.Text(x-Width(label)/2, w.y+4, pdf.Regular, 7, gray, label)
	}
	w.y += 22
}

// Width of a 7 pt label.
func Width(s string) float64 { return pdf.Width(s, pdf.Regular, 7) }

func (w *writer) syncChart(r Report) {
	maxV := float64(r.Line.DownMax) / 1000
	if c := r.Contract.MaxDown; c > maxV {
		maxV = c
	}
	maxV = niceCeil(maxV)
	x0, y0, cw, ch := w.chartFrame(w.tr("chartSync"), 120, maxV, "Mbit/s")
	n := len(r.Days)
	bw := cw / float64(n)
	yOf := func(v float64) float64 { return y0 + ch - ch*min(v/maxV, 1) }
	for i, d := range r.Days {
		if d.SyncDownMax == 0 {
			continue
		}
		lo, hi := float64(d.SyncDownMin)/1000, float64(d.SyncDownMax)/1000
		c := green
		if r.Contract.MinDown > 0 && lo < r.Contract.MinDown {
			c = red
		} else if r.Contract.NormalDown > 0 && lo < r.Contract.NormalDown {
			c = amber
		}
		x := x0 + bw*float64(i) + bw*0.2
		top := yOf(hi)
		w.page.Rect(x, top, bw*0.6, max(yOf(lo)-top, 2.5), c)
	}
	for _, ref := range []struct {
		v float64
		k string
	}{{r.Contract.MaxDown, "max"}, {r.Contract.NormalDown, "normal"}, {r.Contract.MinDown, "min"}} {
		if ref.v <= 0 {
			continue
		}
		y := yOf(ref.v)
		w.page.Line(x0, y, x0+cw, y, 0.6, gray)
		w.page.Text(x0+cw+4, y+2.5, pdf.Regular, 7, gray, w.tr("ref."+ref.k))
	}
	w.dayLabels(r, x0, cw)
}

func (w *writer) latencyChart(r Report) {
	maxV := 10.0
	for _, d := range r.Days {
		maxV = max(maxV, float64(d.P95))
	}
	maxV = niceCeil(maxV)
	x0, y0, cw, ch := w.chartFrame(w.tr("chartLatency"), 100, maxV, "ms")
	n := len(r.Days)
	var med, p95 []float64
	for i, d := range r.Days {
		if d.Median < 0 {
			continue
		}
		x := x0 + cw*(float64(i)+0.5)/float64(n)
		med = append(med, x, y0+ch-ch*min(float64(d.Median)/maxV, 1))
		p95 = append(p95, x, y0+ch-ch*min(float64(d.P95)/maxV, 1))
	}
	w.page.Polyline(p95, 1, amber)
	w.page.Polyline(med, 1.4, green)
	w.dayLabels(r, x0, cw)
	w.page.Rect(margin+40, w.y-12, 10, 3, green)
	w.page.Text(margin+54, w.y-9, pdf.Regular, 7, gray, w.tr("legendMedian"))
	w.page.Rect(margin+150, w.y-12, 10, 3, amber)
	w.page.Text(margin+164, w.y-9, pdf.Regular, 7, gray, w.tr("legendP95"))
	w.y += 6
}

func niceCeil(v float64) float64 {
	// Steps that divide into four round ticks.
	for _, s := range []float64{20, 40, 60, 80, 100, 120, 160, 200, 400, 600, 800, 1000, 1200, 2000, 4000, 10000} {
		if v <= s {
			return s
		}
	}
	return v
}

func (w *writer) dailyTable(r Report) {
	w.heading(w.tr("daily"))
	cols := []float64{margin, 160, 230, 290, 350, 420}
	header := func() {
		for i, k := range []string{"colDate", "colOutage", "colMedian", "colP95", "colLoss", "colSync"} {
			w.page.Text(cols[i], w.y, pdf.Bold, 8, gray, w.tr(k))
		}
		w.y += 13
	}
	header()
	for _, d := range r.Days {
		if w.y+12 > bottom {
			w.newPage()
			header()
		}
		c := black
		if d.OutageMS > 0 {
			c = red
		}
		w.page.Text(cols[0], w.y, pdf.Regular, 9, black, w.date(d.Date))
		out := "–"
		if d.OutageMS > 0 {
			out = w.dur(d.OutageMS)
		}
		w.page.Text(cols[1], w.y, pdf.Regular, 9, c, out)
		dash := func(v int, unit string) string {
			if v < 0 {
				return "–"
			}
			return fmt.Sprintf("%d %s", v, unit)
		}
		w.page.Text(cols[2], w.y, pdf.Regular, 9, black, dash(d.Median, "ms"))
		w.page.Text(cols[3], w.y, pdf.Regular, 9, black, dash(d.P95, "ms"))
		loss := "–"
		if d.MeasuredMS > 0 {
			loss = w.dec(d.LossPct, 1) + " %"
		} else {
			loss = w.tr("notMeasured")
		}
		w.page.Text(cols[4], w.y, pdf.Regular, 9, black, loss)
		sync := "–"
		if d.SyncDownMax > 0 {
			if d.SyncDownMin == d.SyncDownMax {
				sync = w.mbit(d.SyncDownMax)
			} else {
				sync = w.mbit(d.SyncDownMin) + " – " + w.mbit(d.SyncDownMax)
			}
		}
		w.page.Text(cols[5], w.y, pdf.Regular, 9, black, sync)
		w.y += 12
	}
}

func (w *writer) outageList(r Report) {
	w.heading(w.tr("outages"))
	if len(r.Outages) == 0 {
		w.para(w.tr("noOutages"), 10, black)
		return
	}
	cols := []float64{margin, 160, 270, 350}
	header := func() {
		for i, k := range []string{"colStart", "colEnd", "colDuration", "colKind"} {
			w.page.Text(cols[i], w.y, pdf.Bold, 8, gray, w.tr(k))
		}
		w.y += 13
	}
	header()
	for _, o := range r.Outages {
		if w.y+12 > bottom {
			w.newPage()
			header()
		}
		end := w.tr("ongoing")
		if o.End > 0 {
			end = w.when(o.End)
		}
		c := black
		if o.Kind == store.OutageInternet {
			c = red
		}
		w.page.Text(cols[0], w.y, pdf.Regular, 9, black, w.when(o.Start))
		w.page.Text(cols[1], w.y, pdf.Regular, 9, black, end)
		w.page.Text(cols[2], w.y, pdf.Regular, 9, c, w.dur(o.MS))
		w.page.Text(cols[3], w.y, pdf.Regular, 9, black, w.tr("kind."+o.Kind))
		w.y += 12
	}
}
