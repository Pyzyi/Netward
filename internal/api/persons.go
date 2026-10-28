package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/Pyzyi/Netward/internal/store"
	"github.com/Pyzyi/Netward/internal/textutil"
)

// Persons (docs/09, F4).

const maxPersonDevices = 20

func (s *Server) handlePersons(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	loc, err := time.LoadLocation(s.d.Settings().Timezone)
	if err != nil {
		loc = time.UTC
	}
	out, err := s.d.Persons.Statuses(r.Context(), s.now(), loc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func personName(raw string) (string, bool) {
	n := textutil.Clean(raw, 40)
	return n, n != ""
}

func (s *Server) handleCreatePerson(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	name, ok := personName(req.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_input", "name")
		return
	}
	p, err := s.d.Store.CreatePerson(r.Context(), name)
	if errors.Is(err, store.ErrLimit) {
		writeError(w, http.StatusConflict, "limit", "")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	_ = s.d.Persons.Changed(r.Context())
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) person(w http.ResponseWriter, r *http.Request) (store.Person, bool) {
	id, ok := pathID(r, "id")
	if ok {
		for _, p := range s.d.Persons.List() {
			if p.ID == id {
				return p, true
			}
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "")
	return store.Person{}, false
}

func (s *Server) handleUpdatePerson(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	p, ok := s.person(w, r)
	if !ok {
		return
	}
	var req struct {
		Name          string `json:"name"`
		TrackTime     bool   `json:"trackTime"`
		Consent       bool   `json:"consent"` // the person knows that their time at home is recorded
		HomeAssistant bool   `json:"homeAssistant"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	name, ok := personName(req.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_input", "name")
		return
	}
	// Time per day and Home Assistant (which keeps its own history of
	// arrivals and departures) both record a person: each needs the
	// confirmation that the person knows. Switching both off withdraws it.
	switch {
	case !req.TrackTime && !req.HomeAssistant:
		p.ConsentAt = 0
	case p.ConsentAt == 0 && !req.Consent:
		writeError(w, http.StatusBadRequest, "consent_required", "")
		return
	case p.ConsentAt == 0:
		p.ConsentAt = s.now().UnixMilli()
	}
	p.Name, p.TrackTime, p.HA = name, req.TrackTime, req.HomeAssistant
	if err := s.d.Store.UpdatePerson(r.Context(), p); err != nil {
		s.fail(w, r, err)
		return
	}
	_ = s.d.Persons.Changed(r.Context())
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePerson(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	p, ok := s.person(w, r)
	if !ok {
		return
	}
	if err := s.d.Store.DeletePerson(r.Context(), p.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	_ = s.d.Persons.Changed(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePersonDevices(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	p, ok := s.person(w, r)
	if !ok {
		return
	}
	var req struct {
		Devices []store.PersonDevice `json:"devices"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	seen := map[int64]bool{}
	if len(req.Devices) > maxPersonDevices {
		writeError(w, http.StatusBadRequest, "invalid_input", "devices")
		return
	}
	for _, d := range req.Devices {
		if _, exists := s.d.Monitor.Device(d.DeviceID); !exists || seen[d.DeviceID] {
			writeError(w, http.StatusBadRequest, "invalid_input", "devices")
			return
		}
		seen[d.DeviceID] = true
	}
	if err := s.d.Store.SetPersonDevices(r.Context(), p.ID, req.Devices); err != nil {
		s.fail(w, r, err)
		return
	}
	_ = s.d.Persons.Changed(r.Context())
	w.WriteHeader(http.StatusNoContent)
}
