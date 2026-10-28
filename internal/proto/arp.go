// Package proto contains small, allocation-light encoders and decoders for
// the link-local protocols Netward speaks. Every decoder treats its input as
// hostile: lengths are checked before every read and nothing panics.
package proto

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
)

const (
	EtherTypeARP  = 0x0806
	EtherTypeIPv4 = 0x0800

	ARPRequest = 1
	ARPReply   = 2

	ethHeaderLen = 14
	arpLen       = 28
	minFrameLen  = 60 // Ethernet minimum without FCS
)

// Broadcast is the Ethernet broadcast address.
var Broadcast = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// ARP is a decoded IPv4-over-Ethernet ARP packet.
type ARP struct {
	Op        uint16
	SenderMAC net.HardwareAddr
	SenderIP  netip.Addr
	TargetMAC net.HardwareAddr
	TargetIP  netip.Addr
}

var errNotARP = errors.New("not an IPv4/Ethernet ARP frame")

// EncodeARPRequest builds a broadcast "who-has target tell sender" frame,
// padded to the Ethernet minimum length.
func EncodeARPRequest(srcMAC net.HardwareAddr, srcIP, target netip.Addr) []byte {
	f := make([]byte, minFrameLen)
	copy(f[0:6], Broadcast)
	copy(f[6:12], srcMAC)
	binary.BigEndian.PutUint16(f[12:14], EtherTypeARP)
	a := f[ethHeaderLen:]
	binary.BigEndian.PutUint16(a[0:2], 1)             // hardware type: Ethernet
	binary.BigEndian.PutUint16(a[2:4], EtherTypeIPv4) // protocol type
	a[4] = 6
	a[5] = 4
	binary.BigEndian.PutUint16(a[6:8], ARPRequest)
	copy(a[8:14], srcMAC)
	s4 := srcIP.As4()
	copy(a[14:18], s4[:])
	// target hardware address stays zero
	t4 := target.As4()
	copy(a[24:28], t4[:])
	return f
}

// DecodeARP parses an Ethernet frame carrying an ARP packet.
func DecodeARP(frame []byte) (ARP, error) {
	if len(frame) < ethHeaderLen+arpLen {
		return ARP{}, errNotARP
	}
	if binary.BigEndian.Uint16(frame[12:14]) != EtherTypeARP {
		return ARP{}, errNotARP
	}
	a := frame[ethHeaderLen:]
	if binary.BigEndian.Uint16(a[0:2]) != 1 || binary.BigEndian.Uint16(a[2:4]) != EtherTypeIPv4 || a[4] != 6 || a[5] != 4 {
		return ARP{}, errNotARP
	}
	op := binary.BigEndian.Uint16(a[6:8])
	if op != ARPRequest && op != ARPReply {
		return ARP{}, errNotARP
	}
	return ARP{
		Op:        op,
		SenderMAC: append(net.HardwareAddr(nil), a[8:14]...),
		SenderIP:  netip.AddrFrom4([4]byte(a[14:18])),
		TargetMAC: append(net.HardwareAddr(nil), a[18:24]...),
		TargetIP:  netip.AddrFrom4([4]byte(a[24:28])),
	}, nil
}

// ValidUnicastMAC reports whether mac can identify a single device.
func ValidUnicastMAC(mac net.HardwareAddr) bool {
	if len(mac) != 6 || mac[0]&0x01 != 0 {
		return false
	}
	for _, b := range mac {
		if b != 0 {
			return true
		}
	}
	return false
}
