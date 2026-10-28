package report_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Pyzyi/Netward/internal/app"
	"github.com/Pyzyi/Netward/internal/report"
	"github.com/Pyzyi/Netward/internal/store"
)

func open(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestBuildCountsOnlyTheProvidersOutages(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) int64 { return day.Add(d).UnixMilli() }
	// Measured all day except 22:00-24:00 (Netward not running).
	var ls []store.Latency
	for d := time.Duration(0); d < 22*time.Hour; d += 5 * time.Minute {
		ls = append(ls, store.Latency{TS: ms(d), Median: 12, Max: 20, Samples: 10})
	}
	ls[0].Lost = 10
	if err := st.AddLatency(ctx, ls...); err != nil {
		t.Fatal(err)
	}
	_ = st.AddGap(ctx, ms(22*time.Hour), ms(24*time.Hour))
	add := func(start, end time.Duration, kind string) {
		if _, err := st.OpenOutage(ctx, ms(start), kind); err != nil {
			t.Fatal(err)
		}
		if kind == store.OutageDNS {
			_ = st.CloseDNSOutages(ctx, ms(end))
		} else {
			_ = st.CloseOutages(ctx, ms(end))
		}
	}
	add(time.Hour, time.Hour+30*time.Minute, store.OutageInternet)
	add(3*time.Hour, 3*time.Hour+10*time.Minute, store.OutageRouter)
	add(5*time.Hour, 5*time.Hour+5*time.Minute, store.OutageDNS)
	add(21*time.Hour+30*time.Minute, 23*time.Hour, store.OutageInternet) // runs into the gap
	_ = st.AddLineSamples(ctx,
		store.LineSample{TS: ms(0), Access: "dsl", Up: true, DownKbps: 90000, UpKbps: 40000},
		store.LineSample{TS: ms(time.Hour), Access: "dsl", Up: true, DownKbps: 50000, UpKbps: 30000, Reconnect: true},
		store.LineSample{TS: ms(2 * time.Hour), Access: "dsl", Up: true, DownKbps: 90000, UpKbps: 40000, Reconnect: true},
	)

	r, err := report.Build(ctx, st, report.Params{From: day, To: day.Add(24 * time.Hour),
		Contract: report.Contract{MaxDown: 100, NormalDown: 80, MinDown: 60}})
	if err != nil {
		t.Fatal(err)
	}
	if r.MeasuredMS != (22 * time.Hour).Milliseconds() {
		t.Fatalf("measured %v", time.Duration(r.MeasuredMS)*time.Millisecond)
	}
	// 30 min plus the 30 min before the gap; the router outage does not count.
	if r.Internet.Count != 2 || r.Internet.MS != (time.Hour).Milliseconds() || r.Internet.LongestMS != (30*time.Minute).Milliseconds() {
		t.Fatalf("internet %+v", r.Internet)
	}
	if r.Router.Count != 1 || r.DNS.Count != 1 || r.DNS.MS != (5*time.Minute).Milliseconds() {
		t.Fatalf("router %+v dns %+v", r.Router, r.DNS)
	}
	if want := 1 - 1.0/22; r.Availability < want-1e-9 || r.Availability > want+1e-9 {
		t.Fatalf("availability %v want %v", r.Availability, want)
	}
	if r.Median != 12 || r.P95 != 20 || r.LossPct <= 0 {
		t.Fatalf("latency %d %d %v", r.Median, r.P95, r.LossPct)
	}
	l := r.Line
	if l == nil || l.Access != "dsl" || l.DownMin != 50000 || l.DownMax != 90000 || l.Reconnects != 2 {
		t.Fatalf("line %+v", l)
	}
	// Samples hold for at most 65 minutes: 0-1 h, 1-2 h, 2-3:05 h.
	if l.LineMS != (3*time.Hour+5*time.Minute).Milliseconds() || l.BelowMinPct < 32 || l.BelowMinPct > 33 {
		t.Fatalf("line time %v, below min %.2f", time.Duration(l.LineMS)*time.Millisecond, l.BelowMinPct)
	}
	if len(r.Days) != 1 || r.Days[0].OutageMS != r.Internet.MS || r.Days[0].SyncDownMin != 50000 {
		t.Fatalf("days %+v", r.Days)
	}
	if _, err := report.Build(ctx, st, report.Params{From: day, To: day}); err == nil {
		t.Fatal("empty period accepted")
	}
}

func TestDemoReportPDF(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	if err := app.SeedDemo(ctx, st); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Europe/Berlin")
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day()-30, 0, 0, 0, 0, loc)
	to := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	r, err := report.Build(ctx, st, report.Params{From: from, To: to, Loc: loc, Contract: report.Contract{MaxDown: 100, NormalDown: 90, MinDown: 70, MaxUp: 40, NormalUp: 35, MinUp: 30}})
	if err != nil {
		t.Fatal(err)
	}
	csv, err := report.RawCSV(ctx, st, from, to)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := report.RawCSV(ctx, st, from, to)
	if report.Checksum(csv) != report.Checksum(again) {
		t.Fatal("CSV is not deterministic")
	}
	if r.Internet.Count < 5 || r.Router.Count != 1 || r.Line == nil || r.Line.BelowMinPct <= 0 {
		t.Fatalf("demo report %+v line %+v", r, r.Line)
	}
	for _, lang := range []string{"de", "en"} {
		out, err := report.PDF(r, report.Info{Holder: "Erika Mustermann", Customer: "K-123", Address: "Musterstraße 1\n12345 Musterstadt",
			Version: "0.2.0", Lang: lang, Loc: loc, Created: now, CSVName: report.FileName(from, to, loc) + ".csv", CSVSum: report.Checksum(csv)})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "r.pdf")
		if err := os.WriteFile(path, out, 0o600); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("NETWARD_KEEP_PDF") != "" {
			_ = os.WriteFile(os.Getenv("NETWARD_KEEP_PDF")+"-"+lang+".pdf", out, 0o600)
		}
		if _, err := exec.LookPath("qpdf"); err == nil {
			if res, err := exec.Command("qpdf", "--check", path).CombinedOutput(); err != nil {
				t.Fatalf("qpdf: %v\n%s", err, res)
			}
		}
		if _, err := exec.LookPath("pdftotext"); err == nil {
			txt, err := exec.Command("pdftotext", "-layout", path, "-").Output()
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"Nachweis über den Internetanschluss", "Musterstraße 1, 12345 Musterstadt", "§ 57", report.Checksum(csv)[:32]}
			if lang == "en" {
				want = []string{"Evidence report", "Erika Mustermann", "section 57(4)", report.Checksum(csv)[32:]}
			}
			for _, w := range want {
				if !strings.Contains(string(txt), w) {
					t.Fatalf("%s: %q missing in\n%s", lang, w, txt)
				}
			}
		}
	}
}
