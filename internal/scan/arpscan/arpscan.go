// Package arpscan discovers IPv4 hosts on a directly attached subnet.
//
// The primary implementation sends raw ARP requests through an AF_PACKET
// socket (requires CAP_NET_RAW). Without that capability the fallback makes
// the kernel resolve each address by sending a tiny UDP datagram and then
// reads the kernel's neighbour table.
package arpscan

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Pyzyi/netzwart/internal/netinfo"
	"github.com/Pyzyi/netzwart/internal/proto"
)

// Result is one host that answered.
type Result struct {
	IP    netip.Addr
	MAC   net.HardwareAddr
	Iface string
}

// Options tune a sweep. Zero values select safe defaults.
type Options struct {
	Rate    int           // packets per second (default 500)
	Rounds  int           // attempts per address (default 3)
	Linger  time.Duration // wait after each round (default 800 ms)
	Timeout time.Duration // overall cap (default 60 s)
}

func (o *Options) defaults() {
	if o.Rate <= 0 {
		o.Rate = 500
	}
	if o.Rounds <= 0 {
		o.Rounds = 3
	}
	if o.Linger <= 0 {
		o.Linger = 800 * time.Millisecond
	}
	if o.Timeout <= 0 {
		o.Timeout = 60 * time.Second
	}
}

// ErrNoRawSocket is returned when AF_PACKET sockets cannot be opened.
var ErrNoRawSocket = errors.New("raw sockets unavailable (missing CAP_NET_RAW)")

// RawAvailable reports whether this process may open AF_PACKET sockets.
func RawAvailable() bool {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return false
	}
	unix.Close(fd)
	return true
}

func htons(v uint16) uint16 { return v<<8 | v>>8 }

// openPacket opens a non-blocking AF_PACKET socket bound to ifindex for the
// given ethertype and wraps it in an *os.File so deadlines work.
func openPacket(ifindex int, ethertype uint16) (*os.File, error) {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, int(htons(ethertype)))
	if err != nil {
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
			return nil, ErrNoRawSocket
		}
		return nil, err
	}
	sa := &unix.SockaddrLinklayer{Protocol: htons(ethertype), Ifindex: ifindex}
	if err := unix.Bind(fd, sa); err != nil {
		unix.Close(fd)
		return nil, err
	}
	// Our own requests are of no interest; ignore failures on old kernels.
	_ = unix.SetsockoptInt(fd, unix.SOL_PACKET, unix.PACKET_IGNORE_OUTGOING, 1)
	return os.NewFile(uintptr(fd), "arp"), nil
}

// Sweep sends ARP requests to every target on iface and returns the hosts
// that replied. The local interface address itself is never probed.
func Sweep(ctx context.Context, iface netinfo.Interface, targets []netip.Addr, opt Options) ([]Result, error) {
	opt.defaults()
	ctx, cancel := context.WithTimeout(ctx, opt.Timeout)
	defer cancel()

	f, err := openPacket(iface.Index, proto.EtherTypeARP)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pending := make(map[netip.Addr]bool, len(targets))
	for _, t := range targets {
		if t != iface.Addr && iface.Prefix.Contains(t) {
			pending[t] = true
		}
	}

	var (
		mu    sync.Mutex
		found = map[netip.Addr]net.HardwareAddr{}
	)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		buf := make([]byte, 1600)
		for {
			if ctx.Err() != nil {
				return
			}
			_ = f.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			n, err := f.Read(buf)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					continue
				}
				return
			}
			a, err := proto.DecodeARP(buf[:n])
			if err != nil || !proto.ValidUnicastMAC(a.SenderMAC) || !a.SenderIP.IsValid() || a.SenderIP.IsUnspecified() {
				continue
			}
			if !iface.Prefix.Contains(a.SenderIP) || a.SenderIP == iface.Addr {
				continue
			}
			mu.Lock()
			if _, ok := found[a.SenderIP]; !ok {
				found[a.SenderIP] = a.SenderMAC
			}
			mu.Unlock()
		}
	}()

	interval := time.Second / time.Duration(opt.Rate)
	const burst = 8
sendLoop:
	for round := 0; round < opt.Rounds; round++ {
		sent := 0
		for _, t := range targets {
			if !pending[t] {
				continue
			}
			mu.Lock()
			_, done := found[t]
			mu.Unlock()
			if done {
				delete(pending, t)
				continue
			}
			if _, err := f.Write(proto.EncodeARPRequest(iface.MAC, iface.Addr, t)); err != nil {
				if errors.Is(err, unix.ENETDOWN) || errors.Is(err, unix.ENXIO) {
					break sendLoop
				}
				continue
			}
			sent++
			if sent%burst == 0 {
				select {
				case <-ctx.Done():
					break sendLoop
				case <-time.After(interval * burst):
				}
			}
		}
		select {
		case <-ctx.Done():
			break sendLoop
		case <-time.After(opt.Linger):
		}
		if len(pending) == 0 {
			break
		}
	}
	cancel()
	<-readerDone

	mu.Lock()
	defer mu.Unlock()
	out := make([]Result, 0, len(found))
	for ip, mac := range found {
		out = append(out, Result{IP: ip, MAC: mac, Iface: iface.Name})
	}
	return out, nil
}

// SweepUnprivileged discovers hosts without raw sockets: it nudges the kernel
// into ARP-resolving every target by sending a one-byte UDP datagram to the
// discard port, waits for resolution and then reads /proc/net/arp.
func SweepUnprivileged(ctx context.Context, iface netinfo.Interface, targets []netip.Addr, opt Options) ([]Result, error) {
	opt.defaults()
	interval := time.Second / time.Duration(opt.Rate)
	for i, t := range targets {
		if t == iface.Addr || !iface.Prefix.Contains(t) {
			continue
		}
		if c, err := net.DialUDP("udp4", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(t, 9))); err == nil {
			_, _ = c.Write([]byte{0})
			c.Close()
		}
		if i%8 == 7 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval * 8):
			}
		}
	}
	// The kernel sends up to 3 unicast/broadcast solicitations one second apart.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(3500 * time.Millisecond):
	}
	neigh, err := netinfo.ARPCache()
	if err != nil {
		return nil, err
	}
	want := make(map[netip.Addr]bool, len(targets))
	for _, t := range targets {
		want[t] = true
	}
	var out []Result
	for _, n := range neigh {
		if n.Interface == iface.Name && want[n.IP] && proto.ValidUnicastMAC(n.MAC) {
			out = append(out, Result{IP: n.IP, MAC: n.MAC, Iface: iface.Name})
		}
	}
	return out, nil
}
