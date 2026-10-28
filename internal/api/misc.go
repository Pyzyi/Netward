package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/software"
	"github.com/Pyzyi/netzwart/internal/store"
	"github.com/Pyzyi/netzwart/internal/textutil"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	writeJSON(w, http.StatusOK, s.d.Settings().Public())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	var st config.Settings
	if !readJSON(w, r, &st) {
		return
	}
	current := s.d.Settings()
	if err := st.MergeSecrets(current); err != nil {
		writeError(w, http.StatusBadRequest, "secret_required", strings.TrimPrefix(err.Error(), "secret_required: "))
		return
	}
	st.NetworkName = textutil.Clean(st.NetworkName, 64)
	if st.InternetTargets == nil {
		st.InternetTargets = current.InternetTargets
	}
	if err := st.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_settings", err.Error())
		return
	}
	if slices.Contains(st.TripwirePorts, s.d.Env.Port) {
		writeError(w, http.StatusBadRequest, "invalid_settings", "tripwirePorts: must not be the port of the web interface")
		return
	}
	if err := s.d.ApplySettings(r.Context(), st); err != nil {
		s.fail(w, r, err)
		return
	}
	s.d.Monitor.AddEvent(r.Context(), "config_settings", "info", 0, map[string]any{"ip": s.clientIP(r).String()})
	writeJSON(w, http.StatusOK, st.Public())
}

func publicChannel(c store.Channel) store.Channel {
	c.Config = notify.Masked(c.Kind, c.Config)
	if c.Events == nil {
		c.Events = []string{}
	}
	return c
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	chs, err := s.d.Store.ListChannels(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]store.Channel, 0, len(chs))
	for _, c := range chs {
		out = append(out, publicChannel(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": out, "kinds": notify.Kinds(), "events": notify.Subscribable})
}

type channelRequest struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind"`
	Config  json.RawMessage `json:"config"`
	Events  []string        `json:"events"`
	Enabled bool            `json:"enabled"`
}

func (s *Server) buildChannel(w http.ResponseWriter, req channelRequest, old *store.Channel) (store.Channel, bool) {
	name := textutil.Clean(req.Name, 64)
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_name", "")
		return store.Channel{}, false
	}
	if !notify.ValidKind(req.Kind) || (old != nil && old.Kind != req.Kind) {
		writeError(w, http.StatusBadRequest, "invalid_kind", "")
		return store.Channel{}, false
	}
	var events []string
	for _, e := range req.Events {
		if slices.Contains(notify.Subscribable, e) && !slices.Contains(events, e) {
			events = append(events, e)
		}
	}
	var oldCfg json.RawMessage
	if old != nil && old.Kind == req.Kind {
		oldCfg = old.Config
	}
	if len(req.Config) == 0 {
		req.Config = json.RawMessage("{}")
	}
	cfg, err := notify.MergeConfig(req.Kind, req.Config, oldCfg)
	if errors.Is(err, notify.ErrSecretRequired) {
		writeError(w, http.StatusBadRequest, "secret_required", strings.TrimPrefix(err.Error(), "secret_required: "))
		return store.Channel{}, false
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return store.Channel{}, false
	}
	c := store.Channel{Name: name, Kind: req.Kind, Config: cfg, Events: events, Enabled: req.Enabled}
	if old != nil {
		c.ID, c.CreatedAt = old.ID, old.CreatedAt
	}
	return c, true
}

func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	var req channelRequest
	if !readJSON(w, r, &req) {
		return
	}
	chs, err := s.d.Store.ListChannels(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(chs) >= 20 {
		writeError(w, http.StatusBadRequest, "too_many_channels", "")
		return
	}
	c, ok := s.buildChannel(w, req, nil)
	if !ok {
		return
	}
	if err := s.d.Store.SaveChannel(r.Context(), &c); err != nil {
		s.fail(w, r, err)
		return
	}
	s.afterChannelChange(r, "created", c)
	writeJSON(w, http.StatusCreated, publicChannel(c))
}

