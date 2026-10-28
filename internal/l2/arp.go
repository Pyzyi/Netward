package l2

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Pyzyi/Netward/internal/proto"
)

// SweepOptions tune a sweep. Zero values select safe defaults.
type SweepOptions struct {
	Rate   int           // packets per second (default 500)
	Rounds int           // attempts per address (default 3)
	Linger time.Duration // wait after each round (default 800 ms)
}

func (o *SweepOptions) defaults() {
	if o.Rate <= 0 {
		o.Rate = 500
	}
	if o.Rounds <= 0 {
		o.Rounds = 3
	}
	if o.Linger <= 0 {
		o.Linger = 800 * time.Millisecond
	}
}

// SweepResult maps each answering address to the distinct MACs that
// answered for it. More than one MAC per address indicates an IP conflict.
type SweepResult map[netip.Addr][]net.HardwareAddr

type collector struct {
	mu      sync.Mutex
	replies SweepResult
}

func (c *collector) add(ip netip.Addr, mac net.HardwareAddr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.replies[ip] {
		if string(m) == string(mac) {
			return
		}
	}
	c.replies[ip] = append(c.replies[ip], mac)
}

func (c *collector) has(ip netip.Addr) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.replies[ip]) > 0
}

// ARP drives the ARP socket of a link: it turns every received ARP packet
// into a sighting and serves sweeps and probes.
type ARP struct {
	link *Link
	out  chan<- Sighting
	log  *slog.Logger

	mu       sync.Mutex
	sweep    *collector
	sweepMu  sync.Mutex // one sweep at a time per link
	probes   map[netip.Addr][]chan net.HardwareAddr
	lastSeen map[[6]byte]time.Time
}

// NewARP returns the ARP driver for link. Sightings are dropped if out is full.
func NewARP(link *Link, out chan<- Sighting, log *slog.Logger) *ARP {
	return &ARP{link: link, out: out, log: log,
		probes: map[netip.Addr][]chan net.HardwareAddr{}, lastSeen: map[[6]byte]time.Time{}}
}

// passiveInterval bounds how often the same MAC yields a passive sighting.
const passiveInterval = 10 * time.Second

// Run reads the socket until ctx ends. A socket error (e.g. the interface
// vanished) is returned so the caller can restart the process.
func (a *ARP) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = a.link.arp.SetReadDeadline(time.Now())
	}()
	buf := make([]byte, 1600)
	for {
		n, err := a.link.arp.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			return err
		}
		a.handle(buf[:n], time.Now())
	}
}

func (a *ARP) handle(frame []byte, now time.Time) {
	p, err := proto.DecodeARP(frame)
	if err != nil {
		return
	}
	iface := a.link.Interface()
	// Only the sender fields are evidence; a "who-has" for a departed phone
	// says nothing about the phone. Probes (sender 0.0.0.0) carry no address.
	if !proto.ValidUnicastMAC(p.SenderMAC) || !p.SenderIP.IsValid() || p.SenderIP.IsUnspecified() {
		return
	}
	if !iface.Prefix.Contains(p.SenderIP) || p.SenderIP == iface.Addr || string(p.SenderMAC) == string(iface.MAC) {
		return
	}
	isReplyToUs := p.Op == proto.ARPReply && p.TargetIP == iface.Addr

	a.mu.Lock()
	sw := a.sweep
	waiters := a.probes[p.SenderIP]
	if isReplyToUs && len(waiters) > 0 {
		delete(a.probes, p.SenderIP)
	}
	a.mu.Unlock()

	if isReplyToUs {
		if sw != nil {
			sw.add(p.SenderIP, p.SenderMAC)
		}
		for _, w := range waiters {
			select {
			case w <- p.SenderMAC:
			default:
			}
		}
	}
	if !a.allowPassive(p.SenderMAC, now) {
		return
	}
	select {
	case a.out <- Sighting{MAC: p.SenderMAC, IP: p.SenderIP, Iface: iface.Name, Source: "arp", ARP: true, At: now}:
	default:
	}
}

