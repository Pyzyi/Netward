package api

import (
	"net/http"

	"github.com/Pyzyi/Netward/internal/wlan"
)

// handleWLAN returns the Wi-Fi diagnosis (docs/09, F3) and why it might be
// missing: no FRITZ!Box connected, or not enough rights for the mesh list.
func (s *Server) handleWLAN(w http.ResponseWriter, r *http.Request, _ *sessionInfo) {
	out := struct {
		wlan.View
		FritzBox string `json:"fritzbox"` // disabled | connecting | connected | error
		Status   string `json:"status,omitempty"`
	}{View: wlan.View{APs: []wlan.AccessPoint{}}, FritzBox: "disabled"}
	if s.d.WLAN != nil {
		out.View = s.d.WLAN.View()
	}
	if s.d.FritzBox != nil {
		st := s.d.FritzBox.Status()
		out.FritzBox, out.Status = st.State, st.WLAN
		if st.State == "disabled" && !s.d.Env.Demo {
			out.View = wlan.View{APs: []wlan.AccessPoint{}} // switched off: nothing old lingers
		}
	}
	if s.d.Env.Demo {
		out.FritzBox, out.Status = "connected", out.Source
	}
	writeJSON(w, http.StatusOK, out)
}
