package fritzbox

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Pyzyi/Netward/internal/textutil"
)

// WLAN diagnosis (docs/09, F3): which access point each Wi-Fi device uses,
// on which band, how strong the signal is and how fast the link runs. The
// mesh list of the FRITZ!Box knows repeaters and signal strength; without
// the right for it, the plain Wi-Fi device lists are the fallback.

const maxWLANClients = 1000

// WLANClient is one Wi-Fi device as the router sees it.
type WLANClient struct {
	MAC       string `json:"mac"`
	APMAC     string `json:"apMac"` // access point the device is connected to
	Band      string `json:"band"`  // "2.4", "5", "6" or ""
	Channel   int    `json:"channel,omitempty"`
	SignalDBm int    `json:"signalDbm,omitempty"` // 0 = unknown (mesh list only)
	SignalPct int    `json:"signalPct,omitempty"` // fallback list only
	RateRx    int    `json:"rateRx,omitempty"`    // Mbit/s, current
	RateTx    int    `json:"rateTx,omitempty"`
	MaxRate   int    `json:"maxRate,omitempty"` // Mbit/s, best the link could do
	// Can5GHz: the device reported 5 GHz capable modes (a/ac). Unknown and
	// false look the same, so it is only ever used to suggest, never to warn.
	Can5GHz bool `json:"can5ghz,omitempty"`
}

// WLANBackhaul is how a repeater is connected to the mesh.
type WLANBackhaul struct {
	Type      string `json:"type"` // "wlan" or "lan"
	Band      string `json:"band,omitempty"`
	SignalDBm int    `json:"signalDbm,omitempty"`
	Rate      int    `json:"rate,omitempty"` // Mbit/s, the slower direction
}

// WLANAccessPoint is the FRITZ!Box or a repeater.
type WLANAccessPoint struct {
	MAC      string        `json:"mac"`
	Name     string        `json:"name"`
	Model    string        `json:"model,omitempty"`
	Master   bool          `json:"master"`
	Bands    []string      `json:"bands,omitempty"` // bands it offers
	Backhaul *WLANBackhaul `json:"backhaul,omitempty"`
}

// WLANState is one snapshot of the Wi-Fi.
type WLANState struct {
	Source  string            `json:"source"` // "mesh" or "wlanlist"
	APs     []WLANAccessPoint `json:"aps"`
	Clients []WLANClient      `json:"clients"`
}

// WLAN reads the current Wi-Fi state. The mesh list comes first; without
// the right for it (or on old firmware) the per-band device lists follow.
func (c *Client) WLAN(ctx context.Context) (WLANState, error) {
	base, err := c.secureBase(ctx)
	if err != nil {
		return WLANState{}, err
	}
	res, err := c.soap(ctx, base, "/upnp/control/hosts", "urn:dslforum-org:service:Hosts:1", "X_AVM-DE_GetMeshListPath", nil)
	if err == nil {
		var data []byte
		if data, err = c.fetch(ctx, base, res["NewX_AVM-DE_MeshListPath"]); err == nil {
			var st WLANState
			if st, err = ParseMeshList(data); err == nil {
				return st, nil
			}
		}
	}
	var fault *soapFault
	if !errors.Is(err, ErrAuth) && !errors.As(err, &fault) {
		return WLANState{}, err
	}
	return c.wlanLists(ctx, base)
}

// fetch downloads a file the router offers at path (a path on the router
// itself, never another host).
func (c *Client) fetch(ctx context.Context, base *url.URL, path string) ([]byte, error) {
	ref, err := url.Parse(path)
	if err != nil || ref.Scheme != "" || ref.Host != "" || ref.User != nil || !strings.HasPrefix(ref.Path, "/") || strings.HasPrefix(ref.Path, "//") {
		return nil, fmt.Errorf("%w: invalid path", ErrProtocol)
	}
	u := *base
	u.Path, u.RawQuery = ref.Path, ref.RawQuery
	status, data, err := c.do(ctx, http.MethodGet, u.String(), nil, nil, true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrProtocol, status)
	}
	return data, nil
}

// rcpiDBm converts an RCPI value (IEEE 802.11k) to dBm; 255 means unknown.
func rcpiDBm(rcpi int) int {
	if rcpi <= 0 || rcpi >= 255 {
		return 0
	}
	return rcpi/2 - 110
}

// bandOf derives the band from an interface name ("AP:5G:0") or channel.
func bandOf(name string, channel int) string {
	switch n := strings.ToUpper(name); {
	case strings.Contains(n, ":2G"):
		return "2.4"
	case strings.Contains(n, ":5G"):
		return "5"
	case strings.Contains(n, ":6G"):
		return "6"
	}
	switch {
	case channel >= 1 && channel <= 14:
		return "2.4"
	case channel >= 32 && channel <= 177:
		return "5"
	}
	return ""
}

