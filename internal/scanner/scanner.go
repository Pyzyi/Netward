// Package scanner drives discovery: periodic ARP sweeps, passive listening,
// mDNS/SSDP browsing, name resolution, wake probes, port scans and
// Wake-on-LAN. It feeds everything it learns into the monitor.
package scanner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/discovery"
	"github.com/Pyzyi/netzwart/internal/egress"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/l2"
	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/netinfo"
	"github.com/Pyzyi/netzwart/internal/proto"
	"github.com/Pyzyi/netzwart/internal/store"
)

// MaxHosts is the largest subnet Netzwart will sweep at all (/16).
const MaxHosts = 1 << 16

// DefaultMaxHosts is the largest subnet swept without explicit opt-in (/22).
const DefaultMaxHosts = 1022

// ErrLinkLost means an interface whose sockets were opened at start-up has
// disappeared; the process must restart to open new sockets.
var ErrLinkLost = errors.New("network interface changed; restart required")

// IfaceStatus describes one interface in the UI.
type IfaceStatus struct {
	Name    string `json:"name"`
	MAC     string `json:"mac"`
	Addr    string `json:"addr"`
	Prefix  string `json:"prefix"`
	Hosts   int    `json:"hosts"`
	Active  bool   `json:"active"`
	Default bool   `json:"default"`
	Skipped string `json:"skipped,omitempty"` // reason why an active interface is not swept
}

// Status is the scanner state shown in the UI.
type Status struct {
	Mode             string        `json:"mode"` // raw | fallback
	Scanning         bool          `json:"scanning"`
	LastScanAt       int64         `json:"lastScanAt"`
	LastScanDuration int64         `json:"lastScanDuration"`
	NextScanAt       int64         `json:"nextScanAt"`
	Interfaces       []IfaceStatus `json:"interfaces"`
	Gateways         []string      `json:"gateways"`
	Error            string        `json:"error,omitempty"`
}

type ssdpEntry struct {
	loc       *url.URL
	fetchedAt time.Time
}

// Scanner coordinates all active and passive discovery.
type Scanner struct {
	mon    *monitor.Monitor
	hub    *hub.Hub
	log    *slog.Logger
	links  map[string]*l2.Link // opened before the privilege drop; empty in fallback mode
	arps   map[string]*l2.ARP
	pinger *l2.Pinger
	extra  []string // NETZWART_INTERFACES

	sightings chan l2.Sighting
	trigger   chan struct{}
	enrichQ   chan int64

	descPolicy *egress.Policy
	descClient *http.Client

	mu        sync.Mutex
	settings  config.Settings
	status    Status
	ifaces    []netinfo.Interface // currently swept
	scanSince time.Time           // start of the running sweep, zero when idle
	enqueued  map[int64]bool
	ssdp      map[netip.Addr]*ssdpEntry
	portScans map[int64]bool
}

// New creates the scanner. links may be empty (fallback mode); pinger may be nil.
func New(mon *monitor.Monitor, h *hub.Hub, links []*l2.Link, pinger *l2.Pinger, extra []string, s config.Settings, log *slog.Logger) *Scanner {
	sc := &Scanner{
		mon: mon, hub: h, log: log, links: map[string]*l2.Link{}, arps: map[string]*l2.ARP{}, pinger: pinger,
		extra: extra, sightings: make(chan l2.Sighting, 2048), trigger: make(chan struct{}, 1),
		enrichQ: make(chan int64, 4096), settings: s, enqueued: map[int64]bool{}, ssdp: map[netip.Addr]*ssdpEntry{},
		portScans: map[int64]bool{}, descPolicy: &egress.Policy{},
	}
	sc.descClient = sc.descPolicy.HTTPClient(3*time.Second, 0)
	sc.descClient.CheckRedirect = proto.NoRedirect
	sc.descClient.Jar = nil
	for _, l := range links {
		name := l.Interface().Name
		sc.links[name] = l
		sc.arps[name] = l2.NewARP(l, sc.sightings, log)
	}
	sc.status.Mode = "raw"
	if len(links) == 0 {
		sc.status.Mode = "fallback"
	}
	return sc
}

// UpdateSettings applies new settings and starts a scan if the interface
// selection changed.
func (s *Scanner) UpdateSettings(st config.Settings) {
	s.mu.Lock()
	changed := fmt.Sprint(st.Interfaces, st.AllowLargeSubnets) != fmt.Sprint(s.settings.Interfaces, s.settings.AllowLargeSubnets)
	s.settings = st
	s.mu.Unlock()
	if changed {
		s.TriggerScan()
	}
}

