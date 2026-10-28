package l2

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Pinger sends ICMP echo requests over one shared socket. With a raw socket
// (opened before the privilege drop) replies are matched by identifier and
// sequence; with an unprivileged "ping socket" the kernel does the matching.
type Pinger struct {
	conn *icmp.PacketConn
	raw  bool
	id   int
	seq  atomic.Uint32

	mu      sync.Mutex
	waiting map[uint16]waiter
}

// waiter is a sent request: only a reply from its target counts.
type waiter struct {
	ip netip.Addr
	ch chan time.Time
}

// OpenPinger tries a raw ICMP socket first, then an unprivileged one.
// It returns nil if neither is available.
func OpenPinger() *Pinger {
	if c, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0"); err == nil {
		return newPinger(c, true)
	}
	if c, err := icmp.ListenPacket("udp4", "0.0.0.0"); err == nil {
		return newPinger(c, false)
	}
	return nil
}

func newPinger(c *icmp.PacketConn, raw bool) *Pinger {
	// A random identifier: the PID is 1 in a container, easy to guess for
	// anyone who wants to answer in another device's name.
	var b [2]byte
	_, _ = rand.Read(b[:])
	p := &Pinger{conn: c, raw: raw, id: int(binary.BigEndian.Uint16(b[:])), waiting: map[uint16]waiter{}}
	go p.readLoop()
	return p
}

// Raw reports whether the pinger uses a raw socket.
func (p *Pinger) Raw() bool { return p != nil && p.raw }

func (p *Pinger) readLoop() {
	buf := make([]byte, 1500)
	for {
		n, peer, err := p.conn.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		msg, err := icmp.ParseMessage(1, buf[:n])
		if err != nil || msg.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		echo, ok := msg.Body.(*icmp.Echo)
		if !ok || (p.raw && echo.ID != p.id) {
			continue
		}
		p.mu.Lock()
		w, ok := p.waiting[uint16(echo.Seq)]
		p.mu.Unlock()
		if ok && peerAddr(peer) == w.ip {
			select {
			case w.ch <- time.Now():
			default:
			}
		}
	}
}

// peerAddr is the sender of a received packet.
func peerAddr(a net.Addr) netip.Addr {
	var ip net.IP
	switch v := a.(type) {
	case *net.IPAddr:
		ip = v.IP
	case *net.UDPAddr:
		ip = v.IP
	}
	addr, _ := netip.AddrFromSlice(ip)
	return addr.Unmap()
}

// Ping sends up to three echo requests and returns the first round-trip time.
func (p *Pinger) Ping(ctx context.Context, ip netip.Addr, timeout time.Duration) (time.Duration, bool) {
	if p == nil || !ip.Is4() {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for attempt := 0; attempt < 3; attempt++ {
		seq := uint16(p.seq.Add(1))
		ch := make(chan time.Time, 1)
		p.mu.Lock()
		p.waiting[seq] = waiter{ip: ip, ch: ch}
		p.mu.Unlock()
		msg := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: p.id, Seq: int(seq), Data: []byte("netward")}}
		b, _ := msg.Marshal(nil)
		var dst net.Addr = &net.IPAddr{IP: ip.AsSlice()}
		if !p.raw {
			dst = &net.UDPAddr{IP: ip.AsSlice()}
		}
		start := time.Now()
		_, err := p.conn.WriteTo(b, dst)
		var rtt time.Duration
		ok := false
		if err == nil {
			wait := time.NewTimer(timeout / 3)
			select {
			case t := <-ch:
				rtt, ok = t.Sub(start), true
			case <-wait.C:
			case <-ctx.Done():
			}
			wait.Stop()
		}
		p.mu.Lock()
		delete(p.waiting, seq)
		p.mu.Unlock()
		if ok {
			return rtt, true
		}
		if ctx.Err() != nil {
			break
		}
	}
	return 0, false
}

// Close releases the socket.
func (p *Pinger) Close() {
	if p != nil {
		p.conn.Close()
	}
}
