// Package passive listens to ARP and DHCP traffic that reaches the host and
// turns it into sightings, so new devices show up within seconds instead of
// at the next sweep. A classic BPF program runs in the kernel and discards
// everything else before it is copied to user space.
package passive

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"

	"github.com/Pyzyi/netzwart/internal/netinfo"
	"github.com/Pyzyi/netzwart/internal/proto"
)

// Sighting is evidence that a device is present.
type Sighting struct {
	MAC   net.HardwareAddr
	IP    netip.Addr // may be invalid (e.g. DHCP discover before a lease)
	Iface string
	DHCP  *proto.DHCP
	At    time.Time
}

// filter accepts ARP frames and unfragmented IPv4/UDP datagrams to port 67 or 68.
func filter() ([]bpf.RawInstruction, error) {
	return bpf.Assemble([]bpf.Instruction{
		bpf.LoadAbsolute{Off: 12, Size: 2},                                    // 0: ethertype
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: proto.EtherTypeARP, SkipTrue: 9}, // 1: -> accept
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: proto.EtherTypeIPv4, SkipFalse: 9},
		bpf.LoadAbsolute{Off: 23, Size: 1},                          // 3: IP protocol
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 17, SkipFalse: 7},      // 4: UDP?
		bpf.LoadAbsolute{Off: 20, Size: 2},                          // 5: flags/fragment offset
		bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x1fff, SkipTrue: 5}, // 6: fragment -> drop
		bpf.LoadMemShift{Off: 14},                                   // 7: X = IP header length
		bpf.LoadIndirect{Off: 16, Size: 2},                          // 8: UDP destination port
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 67, SkipTrue: 1},       // 9
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 68, SkipFalse: 1},      // 10
		bpf.RetConstant{Val: 1600},                                  // 11: accept
		bpf.RetConstant{Val: 0},                                     // 12: drop
	})
}

func htons(v uint16) uint16 { return v<<8 | v>>8 }

// open creates the socket with protocol 0 (receives nothing), attaches the
// filter and only then binds to ETH_P_ALL, so no unfiltered frame is queued.
func open(ifindex int) (*os.File, error) {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	prog, err := filter()
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	fprog := unix.SockFprog{
		Len:    uint16(len(prog)),
		Filter: (*unix.SockFilter)(unsafe.Pointer(&prog[0])),
	}
	if err := unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &fprog); err != nil {
		unix.Close(fd)
		return nil, err
	}
	_ = unix.SetsockoptInt(fd, unix.SOL_PACKET, unix.PACKET_IGNORE_OUTGOING, 1)
	if err := unix.Bind(fd, &unix.SockaddrLinklayer{Protocol: htons(unix.ETH_P_ALL), Ifindex: ifindex}); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), "passive"), nil
}

// Listener captures sightings on one interface.
type Listener struct {
	iface netinfo.Interface
	out   chan<- Sighting
	log   *slog.Logger

	mu   sync.Mutex
	last map[[6]byte]time.Time // per-MAC rate limit for ARP sightings
}

// New returns a listener that writes to out. Sightings are dropped when out is full.
func New(iface netinfo.Interface, out chan<- Sighting, log *slog.Logger) *Listener {
	return &Listener{iface: iface, out: out, log: log, last: map[[6]byte]time.Time{}}
}

// arpInterval bounds how often the same MAC produces an ARP sighting.
const arpInterval = 10 * time.Second

// Run blocks until ctx is cancelled or the socket fails.
func (l *Listener) Run(ctx context.Context) error {
	f, err := open(l.iface.Index)
	if err != nil {
		return err
	}
	defer f.Close()
	go func() {
		<-ctx.Done()
		_ = f.SetReadDeadline(time.Now())
	}()
	buf := make([]byte, 1600)
	for {
		n, err := f.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			return err
		}
		l.handle(buf[:n])
	}
}

func (l *Listener) handle(frame []byte) {
	defer func() {
		if r := recover(); r != nil { // parsers are fuzzed; this is a last line of defence
			l.log.Error("passive: recovered from panic", "panic", r)
		}
	}()
	if len(frame) >= 12 && net.HardwareAddr(frame[6:12]).String() == l.iface.MACStr {
		return // our own traffic
	}
	now := time.Now()
	if a, err := proto.DecodeARP(frame); err == nil {
		if !proto.ValidUnicastMAC(a.SenderMAC) || !a.SenderIP.IsValid() || a.SenderIP.IsUnspecified() {
			return // ARP probes (RFC 5227) carry no sender address yet
		}
		if !l.iface.Prefix.Contains(a.SenderIP) || a.SenderIP == l.iface.Addr {
			return
		}
		if !l.allow(a.SenderMAC, now) {
			return
		}
		l.emit(Sighting{MAC: a.SenderMAC, IP: a.SenderIP, Iface: l.iface.Name, At: now})
		return
	}
	_, _, sport, dport, payload, err := proto.UDPPayload(frame)
	if err != nil || !((sport == 68 && dport == 67) || (sport == 67 && dport == 68)) {
		return
	}
	d, err := proto.DecodeDHCP(payload)
	if err != nil || !proto.ValidUnicastMAC(d.ClientMAC) {
		return
	}
	s := Sighting{MAC: d.ClientMAC, Iface: l.iface.Name, DHCP: &d, At: now}
	switch {
	case d.Op == 2 && d.MsgType == proto.DHCPAck && l.iface.Prefix.Contains(d.YourIP):
		s.IP = d.YourIP
	case d.ClientIP.IsValid() && !d.ClientIP.IsUnspecified() && l.iface.Prefix.Contains(d.ClientIP):
		s.IP = d.ClientIP
	case d.RequestedIP.IsValid() && l.iface.Prefix.Contains(d.RequestedIP):
		// Requested is only a wish; do not treat it as the device's address.
	}
	if d.Op == 2 && !s.IP.IsValid() {
		return // server replies without a usable lease tell us nothing
	}
	l.emit(s)
}

func (l *Listener) allow(mac net.HardwareAddr, now time.Time) bool {
	var k [6]byte
	copy(k[:], mac)
	l.mu.Lock()
	defer l.mu.Unlock()
	if t, ok := l.last[k]; ok && now.Sub(t) < arpInterval {
		return false
	}
	l.last[k] = now
	if len(l.last) > 4096 { // bounded memory even under a MAC flood
		for m, t := range l.last {
			if now.Sub(t) >= arpInterval {
				delete(l.last, m)
			}
		}
		if len(l.last) > 4096 {
			l.last = map[[6]byte]time.Time{k: now}
		}
	}
	return true
}

func (l *Listener) emit(s Sighting) {
	select {
	case l.out <- s:
	default: // consumer is busy; the next sweep will catch up
	}
}