func (a *ARP) allowPassive(mac net.HardwareAddr, now time.Time) bool {
	var k [6]byte
	copy(k[:], mac)
	a.mu.Lock()
	defer a.mu.Unlock()
	if t, ok := a.lastSeen[k]; ok && now.Sub(t) < passiveInterval {
		return false
	}
	a.lastSeen[k] = now
	if len(a.lastSeen) > 4096 { // bounded memory under a MAC flood
		for m, t := range a.lastSeen {
			if now.Sub(t) >= passiveInterval {
				delete(a.lastSeen, m)
			}
		}
		if len(a.lastSeen) > 4096 {
			a.lastSeen = map[[6]byte]time.Time{k: now}
		}
	}
	return true
}

func (a *ARP) write(frame []byte) error {
	_, err := a.link.arp.Write(frame)
	return err
}

// Sweep asks every target for its MAC and returns who answered.
func (a *ARP) Sweep(ctx context.Context, targets []netip.Addr, opt SweepOptions) (SweepResult, error) {
	opt.defaults()
	a.sweepMu.Lock()
	defer a.sweepMu.Unlock()

	iface := a.link.Interface()
	col := &collector{replies: SweepResult{}}
	a.mu.Lock()
	a.sweep = col
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.sweep = nil
		a.mu.Unlock()
	}()

	pending := make([]netip.Addr, 0, len(targets))
	for _, t := range targets {
		if t != iface.Addr && iface.Prefix.Contains(t) {
			pending = append(pending, t)
		}
	}
	const burst = 8
	pause := time.Second / time.Duration(opt.Rate) * burst
	for round := 0; round < opt.Rounds && len(pending) > 0; round++ {
		for i, t := range pending {
			if err := a.write(proto.EncodeARPRequest(iface.MAC, iface.Addr, t)); err != nil {
				if errors.Is(err, unix.ENXIO) || errors.Is(err, unix.ENODEV) {
					return col.replies, err // interface is gone
				}
				continue // e.g. ENOBUFS under load: retried next round
			}
			if i%burst == burst-1 {
				if err := sleep(ctx, pause); err != nil {
					return col.replies, err
				}
			}
		}
		if err := sleep(ctx, opt.Linger); err != nil {
			return col.replies, err
		}
		next := pending[:0]
		for _, t := range pending {
			if !col.has(t) {
				next = append(next, t)
			}
		}
		pending = next
	}
	// Late replies from slow devices.
	_ = sleep(ctx, 300*time.Millisecond)
	col.mu.Lock()
	defer col.mu.Unlock()
	out := make(SweepResult, len(col.replies))
	for ip, macs := range col.replies {
		out[ip] = append([]net.HardwareAddr(nil), macs...)
	}
	return out, nil
}

// Probe sends unicast ARP requests directly to mac (which bypasses the
// broadcast suppression some Wi-Fi power-save implementations do) and
// reports whether ip answered within timeout.
func (a *ARP) Probe(ctx context.Context, ip netip.Addr, mac net.HardwareAddr, timeout time.Duration) bool {
	iface := a.link.Interface()
	if !iface.Prefix.Contains(ip) {
		return false
	}
	ch := make(chan net.HardwareAddr, 1)
	a.mu.Lock()
	a.probes[ip] = append(a.probes[ip], ch)
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		ws := a.probes[ip]
		for i, w := range ws {
			if w == ch {
				a.probes[ip] = append(ws[:i], ws[i+1:]...)
				break
			}
		}
		if len(a.probes[ip]) == 0 {
			delete(a.probes, ip)
		}
		a.mu.Unlock()
	}()
	frame := proto.EncodeARPRequest(iface.MAC, iface.Addr, ip)
	if proto.ValidUnicastMAC(mac) {
		copy(frame[0:6], mac)
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(timeout / 4)
	defer tick.Stop()
	_ = a.write(frame)
	for {
		select {
		case got := <-ch:
			return mac == nil || string(got) == string(mac)
		case <-tick.C:
			_ = a.write(frame)
		case <-deadline.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