func normMAC(s string) string {
	hw, err := net.ParseMAC(strings.TrimSpace(s))
	if err != nil || len(hw) != 6 {
		return ""
	}
	return hw.String()
}

type meshList struct {
	SchemaVersion string `json:"schema_version"`
	Nodes         []struct {
		UID          string `json:"uid"`
		Name         string `json:"device_name"`
		Model        string `json:"device_model"`
		Manufacturer string `json:"device_manufacturer"`
		MAC          string `json:"device_mac_address"`
		IsMeshed     bool   `json:"is_meshed"`
		MeshRole     string `json:"mesh_role"`
		Interfaces   []struct {
			UID      string   `json:"uid"`
			Name     string   `json:"name"`
			Type     string   `json:"type"`
			MAC      string   `json:"mac_address"`
			OpMode   string   `json:"opmode"`
			Channel  int      `json:"current_channel"`
			PhyModes []string `json:"phymodes"`
			Links    []struct {
				Type   string `json:"type"`
				State  string `json:"state"`
				Node1  string `json:"node_1_uid"`
				Node2  string `json:"node_2_uid"`
				Iface1 string `json:"node_interface_1_uid"`
				Iface2 string `json:"node_interface_2_uid"`
				MaxRx  int    `json:"max_data_rate_rx"` // kbit/s
				MaxTx  int    `json:"max_data_rate_tx"`
				CurRx  int    `json:"cur_data_rate_rx"`
				CurTx  int    `json:"cur_data_rate_tx"`
				RxRCPI int    `json:"rx_rcpi"`
				TxRCPI int    `json:"tx_rcpi"`
			} `json:"node_links"`
		} `json:"node_interfaces"`
	} `json:"nodes"`
}

