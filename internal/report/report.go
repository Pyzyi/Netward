// Package report builds the evidence report for the internet provider
// (docs/09, F5): availability, outages by kind, latency per day and the
// line's sync rates against the contract, as PDF with the raw data as CSV.
//
// Honesty rules: time Netzwart was not running counts neither for nor
// against the provider; outages where the router itself did not answer are
// listed but not counted against the provider; sync rates are not
// throughput. The report says so, and that the Bundesnetzagentur expects
// its own measurement protocol for a price reduction.
package report

import (
	"context"
	"errors"
	"slices"
	"sort"
	"time"

	"github.com/Pyzyi/netzwart/internal/store"
)

// MaxPeriod bounds a report; the history is not kept longer anyway.
const MaxPeriod = 366 * 24 * time.Hour

// bucketMS is the length of one latency bucket.
const bucketMS = int64(5 * time.Minute / time.Millisecond)

// lineValidMS: a line sample describes the line until the next one, but
// at most this long (samples are stored at least hourly).
const lineValidMS = int64(65 * time.Minute / time.Millisecond)

// Contract are the speeds of the contract in Mbit/s (0 = not given).
type Contract struct {
	MaxDown    float64 `json:"maxDown"`
	NormalDown float64 `json:"normalDown"`
	MinDown    float64 `json:"minDown"`
	MaxUp      float64 `json:"maxUp"`
	NormalUp   float64 `json:"normalUp"`
	MinUp      float64 `json:"minUp"`
}

// Params select what the report covers.
type Params struct {
	From, To time.Time
	Contract Contract
	Loc      *time.Location
}

// ErrPeriod means the period is empty, reversed or too long.
var ErrPeriod = errors.New("report: invalid period")

// Outage is one outage, clipped to the period.
type Outage struct {
	Start, End int64
	Kind       string
	MS         int64 // duration within the period, without monitoring gaps
}

// Tally sums up the outages of one kind.
type Tally struct {
	Count     int
	MS        int64
	LongestMS int64
}

// Day is one calendar day in the report's time zone.
type Day struct {
	Date        time.Time
	OutageMS    int64   // internet outages
	Median, P95 int     // ms, -1 = no data
	LossPct     float64 // lost checks while the router answered
	MeasuredMS  int64
	SyncDownMin int // kbit/s, 0 = no line data
	SyncDownMax int
}

// Line summarises the sync rates.
type Line struct {
	Access                    string
	DownMin, DownAvg, DownMax int // kbit/s
	UpMin, UpAvg, UpMax       int
	Reconnects                int
	LineMS                    int64   // time covered by line data
	DownMS                    int64   // of it, time the line was down
	BelowMinPct               float64 // share of LineMS below the contract minimum (down)
	BelowNormalPct            float64 // share of LineMS below the normally available rate (down)
}

// Report is the computed content.
type Report struct {
	From, To     int64
	PeriodMS     int64
	MeasuredMS   int64
	Availability float64 // of the measured time, internet outages only; -1 = nothing measured
	Internet     Tally
	Router       Tally
	DNS          Tally
	Outages      []Outage
	Median, P95  int
	LossPct      float64
	Days         []Day
	Line         *Line // nil without FRITZ!Box data
	Contract     Contract
}

type span struct{ a, b int64 }

// overlap returns the length of [a,b) inside [c,d).
func overlap(a, b, c, d int64) int64 {
	return max(0, min(b, d)-max(a, c))
}

// without returns the length of [a,b) minus the parts covered by gaps.
func without(a, b int64, gaps []span) int64 {
	n := b - a
	for _, g := range gaps {
		n -= overlap(a, b, g.a, g.b)
	}
	return max(n, 0)
}

func percentile(vals []int, p float64) int {
	if len(vals) == 0 {
		return -1
	}
	s := slices.Clone(vals)
	slices.Sort(s)
	i := int(p*float64(len(s)-1) + 0.5)
	return s[i]
}

