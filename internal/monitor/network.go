package monitor

import (
	"context"
	"crypto/rand"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/Pyzyi/netzwart/internal/dnscheck"
	"github.com/Pyzyi/netzwart/internal/l2"
	"github.com/Pyzyi/netzwart/internal/oui"
	"github.com/Pyzyi/netzwart/internal/textutil"
)

// Who answers for the router address and who hands out addresses (docs/09,
// F1). Both are kept as small JSON settings: a handful of entries at most.

// Activity log entries.
const (
	KindGatewayChanged = "gateway_changed" // router address answered by another MAC
	KindDHCPServerNew  = "dhcp_server_new" // a DHCP server not seen before
)

const (
	keyDHCPServers = "net_dhcp_servers"
	keyGatewayRefs = "net_gateway_refs"
	keyProbeMAC    = "net_dhcp_probe_mac"
	keyDHCPProbe   = "net_dhcp_probe"
	keyDNSCheck    = "net_dns_check"
	maxDHCPServers = 16
	// dhcpWindow: servers not seen for this long no longer count.
	dhcpWindow = 7 * 24 * time.Hour
	// gatewaySettle: within this time after first seeing the router, a new
	// MAC simply replaces the reference (installation, router swap on day one).
	gatewaySettle = 24 * time.Hour
)

// DHCPServer is a device seen answering DHCP requests.
type DHCPServer struct {
	ServerID  string   `json:"serverId"`
	MAC       string   `json:"mac"`
	Vendor    string   `json:"vendor,omitempty"`
	Router    string   `json:"router,omitempty"`
	DNS       []string `json:"dns,omitempty"`
	FirstSeen int64    `json:"firstSeen"`
	LastSeen  int64    `json:"lastSeen"`
	Accepted  bool     `json:"accepted"`
}

// GatewayRef is the MAC the router address normally belongs to, and a
// pending change if another device has taken it over.
type GatewayRef struct {
	IP     string `json:"ip"`
	MAC    string `json:"mac"`
	Vendor string `json:"vendor,omitempty"`
	Since  int64  `json:"since"`
	// Candidate is a different MAC answering for the address; it becomes a
	// Change after two sweeps in a row.
	Candidate     string         `json:"candidate,omitempty"`
	CandidateRuns int            `json:"candidateRuns,omitempty"`
	Change        *GatewayChange `json:"change,omitempty"`
	// Conflict: two MACs answered for the router address in two sweeps.
	Conflict     bool     `json:"conflict,omitempty"`
	Others       []string `json:"others,omitempty"` // the MACs that answered during the conflict
	ConflictRuns int      `json:"-"`
}

// GatewayChange describes the router address now answered by another MAC.
type GatewayChange struct {
	MAC       string `json:"mac"`
	Vendor    string `json:"vendor,omitempty"`
	At        int64  `json:"at"`
	OldActive bool   `json:"oldActive"` // the previous MAC is still on the network
}

// DHCPProbe is the outcome of the last active DHCP check (F1b).
type DHCPProbe struct {
	At      int64 `json:"at"`
	Answers int   `json:"answers"`
}

// DNSCheck is the outcome of the last check of the DNS servers (docs/10, F2).
type DNSCheck struct {
	At      int64             `json:"at"`
	Results []dnscheck.Result `json:"results"`
}

// NetworkState is what the security evaluation and the UI read.
type NetworkState struct {
	Gateways    []GatewayRef `json:"gateways"`
	DHCPServers []DHCPServer `json:"dhcpServers"`
	Probe       *DHCPProbe   `json:"probe,omitempty"`
	DNS         *DNSCheck    `json:"dns,omitempty"`
}

func vendorOf(mac string) string {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return ""
	}
	if v, ok := oui.Lookup(hw); ok {
		return v.Name
	}
	return ""
}

