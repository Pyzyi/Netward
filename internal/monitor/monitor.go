// Package monitor is the single source of truth about devices. It turns
// sightings and discovery results into device records, tracks presence,
// writes the activity log and decides what is worth a notification.
//
// All state changes go through the monitor's mutex; database writes happen
// while holding it so the in-memory view and SQLite never diverge. Writes are
// small and SQLite runs in WAL mode, so this stays fast.
package monitor

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/classify"
	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/l2"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/oui"
	"github.com/Pyzyi/netzwart/internal/proto"
	"github.com/Pyzyi/netzwart/internal/store"
	"github.com/Pyzyi/netzwart/internal/textutil"
)

// Event kinds written to the activity log (in addition to notify.Kind*).
const (
	KindIPChanged     = "ip_changed"
	KindScanBaseline  = "scan_baseline"
	KindDeviceRemoved = "device_removed"
	KindMerged        = "device_merged"
)

// Prober checks whether a device that stopped answering is really gone.
type Prober interface {
	Probe(ctx context.Context, d store.Device) bool
}

// Enricher resolves names and services for a device asynchronously.
type Enricher interface {
	Enqueue(id int64)
}

// Admission limits protect against floods of fake MAC addresses.
const (
	admissionPerMinute = 20
	maxDevices         = 4000
)

type entry struct {
	d          store.Device
	names      map[string]string    // source -> name
	addrs      map[netip.Addr]int64 // ip -> last ARP-confirmed sighting (ms)
	addrWrites map[netip.Addr]int64 // ip -> last time persisted (ms)
	services   []string             // mDNS service types
	instances  map[string]string    // mDNS service type -> instance name
	model      string               // mDNS/UPnP model hint
	homekit    string               // HomeKit category
	upnpTypes  []string
	upnpMaker  string
	ports      []int
	probing    bool
}

type stash struct {
	dhcp *proto.DHCP
	mdns *proto.MDNSInfo
	at   time.Time
}

// Monitor tracks devices.
type Monitor struct {
	st     *store.Store
	log    *slog.Logger
	hub    *hub.Hub
	notify notify.Notifier
	now    func() time.Time

	mu        sync.Mutex
	settings  config.Settings
	byID      map[int64]*entry
	byIdent   map[string]*entry
	byIP      map[netip.Addr]*entry
	proxies   map[string]bool // MACs that answer for several IPs
	gateways  map[netip.Addr]bool
	selfMACs  map[string]bool
	stashed   map[string]stash // DHCP/mDNS info for MACs not yet confirmed by ARP
	dirty     map[int64]int64  // id -> last_seen to flush
	conflicts map[string]map[netip.Addr]string
	reported  map[string]time.Time // conflict/flood dedup

	baselineAt  int64 // 0 = still learning
	gateOpen    bool
	wasOnline   map[int64]bool // online before a monitoring gap; their return is not news
	startedAt   time.Time
	lastSweepAt time.Time
	admitTokens float64
	admitRefill time.Time
	floodLogged time.Time

	prober   Prober
	enricher Enricher
	probeSem chan struct{}
}