// TriggerScan requests a scan as soon as possible.
func (s *Scanner) TriggerScan() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Status returns a copy of the current status.
func (s *Scanner) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	st.Interfaces = append([]IfaceStatus(nil), s.status.Interfaces...)
	st.Gateways = append([]string(nil), s.status.Gateways...)
	return st
}

func (s *Scanner) setStatus(fn func(*Status)) {
	s.mu.Lock()
	fn(&s.status)
	st := s.status
	s.mu.Unlock()
	s.hub.Publish("scanner", st)
}

// Run starts all loops and blocks until ctx ends or a link is lost.
func (s *Scanner) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 2*len(s.links)+1)
	for name, l := range s.links {
		arp := s.arps[name]
		lis := l2.NewListener(l, s.sightings, s.log)
		go func() { errc <- arp.Run(ctx) }()
		go func() { errc <- lis.Run(ctx) }()
	}
	go s.consume(ctx)
	for i := 0; i < 2; i++ {
		go s.enrichWorker(ctx)
	}
	go s.discoveryLoop(ctx)
	go func() { errc <- s.scanLoop(ctx) }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errc:
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			err = ErrLinkLost
		}
		return fmt.Errorf("%w: %v", ErrLinkLost, err)
	}
}

func (s *Scanner) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case sg := <-s.sightings:
			s.mon.Observe(ctx, sg)
		}
	}
}

// refresh re-reads interface and routing information and decides which
// interfaces are swept.
func (s *Scanner) refresh() ([]netinfo.Interface, error) {
	s.mu.Lock()
	settings := s.settings
	s.mu.Unlock()

	routes, _ := netinfo.DefaultGateways()
	defaultIf := map[string]bool{}
	var gateways []netip.Addr
	var gwStrings []string
	for _, r := range routes {
		defaultIf[r.Interface] = true
		gateways = append(gateways, r.Gateway)
		gwStrings = append(gwStrings, r.Gateway.String())
	}

	var all []netinfo.Interface
	var err error
	if len(s.links) > 0 {
		names := make([]string, 0, len(s.links))
		for n := range s.links {
			names = append(names, n)
		}
		all, err = netinfo.Interfaces(names)
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, i := range all {
			l := s.links[i.Name]
			if l == nil || seen[i.Name] {
				continue
			}
			seen[i.Name] = true
			if !l.UpdateInterface(i) {
				return nil, fmt.Errorf("%w: %s has a new index", ErrLinkLost, i.Name)
			}
		}
		for n := range s.links {
			if !seen[n] {
				return nil, fmt.Errorf("%w: %s is gone or has no IPv4 address", ErrLinkLost, n)
			}
		}
	} else {
		all, err = netinfo.Interfaces(nil)
		if err != nil {
			return nil, err
		}
		if len(s.extra) > 0 {
			more, _ := netinfo.Interfaces(s.extra)
			all = append(all, more...)
		}
	}

	want := map[string]bool{}
	for _, n := range settings.Interfaces {
		want[n] = true
	}
	if len(want) == 0 {
		for n := range defaultIf {
			want[n] = true
		}
		if len(want) == 0 && len(all) > 0 {
			want[all[0].Name] = true
		}
	}
	var active []netinfo.Interface
	var statuses []IfaceStatus
	var selfMACs []net.HardwareAddr
	seen := map[string]bool{}
	for _, i := range all {
		st := IfaceStatus{Name: i.Name, MAC: i.MACStr, Addr: i.Addr.String(), Prefix: i.Prefix.String(),
			Hosts: i.Hosts, Active: want[i.Name], Default: defaultIf[i.Name]}
		selfMACs = append(selfMACs, i.MAC)
		if st.Active && !seen[i.Name] {
			switch {
			case i.Hosts > MaxHosts:
				st.Skipped = "too_large"
			case i.Hosts > DefaultMaxHosts && !settings.AllowLargeSubnets:
				st.Skipped = "large"
			case len(s.links) == 0 && i.Hosts > l2.FallbackMaxHosts:
				st.Skipped = "fallback_limit"
				active = append(active, i) // swept, but truncated
			default:
				active = append(active, i)
			}
			seen[i.Name] = true
		}
		statuses = append(statuses, st)
	}
	sort.Slice(statuses, func(a, b int) bool { return statuses[a].Name < statuses[b].Name })
	s.mon.SetNetwork(gateways, selfMACs)
	var own []netip.Addr
	for _, i := range all {
		own = append(own, i.Addr)
	}
	s.descPolicy.SetDenied(own) // never fetch "descriptions" from ourselves
	s.mu.Lock()
	s.ifaces = active
	s.status.Interfaces = statuses
	s.status.Gateways = gwStrings
	s.mu.Unlock()
	return active, nil
}