func (m *Monitor) loadNetwork(ctx context.Context) {
	var servers []DHCPServer
	if err := m.st.GetSetting(ctx, keyDHCPServers, &servers); err == nil {
		for _, s := range servers {
			s := s
			m.dhcpServers[s.ServerID] = &s
		}
	}
	var probe DHCPProbe
	if err := m.st.GetSetting(ctx, keyDHCPProbe, &probe); err == nil && probe.At > 0 {
		m.dhcpProbe = &probe
	}
	var dns DNSCheck
	if err := m.st.GetSetting(ctx, keyDNSCheck, &dns); err == nil && dns.At > 0 {
		m.dnsCheck = &dns
	}
	// One fixed, locally administered client MAC per installation for the
	// active DHCP check, so routers do not collect a new entry every time.
	var mac string
	if err := m.st.GetSetting(ctx, keyProbeMAC, &mac); err == nil {
		if hw, err := net.ParseMAC(mac); err == nil && len(hw) == 6 {
			m.probeMAC = hw
		}
	}
	if m.probeMAC == nil {
		hw := make(net.HardwareAddr, 6)
		_, _ = rand.Read(hw)
		hw[0] = hw[0]&0xfc | 0x02 // locally administered, unicast
		m.probeMAC = hw
		_ = m.st.SetSetting(ctx, keyProbeMAC, hw.String())
	}
	var refs []GatewayRef
	if err := m.st.GetSetting(ctx, keyGatewayRefs, &refs); err == nil {
		for _, r := range refs {
			r := r
			m.gatewayRefs[r.IP] = &r
		}
	}
}

func (m *Monitor) saveNetwork(ctx context.Context) {
	servers := make([]DHCPServer, 0, len(m.dhcpServers))
	for _, s := range m.dhcpServers {
		servers = append(servers, *s)
	}
	refs := make([]GatewayRef, 0, len(m.gatewayRefs))
	for _, r := range m.gatewayRefs {
		refs = append(refs, *r)
	}
	_ = m.st.SetSetting(ctx, keyDHCPServers, servers)
	_ = m.st.SetSetting(ctx, keyGatewayRefs, refs)
}

// observeDHCPServer records a DHCP OFFER/ACK; m.mu must be held.
func (m *Monitor) observeDHCPServer(ctx context.Context, s l2.Sighting) {
	d := s.DHCP
	if d == nil || !d.ServerID.IsValid() {
		return
	}
	now := m.nowMS()
	id := d.ServerID.String()
	srv := m.dhcpServers[id]
	isNew := srv == nil
	if isNew {
		// The list is bounded and a flood of spoofed server IDs must not push
		// out the router's own server: when full, only an entry not seen for
		// a week makes room, and the router's server always gets in (T-36).
		if len(m.dhcpServers) >= maxDHCPServers && !m.dropStaleServer(now, m.gateways[d.ServerID]) {
			return
		}
		m.addEvent(ctx, KindDHCPServerNew, "info", nil, map[string]any{"ip": id, "vendor": vendorOf(s.MAC.String())})
		srv = &DHCPServer{ServerID: id, FirstSeen: now}
		m.dhcpServers[id] = srv
	}
	changed := isNew || srv.MAC != s.MAC.String()
	srv.MAC = s.MAC.String()
	srv.Vendor = textutil.Clean(vendorOf(srv.MAC), 64)
	if d.Router.IsValid() {
		changed = changed || srv.Router != d.Router.String()
		srv.Router = d.Router.String()
	}
	if len(d.DNS) > 0 {
		dns := make([]string, 0, len(d.DNS))
		for _, a := range d.DNS {
			dns = append(dns, a.String())
		}
		changed = changed || strings.Join(dns, ",") != strings.Join(srv.DNS, ",")
		srv.DNS = dns
	}
	// Writing on every lease would be wasteful; an hour is precise enough.
	if changed || now-srv.LastSeen > time.Hour.Milliseconds() {
		srv.LastSeen = now
		m.saveNetwork(ctx)
	}
	if changed {
		m.netChanged = true
	}
}

// dropStaleServer makes room in the server list: it removes the oldest entry
// that is neither accepted nor the router's, if that entry was not seen for
// dhcpWindow (or at all, when force is set). It reports whether it did.
func (m *Monitor) dropStaleServer(now int64, force bool) bool {
	var oldest *DHCPServer
	for _, s := range m.dhcpServers {
		if a, err := netip.ParseAddr(s.ServerID); s.Accepted || (err == nil && m.gateways[a]) {
			continue
		}
		if oldest == nil || s.LastSeen < oldest.LastSeen {
			oldest = s
		}
	}
	if oldest == nil || (!force && now-oldest.LastSeen < dhcpWindow.Milliseconds()) {
		return false
	}
	delete(m.dhcpServers, oldest.ServerID)
	return true
}