// New loads the device table and prepares the monitor.
func New(ctx context.Context, st *store.Store, h *hub.Hub, n notify.Notifier, s config.Settings, log *slog.Logger) (*Monitor, error) {
	m := &Monitor{
		st: st, log: log, hub: h, notify: n, now: time.Now, settings: s,
		byID: map[int64]*entry{}, byIdent: map[string]*entry{}, byIP: map[netip.Addr]*entry{},
		proxies: map[string]bool{}, gateways: map[netip.Addr]bool{}, selfMACs: map[string]bool{},
		stashed: map[string]stash{}, dirty: map[int64]int64{}, conflicts: map[string]map[netip.Addr]string{},
		reported: map[string]time.Time{}, probeSem: make(chan struct{}, 8),
		admitTokens: admissionPerMinute,
	}
	if m.notify == nil {
		m.notify = notify.Discard{}
	}
	m.startedAt = m.now()
	m.admitRefill = m.startedAt
	if err := st.GetSetting(ctx, "baseline_at", &m.baselineAt); err != nil && err != store.ErrNotFound {
		return nil, err
	}
	if err := m.recoverGap(ctx); err != nil {
		return nil, err
	}
	if err := m.load(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

// SetClock replaces the time source (tests only).
func (m *Monitor) SetClock(now func() time.Time) {
	m.mu.Lock()
	m.now = now
	m.startedAt = now()
	m.admitRefill = m.startedAt
	m.mu.Unlock()
}

// SetProber and SetEnricher connect the scanner; both may be nil.
func (m *Monitor) SetProber(p Prober)     { m.mu.Lock(); m.prober = p; m.mu.Unlock() }
func (m *Monitor) SetEnricher(e Enricher) { m.mu.Lock(); m.enricher = e; m.mu.Unlock() }

// UpdateSettings applies new settings.
func (m *Monitor) UpdateSettings(s config.Settings) {
	m.mu.Lock()
	m.settings = s
	m.mu.Unlock()
}

// SetNetwork tells the monitor which addresses are gateways and which MACs
// are our own, and updates devices that were seen before this was known.
func (m *Monitor) SetNetwork(gateways []netip.Addr, selfMACs []net.HardwareAddr) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gateways = map[netip.Addr]bool{}
	for _, g := range gateways {
		m.gateways[g] = true
	}
	m.selfMACs = map[string]bool{}
	for _, s := range selfMACs {
		m.selfMACs[s.String()] = true
	}
	ctx := context.Background()
	for _, e := range m.byID {
		ip, _ := netip.ParseAddr(e.d.IP)
		gw := m.gateways[ip]
		self := e.d.IsSelf || m.selfMACs[e.d.MAC]
		if gw != e.d.IsGateway || self != e.d.IsSelf {
			e.d.IsGateway, e.d.IsSelf = gw, self
			m.reclassify(e)
			m.persist(ctx, e)
			m.publish(e)
		}
	}
}

func (m *Monitor) load(ctx context.Context) error {
	devs, err := m.st.ListDevices(ctx)
	if err != nil {
		return err
	}
	ports, err := m.st.ListPorts(ctx, 0)
	if err != nil {
		return err
	}
	for _, d := range devs {
		e := newEntry(d)
		names, err := m.st.ListNames(ctx, d.ID)
		if err != nil {
			return err
		}
		for _, n := range names {
			e.names[n.Source] = n.Name
		}
		svcs, err := m.st.ListServices(ctx, d.ID)
		if err != nil {
			return err
		}
		for _, s := range svcs {
			switch s.Source {
			case "mdns":
				e.services = append(e.services, s.Service)
				if s.Detail != "" {
					e.instances[s.Service] = s.Detail
				}
			case "ssdp":
				e.upnpTypes = append(e.upnpTypes, s.Service)
				if e.upnpMaker == "" {
					e.upnpMaker = s.Detail
				}
			}
		}
		for _, p := range ports[d.ID] {
			e.ports = append(e.ports, p.Port)
		}
		if ip, err := netip.ParseAddr(d.IP); err == nil {
			e.addrs[ip] = d.LastSeen
			e.addrWrites[ip] = d.LastSeen
		}
		m.index(e)
		if d.Proxy {
			m.proxies[d.MAC] = true
		}
	}
	return nil
}

func newEntry(d store.Device) *entry {
	return &entry{d: d, names: map[string]string{}, addrs: map[netip.Addr]int64{},
		addrWrites: map[netip.Addr]int64{}, instances: map[string]string{}}
}

func (m *Monitor) index(e *entry) {
	m.byID[e.d.ID] = e
	m.byIdent[e.d.Ident] = e
	if ip, err := netip.ParseAddr(e.d.IP); err == nil {
		m.byIP[ip] = e
	}
}

func (m *Monitor) unindex(e *entry) {
	delete(m.byID, e.d.ID)
	delete(m.byIdent, e.d.Ident)
	for ip, x := range m.byIP {
		if x == e {
			delete(m.byIP, ip)
		}
	}
}

// virtualPrefixes are OUIs (or locally administered prefixes) of hypervisors
// and container runtimes.
var virtualPrefixes = []string{"02:42:", "52:54:00:", "00:16:3e:", "00:50:56:", "00:0c:29:", "00:05:69:",
	"08:00:27:", "00:15:5d:", "00:1c:42:", "0a:58:"}

// MACKind classifies a MAC as universal, random (private Wi-Fi address) or virtual.
func MACKind(mac net.HardwareAddr) string {
	s := mac.String()
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(s, p) {
			return "virtual"
		}
	}
	if oui.IsRandomized(mac) {
		return "random"
	}
	return "universal"
}

func (m *Monitor) nowMS() int64 { return m.now().UnixMilli() }