// ParseMeshList turns the router's mesh topology (JSON) into a WLANState.
// Unknown fields and schema versions are tolerated; what cannot be
// understood is left out rather than guessed.
func ParseMeshList(data []byte) (WLANState, error) {
	var ml meshList
	if err := json.Unmarshal(data, &ml); err != nil {
		return WLANState{}, fmt.Errorf("%w: mesh list: %v", ErrProtocol, err)
	}
	type ifRef struct {
		node, name, opmode, mac string
		channel                 int
		phymodes                []string
	}
	type nodeRef struct {
		name, model, mac string
		master, avm      bool
	}
	nodes := map[string]nodeRef{}
	ifs := map[string]ifRef{}
	for _, n := range ml.Nodes {
		role := strings.ToLower(n.MeshRole)
		nodes[n.UID] = nodeRef{name: textutil.Clean(n.Name, 64), model: textutil.Clean(n.Model, 64), mac: normMAC(n.MAC),
			master: role == "master", avm: n.IsMeshed && (role == "master" || role == "slave")}
		for _, i := range n.Interfaces {
			ifs[i.UID] = ifRef{node: n.UID, name: i.Name, opmode: strings.ToUpper(i.OpMode), mac: normMAC(i.MAC), channel: i.Channel, phymodes: i.PhyModes}
		}
	}
	isAP := func(i ifRef) bool { return i.opmode == "AP" || strings.HasPrefix(strings.ToUpper(i.name), "AP:") }

	st := WLANState{Source: "mesh", APs: []WLANAccessPoint{}, Clients: []WLANClient{}}
	apIdx := map[string]int{}
	ap := func(uid string) *WLANAccessPoint {
		if i, ok := apIdx[uid]; ok {
			return &st.APs[i]
		}
		n := nodes[uid]
		st.APs = append(st.APs, WLANAccessPoint{MAC: n.mac, Name: n.name, Model: n.model, Master: n.master})
		apIdx[uid] = len(st.APs) - 1
		return &st.APs[len(st.APs)-1]
	}
	// Every mesh node with an access point interface is an access point.
	for _, n := range ml.Nodes {
		if !nodes[n.UID].avm {
			continue
		}
		for _, i := range n.Interfaces {
			if ref := ifs[i.UID]; strings.EqualFold(i.Type, "WLAN") && isAP(ref) {
				a := ap(n.UID)
				if b := bandOf(i.Name, i.Channel); b != "" && !contains(a.Bands, b) {
					a.Bands = append(a.Bands, b)
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, n := range ml.Nodes {
		for _, i := range n.Interfaces {
			for _, l := range i.Links {
				key := l.Iface1 + "|" + l.Iface2
				if seen[key] || !strings.EqualFold(l.State, "CONNECTED") {
					continue
				}
				seen[key] = true
				i1, ok1 := ifs[l.Iface1]
				i2, ok2 := ifs[l.Iface2]
				if !ok1 || !ok2 {
					continue
				}
				// Repeater connected by cable: a LAN link between two mesh nodes.
				if strings.EqualFold(l.Type, "LAN") {
					n1, n2 := nodes[i1.node], nodes[i2.node]
					if n1.avm && n2.avm && n1.master != n2.master {
						rep := i2.node
						if n2.master {
							rep = i1.node
						}
						if a := ap(rep); a.Backhaul == nil {
							a.Backhaul = &WLANBackhaul{Type: "lan"}
						}
					}
					continue
				}
				if !strings.EqualFold(l.Type, "WLAN") {
					continue
				}
				apIf, cl := i1, i2
				if !isAP(i1) && isAP(i2) {
					apIf, cl = i2, i1
				} else if !isAP(i1) {
					continue
				}
				band := bandOf(apIf.name, apIf.channel)
				sig := rcpiDBm(l.RxRCPI)
				if sig == 0 {
					sig = rcpiDBm(l.TxRCPI)
				}
				if nodes[cl.node].avm {
					// A repeater's uplink to the mesh.
					rate := min(l.CurRx, l.CurTx) / 1000
					if rate == 0 {
						rate = max(l.CurRx, l.CurTx) / 1000
					}
					ap(cl.node).Backhaul = &WLANBackhaul{Type: "wlan", Band: band, SignalDBm: sig, Rate: rate}
					continue
				}
				mac := cl.mac
				if mac == "" {
					mac = nodes[cl.node].mac
				}
				if mac == "" || len(st.Clients) >= maxWLANClients {
					continue
				}
				c := WLANClient{MAC: mac, APMAC: nodes[apIf.node].mac, Band: band, Channel: apIf.channel, SignalDBm: sig,
					RateRx: l.CurRx / 1000, RateTx: l.CurTx / 1000, MaxRate: max(l.MaxRx, l.MaxTx) / 1000}
				for _, m := range cl.phymodes {
					if m = strings.ToLower(m); m == "a" || m == "ac" {
						c.Can5GHz = true
					}
				}
				ap(apIf.node) // make sure the access point is listed
				st.Clients = append(st.Clients, c)
			}
		}
	}
	return st, nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// wlanLists reads the device list of each Wi-Fi radio (WLANConfiguration:1-4).
// It knows band, speed and signal in percent, but no repeaters.
func (c *Client) wlanLists(ctx context.Context, base *url.URL) (WLANState, error) {
	st := WLANState{Source: "wlanlist", APs: []WLANAccessPoint{}, Clients: []WLANClient{}}
	var master WLANAccessPoint
	master.Master = true
	found := false
	for n := 1; n <= 4; n++ {
		control := "/upnp/control/wlanconfig" + strconv.Itoa(n)
		svc := "urn:dslforum-org:service:WLANConfiguration:" + strconv.Itoa(n)
		info, err := c.soap(ctx, base, control, svc, "GetInfo", nil)
		if err != nil {
			var fault *soapFault
			if errors.As(err, &fault) {
				continue // radio not present
			}
			if found {
				break
			}
			return WLANState{}, err
		}
		band := ""
		switch info["NewX_AVM-DE_FrequencyBand"] {
		case "2400":
			band = "2.4"
		case "5000":
			band = "5"
		case "6000":
			band = "6"
		}
		ch, _ := strconv.Atoi(info["NewChannel"])
		if band == "" {
			band = bandOf("", ch)
		}
		if master.MAC == "" {
			master.MAC = normMAC(info["NewBSSID"])
		}
		res, err := c.soap(ctx, base, control, svc, "X_AVM-DE_GetWLANDeviceListPath", nil)
		if err != nil {
			continue
		}
		data, err := c.fetch(ctx, base, res["NewX_AVM-DE_WLANDeviceListPath"])
		if err != nil {
			continue
		}
		var doc struct {
			Items []struct {
				MAC    string `xml:"AssociatedDeviceMACAddress"`
				Auth   string `xml:"AssociatedDeviceAuthState"`
				Speed  int    `xml:"X_AVM-DE_Speed"`
				Signal int    `xml:"X_AVM-DE_SignalStrength"`
			} `xml:"Item"`
		}
		if err := xml.Unmarshal(data, &doc); err != nil {
			continue
		}
		found = true
		if band != "" && !contains(master.Bands, band) {
			master.Bands = append(master.Bands, band)
		}
		for _, it := range doc.Items {
			mac := normMAC(it.MAC)
			if mac == "" || it.Auth == "0" || len(st.Clients) >= maxWLANClients {
				continue
			}
			st.Clients = append(st.Clients, WLANClient{MAC: mac, Band: band, Channel: ch, SignalPct: min(max(it.Signal, 0), 100),
				RateRx: max(it.Speed, 0), RateTx: max(it.Speed, 0)})
		}
	}
	if !found {
		return WLANState{}, fmt.Errorf("%w: no Wi-Fi information", ErrAuth)
	}
	master.Name = "FRITZ!Box"
	st.APs = append(st.APs, master)
	for i := range st.Clients {
		st.Clients[i].APMAC = master.MAC
	}
	return st, nil
}