// checkGateways compares who answered for the router addresses in a sweep
// with the reference; m.mu must be held. Proxy MACs (repeaters answering for
// others) are ignored, like in detectConflicts.
func (m *Monitor) checkGateways(ctx context.Context, res l2.SweepResult) {
	now := m.nowMS()
	changed := false
	for ip := range m.gateways {
		macs := res[ip]
		var real []string
		for _, mac := range macs {
			if !m.proxies[mac.String()] {
				real = append(real, mac.String())
			}
		}
		if len(real) == 0 {
			continue // no answer this time: proves nothing
		}
		ref := m.gatewayRefs[ip.String()]
		if ref == nil {
			if len(real) == 1 {
				m.gatewayRefs[ip.String()] = &GatewayRef{IP: ip.String(), MAC: real[0], Vendor: vendorOf(real[0]), Since: now}
				changed = true
				continue
			}
			// Two answers before there is a reference (a spoofer already active
			// at installation): track the conflict without one.
			ref = &GatewayRef{IP: ip.String(), Since: now}
			m.gatewayRefs[ip.String()] = ref
			changed = true
		}
		// Two MACs for the router address must show up in two sweeps in a
		// row, like any IP conflict (DHCP lease reuse overlaps once).
		if len(real) > 1 {
			ref.ConflictRuns++
			if ref.ConflictRuns >= 2 && (!ref.Conflict || strings.Join(ref.Others, ",") != strings.Join(real, ",")) {
				ref.Conflict, ref.Others = true, append([]string(nil), real...)
				changed = true
			}
			continue
		}
		if ref.Conflict || ref.ConflictRuns > 0 {
			ref.Conflict, ref.ConflictRuns, ref.Others = false, 0, nil
			changed = true
		}
		mac := real[0]
		if ref.MAC == "" {
			// The conflict is over: the one that remains is the reference.
			ref.MAC, ref.Vendor, ref.Since = mac, vendorOf(mac), now
			changed = true
			continue
		}
		switch {
		case mac == ref.MAC:
			if ref.Candidate != "" || ref.Change != nil {
				ref.Candidate, ref.CandidateRuns, ref.Change = "", 0, nil
				changed = true
			}
		case mac == ref.Candidate:
			ref.CandidateRuns++
			if ref.CandidateRuns >= 2 && (ref.Change == nil || ref.Change.MAC != mac) {
				if time.Duration(now-ref.Since)*time.Millisecond < gatewaySettle {
					ref.MAC, ref.Vendor, ref.Since = mac, vendorOf(mac), now
					ref.Candidate, ref.CandidateRuns = "", 0
				} else {
					ref.Change = &GatewayChange{MAC: mac, Vendor: vendorOf(mac), At: now, OldActive: m.macActiveElsewhere(ref.MAC, ip)}
					m.addEvent(ctx, KindGatewayChanged, "warning", nil, map[string]any{
						"ip": ip.String(), "oldVendor": ref.Vendor, "newVendor": ref.Change.Vendor})
				}
			}
			changed = true
		default:
			ref.Candidate, ref.CandidateRuns = mac, 1
			changed = true
		}
	}
	if changed {
		m.saveNetwork(ctx)
		m.netChanged = true
	}
}

// DHCPProbeMAC is the client MAC the active DHCP check asks with.
func (m *Monitor) DHCPProbeMAC() net.HardwareAddr {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append(net.HardwareAddr(nil), m.probeMAC...)
}

// ObserveDHCPProbe records the offers an active DHCP check received.
func (m *Monitor) ObserveDHCPProbe(ctx context.Context, answers []l2.DHCPAnswer) {
	m.mu.Lock()
	now := m.nowMS()
	for _, a := range answers {
		d := a.DHCP
		m.observeDHCPServer(ctx, l2.Sighting{MAC: a.MAC, IP: a.IP, Source: "dhcp-server", DHCP: &d, At: time.UnixMilli(now)})
	}
	m.dhcpProbe = &DHCPProbe{At: now, Answers: len(answers)}
	_ = m.st.SetSetting(ctx, keyDHCPProbe, m.dhcpProbe)
	m.netChanged = true
	m.mu.Unlock()
	m.softwareChanged()
}