func (s *Scanner) scanLoop(ctx context.Context) error {
	first := time.NewTimer(2 * time.Second)
	defer first.Stop()
	for {
		s.mu.Lock()
		interval := time.Duration(s.settings.ScanInterval) * time.Second
		s.mu.Unlock()
		next := time.Now().Add(interval)
		s.setStatus(func(st *Status) { st.NextScanAt = next.UnixMilli() })
		wait := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			wait.Stop()
			return nil
		case <-first.C:
		case <-s.trigger:
		case <-wait.C:
		}
		wait.Stop()
		if err := s.scanOnce(ctx); err != nil {
			if errors.Is(err, ErrLinkLost) {
				return err
			}
			if ctx.Err() == nil {
				s.log.Error("scan failed", "err", err)
			}
		}
	}
}

// Health reports how many interfaces are swept and since when the running
// sweep is going (zero when idle). The watchdog uses it.
func (s *Scanner) Health() (swept int, scanningSince time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ifaces), s.scanSince
}

func (s *Scanner) scanOnce(ctx context.Context) error {
	ifaces, err := s.refresh()
	if err != nil {
		s.setStatus(func(st *Status) { st.Error = err.Error() })
		return err
	}
	start := time.Now()
	s.mu.Lock()
	s.scanSince = start
	s.mu.Unlock()
	s.setStatus(func(st *Status) { st.Scanning = true; st.Error = "" })
	defer func() {
		s.mu.Lock()
		s.scanSince = time.Time{}
		s.mu.Unlock()
		d := time.Since(start)
		s.setStatus(func(st *Status) {
			st.Scanning = false
			st.LastScanAt = time.Now().UnixMilli()
			st.LastScanDuration = d.Milliseconds()
		})
	}()
	for _, i := range ifaces {
		targets := netinfo.Hosts(i.Prefix)
		var res l2.SweepResult
		if arp := s.arps[i.Name]; arp != nil {
			res, err = arp.Sweep(ctx, targets, l2.SweepOptions{})
		} else {
			res, err = l2.SweepUnprivileged(ctx, i, targets)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.log.Warn("sweep failed", "iface", i.Name, "err", err)
			continue
		}
		s.mon.ObserveSelf(ctx, i.Name, i.Addr, i.MAC)
		s.mon.ApplySweep(ctx, i.Name, res, time.Now())
		s.log.Debug("sweep done", "iface", i.Name, "hosts", len(res), "took", time.Since(start))
	}
	s.mu.Lock()
	resolve := s.settings.ResolveNames
	s.mu.Unlock()
	if resolve {
		for _, id := range s.mon.NeedsEnrichment(time.Now().Add(-6 * time.Hour).UnixMilli()) {
			s.Enqueue(id)
		}
	}
	return nil
}

// Enqueue schedules name resolution for a device.
func (s *Scanner) Enqueue(id int64) {
	s.mu.Lock()
	if s.enqueued[id] {
		s.mu.Unlock()
		return
	}
	s.enqueued[id] = true
	s.mu.Unlock()
	select {
	case s.enrichQ <- id:
	default:
		s.mu.Lock()
		delete(s.enqueued, id)
		s.mu.Unlock()
	}
}

func (s *Scanner) enrichWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.enrichQ:
			s.mu.Lock()
			delete(s.enqueued, id)
			resolve := s.settings.ResolveNames
			s.mu.Unlock()
			if resolve {
				s.enrich(ctx, id)
			}
		}
	}
}

func (s *Scanner) enrich(ctx context.Context, id int64) {
	d, ok := s.mon.Device(id)
	if !ok {
		return
	}
	ip, err := netip.ParseAddr(d.IP)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	var en monitor.Enrichment
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); en.DNS = discovery.ReverseDNS(ctx, ip) }()
	go func() { defer wg.Done(); en.NetBIOS, _ = discovery.NetBIOS(ctx, ip) }()
	go func() {
		defer wg.Done()
		if info := discovery.MDNSUnicast(ctx, ip); info.Response {
			en.MDNS = &info
		}
	}()
	wg.Wait()
	s.mu.Lock()
	entry := s.ssdp[ip]
	due := entry != nil && time.Since(entry.fetchedAt) > 6*time.Hour
	if due {
		entry.fetchedAt = time.Now()
	}
	s.mu.Unlock()
	if due {
		if dev, err := discovery.FetchDescription(ctx, s.descClient, entry.loc); err == nil {
			en.UPnP = &dev
		} else {
			s.log.Debug("upnp description", "ip", ip, "err", err)
		}
	}
	en.At = time.Now().UnixMilli()
	s.mon.ApplyEnrichment(ctx, id, en)
}

