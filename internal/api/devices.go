package api

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/scanner"
	"github.com/Pyzyi/netzwart/internal/software"
	"github.com/Pyzyi/netzwart/internal/store"
)

type counts struct {
	Total      int `json:"total"`
	Online     int `json:"online"`
	Offline    int `json:"offline"`
	New        int `json:"new"`
	Unreviewed int `json:"unreviewed"`
}

type securitySummary struct {
	Critical  int   `json:"critical"`
	Warning   int   `json:"warning"`
	Info      int   `json:"info"`
	LastRunAt int64 `json:"lastRunAt"`
	Running   bool  `json:"running"`
}

type overviewResponse struct {
	NetworkName string          `json:"networkName"`
	Counts      counts          `json:"counts"`
	Scanner     scanner.Status  `json:"scanner"`
	Internet    any             `json:"internet"`
	Security    securitySummary `json:"security"`
	Baseline    int64           `json:"baseline"`
	Types       map[string]int  `json:"types"`
	Software    map[string]int  `json:"software"` // status -> device count
}

func (s *Server) scannerStatus() scanner.Status {
	if s.d.Scanner == nil {
		now := s.now()
		return scanner.Status{Mode: "demo", LastScanAt: now.Add(-40 * time.Second).UnixMilli(), LastScanDuration: 4200,
			NextScanAt: now.Add(50 * time.Second).UnixMilli(),
			Interfaces: []scanner.IfaceStatus{{Name: "eth0", MAC: "00:e0:4c:68:10:14", Addr: "192.168.178.20",
				Prefix: "192.168.178.0/24", Hosts: 254, Active: true, Default: true}},
			Gateways: []string{"192.168.178.1"}}
	}
	return s.d.Scanner.Status()
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	devs := s.d.Monitor.Devices()
	resp := overviewResponse{NetworkName: s.d.Settings().NetworkName, Scanner: s.scannerStatus(),
		Internet: s.d.Internet.Status(), Baseline: s.d.Monitor.Baseline(), Types: map[string]int{},
		Software: map[string]int{software.StatusOutdated: 0, software.StatusAging: 0, software.StatusUpdate: 0,
			software.StatusSupported: 0, software.StatusUnknown: 0}}
	for _, d := range devs {
		resp.Counts.Total++
		if d.Online {
			resp.Counts.Online++
		} else {
			resp.Counts.Offline++
		}
		if d.New {
			resp.Counts.New++
		}
		if !d.Known && !d.IsSelf {
			resp.Counts.Unreviewed++
		}
		resp.Types[d.Type]++
		if d.Software != nil {
			resp.Software[d.Software.Status]++
		} else {
			resp.Software[software.StatusUnknown]++
		}
	}
	findings, err := s.d.Store.ListFindings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, f := range findings {
		if f.Dismissed {
			continue
		}
		switch f.Severity {
		case "critical":
			resp.Security.Critical++
		case "warning":
			resp.Security.Warning++
		default:
			resp.Security.Info++
		}
	}
	st := s.d.Security.Status()
	resp.Security.LastRunAt, resp.Security.Running = st.LastRunAt, st.Running
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	writeJSON(w, http.StatusOK, s.d.Monitor.Devices())
}

