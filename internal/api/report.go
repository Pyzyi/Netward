package api

import (
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/Pyzyi/netzwart/internal/report"
	"github.com/Pyzyi/netzwart/internal/textutil"
)

type reportRequest struct {
	From     string          `json:"from"` // YYYY-MM-DD, first day
	To       string          `json:"to"`   // YYYY-MM-DD, last day (inclusive)
	Format   string          `json:"format"`
	Contract report.Contract `json:"contract"`
	// Only printed in the PDF, never stored or logged.
	Holder   string `json:"holder"`
	Customer string `json:"customer"`
	Address  string `json:"address"`
}

// handleReport returns the provider report as PDF or its raw data as CSV
// (docs/09, F5). Only whole days that are over count, so the PDF and the
// CSV fetched a moment later contain the same data and the checksum in the
// PDF matches the file.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	var req reportRequest
	if !readJSON(w, r, &req) {
		return
	}
	st := s.d.Settings()
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		loc = time.UTC
	}
	from, err1 := time.ParseInLocation("2006-01-02", req.From, loc)
	last, err2 := time.ParseInLocation("2006-01-02", req.To, loc)
	now := s.now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	to := time.Date(last.Year(), last.Month(), last.Day()+1, 0, 0, 0, 0, loc)
	if to.After(today) {
		to = today
	}
	c := req.Contract
	validRate := func(v float64) bool { return !math.IsNaN(v) && v >= 0 && v <= 100000 }
	if err1 != nil || err2 != nil || !to.After(from) || to.Sub(from) > report.MaxPeriod ||
		!validRate(c.MaxDown) || !validRate(c.NormalDown) || !validRate(c.MinDown) ||
		!validRate(c.MaxUp) || !validRate(c.NormalUp) || !validRate(c.MinUp) ||
		(req.Format != "pdf" && req.Format != "csv") {
		writeError(w, http.StatusBadRequest, "invalid_input", "")
		return
	}
	name := report.FileName(from, to, loc)
	csv, err := report.RawCSV(r.Context(), s.d.Store, from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if req.Format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
		_, _ = w.Write(csv)
		return
	}
	rep, err := report.Build(r.Context(), s.d.Store, report.Params{From: from, To: to, Contract: c, Loc: loc})
	if errors.Is(err, report.ErrPeriod) {
		writeError(w, http.StatusBadRequest, "invalid_input", "")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := report.PDF(rep, report.Info{
		Holder: textutil.Clean(req.Holder, 100), Customer: textutil.Clean(req.Customer, 64), Address: textutil.Clean(req.Address, 200),
		Version: s.d.Version, Lang: st.Language, Loc: loc, Created: s.now(), CSVName: name + ".csv", CSVSum: report.Checksum(csv),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.pdf"`)
	_, _ = w.Write(out)
}
