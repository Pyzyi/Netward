package l2

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Pyzyi/netzwart/internal/netinfo"
	"github.com/Pyzyi/netzwart/internal/proto"
)

// FallbackMaxHosts caps unprivileged sweeps so the host's neighbour table
// (gc_thresh) cannot overflow.
const FallbackMaxHosts = 256

// Neighbor is a kernel neighbour-table entry.
type Neighbor struct {
	IP      netip.Addr
	MAC     net.HardwareAddr
	Ifindex int
	State   uint16
}

// Reachable reports whether the kernel recently confirmed the entry.
func (n Neighbor) Reachable() bool {
	return n.State&(unix.NUD_REACHABLE|unix.NUD_PERMANENT) != 0
}

// Neighbors dumps the IPv4 neighbour table via rtnetlink. Unlike
// /proc/net/arp this includes the entry state. No privileges are needed.
func Neighbors() ([]Neighbor, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 2})

	req := make([]byte, unix.NLMSG_HDRLEN+12)
	binary.NativeEndian.PutUint32(req[0:4], uint32(len(req)))
	binary.NativeEndian.PutUint16(req[4:6], unix.RTM_GETNEIGH)
	binary.NativeEndian.PutUint16(req[6:8], unix.NLM_F_REQUEST|unix.NLM_F_DUMP)
	binary.NativeEndian.PutUint32(req[8:12], 1)
	req[unix.NLMSG_HDRLEN] = unix.AF_INET
	if err := unix.Sendto(fd, req, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, err
	}
	var out []Neighbor
	buf := make([]byte, 64*1024)
	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			return nil, err
		}
		done, err := parseNeighborDump(buf[:n], &out)
		if err != nil || done {
			return out, err
		}
	}
}

func align4(n int) int { return (n + 3) &^ 3 }

// parseNeighborDump parses one netlink datagram; done is true at NLMSG_DONE.
func parseNeighborDump(b []byte, out *[]Neighbor) (done bool, err error) {
	for len(b) >= unix.NLMSG_HDRLEN {
		l := int(binary.NativeEndian.Uint32(b[0:4]))
		typ := binary.NativeEndian.Uint16(b[4:6])
		if l < unix.NLMSG_HDRLEN || l > len(b) {
			return false, errors.New("netlink: truncated message")
		}
		msg := b[unix.NLMSG_HDRLEN:l]
		switch typ {
		case unix.NLMSG_DONE:
			return true, nil
		case unix.NLMSG_ERROR:
			return true, errors.New("netlink: error reply")
		case unix.RTM_NEWNEIGH:
			if len(msg) >= 12 {
				nb := Neighbor{
					Ifindex: int(int32(binary.NativeEndian.Uint32(msg[4:8]))),
					State:   binary.NativeEndian.Uint16(msg[8:10]),
				}
				attrs := msg[12:]
				for len(attrs) >= 4 {
					al := int(binary.NativeEndian.Uint16(attrs[0:2]))
					at := binary.NativeEndian.Uint16(attrs[2:4])
					if al < 4 || al > len(attrs) {
						break
					}
					v := attrs[4:al]
					switch at {
					case unix.NDA_DST:
						if len(v) == 4 {
							nb.IP = netip.AddrFrom4([4]byte(v))
						}
					case unix.NDA_LLADDR:
						if len(v) == 6 {
							nb.MAC = append(net.HardwareAddr(nil), v...)
						}
					}
					attrs = attrs[min(align4(al), len(attrs)):]
				}
				if nb.IP.IsValid() {
					*out = append(*out, nb)
				}
			}
		}
		b = b[min(align4(l), len(b)):]
	}
	return false, nil
}

// poke makes the kernel resolve ip by sending one byte to the discard port.
func poke(ip netip.Addr) {
	c, err := net.DialUDP("udp4", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(ip, 9)))
	if err != nil {
		return
	}
	_, _ = c.Write([]byte{0})
	c.Close()
}

// SweepUnprivileged discovers hosts without raw sockets: every target is
// nudged with a UDP datagram, then neighbour entries the kernel confirmed as
// REACHABLE are collected. Larger subnets are truncated to FallbackMaxHosts.
func SweepUnprivileged(ctx context.Context, iface netinfo.Interface, targets []netip.Addr) (SweepResult, error) {
	if len(targets) > FallbackMaxHosts {
		targets = targets[:FallbackMaxHosts]
	}
	want := make(map[netip.Addr]bool, len(targets))
	for i, t := range targets {
		if t == iface.Addr || !iface.Prefix.Contains(t) {
			continue
		}
		want[t] = true
		poke(t)
		if i%8 == 7 {
			if err := sleep(ctx, 16*time.Millisecond); err != nil {
				return nil, err
			}
		}
	}
	// Stale entries are re-validated after delay_first_probe_time (5 s) and
	// a few unicast probes; give the kernel time to settle.
	if err := sleep(ctx, 8*time.Second); err != nil {
		return nil, err
	}
	neigh, err := Neighbors()
	if err != nil {
		return nil, err
	}
	out := SweepResult{}
	for _, n := range neigh {
		if n.Ifindex == iface.Index && want[n.IP] && n.Reachable() && proto.ValidUnicastMAC(n.MAC) {
			out[n.IP] = append(out[n.IP], n.MAC)
		}
	}
	return out, nil
}

// ProbeUnprivileged checks a single address via the kernel's resolver.
func ProbeUnprivileged(ctx context.Context, iface netinfo.Interface, ip netip.Addr, mac net.HardwareAddr, timeout time.Duration) bool {
	poke(ip)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := sleep(ctx, 500*time.Millisecond); err != nil {
			return false
		}
		neigh, err := Neighbors()
		if err != nil {
			return false
		}
		for _, n := range neigh {
			if n.IP == ip && n.Ifindex == iface.Index && n.State&unix.NUD_REACHABLE != 0 &&
				(mac == nil || string(n.MAC) == string(mac)) {
				return true
			}
		}
	}
	return false
}
