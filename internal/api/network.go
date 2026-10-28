package api

import (
	"net/http"
	"net/netip"

	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/tripwire"
)

// Router address and DHCP servers (docs/09, F1), DNS check and tripwire
// (docs/10, F2/F3).

type networkView struct {
	monitor.NetworkState
	Tripwire *tripwire.Status `json:"tripwire,omitempty"`
}

func (s *Server) network() networkView {
	v := networkView{NetworkState: s.d.Monitor.Network()}
	if s.d.Tripwire != nil {
		t := s.d.Tripwire()
		v.Tripwire = &t
	}
	return v
}

func (s *Server) handleNetwork(w http.ResponseWriter, _ *http.Request, _ *sessionInfo) {
	writeJSON(w, http.StatusOK, s.network())
}

type acceptRequest struct {
	IP     string `json:"ip"`     // router address whose new MAC is accepted
	Server string `json:"server"` // DHCP server ID that is accepted
}

// handleNetworkAccept confirms a change: "this is my new router" or "this
// DHCP server is mine". The findings are re-evaluated right away.
func (s *Server) handleNetworkAccept(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	var req acceptRequest
	if !readJSON(w, r, &req) {
		return
	}
	ok := false
	switch {
	case req.IP != "" && req.Server == "":
		if _, err := netip.ParseAddr(req.IP); err == nil {
			ok = s.d.Monitor.AcceptGateway(r.Context(), req.IP)
		}
	case req.Server != "" && req.IP == "":
		if _, err := netip.ParseAddr(req.Server); err == nil {
			ok = s.d.Monitor.AcceptDHCPServer(r.Context(), req.Server)
		}
	default:
		writeError(w, http.StatusBadRequest, "invalid_input", "")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	// Accepting a router or DHCP server silences an alarm: keep a trace.
	s.d.Monitor.AddEvent(r.Context(), "config_network", "info", 0, map[string]any{"ip": s.clientIP(r).String(), "target": req.IP + req.Server})
	s.d.Security.EvaluateNetwork(r.Context())
	writeJSON(w, http.StatusOK, s.network())
}

// handleDeviceIdentity answers a change of the operating system family
// (F2): "set up anew" accepts it, otherwise the device is marked as not
// known so it can be reviewed.
func (s *Server) handleDeviceIdentity(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	var req struct {
		SetUpAnew bool `json:"setUpAnew"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !s.d.Monitor.ResolveIdentity(r.Context(), id, req.SetUpAnew) {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	s.d.Security.EvaluateNetwork(r.Context())
	w.WriteHeader(http.StatusNoContent)
}