// SetDNSCheck stores the outcome of a DNS check.
func (m *Monitor) SetDNSCheck(ctx context.Context, results []dnscheck.Result) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if results == nil {
		results = []dnscheck.Result{}
	}
	m.dnsCheck = &DNSCheck{At: m.nowMS(), Results: results}
	_ = m.st.SetSetting(ctx, keyDNSCheck, m.dnsCheck)
}

// DNSServers are the resolvers the home network uses: what the DHCP servers
// hand out, and the router itself. IPv4 only, at most four.
func (m *Monitor) DNSServers() []netip.Addr {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[netip.Addr]bool{}
	var out []netip.Addr
	add := func(s string) {
		ip, err := netip.ParseAddr(s)
		if err != nil || !ip.Is4() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || seen[ip] || len(out) >= 4 {
			return
		}
		seen[ip] = true
		out = append(out, ip)
	}
	ids := make([]string, 0, len(m.dhcpServers))
	for id := range m.dhcpServers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for _, d := range m.dhcpServers[id].DNS {
			add(d)
		}
	}
	gws := make([]string, 0, len(m.gateways))
	for ip := range m.gateways {
		gws = append(gws, ip.String())
	}
	sort.Strings(gws)
	for _, g := range gws {
		add(g)
	}
	return out
}

// macActiveElsewhere reports whether mac was seen recently at another address.
func (m *Monitor) macActiveElsewhere(mac string, ip netip.Addr) bool {
	e := m.byIdent[mac]
	return e != nil && e.d.Online && e.d.IP != ip.String()
}

// Network returns a snapshot of router references and DHCP servers.
func (m *Monitor) Network() NetworkState {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := NetworkState{Gateways: []GatewayRef{}, DHCPServers: []DHCPServer{}}
	if m.dhcpProbe != nil {
		p := *m.dhcpProbe
		st.Probe = &p
	}
	if m.dnsCheck != nil {
		d := *m.dnsCheck
		st.DNS = &d
	}
	for _, r := range m.gatewayRefs {
		st.Gateways = append(st.Gateways, *r)
	}
	cutoff := m.nowMS() - dhcpWindow.Milliseconds()
	for _, s := range m.dhcpServers {
		if s.LastSeen >= cutoff || s.Accepted {
			st.DHCPServers = append(st.DHCPServers, *s)
		}
	}
	sort.Slice(st.Gateways, func(i, j int) bool { return st.Gateways[i].IP < st.Gateways[j].IP })
	sort.Slice(st.DHCPServers, func(i, j int) bool { return st.DHCPServers[i].ServerID < st.DHCPServers[j].ServerID })
	return st
}

// IsGatewayIP reports whether ip is one of the router addresses.
func (m *Monitor) IsGatewayIP(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gateways[a]
}

// AcceptGateway makes the pending MAC of a router address the new reference
// ("this is my new router").
func (m *Monitor) AcceptGateway(ctx context.Context, ip string) bool {
	m.mu.Lock()
	ref := m.gatewayRefs[ip]
	ok := ref != nil && ref.Change != nil
	if ok {
		ref.MAC, ref.Vendor, ref.Since = ref.Change.MAC, ref.Change.Vendor, m.nowMS()
		ref.Change, ref.Candidate, ref.CandidateRuns = nil, "", 0
		m.saveNetwork(ctx)
		m.netChanged = true
	}
	m.mu.Unlock()
	m.softwareChanged()
	return ok
}

// AcceptDHCPServer marks a DHCP server as known ("this one is mine").
func (m *Monitor) AcceptDHCPServer(ctx context.Context, serverID string) bool {
	m.mu.Lock()
	srv := m.dhcpServers[serverID]
	if srv != nil {
		srv.Accepted = true
		m.saveNetwork(ctx)
		m.netChanged = true
	}
	m.mu.Unlock()
	m.softwareChanged()
	return srv != nil
}