func (s *Scanner) discoveryLoop(ctx context.Context) {
	t := time.NewTimer(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.mu.Lock()
		ifaces := append([]netinfo.Interface(nil), s.ifaces...)
		resolve := s.settings.ResolveNames
		s.mu.Unlock()
		if resolve {
			for _, i := range ifaces {
				s.discover(ctx, i)
			}
		}
		t.Reset(10 * time.Minute)
	}
}

func (s *Scanner) discover(ctx context.Context, i netinfo.Interface) {
	ifi, err := net.InterfaceByName(i.Name)
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	var mdns map[netip.Addr]*proto.MDNSInfo
	var ssdp map[netip.Addr][]proto.SSDPResponse
	wg.Add(2)
	go func() { defer wg.Done(); mdns = discovery.MDNSBrowse(ctx, ifi, i.Prefix) }()
	go func() { defer wg.Done(); ssdp = discovery.SSDPSearch(ctx, ifi, i.Prefix) }()
	wg.Wait()
	for ip, info := range mdns {
		if d, ok := s.mon.DeviceByIP(ip); ok {
			s.mon.ApplyEnrichment(ctx, d.ID, monitor.Enrichment{MDNS: info})
		}
	}
	for ip, rs := range ssdp {
		if ip == i.Addr {
			continue
		}
		for _, r := range rs {
			u, ok := proto.SafeLocation(r.Location, ip)
			if !ok {
				continue
			}
			s.mu.Lock()
			if e := s.ssdp[ip]; e == nil || e.loc.String() != u.String() {
				if len(s.ssdp) < 1024 {
					s.ssdp[ip] = &ssdpEntry{loc: u}
				}
			}
			s.mu.Unlock()
			if d, ok := s.mon.DeviceByIP(ip); ok {
				s.Enqueue(d.ID)
			}
			break
		}
	}
}

// Probe actively checks whether d is still present: unicast ARP, ICMP,
// TCP to the iOS lockdown port and a unicast mDNS query run in parallel.
func (s *Scanner) Probe(ctx context.Context, d store.Device) bool {
	ip, err := netip.ParseAddr(d.IP)
	if err != nil {
		return false
	}
	mac, _ := net.ParseMAC(d.MAC)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	results := make(chan bool, 4)
	n := 0
	run := func(f func() bool) {
		n++
		go func() { results <- f() }()
	}
	if arp := s.arps[d.Interface]; arp != nil {
		run(func() bool { return arp.Probe(ctx, ip, mac, 3*time.Second) })
	} else {
		s.mu.Lock()
		var iface *netinfo.Interface
		for k := range s.ifaces {
			if s.ifaces[k].Prefix.Contains(ip) {
				iface = &s.ifaces[k]
			}
		}
		s.mu.Unlock()
		if iface != nil {
			i := *iface
			run(func() bool { return l2.ProbeUnprivileged(ctx, i, ip, mac, 4*time.Second) })
		}
	}
	if s.pinger != nil {
		run(func() bool { _, ok := s.pinger.Ping(ctx, ip, 3*time.Second); return ok })
	}
	run(func() bool { return discovery.TCPAlive(ctx, ip, 62078, 2*time.Second) })
	run(func() bool { return discovery.MDNSUnicast(ctx, ip).Response })
	for i := 0; i < n; i++ {
		select {
		case ok := <-results:
			if ok {
				return true
			}
		case <-ctx.Done():
			return false
		}
	}
	return false
}

// Ping measures the round-trip time to a device.
func (s *Scanner) Ping(ctx context.Context, d store.Device) (time.Duration, bool) {
	ip, err := netip.ParseAddr(d.IP)
	if err != nil {
		return 0, false
	}
	if s.pinger != nil {
		if rtt, ok := s.pinger.Ping(ctx, ip, 3*time.Second); ok {
			return rtt, true
		}
	}
	if arp := s.arps[d.Interface]; arp != nil {
		mac, _ := net.ParseMAC(d.MAC)
		start := time.Now()
		if arp.Probe(ctx, ip, mac, 2*time.Second) {
			return time.Since(start), true
		}
	}
	return 0, false
}