// lookup finds the entry a sighting belongs to. For proxy MACs (MAC-NAT
// repeaters) each IP is its own device, except the IP the base record uses.
func (m *Monitor) lookup(mac string, ip netip.Addr) *entry {
	if m.proxies[mac] && ip.IsValid() {
		base := m.byIdent[mac]
		if base != nil && base.d.IP == ip.String() {
			return base
		}
		return m.byIdent[mac+"/"+ip.String()]
	}
	if e := m.byIdent[mac]; e != nil {
		return e
	}
	return nil
}

func (m *Monitor) identFor(mac string, ip netip.Addr) (string, bool) {
	if m.proxies[mac] && ip.IsValid() {
		if base := m.byIdent[mac]; base != nil && base.d.IP != ip.String() {
			return mac + "/" + ip.String(), true
		}
	}
	return mac, false
}

// Observe processes one sighting.
func (m *Monitor) Observe(ctx context.Context, s l2.Sighting) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeLocked(ctx, s)
}

// presence-proving sources: the device itself sent something.
func provesPresence(s l2.Sighting) bool {
	switch s.Source {
	case "arp", "sweep", "probe", "mdns", "self":
		return true
	case "dhcp":
		return s.DHCP != nil && s.DHCP.Op == 1
	}
	return false
}

func (m *Monitor) observeLocked(ctx context.Context, s l2.Sighting) {
	if !proto.ValidUnicastMAC(s.MAC) {
		return
	}
	mac := s.MAC.String()
	e := m.lookup(mac, s.IP)
	if e == nil {
		if !s.ARP || !s.IP.IsValid() {
			m.stashInfo(mac, s)
			return
		}
		e = m.create(ctx, s)
		if e == nil {
			return
		}
	}
	now := s.At.UnixMilli()
	changed := false
	if provesPresence(s) && now > e.d.LastSeen {
		e.d.LastSeen = now
		m.dirty[e.d.ID] = now
	}
	if s.Iface != "" && e.d.Interface != s.Iface && s.ARP {
		e.d.Interface = s.Iface
		changed = true
	}
	if s.IP.IsValid() && (s.ARP || s.Source == "dhcp-ack") {
		if m.trackAddress(ctx, e, s.IP, s.Iface, now, s.Source == "dhcp-ack") {
			changed = true
		}
	}
	if provesPresence(s) && !e.d.Online {
		m.setOnline(ctx, e, now)
		changed = true
	}
	if s.DHCP != nil && m.applyDHCP(e, s.DHCP) {
		changed = true
	}
	if s.MDNS != nil && m.applyMDNS(e, s.MDNS) {
		changed = true
	}
	if changed {
		m.reclassify(e)
		m.persist(ctx, e)
		m.publish(e)
	}
}

func (m *Monitor) stashInfo(mac string, s l2.Sighting) {
	if s.DHCP == nil && s.MDNS == nil {
		return
	}
	st := m.stashed[mac]
	if s.DHCP != nil {
		st.dhcp = s.DHCP
	}
	if s.MDNS != nil {
		st.mdns = s.MDNS
	}
	st.at = s.At
	m.stashed[mac] = st
	if len(m.stashed) > 512 {
		cutoff := m.now().Add(-10 * time.Minute)
		for k, v := range m.stashed {
			if v.at.Before(cutoff) {
				delete(m.stashed, k)
			}
		}
		if len(m.stashed) > 512 {
			m.stashed = map[string]stash{mac: st}
		}
	}
}

func (m *Monitor) admit() bool {
	now := m.now()
	elapsed := now.Sub(m.admitRefill).Minutes()
	m.admitRefill = now
	m.admitTokens = min(admissionPerMinute, m.admitTokens+elapsed*admissionPerMinute)
	if m.admitTokens < 1 || len(m.byID) >= maxDevices {
		return false
	}
	m.admitTokens--
	return true
}

