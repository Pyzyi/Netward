package security

import (
	"context"
	"encoding/json"
	"net/netip"
	"strings"

	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/store"
)

// Network findings (docs/09, F1): who owns the router address and who hands
// out addresses. Evaluated together with the software findings. MAC
// addresses are never part of a finding's identity, so a spoofer changing
// MACs cannot cause a flood of alerts.
const (
	CheckGatewayMAC = "gateway_mac" // router address answered by another or a second device
	CheckDHCPRogue  = "dhcp_rogue"  // more than one device hands out addresses
)

// EvaluateNetwork turns the monitor's network state into findings.
func (c *Checker) EvaluateNetwork(ctx context.Context) {
	runStart := c.nextRun()
	st := c.mon.Network()
	var found []store.Finding
	add := func(check string, dev int64, sev string, data map[string]string) {
		raw, _ := json.Marshal(data)
		found = append(found, store.Finding{CheckID: check, DeviceID: dev, Severity: sev, Data: raw, LastSeen: runStart})
	}

	for _, g := range st.Gateways {
		switch {
		case g.Conflict:
			add(CheckGatewayMAC, 0, "critical", map[string]string{"ip": g.IP, "kind": "conflict"})
		case g.Change != nil:
			sev := "info" // e.g. a new FRITZ!Box replacing the old one
			if g.Change.OldActive || !sameVendor(g.Vendor, g.Change.Vendor) {
				sev = "warning"
			}
			add(CheckGatewayMAC, 0, sev, map[string]string{"ip": g.IP, "kind": "changed"})
		}
	}

	if len(st.DHCPServers) > 1 {
		var refs []monitor.DHCPServer
		for _, s := range st.DHCPServers {
			if s.Accepted || c.mon.IsGatewayIP(s.ServerID) {
				refs = append(refs, s)
			}
		}
		for _, s := range st.DHCPServers {
			if isRef(refs, s) {
				continue
			}
			sev := "warning"
			switch {
			case len(refs) == 0:
				sev = "info" // nobody is known yet: ask which one is yours
			case handsOutOther(refs, s):
				sev = "critical" // a different router or DNS: traffic can be redirected
			}
			var dev int64
			if ip, err := netip.ParseAddr(s.ServerID); err == nil {
				if d, ok := c.mon.DeviceByIP(ip); ok && strings.EqualFold(d.MAC, s.MAC) {
					dev = d.ID
				}
			}
			add(CheckDHCPRogue, dev, sev, map[string]string{"server": s.ServerID})
		}
	}

	var alerts []store.Finding
	for _, f := range found {
		isNew, err := c.st.UpsertFinding(ctx, f)
		if err != nil {
			c.log.Error("store network finding", "err", err)
			continue
		}
		if isNew && (f.Severity == "warning" || f.Severity == "critical") {
			alerts = append(alerts, f)
		}
	}
	if ctx.Err() != nil {
		return
	}
	devs := c.mon.Devices()
	ids := make([]int64, 0, len(devs))
	for _, d := range devs {
		ids = append(ids, d.ID)
	}
	if err := c.st.PruneFindings(ctx, runStart, ids, []string{CheckGatewayMAC, CheckDHCPRogue}); err != nil {
		c.log.Error("prune network findings", "err", err)
	}
	for _, f := range alerts {
		var data map[string]any
		_ = json.Unmarshal(f.Data, &data)
		data["check"] = f.CheckID
		data["title"], data["detail"] = Describe(f.CheckID, data, "de")
		var dev *store.Device
		if d, ok := c.mon.Device(f.DeviceID); ok {
			dev = &d
		}
		c.mon.AddEvent(ctx, notify.KindSecurity, f.Severity, f.DeviceID, map[string]any{"check": f.CheckID, "ip": data["ip"], "server": data["server"]})
		c.notifier.Notify(notify.Event{Kind: notify.KindSecurity, Severity: f.Severity, TS: runStart, Device: dev, Data: data})
	}
}

func sameVendor(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	return a != "" && b != "" && (strings.Contains(a, b) || strings.Contains(b, a))
}

func isRef(refs []monitor.DHCPServer, s monitor.DHCPServer) bool {
	for _, r := range refs {
		if r.ServerID == s.ServerID {
			return true
		}
	}
	return false
}

// handsOutOther reports whether s announces a router or DNS server none of
// the known servers announces.
func handsOutOther(refs []monitor.DHCPServer, s monitor.DHCPServer) bool {
	known := map[string]bool{}
	for _, r := range refs {
		if r.Router != "" {
			known[r.Router] = true
		}
		for _, d := range r.DNS {
			known[d] = true
		}
	}
	if s.Router != "" && !known[s.Router] {
		return true
	}
	for _, d := range s.DNS {
		if !known[d] {
			return true
		}
	}
	return false
}
