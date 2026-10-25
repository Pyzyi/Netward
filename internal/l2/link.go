// Package l2 talks to the local network segment: it sends ARP sweeps and
// probes, and listens to ARP, DHCP and mDNS traffic.
//
// Raw sockets are opened once, while the process still holds CAP_NET_RAW
// (see internal/privdrop). The kernel checks that capability only when a
// socket is created, so the sockets keep working after all privileges have
// been dropped.
package l2

import (
	"errors"
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

// Sighting is evidence that a device is present on the segment.
type Sighting struct {
	MAC    net.HardwareAddr
	IP     netip.Addr // invalid when the packet does not prove an address
	Iface  string
	Source string // "arp", "sweep", "dhcp", "mdns", "probe"
	ARP    bool   // true if MAC and IP come from ARP sender fields
	DHCP   *proto.DHCP
	MDNS   *proto.MDNSInfo
	At     time.Time
}

// ErrNoRawSocket is returned when AF_PACKET sockets cannot be created.
var ErrNoRawSocket = errors.New("raw sockets unavailable (missing CAP_NET_RAW)")

// Link holds the raw sockets of one interface.
type Link struct {
	mu    sync.RWMutex
	iface netinfo.Interface

	arp *os.File // ETH_P_ARP: sweep requests out, all ARP in
	ip  *os.File // ETH_P_IP with BPF: DHCP and mDNS in
}

func htons(v uint16) uint16 { return v<<8 | v>>8 }

// Open creates both sockets for iface. It must run while CAP_NET_RAW is held.
func Open(iface netinfo.Interface) (*Link, error) {
	arpSock, err := openPacket(iface.Index, proto.EtherTypeARP, nil, nil)
	if err != nil {
		return nil, err
	}
	prog, err := ipFilter()
	if err != nil {
		arpSock.Close()
		return nil, err
	}
	// Join the mDNS group at link level so the NIC delivers announcements even
	// if no other process on the host listens for them.
	mdnsMAC := net.HardwareAddr{0x01, 0x00, 0x5e, 0x00, 0x00, 0xfb}
	ipSock, err := openPacket(iface.Index, proto.EtherTypeIPv4, prog, []net.HardwareAddr{mdnsMAC})
	if err != nil {
		arpSock.Close()
		return nil, err
	}
	return &Link{iface: iface, arp: arpSock, ip: ipSock}, nil
}

// Interface returns the current interface configuration.
func (l *Link) Interface() netinfo.Interface {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.iface
}

// UpdateInterface refreshes address information after a DHCP renewal on the
// host. The interface index must stay the same.
func (l *Link) UpdateInterface(i netinfo.Interface) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i.Index != l.iface.Index {
		return false
	}
	l.iface = i
	return true
}

// Close releases the sockets.
func (l *Link) Close() {
	l.arp.Close()
	l.ip.Close()
}

// openPacket creates the socket with protocol 0 (it receives nothing), attaches
// the optional filter and only then binds it, so no unfiltered frame is ever
// queued.
func openPacket(ifindex int, ethertype uint16, prog []bpf.RawInstruction, groups []net.HardwareAddr) (*os.File, error) {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
			return nil, ErrNoRawSocket
		}
		return nil, err
	}
	if len(prog) > 0 {
		fprog := unix.SockFprog{Len: uint16(len(prog)), Filter: (*unix.SockFilter)(unsafe.Pointer(&prog[0]))}
		if err := unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &fprog); err != nil {
			unix.Close(fd)
			return nil, err
		}
	}
	// Our own traffic (and the NAS's outgoing bulk transfers) never reaches us.
	_ = unix.SetsockoptInt(fd, unix.SOL_PACKET, unix.PACKET_IGNORE_OUTGOING, 1)
	_ = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, 1<<20)
	if err := unix.Bind(fd, &unix.SockaddrLinklayer{Protocol: htons(ethertype), Ifindex: ifindex}); err != nil {
		unix.Close(fd)
		return nil, err
	}
	for _, g := range groups {
		mreq := unix.PacketMreq{Ifindex: int32(ifindex), Type: unix.PACKET_MR_MULTICAST, Alen: uint16(len(g))}
		copy(mreq.Address[:], g)
		_ = unix.SetsockoptPacketMreq(fd, unix.SOL_PACKET, unix.PACKET_ADD_MEMBERSHIP, &mreq)
	}
	return os.NewFile(uintptr(fd), "packet"), nil
}

// ipFilter accepts unfragmented IPv4/UDP datagrams to ports 67, 68 and 5353.
func ipFilter() ([]bpf.RawInstruction, error) {
	return bpf.Assemble([]bpf.Instruction{
		bpf.LoadAbsolute{Off: 12, Size: 2},                                       // 0: ethertype
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: proto.EtherTypeIPv4, SkipFalse: 10}, // 1
		bpf.LoadAbsolute{Off: 23, Size: 1},                                       // 2: IP protocol
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 17, SkipFalse: 8},                   // 3: UDP
		bpf.LoadAbsolute{Off: 20, Size: 2},                                       // 4: flags/fragment offset
		bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x3fff, SkipTrue: 6},              // 5: MF or offset -> drop
		bpf.LoadMemShift{Off: 14},                                                // 6: X = IP header length
		bpf.LoadIndirect{Off: 16, Size: 2},                                       // 7: UDP destination port
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 67, SkipTrue: 2},                    // 8
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 68, SkipTrue: 1},                    // 9
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 5353, SkipFalse: 1},                 // 10
		bpf.RetConstant{Val: 1600},                                               // 11: accept
		bpf.RetConstant{Val: 0},                                                  // 12: drop
	})
}

// RawAvailable reports whether this process may create AF_PACKET sockets.
func RawAvailable() bool {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return false
	}
	unix.Close(fd)
	return true
}