func (m *Monitor) create(ctx context.Context, s l2.Sighting) *entry {
	mac := s.MAC.String()
	ident, viaProxy := m.identFor(mac, s.IP)
	if !m.admit() {
		if m.now().Sub(m.floodLogged) > time.Hour {
			m.floodLogged = m.now()
			m.log.Warn("device admission limit reached; ignoring new devices for now", "limit_per_min", admissionPerMinute)
			m.addEvent(ctx, notify.KindDeviceFlood, "warning", nil, map[string]any{"limit": admissionPerMinute})
		}
		return nil
	}
	now := s.At.UnixMilli()
	d := store.Device{
		Ident: ident, MAC: mac, IP: s.IP.String(), Interface: s.Iface,
		FirstSeen: now, LastSeen: now, Online: true, MACKind: MACKind(s.MAC), ViaProxy: viaProxy,
		IsGateway: m.gateways[s.IP], IsSelf: m.selfMACs[mac] || s.Source == "self",
		Proxy: m.proxies[mac] && !viaProxy,
	}
	if d.MACKind != "random" {
		if v, ok := oui.Lookup(s.MAC); ok {
			d.Vendor, d.VendorOfficial = v.Name, v.Official
		}
	}
	e := newEntry(d)
	if st, ok := m.stashed[mac]; ok {
		delete(m.stashed, mac)
		if st.dhcp != nil {
			m.applyDHCP(e, st.dhcp)
		}
		if st.mdns != nil {
			m.applyMDNS(e, st.mdns)
		}
	}
	if s.DHCP != nil {
		m.applyDHCP(e, s.DHCP)
	}
	if s.MDNS != nil {
		m.applyMDNS(e, s.MDNS)
	}
	m.reclassify(e)
	if err := m.st.InsertDevice(ctx, &e.d); err != nil {
		m.log.Error("insert device", "mac", mac, "err", err)
		return nil
	}
	e.addrs[s.IP] = now
	e.addrWrites[s.IP] = now
	m.index(e)
	_ = m.st.TouchAddress(ctx, e.d.ID, e.d.IP, s.Iface, now)
	_ = m.st.OpenPresence(ctx, e.d.ID, now)
	m.persistNames(ctx, e)
	if m.baselineAt != 0 {
		snap := m.view(e)
		m.addEvent(ctx, notify.KindDeviceNew, "info", &snap, map[string]any{"ip": snap.IP, "vendor": snap.Vendor})
		if m.shouldNotifyNew(e) {
			m.notify.Notify(notify.Event{Kind: notify.KindDeviceNew, Severity: "info", TS: now, Device: &snap})
		}
	}
	if m.enricher != nil {
		m.enricher.Enqueue(e.d.ID)
	}
	m.publish(e)
	return e
}

func (m *Monitor) shouldNotifyNew(e *entry) bool {
	if !m.settings.NotifyNewDevice || e.d.IsSelf {
		return false
	}
	if e.d.MACKind == "random" && !m.settings.NotifyRandomMAC {
		return false
	}
	return true
}

// trackAddress records ip for e and decides whether it becomes the primary
// address. It returns true if the device record changed.
func (m *Monitor) trackAddress(ctx context.Context, e *entry, ip netip.Addr, iface string, now int64, authoritative bool) bool {
	e.addrs[ip] = now
	if now-e.addrWrites[ip] > 60_000 {
		e.addrWrites[ip] = now
		_ = m.st.TouchAddress(ctx, e.d.ID, ip.String(), iface, now)
	}
	cur, err := netip.ParseAddr(e.d.IP)
	if err == nil && cur == ip {
		return false
	}
	if err == nil && !authoritative {
		// Several addresses at once (secondary IPs, VRRP): keep the primary
		// until the old address has been silent for the offline threshold.
		if last, ok := e.addrs[cur]; ok && time.Duration(now-last)*time.Millisecond < m.settings.OfflineAfterDur(false) {
			return false
		}
	}
	old := e.d.IP
	e.d.IP = ip.String()
	if err == nil {
		if m.byIP[cur] == e {
			delete(m.byIP, cur)
		}
	}
	m.byIP[ip] = e
	e.d.IsGateway = m.gateways[ip]
	if old != "" {
		snap := m.view(e)
		m.addEvent(ctx, KindIPChanged, "info", &snap, map[string]any{"from": old, "to": e.d.IP})
	}
	return true
}

func (m *Monitor) setOnline(ctx context.Context, e *entry, now int64) {
	e.d.Online = true
	_ = m.st.OpenPresence(ctx, e.d.ID, now)
	if m.wasOnline[e.d.ID] {
		delete(m.wasOnline, e.d.ID) // it was there before Netzwart restarted
		return
	}
	if e.d.Watch {
		snap := m.view(e)
		m.addEvent(ctx, notify.KindDeviceOnline, "info", &snap, nil)
		m.notify.Notify(notify.Event{Kind: notify.KindDeviceOnline, Severity: "info", TS: now, Device: &snap})
	}
}