func (s *Server) handleUpdateChannel(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	var req channelRequest
	if !readJSON(w, r, &req) {
		return
	}
	old, err := s.d.Store.GetChannel(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c, ok := s.buildChannel(w, req, &old)
	if !ok {
		return
	}
	if err := s.d.Store.SaveChannel(r.Context(), &c); err != nil {
		s.fail(w, r, err)
		return
	}
	s.afterChannelChange(r, "updated", c)
	writeJSON(w, http.StatusOK, publicChannel(c))
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	old, err := s.d.Store.GetChannel(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.d.Store.DeleteChannel(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.afterChannelChange(r, "deleted", old)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) afterChannelChange(r *http.Request, action string, c store.Channel) {
	if err := s.d.Notify.Reload(r.Context()); err != nil {
		s.d.Log.Error("reload channels", "err", err)
	}
	s.d.Monitor.AddEvent(r.Context(), "config_channel", "info", 0, map[string]any{"action": action, "channel": c.Name,
		"channelKind": c.Kind, "ip": s.clientIP(r).String()})
}

func (s *Server) handleTestChannel(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	c, err := s.d.Store.GetChannel(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := s.d.Notify.Test(ctx, c); err != nil {
		var se *notify.SendError
		msg := "delivery failed"
		if errors.As(err, &se) {
			msg = se.Reason
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "reason": msg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleInternet(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	outs, err := s.d.Store.ListOutages(r.Context(), s.now().Add(-30*24*time.Hour).UnixMilli())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": s.d.Internet.Status(), "history": s.d.Internet.History(), "outages": outs,
		"line": s.d.Internet.Line()})
}

// csvSafe prevents spreadsheet formula injection (CWE-1236).
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	devs := s.d.Monitor.Devices()
	stamp := s.now().Format("2006-01-02")
	switch r.URL.Query().Get("format") {
	case "json":
		w.Header().Set("Content-Disposition", `attachment; filename="netzwart-`+stamp+`.json"`)
		writeJSON(w, http.StatusOK, map[string]any{"exportedAt": s.now().UTC().Format(time.RFC3339), "version": s.d.Version, "devices": devs})
	default:
		var buf bytes.Buffer
		buf.WriteString("\xef\xbb\xbf") // UTF-8 BOM so spreadsheet apps detect the encoding
		cw := csv.NewWriter(&buf)
		_ = cw.Write([]string{"name", "hostname", "ip", "mac", "vendor", "type", "location", "known", "favorite", "watch",
			"online", "first_seen", "last_seen", "notes", "model", "software", "purchase_date", "warranty_until", "serial"})
		ts := func(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339) }
		sw := func(d store.Device) string {
			if d.Software == nil {
				return ""
			}
			return csvSafe(d.Software.Name)
		}
		for _, d := range devs {
			_ = cw.Write([]string{csvSafe(d.Name), csvSafe(d.Hostname), d.IP, d.MAC, csvSafe(d.Vendor), d.Type,
				csvSafe(d.Location), strconv.FormatBool(d.Known), strconv.FormatBool(d.Favorite), strconv.FormatBool(d.Watch),
				strconv.FormatBool(d.Online), ts(d.FirstSeen), ts(d.LastSeen), csvSafe(d.Notes), csvSafe(d.Model), sw(d),
				d.PurchaseDate, d.WarrantyUntil, csvSafe(d.Serial)})
		}
		cw.Flush()
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="netzwart-`+stamp+`.csv"`)
		_, _ = w.Write(buf.Bytes())
	}
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	var mqttStatus any
	if s.d.MQTT != nil {
		mqttStatus = s.d.MQTT.Status()
	}
	var fritzStatus any
	if s.d.FritzBox != nil {
		fritzStatus = s.d.FritzBox.Status()
	}
	var upd any
	if s.d.Update != nil {
		upd = s.d.Update.Status()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"update": upd, "softwareDataAsOf": software.DataAsOf,
		"version": s.d.Version, "startedAt": s.d.StartedAt.UnixMilli(), "goVersion": runtime.Version(),
		"uid": os.Getuid(), "rawSockets": s.d.RawSockets, "mode": s.scannerStatus().Mode,
		"dbSize": s.d.Store.Size(), "memory": ms.Sys, "mqtt": mqttStatus, "fritzbox": fritzStatus, "https": s.isHTTPS(r),
		"streams": s.streams.count(),
	})
}
