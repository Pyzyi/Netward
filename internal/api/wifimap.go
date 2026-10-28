package api

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // floor plans: JPEG and PNG
	_ "image/png"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/Pyzyi/netzwart/internal/netinfo"
	"github.com/Pyzyi/netzwart/internal/store"
	"github.com/Pyzyi/netzwart/internal/textutil"
	"github.com/Pyzyi/netzwart/internal/wifimap"
)

// Wi-Fi map (docs/10, F4): floor plans, measuring points and the
// endpoints the browser measures against.

const (
	maxPlanBytes   = 8 << 20
	maxPlanPixels  = 6000 // per side
	maxProbeBytes  = 64 << 20
	probeChunkSize = 1 << 20
)

// probeData is sent by the download test. Random, so no proxy or browser
// can compress it; generated once.
var probeData = func() []byte {
	b := make([]byte, probeChunkSize)
	r := rand.New(rand.NewChaCha8([32]byte{'n', 'e', 't', 'z', 'w', 'a', 'r', 't'}))
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	return b
}()

// probeSlots limits concurrent throughput tests; two phones may measure at
// once, more would only measure each other.
var probeSlots = make(chan struct{}, 2)

func takeProbeSlot(w http.ResponseWriter) bool {
	select {
	case probeSlots <- struct{}{}:
		return true
	default:
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusTooManyRequests, "busy", "")
		return false
	}
}

// handleWifiPing answers the latency test. With ?check=1 it says whether
// the browser runs on the Netzwart host itself (or behind an untrusted proxy
// there), where measuring the Wi-Fi makes no sense; asked before measuring.
func (s *Server) handleWifiPing(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	if r.URL.Query().Has("check") {
		writeJSON(w, http.StatusOK, map[string]bool{"local": s.measuredLocally(r)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) measuredLocally(r *http.Request) bool {
	ip := s.clientIP(r)
	return !s.d.Env.Demo && (ip.IsLoopback() || netinfo.IsOwnAddr(ip))
}

// handleWifiDownload streams up to ?bytes= random bytes. The browser stops
// reading after a few seconds and computes the rate itself.
func (s *Server) handleWifiDownload(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	n, err := strconv.Atoi(r.URL.Query().Get("bytes"))
	if err != nil || n <= 0 || n > maxProbeBytes {
		writeError(w, http.StatusBadRequest, "invalid_input", "")
		return
	}
	if !takeProbeSlot(w) {
		return
	}
	defer func() { <-probeSlots }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(n))
	w.Header().Set("Content-Encoding", "identity")
	for n > 0 && r.Context().Err() == nil {
		k := min(n, len(probeData))
		if _, err := w.Write(probeData[:k]); err != nil {
			return
		}
		n -= k
	}
}

// handleWifiUpload reads and discards the body and reports how long that took.
func (s *Server) handleWifiUpload(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	if !takeProbeSlot(w) {
		return
	}
	defer func() { <-probeSlots }()
	start := time.Now()
	n, err := io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, maxProbeBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"bytes": n, "ms": time.Since(start).Milliseconds()})
}

func (s *Server) handleFloorPlans(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	plans, err := s.d.Store.FloorPlans(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "")
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

func planName(v string) (string, bool) {
	v = textutil.Clean(v, 40)
	return v, v != ""
}

// handleAddFloorPlan stores an image sent as the raw body (image/png or
// image/jpeg). The browser shrinks and re-encodes photos before, which
// also drops their location data.
func (s *Server) handleAddFloorPlan(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	name, ok := planName(r.URL.Query().Get("name"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_input", "name")
		return
	}
	mime := r.Header.Get("Content-Type")
	if mime != "image/png" && mime != "image/jpeg" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "use image/png or image/jpeg")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPlanBytes))
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "upload_failed", "") // e.g. a slow link hit the read timeout
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || "image/"+format != mime || cfg.Width < 16 || cfg.Height < 16 || cfg.Width > maxPlanPixels || cfg.Height > maxPlanPixels {
		writeError(w, http.StatusBadRequest, "invalid_image", "")
		return
	}
	p, err := s.d.Store.AddFloorPlan(r.Context(), store.FloorPlan{Name: name, Mime: mime, Width: cfg.Width, Height: cfg.Height,
		CreatedAt: time.Now().UnixMilli()}, data)
	switch {
	case errors.Is(err, store.ErrLimit):
		writeError(w, http.StatusConflict, "limit", "")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "")
	default:
		writeJSON(w, http.StatusCreated, p)
	}
}