func (m *Monitor) setOffline(ctx context.Context, e *entry) {
	e.d.Online = false
	_ = m.st.ClosePresence(ctx, e.d.ID, e.d.LastSeen)
	m.persist(ctx, e)
	if e.d.Watch {
		snap := m.view(e)
		m.addEvent(ctx, notify.KindDeviceOffline, "info", &snap, nil)
		m.notify.Notify(notify.Event{Kind: notify.KindDeviceOffline, Severity: "info", TS: m.nowMS(), Device: &snap})
	}
	m.publish(e)
}

func (m *Monitor) applyDHCP(e *entry, d *proto.DHCP) bool {
	changed := false
	host := textutil.Clean(d.Hostname, 64)
	if host == "" && d.FQDN != "" {
		host = textutil.Hostname(d.FQDN)
	}
	if host != "" && e.names["dhcp"] != host {
		e.names["dhcp"] = host
		changed = true
	}
	if d.Op == 1 {
		if fp := proto.ParamListString(d.ParamList); fp != "" && fp != e.d.DHCPFingerprint && len(fp) <= 256 {
			e.d.DHCPFingerprint = fp
			changed = true
		}
		if vc := textutil.Clean(d.VendorClass, 96); vc != "" && vc != e.d.DHCPVendor {
			e.d.DHCPVendor = vc
			changed = true
		}
	}
	return changed
}

// instancePriority orders DNS-SD services by how human-friendly their
// instance names usually are.
var instancePriority = []string{"_companion-link._tcp", "_airplay._tcp", "_raop._tcp", "_device-info._tcp",
	"_sonos._tcp", "_spotify-connect._tcp", "_ipp._tcp", "_ipps._tcp", "_printer._tcp", "_hap._tcp",
	"_smb._tcp", "_afpovertcp._tcp", "_workstation._tcp", "_ssh._tcp", "_http._tcp"}

func (m *Monitor) applyMDNS(e *entry, info *proto.MDNSInfo) bool {
	changed := false
	if h := textutil.Hostname(info.Hostname); h != "" && e.names["mdns"] != h {
		e.names["mdns"] = h
		changed = true
	}
	for _, s := range info.ServiceTypes {
		s = textutil.Clean(s, 64)
		if s != "" && !contains(e.services, s) && len(e.services) < 48 {
			e.services = append(e.services, s)
			changed = true
		}
	}
	for _, in := range info.Instances {
		name := cleanInstance(in.Service, in.Instance)
		if name != "" && e.instances[in.Service] != name && len(e.instances) < 48 {
			e.instances[in.Service] = name
			changed = true
		}
	}
	if mdl := textutil.Clean(info.Model, 64); mdl != "" && mdl != e.model {
		e.model = mdl
		changed = true
	}
	if fn := textutil.Clean(info.FriendlyName, 64); fn != "" && e.names["cast"] != fn {
		e.names["cast"] = fn
		changed = true
	}
	if info.HomeKitCat != "" && info.HomeKitCat != e.homekit && len(info.HomeKitCat) <= 3 {
		e.homekit = info.HomeKitCat
		changed = true
	}
	if changed {
		best := ""
		for _, svc := range instancePriority {
			if n := e.instances[svc]; n != "" {
				best = n
				break
			}
		}
		if best != e.names["mdns_instance"] {
			e.names["mdns_instance"] = best
		}
	}
	return changed
}

func cleanInstance(service, name string) string {
	name = strings.ReplaceAll(name, `\032`, " ")
	if service == "_raop._tcp" { // "A1B2C3D4E5F6@Wohnzimmer"
		if _, after, ok := strings.Cut(name, "@"); ok {
			name = after
		}
	}
	name = textutil.Clean(name, 64)
	if looksGenerated(name) {
		return ""
	}
	return name
}

// looksGenerated filters instance names made of hex/UUIDs (e.g. Chromecast-<uuid>).
func looksGenerated(s string) bool {
	hex := 0
	for _, r := range s {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' || r == '-' {
			hex++
		}
	}
	return len(s) >= 16 && hex*10 >= len(s)*8
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// namePriority defines which discovered name is shown when the user set none.
var namePriority = []string{"mdns_instance", "cast", "ssdp", "fritzbox", "dhcp", "mdns", "netbios", "dns"}

