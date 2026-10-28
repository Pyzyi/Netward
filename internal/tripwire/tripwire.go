// Package tripwire opens decoy services that no normal device ever talks to
// (docs/10, F3). Telnet, remote desktop, ADB or a camera login are what
// malware and intruders probe first; a connection from inside the home
// network is a strong sign of an infected device, with almost no false
// alarms. Nothing behind the decoys works: they greet, read a little,
// close, and never keep what was sent.
package tripwire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Port states shown in the UI.
const (
	StateOpen    = "open"
	StateInUse   = "in_use"  // another program on the host uses the port
	StateRestart = "restart" // a port below 1024: only opened at start (NETZWART_TRIPWIRE_PORTS)
	StateError   = "error"
)

// Hit is one connection to a decoy.
type Hit struct {
	IP      netip.Addr
	Port    int
	Service string
	At      time.Time
}

// PortStatus describes one decoy.
type PortStatus struct {
	Port    int    `json:"port"`
	Service string `json:"service"`
	State   string `json:"state"`
}

// Status is what the UI shows.
type Status struct {
	Enabled bool         `json:"enabled"`
	Ports   []PortStatus `json:"ports"`
	LastHit *HitView     `json:"lastHit,omitempty"`
}

// HitView is the last hit as shown in the UI.
type HitView struct {
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	Service string `json:"service"`
	At      int64  `json:"at"`
}

// Service names the decoy offered on a port.
func Service(port int) string {
	switch port {
	case 23, 2323:
		return "telnet"
	case 21, 2121:
		return "ftp"
	case 22, 2222:
		return "ssh"
	case 3389:
		return "rdp"
	case 5555:
		return "adb"
	case 5900, 5901:
		return "vnc"
	case 445, 139:
		return "smb"
	case 80, 8000, 8080, 8081, 8888:
		return "camera"
	case 1883:
		return "mqtt"
	case 6379:
		return "redis"
	}
	return "tcp"
}

// greeting is what a decoy says first, as the real service would.
func greeting(service string) []byte {
	switch service {
	case "telnet":
		return []byte("\r\nlogin: ")
	case "ftp":
		return []byte("220 FTP server ready\r\n")
	case "ssh":
		return []byte("SSH-2.0-OpenSSH_7.4\r\n")
	case "vnc":
		return []byte("RFB 003.008\n")
	case "camera":
		body := "<html><body><h1>401 Unauthorized</h1></body></html>"
		return []byte("HTTP/1.1 401 Unauthorized\r\nWWW-Authenticate: Basic realm=\"IP Camera\"\r\n" +
			"Content-Type: text/html\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\nConnection: close\r\n\r\n" + body)
	}
	return nil
}

const (
	maxConns   = 32               // decoy connections handled at once
	readLimit  = 4096             // bytes read from a visitor, then discarded
	holdFor    = 10 * time.Second // how long a visitor is kept talking
	quiet      = 10 * time.Minute // one report per address (IPv6: per /64) and port in this time
	maxTracked = 4096             // addresses remembered for that; beyond, reports are dropped
	perMinute  = 20               // reports per minute over all addresses (token bucket)
	queueLen   = 64               // reports waiting for the handler
)

// Manager runs the decoys.
type Manager struct {
	log *slog.Logger
	hit func(Hit)

	mu       sync.Mutex
	enabled  bool
	want     map[int]bool
	ln       map[int]net.Listener
	state    map[int]string
	reserved map[int]bool          // privileged listeners from the start; cannot be reopened
	own      func(netip.Addr) bool // addresses of this host
	onLink   func(netip.Addr) bool // inside a network of this host (global IPv6 of the LAN)
	allow    []netip.Prefix
	last     map[string]time.Time // ip/port -> last report
	sweptAt  time.Time
	tokens   float64
	tokensAt time.Time
	dropped  int // reports dropped by the limits since the last log line
	lastHit  *HitView
	sem      chan struct{}
	hits     chan Hit
	listen   func(port int) (net.Listener, error)
	now      func() time.Time
}

// Host tells the manager about the machine it runs on.
type Host struct {
	Own    func(netip.Addr) bool // an address of this host: never reported
	OnLink func(netip.Addr) bool // inside one of the host's networks: counts as home network
}

// New creates a manager. hit is called one report at a time from a single
// goroutine: at most once per address and port in ten minutes and at most
// 20 per minute overall, so a flood of spoofed addresses cannot flood the
// database or the notifications. pre are listeners opened at start while
// privileged (ports below 1024).
func New(log *slog.Logger, pre map[int]net.Listener, host Host, hit func(Hit)) *Manager {
	m := &Manager{log: log, hit: hit, own: host.Own, onLink: host.OnLink, want: map[int]bool{}, ln: map[int]net.Listener{},
		state: map[int]string{}, reserved: map[int]bool{}, last: map[string]time.Time{}, sem: make(chan struct{}, maxConns),
		hits: make(chan Hit, queueLen), tokens: perMinute, now: time.Now}
	go func() {
		for h := range m.hits {
			if m.hit != nil {
				m.hit(h)
			}
		}
	}()
	m.listen = func(port int) (net.Listener, error) {
		var lc net.ListenConfig
		return lc.Listen(context.Background(), "tcp", ":"+strconv.Itoa(port))
	}
	for port, l := range pre {
		m.reserved[port] = true
		m.ln[port] = l
		m.state[port] = StateOpen
		go m.serve(port, l)
	}
	return m
}