func (s *Server) handleFloorPlanImage(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	img, mime, err := s.d.Store.FloorPlanImage(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=86400") // the image of an ID never changes
	_, _ = w.Write(img)
}

func (s *Server) handleRenameFloorPlan(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	name, ok := planName(req.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_input", "name")
		return
	}
	s.writeStoreResult(w, s.d.Store.RenameFloorPlan(r.Context(), id, name))
}

func (s *Server) handleDeleteFloorPlan(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	s.writeStoreResult(w, s.d.Store.DeleteFloorPlan(r.Context(), id))
}

func (s *Server) writeStoreResult(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

type wifiMapView struct {
	Points []store.WifiPoint `json:"points"`
	Hints  []wifimap.Hint    `json:"hints"`
	Ref    float64           `json:"refMbps"` // what the colours compare with
}

func (s *Server) wifiMap(r *http.Request, id int64) (wifiMapView, error) {
	plans, err := s.d.Store.FloorPlans(r.Context())
	if err != nil {
		return wifiMapView{}, err
	}
	for _, p := range plans {
		if p.ID == id {
			pts, err := s.d.Store.WifiPoints(r.Context(), id)
			if err != nil {
				return wifiMapView{}, err
			}
			ref, _ := wifimap.Reference(pts)
			return wifiMapView{Points: pts, Hints: wifimap.Hints(pts, p.Width, p.Height), Ref: ref.DownMbps}, nil
		}
	}
	return wifiMapView{}, store.ErrNotFound
}

func (s *Server) handleWifiPoints(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	v, err := s.wifiMap(r, id)
	if err != nil {
		s.writeStoreResult(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type pointRequest struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Room      string  `json:"room"`
	DownMbps  float64 `json:"downMbps"`
	UpMbps    float64 `json:"upMbps"`
	LatencyMs float64 `json:"latencyMs"`
	JitterMs  float64 `json:"jitterMs"`
}

func inRange(v, lo, hi float64) bool { return !math.IsNaN(v) && v >= lo && v <= hi }

// handleAddWifiPoint stores a measurement. Which device measured is taken
// from the client address; with a FRITZ!Box also its band and signal.
func (s *Server) handleAddWifiPoint(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	var req pointRequest
	if !readJSON(w, r, &req) {
		return
	}
	req.Room = textutil.Clean(req.Room, 40)
	if !inRange(req.X, 0, 1) || !inRange(req.Y, 0, 1) ||
		!inRange(req.DownMbps, 0, 100_000) || !inRange(req.UpMbps, 0, 100_000) ||
		!inRange(req.LatencyMs, 0, 60_000) || !inRange(req.JitterMs, 0, 60_000) {
		writeError(w, http.StatusBadRequest, "invalid_input", "")
		return
	}
	// Measured on the Netzwart host itself, the Wi-Fi is not involved at all.
	if s.measuredLocally(r) {
		writeError(w, http.StatusBadRequest, "measure_local", "")
		return
	}
	p := store.WifiPoint{PlanID: id, X: req.X, Y: req.Y, Room: req.Room, DownMbps: req.DownMbps, UpMbps: req.UpMbps,
		LatencyMs: req.LatencyMs, JitterMs: req.JitterMs, TS: time.Now().UnixMilli()}
	if d, ok := s.d.Monitor.DeviceByIP(s.clientIP(r)); ok {
		p.DeviceID = d.ID
		if s.d.WLAN != nil {
			if c, ap, ok := s.d.WLAN.Link(d.MAC); ok {
				p.Band, p.SignalDBm, p.AP = c.Band, c.SignalDBm, ap
			}
		}
	}
	if _, err := s.wifiMap(r, id); err != nil {
		s.writeStoreResult(w, err)
		return
	}
	if _, err := s.d.Store.AddWifiPoint(r.Context(), p); err != nil {
		if errors.Is(err, store.ErrLimit) {
			writeError(w, http.StatusConflict, "limit", "")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "")
		return
	}
	v, err := s.wifiMap(r, id)
	if err != nil {
		s.writeStoreResult(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) handleDeleteWifiPoint(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok1 := pathID(r, "id")
	pid, ok2 := pathID(r, "point")
	if !ok1 || !ok2 {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	if err := s.d.Store.DeleteWifiPoint(r.Context(), id, pid); err != nil {
		s.writeStoreResult(w, err)
		return
	}
	v, err := s.wifiMap(r, id)
	if err != nil {
		s.writeStoreResult(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