// Build computes the report from the stored history.
func Build(ctx context.Context, st *store.Store, p Params) (Report, error) {
	if p.Loc == nil {
		p.Loc = time.UTC
	}
	if now := time.Now(); p.To.After(now) {
		p.To = now // the future is not "not measured"
	}
	from, to := p.From.UnixMilli(), p.To.UnixMilli()
	if to <= from || p.To.Sub(p.From) > MaxPeriod {
		return Report{}, ErrPeriod
	}
	r := Report{From: from, To: to, PeriodMS: to - from, Availability: -1, Median: -1, P95: -1, Contract: p.Contract}

	gapRows, err := st.ListGaps(ctx, from, to)
	if err != nil {
		return Report{}, err
	}
	var gaps []span
	for _, g := range gapRows {
		gaps = append(gaps, span{g.Start, g.End})
	}

	// Days in the report's time zone.
	dayIdx := func(ts int64) int {
		t := time.UnixMilli(ts).In(p.Loc)
		start := time.Date(p.From.In(p.Loc).Year(), p.From.In(p.Loc).Month(), p.From.In(p.Loc).Day(), 0, 0, 0, 0, p.Loc)
		return int(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, p.Loc).Sub(start).Hours()/24 + 0.5)
	}
	for d := p.From.In(p.Loc); d.Before(p.To); d = d.AddDate(0, 0, 1) {
		day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, p.Loc)
		r.Days = append(r.Days, Day{Date: day, Median: -1, P95: -1})
	}
	inDays := func(i int) bool { return i >= 0 && i < len(r.Days) }

	// Latency: the buckets also tell when something was measured.
	lat, err := st.LatencyBetween(ctx, from-bucketMS+1, to)
	if err != nil {
		return Report{}, err
	}
	var allMed, allMax []int
	var lost, samples int
	dayMed := make([][]int, len(r.Days))
	dayMax := make([][]int, len(r.Days))
	dayLost := make([][2]int, len(r.Days))
	for _, l := range lat {
		if l.Samples == 0 {
			continue
		}
		m := overlap(l.TS, l.TS+bucketMS, from, to)
		if m == 0 {
			continue
		}
		r.MeasuredMS += m
		i := dayIdx(l.TS)
		if inDays(i) {
			r.Days[i].MeasuredMS += m
			dayLost[i][0] += l.Lost
			dayLost[i][1] += l.Samples
		}
		lost += l.Lost
		samples += l.Samples
		if l.Median >= 0 {
			allMed = append(allMed, l.Median)
			allMax = append(allMax, l.Max)
			if inDays(i) {
				dayMed[i] = append(dayMed[i], l.Median)
				dayMax[i] = append(dayMax[i], l.Max)
			}
		}
	}
	r.Median, r.P95 = percentile(allMed, 0.5), percentile(allMax, 0.95)
	if samples > 0 {
		r.LossPct = 100 * float64(lost) / float64(samples)
	}
	for i := range r.Days {
		r.Days[i].Median, r.Days[i].P95 = percentile(dayMed[i], 0.5), percentile(dayMax[i], 0.95)
		if dayLost[i][1] > 0 {
			r.Days[i].LossPct = 100 * float64(dayLost[i][0]) / float64(dayLost[i][1])
		}
	}

	// Outages, clipped to the period, without the time Netzwart was down.
	outs, err := st.OutagesBetween(ctx, from, to)
	if err != nil {
		return Report{}, err
	}
	now := time.Now().UnixMilli()
	for _, o := range outs {
		end := o.End
		if end == 0 {
			end = min(now, to)
		}
		a, b := max(o.Start, from), min(end, to)
		if b <= a {
			continue
		}
		ms := without(a, b, gaps)
		r.Outages = append(r.Outages, Outage{Start: o.Start, End: o.End, Kind: o.Kind, MS: ms})
		t := &r.Internet
		switch o.Kind {
		case store.OutageRouter:
			t = &r.Router
		case store.OutageDNS:
			t = &r.DNS
		}
		t.Count++
		t.MS += ms
		t.LongestMS = max(t.LongestMS, ms)
		if o.Kind == store.OutageInternet {
			// Split across days for the daily table.
			for d := a; d < b; {
				i := dayIdx(d)
				dt := time.UnixMilli(d).In(p.Loc)
				next := time.Date(dt.Year(), dt.Month(), dt.Day()+1, 0, 0, 0, 0, p.Loc).UnixMilli()
				if inDays(i) {
					r.Days[i].OutageMS += without(d, min(next, b), gaps)
				}
				d = next
			}
		}
	}
	if r.MeasuredMS > 0 {
		r.Availability = max(0, 1-float64(r.Internet.MS)/float64(r.MeasuredMS))
	}

	line, err := buildLine(ctx, st, p, from, to, gaps, &r, dayIdx)
	if err != nil {
		return Report{}, err
	}
	r.Line = line
	return r, nil
}

