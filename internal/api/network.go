package api

import (
	"net/http"
	"net/netip"
)

// Router address and DHCP servers (docs/09, F1).

func (s *Server) handleNetwork(w http.ResponseWriter, _ *http.Request, _ *sessionInfo) {
	writeJSON(w, http.StatusOK, s.d.Monitor.Network())
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
	writeJSON(w, http.StatusOK, s.d.Monitor.Network())
}