// Configure sets the decoy ports and the addresses never reported.
// Disabling closes every decoy, also those opened at start.
func (m *Manager) Configure(enabled bool, ports []int, allow []netip.Prefix) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
	m.allow = allow
	m.want = map[int]bool{}
	if enabled {
		for _, p := range ports {
			m.want[p] = true
		}
		for p := range m.reserved {
			m.want[p] = true
		}
	}
	for port, l := range m.ln {
		if !m.want[port] {
			_ = l.Close()
			delete(m.ln, port)
			if m.reserved[port] {
				m.state[port] = StateRestart
			} else {
				delete(m.state, port)
			}
		}
	}
	for port := range m.want {
		if _, ok := m.ln[port]; ok {
			continue
		}
		if port < 1024 {
			m.state[port] = StateRestart
			continue
		}
		l, err := m.listen(port)
		switch {
		case errors.Is(err, syscall.EADDRINUSE):
			m.state[port] = StateInUse
			continue
		case err != nil:
			m.log.Warn("tripwire port not opened", "port", port, "err", err)
			m.state[port] = StateError
			continue
		}
		m.ln[port] = l
		m.state[port] = StateOpen
		go m.serve(port, l)
	}
	for port := range m.state {
		if !m.want[port] && !m.reserved[port] {
			delete(m.state, port)
		}
	}
}

// Close stops every decoy.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = false
	for port, l := range m.ln {
		_ = l.Close()
		delete(m.ln, port)
		delete(m.state, port)
	}
}

// Status reports the decoys for the UI.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{Enabled: m.enabled, Ports: []PortStatus{}}
	for port, s := range m.state {
		st.Ports = append(st.Ports, PortStatus{Port: port, Service: Service(port), State: s})
	}
	sort.Slice(st.Ports, func(i, j int) bool { return st.Ports[i].Port < st.Ports[j].Port })
	if m.lastHit != nil {
		h := *m.lastHit
		st.LastHit = &h
	}
	return st
}

func (m *Manager) serve(port int, l net.Listener) {
	service := Service(port)
	failures := 0
	for {
		c, err := l.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			// Out of file descriptors and the like: wait and try again, but
			// do not keep claiming the port is watched if it never recovers.
			if failures++; failures > 30 {
				m.log.Error("tripwire port stopped", "port", port, "err", err)
				m.mu.Lock()
				if m.ln[port] == l {
					m.state[port] = StateError
				}
				m.mu.Unlock()
				return
			}
			time.Sleep(min(time.Second, time.Duration(failures)*50*time.Millisecond))
			continue
		}
		failures = 0
		select {
		case m.sem <- struct{}{}:
			go func() {
				defer func() { <-m.sem }()
				m.handle(c, port, service)
			}()
		default:
			m.note(c.RemoteAddr(), port, service) // busy: still counts, but no conversation
			_ = c.Close()
		}
	}
}

func (m *Manager) handle(c net.Conn, port int, service string) {
	defer c.Close()
	m.note(c.RemoteAddr(), port, service)
	_ = c.SetDeadline(time.Now().Add(holdFor))
	if g := greeting(service); g != nil {
		_, _ = c.Write(g)
	}
	// Read a little so the visitor sends what it came for; nothing is kept.
	_, _ = io.Copy(io.Discard, io.LimitReader(c, readLimit))
}

// note reports a connection unless it comes from the host itself, from an
// allowed address or from outside the home network, or was just reported.
func (m *Manager) note(addr net.Addr, port int, service string) {
	ap, err := netip.ParseAddrPort(addr.String())
	if err != nil {
		return
	}
	ip := ap.Addr().Unmap()
	if ip.IsLoopback() || !(internal(ip) || (m.onLink != nil && m.onLink(ip))) || (m.own != nil && m.own(ip)) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.allow {
		if p.Contains(ip) {
			return
		}
	}
	now := m.now()
	key := Group(ip) + "/" + strconv.Itoa(port)
	if t, ok := m.last[key]; ok && now.Sub(t) < quiet {
		return
	}
	if len(m.last) >= maxTracked && now.Sub(m.sweptAt) >= time.Minute {
		m.sweptAt = now
		for k, t := range m.last {
			if now.Sub(t) >= quiet {
				delete(m.last, k)
			}
		}
	}
	m.tokens = min(perMinute, m.tokens+now.Sub(m.tokensAt).Minutes()*perMinute)
	m.tokensAt = now
	if len(m.last) >= maxTracked || m.tokens < 1 {
		m.dropped++
		return
	}
	m.tokens--
	m.last[key] = now
	m.lastHit = &HitView{IP: ip.String(), Port: port, Service: service, At: now.UnixMilli()}
	select {
	case m.hits <- Hit{IP: ip, Port: port, Service: service, At: now}:
	default:
		m.dropped++
		return
	}
	m.log.Warn("tripwire: connection to a decoy service", "ip", ip, "port", port, "service", service, "droppedBefore", m.dropped)
	m.dropped = 0
}

// Group is what one source counts as: the address, for IPv6 its /64 (a host
// can pick any address in its network, privacy addresses change daily).
func Group(ip netip.Addr) string {
	if ip.Is6() && !ip.Is4In6() {
		p, _ := ip.Prefix(64)
		return p.String()
	}
	return ip.String()
}

// cgnat is 100.64.0.0/10, used by VPNs such as Tailscale for their clients.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// internal reports addresses from inside the home network (or a VPN into
// it). Connections from the internet, e.g. through a port forwarding, are
// not what the tripwire is about.
func internal(ip netip.Addr) bool {
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || cgnat.Contains(ip)
}

// ParsePorts reads a list like "23,2323" (NETZWART_TRIPWIRE_PORTS).
func ParsePorts(v []string) ([]int, error) {
	var out []int
	for _, s := range v {
		p, err := strconv.Atoi(s)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid port %q", s)
		}
		out = append(out, p)
	}
	return out, nil
}