type deviceResponse struct {
	Device    store.Device        `json:"device"`
	Details   monitor.Details     `json:"details"`
	Addresses []store.Address     `json:"addresses"`
	Ports     []store.Port        `json:"ports"`
	Software  software.Assessment `json:"software"`
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	d, ok := s.d.Monitor.Device(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	det, _ := s.d.Monitor.DeviceDetails(id)
	addrs, err := s.d.Store.ListAddresses(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ports, err := s.d.Store.ListPorts(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := ports[id]
	if p == nil {
		p = []store.Port{}
	}
	sw, _ := s.d.Monitor.Software(id)
	writeJSON(w, http.StatusOK, deviceResponse{Device: d, Details: det, Addresses: addrs, Ports: p, Software: sw})
}

// softwareRequest sets what the user knows about a device's software.
// Manual names a system and major version the network cannot see (iPhones);
// Edition marks a Windows device as Enterprise/LTSC/Server. Empty clears.
type softwareRequest struct {
	Manual *struct {
		Product string `json:"product"`
		Version string `json:"version"`
	} `json:"manual"`
	Edition *string `json:"edition"`
}

var manualVersionRe = regexp.MustCompile(`^[0-9]{1,2}(\.[0-9]{1,2})?$`)

func (s *Server) handleDeviceSoftware(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	var req softwareRequest
	if !readJSON(w, r, &req) {
		return
	}
	if req.Manual != nil {
		p, v := req.Manual.Product, req.Manual.Version
		switch {
		case p == "" && v == "":
		case (p == software.IOS || p == software.IPadOS || p == software.MacOS || p == software.TvOS || p == software.Android) &&
			manualVersionRe.MatchString(v):
		default:
			writeError(w, http.StatusBadRequest, "invalid_input", "manual")
			return
		}
		if err := s.d.Monitor.SetSoftwareManual(r.Context(), id, p, v); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if req.Edition != nil {
		if *req.Edition != "" && *req.Edition != software.EditionExtended {
			writeError(w, http.StatusBadRequest, "invalid_input", "edition")
			return
		}
		if err := s.d.Monitor.SetEdition(r.Context(), id, *req.Edition); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	sw, ok := s.d.Monitor.Software(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	writeJSON(w, http.StatusOK, sw)
}

func (s *Server) handlePatchDevice(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	var p store.DevicePatch
	if !readJSON(w, r, &p) {
		return
	}
	d, err := s.d.Monitor.PatchDevice(r.Context(), id, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	if err := s.d.Monitor.DeleteDevice(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMerge(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	var req struct {
		Into int64 `json:"into"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	d, err := s.d.Monitor.MergeDevices(r.Context(), id, req.Into)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleAcknowledge(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	n, err := s.d.Monitor.AcknowledgeAll(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

type presenceResponse struct {
	From      int64                `json:"from"`
	To        int64                `json:"to"`
	Spans     []store.PresenceSpan `json:"spans"`
	Gaps      []store.Gap          `json:"gaps"`
	OnlinePct float64              `json:"onlinePct"`
}

func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	d, ok := s.d.Monitor.Device(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	ranges := map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}
	dur, ok := ranges[r.URL.Query().Get("range")]
	if !ok {
		dur = 24 * time.Hour
	}
	to := s.now().UnixMilli()
	from := to - dur.Milliseconds()
	spans, err := s.d.Store.ListPresence(r.Context(), id, from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	gaps, err := s.d.Store.ListGaps(r.Context(), from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Online share over the time Netzwart actually observed.
	observedFrom := max(from, d.FirstSeen)
	clip := func(a, b int64) int64 { return max(0, min(b, to)-max(a, observedFrom)) }
	var online, gapTotal int64
	for _, sp := range spans {
		end := sp.End
		if end == 0 {
			end = to
		}
		online += clip(sp.Start, end)
	}
	for _, g := range gaps {
		gapTotal += clip(g.Start, g.End)
	}
	pct := 0.0
	if observed := to - observedFrom - gapTotal; observed > 0 {
		pct = min(100, float64(online)/float64(observed)*100)
	}
	if spans == nil {
		spans = []store.PresenceSpan{}
	}
	writeJSON(w, http.StatusOK, presenceResponse{From: from, To: to, Spans: spans, Gaps: gaps, OnlinePct: pct})
}

// Presence strips: the last 24 hours of every device as 48 half-hour slots,
// for the heartbeat column of the device list.
const (
	stripSlots = 48
	stripSlot  = 30 * time.Minute
)

type stripsResponse struct {
	From  int64            `json:"from"`
	Slot  int64            `json:"slot"`
	Strip map[int64]string `json:"strips"` // per slot: 1 online, 0 offline, - not observed
}

func (s *Server) handlePresenceStrips(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	slot := stripSlot.Milliseconds()
	now := s.now().UnixMilli()
	to := (now/slot + 1) * slot // end of the current slot
	from := to - stripSlots*slot
	spans, err := s.d.Store.ListAllPresence(r.Context(), from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	gaps, err := s.d.Store.ListGaps(r.Context(), from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Adjacent or overlapping gaps (ordered by start) are merged first, so a
	// slot spanned by two back-to-back gaps still counts as unmonitored.
	var merged []store.Gap
	for _, g := range gaps {
		if n := len(merged); n > 0 && g.Start <= merged[n-1].End {
			merged[n-1].End = max(merged[n-1].End, g.End)
			continue
		}
		merged = append(merged, g)
	}
	covered := func(a, b int64) bool { // slot fully inside a monitoring gap
		for _, g := range merged {
			if g.Start <= a && g.End >= b {
				return true
			}
		}
		return false
	}
	out := stripsResponse{From: from, Slot: slot, Strip: map[int64]string{}}
	for _, d := range s.d.Monitor.Devices() {
		b := make([]byte, stripSlots)
		for i := range b {
			a, e := from+int64(i)*slot, from+int64(i+1)*slot
			switch {
			case e <= d.FirstSeen || covered(a, e):
				b[i] = '-'
			default:
				b[i] = '0'
			}
			for _, sp := range spans[d.ID] {
				end := sp.End
				if end == 0 {
					end = now
				}
				if sp.Start < e && end > a {
					b[i] = '1'
					break
				}
			}
		}
		out.Strip[d.ID] = string(b)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeviceEvents(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	evs, err := s.d.Store.ListEvents(r.Context(), store.EventQuery{DeviceID: id, Limit: 50})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *Server) requireScanner(w http.ResponseWriter) bool {
	if s.d.Scanner == nil {
		writeError(w, http.StatusConflict, "demo_mode", "")
		return false
	}
	return true
}

func (s *Server) handleWake(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok || !s.requireScanner(w) {
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_id", "")
		}
		return
	}
	if err := s.d.Scanner.Wake(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.fail(w, r, err)
			return
		}
		writeError(w, http.StatusConflict, "wake_failed", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok || !s.requireScanner(w) {
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_id", "")
		}
		return
	}
	d, found := s.d.Monitor.Device(id)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	rtt, alive := s.d.Scanner.Ping(r.Context(), d)
	writeJSON(w, http.StatusOK, map[string]any{"ok": alive, "rttMs": float64(rtt.Microseconds()) / 1000})
}

func (s *Server) handlePortScan(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok || !s.requireScanner(w) {
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_id", "")
		}
		return
	}
	ports, err := s.d.Scanner.PortScan(r.Context(), id)
	if errors.Is(err, scanner.ErrBusy) {
		writeError(w, http.StatusConflict, "busy", "")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if ports == nil {
		ports = []store.Port{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ports": ports})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	q := r.URL.Query()
	eq := store.EventQuery{Limit: 100}
	if v, err := strconv.ParseInt(q.Get("before"), 10, 64); err == nil && v > 0 {
		eq.Before = v
	}
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 200 {
		eq.Limit = v
	}
	for _, k := range strings.Split(q.Get("kind"), ",") {
		if k = strings.TrimSpace(k); k != "" && len(k) <= 32 && len(eq.Kinds) < 16 {
			eq.Kinds = append(eq.Kinds, k)
		}
	}
	evs, err := s.d.Store.ListEvents(r.Context(), eq)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	if !s.requireScanner(w) {
		return
	}
	s.d.Scanner.TriggerScan()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleSecurity(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	findings, err := s.d.Store.ListFindings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": s.d.Security.Status(), "findings": findings})
}

func (s *Server) handleSecurityRun(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	if s.d.Scanner == nil {
		writeError(w, http.StatusConflict, "demo_mode", "")
		return
	}
	if !s.d.Security.RunAsync(r.Context()) {
		writeError(w, http.StatusConflict, "busy", "")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleDismissFinding(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	var req struct {
		Dismissed bool `json:"dismissed"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.d.Store.SetFindingDismissed(r.Context(), id, req.Dismissed); err != nil {
		s.fail(w, r, err)
		return
	}
	s.d.Hub.Publish("security", s.d.Security.Status())
	w.WriteHeader(http.StatusNoContent)
}