func (m *Monitor) reclassify(e *entry) {
	host := ""
	for _, src := range namePriority {
		if n := e.names[src]; n != "" {
			host = n
			break
		}
	}
	e.d.Hostname = host
	if e.d.Model == "" || e.model != "" {
		e.d.Model = e.model
	}
	var hosts []string
	for _, n := range e.names {
		hosts = append(hosts, n)
	}
	r := classify.Classify(classify.Input{
		Vendor: e.d.Vendor, Hostnames: hosts, Services: e.services, Model: e.model,
		UPnPTypes: e.upnpTypes, UPnPMaker: e.upnpMaker, DHCPVendor: e.d.DHCPVendor, DHCPParams: e.d.DHCPFingerprint,
		HomeKitCat: e.homekit, OpenPorts: e.ports, IsGateway: e.d.IsGateway, IsSelf: e.d.IsSelf, MACKind: e.d.MACKind,
	})
	if e.d.Proxy && r.Type == classify.Unknown {
		r.Type, r.Confidence = classify.AccessPoint, 40
	}
	e.d.DetectedType, e.d.TypeConfidence, e.d.OS = r.Type, r.Confidence, r.OS
	e.d.Type = e.d.DetectedType
	if e.d.TypeOverride != "" {
		e.d.Type = e.d.TypeOverride
	}
}

func (m *Monitor) persist(ctx context.Context, e *entry) {
	if err := m.st.SaveDiscovered(ctx, &e.d); err != nil {
		m.log.Error("save device", "id", e.d.ID, "err", err)
	}
	delete(m.dirty, e.d.ID)
	m.persistNames(ctx, e)
}

func (m *Monitor) persistNames(ctx context.Context, e *entry) {
	for src, n := range e.names {
		_ = m.st.UpsertName(ctx, e.d.ID, src, n)
	}
	svcs := make([]store.Service, 0, len(e.services))
	for _, s := range e.services {
		svcs = append(svcs, store.Service{Source: "mdns", Service: s, Detail: e.instances[s]})
	}
	_ = m.st.ReplaceServices(ctx, e.d.ID, "mdns", svcs)
}

// view returns the device as the API shows it.
func (m *Monitor) view(e *entry) store.Device {
	d := e.d
	d.New = !d.Known && m.baselineAt != 0 && d.FirstSeen > m.baselineAt
	return d
}

func (m *Monitor) publish(e *entry) {
	m.hub.Publish("device", m.view(e))
}

func (m *Monitor) addEvent(ctx context.Context, kind, severity string, d *store.Device, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	ev := store.Event{TS: m.nowMS(), Kind: kind, Severity: severity}
	if d != nil {
		ev.DeviceID = d.ID
		data["name"] = displayName(*d)
		data["deviceType"] = d.Type
		if _, ok := data["ip"]; !ok {
			data["ip"] = d.IP
		}
	}
	ev.Data, _ = json.Marshal(data)
	if err := m.st.AddEvent(ctx, &ev); err != nil {
		m.log.Error("add event", "kind", kind, "err", err)
		return
	}
	m.hub.Publish("event", ev)
}

// AddEvent logs an event that does not concern a single device (used by
// other components such as the internet monitor).
func (m *Monitor) AddEvent(ctx context.Context, kind, severity string, deviceID int64, data map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var d *store.Device
	if e := m.byID[deviceID]; e != nil {
		v := m.view(e)
		d = &v
	}
	m.addEvent(ctx, kind, severity, d, data)
}

// displayName is the best name for logs and notifications: user name,
// discovered name, "<vendor> device", MAC.
func displayName(d store.Device) string {
	switch {
	case d.Name != "":
		return d.Name
	case d.Hostname != "":
		return d.Hostname
	case d.Vendor != "":
		return d.Vendor
	}
	return d.MAC
}

// Devices returns a snapshot of all devices sorted by ID.
func (m *Monitor) Devices() []store.Device {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]store.Device, 0, len(m.byID))
	for _, e := range m.byID {
		out = append(out, m.view(e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Device returns one device.
func (m *Monitor) Device(id int64) (store.Device, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.byID[id]
	if e == nil {
		return store.Device{}, false
	}
	return m.view(e), true
}

// DeviceByIP returns the device that currently uses ip.
func (m *Monitor) DeviceByIP(ip netip.Addr) (store.Device, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.byIP[ip]
	if e == nil {
		return store.Device{}, false
	}
	return m.view(e), true
}

// Baseline returns the end of the learning phase (0 while learning).
func (m *Monitor) Baseline() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.baselineAt
}