func buildLine(ctx context.Context, st *store.Store, p Params, from, to int64, gaps []span, r *Report, dayIdx func(int64) int) (*Line, error) {
	ls, err := st.LineSamplesBetween(ctx, from, to)
	if err != nil || len(ls) == 0 {
		return nil, err
	}
	l := &Line{DownMin: -1, UpMin: -1}
	var downSum, upSum float64
	var upMS int64
	access := map[string]int64{}
	for i, s := range ls {
		end := s.TS + lineValidMS
		if i+1 < len(ls) {
			end = min(end, ls[i+1].TS)
		}
		a, b := max(s.TS, from), min(end, to)
		if s.Reconnect && s.TS >= from && s.TS < to {
			l.Reconnects++
		}
		if b <= a {
			continue
		}
		ms := without(a, b, gaps)
		if ms == 0 {
			continue
		}
		l.LineMS += ms
		access[s.Access] += ms
		if !s.Up || s.DownKbps == 0 {
			l.DownMS += ms
			continue
		}
		upMS += ms
		downSum += float64(s.DownKbps) * float64(ms)
		upSum += float64(s.UpKbps) * float64(ms)
		if l.DownMin < 0 || s.DownKbps < l.DownMin {
			l.DownMin = s.DownKbps
		}
		if l.UpMin < 0 || s.UpKbps < l.UpMin {
			l.UpMin = s.UpKbps
		}
		l.DownMax, l.UpMax = max(l.DownMax, s.DownKbps), max(l.UpMax, s.UpKbps)
		if c := p.Contract.MinDown; c > 0 && float64(s.DownKbps) < c*1000 {
			l.BelowMinPct += float64(ms)
		}
		if c := p.Contract.NormalDown; c > 0 && float64(s.DownKbps) < c*1000 {
			l.BelowNormalPct += float64(ms)
		}
		if i := dayIdx(a); i >= 0 && i < len(r.Days) {
			d := &r.Days[i]
			if d.SyncDownMin == 0 || s.DownKbps < d.SyncDownMin {
				d.SyncDownMin = s.DownKbps
			}
			d.SyncDownMax = max(d.SyncDownMax, s.DownKbps)
		}
	}
	if l.LineMS == 0 {
		return nil, nil
	}
	// A line that was down counts as below every contract value.
	if p.Contract.MinDown > 0 {
		l.BelowMinPct += float64(l.DownMS)
	}
	if p.Contract.NormalDown > 0 {
		l.BelowNormalPct += float64(l.DownMS)
	}
	l.BelowMinPct = 100 * l.BelowMinPct / float64(l.LineMS)
	l.BelowNormalPct = 100 * l.BelowNormalPct / float64(l.LineMS)
	if upMS > 0 {
		l.DownAvg, l.UpAvg = int(downSum/float64(upMS)+0.5), int(upSum/float64(upMS)+0.5)
	}
	l.DownMin, l.UpMin = max(l.DownMin, 0), max(l.UpMin, 0)
	// The access type that covered most of the time.
	kinds := make([]string, 0, len(access))
	for k := range access {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		return access[kinds[i]] > access[kinds[j]] || access[kinds[i]] == access[kinds[j]] && kinds[i] < kinds[j]
	})
	l.Access = kinds[0]
	return l, nil
}
