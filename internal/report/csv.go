package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/Pyzyi/Netward/internal/store"
)

// RawCSV returns the raw data behind a report: outages, monitoring gaps,
// latency buckets and line samples of the period, times in UTC (RFC 3339).
// The same period always gives the same bytes, so the checksum printed in
// the PDF identifies the file.
func RawCSV(ctx context.Context, st *store.Store, from, to time.Time) ([]byte, error) {
	f, t := from.UnixMilli(), to.UnixMilli()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	ts := func(ms int64) string {
		if ms == 0 {
			return ""
		}
		return time.UnixMilli(ms).UTC().Format(time.RFC3339)
	}
	itoa := strconv.Itoa
	_ = w.Write([]string{"record", "start_utc", "end_utc", "kind", "median_ms", "max_ms", "lost", "checks",
		"down_kbps", "up_kbps", "down_attainable_kbps", "up_attainable_kbps", "reconnect"})
	outs, err := st.OutagesBetween(ctx, f, t)
	if err != nil {
		return nil, err
	}
	for _, o := range outs {
		_ = w.Write([]string{"outage", ts(o.Start), ts(o.End), o.Kind, "", "", "", "", "", "", "", "", ""})
	}
	gaps, err := st.ListGaps(ctx, f, t)
	if err != nil {
		return nil, err
	}
	for _, g := range gaps {
		_ = w.Write([]string{"not_measured", ts(g.Start), ts(g.End), "netward_not_running", "", "", "", "", "", "", "", "", ""})
	}
	lat, err := st.LatencyBetween(ctx, f-bucketMS+1, t)
	if err != nil {
		return nil, err
	}
	for _, l := range lat {
		_ = w.Write([]string{"latency", ts(l.TS), ts(l.TS + bucketMS), "", itoa(l.Median), itoa(l.Max), itoa(l.Lost), itoa(l.Samples),
			"", "", "", "", ""})
	}
	lines, err := st.LineSamplesBetween(ctx, f, t)
	if err != nil {
		return nil, err
	}
	for _, l := range lines {
		kind := l.Access
		if !l.Up {
			kind += "_down"
		}
		rc := "0"
		if l.Reconnect {
			rc = "1"
		}
		_ = w.Write([]string{"line", ts(l.TS), "", kind, "", "", "", "", itoa(l.DownKbps), itoa(l.UpKbps), itoa(l.DownMaxKbps), itoa(l.UpMaxKbps), rc})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// Checksum is the hex SHA-256 of b.
func Checksum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// FileName is the base name for the report files of a period.
func FileName(from, to time.Time, loc *time.Location) string {
	return "netward-nachweis-" + from.In(loc).Format("2006-01-02") + "-" + to.Add(-time.Millisecond).In(loc).Format("2006-01-02")
}
