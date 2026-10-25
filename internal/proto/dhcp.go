package proto

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// DHCP message types (option 53).
const (
	DHCPDiscover = 1
	DHCPOffer    = 2
	DHCPRequest  = 3
	DHCPAck      = 5
	DHCPInform   = 8
)

// DHCP holds the fields Netzwart extracts from a BOOTP/DHCP packet.
type DHCP struct {
	Op          uint8 // 1 = client request, 2 = server reply
	MsgType     uint8
	ClientMAC   net.HardwareAddr
	ClientIP    netip.Addr // ciaddr
	YourIP      netip.Addr // yiaddr (server replies)
	RequestedIP netip.Addr // option 50
	Hostname    string     // option 12 (raw, caller must sanitise)
	FQDN        string     // option 81 (raw)
	VendorClass string     // option 60 (raw)
	ParamList   []byte     // option 55
}

var errNotDHCP = errors.New("not a DHCP packet")

// UDPPayload extracts the UDP payload and ports from an Ethernet/IPv4 frame.
// Fragmented datagrams are rejected.
func UDPPayload(frame []byte) (src, dst netip.Addr, sport, dport uint16, payload []byte, err error) {
	if len(frame) < ethHeaderLen+20+8 || binary.BigEndian.Uint16(frame[12:14]) != EtherTypeIPv4 {
		return src, dst, 0, 0, nil, errNotDHCP
	}
	ip := frame[ethHeaderLen:]
	if ip[0]>>4 != 4 {
		return src, dst, 0, 0, nil, errNotDHCP
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < 20 || len(ip) < ihl+8 || ip[9] != 17 {
		return src, dst, 0, 0, nil, errNotDHCP
	}
	if binary.BigEndian.Uint16(ip[6:8])&0x3fff != 0 { // MF flag or fragment offset
		return src, dst, 0, 0, nil, errNotDHCP
	}
	total := int(binary.BigEndian.Uint16(ip[2:4]))
	if total < ihl+8 || total > len(ip) {
		total = len(ip) // tolerate padding or truncated length fields
	}
	src = netip.AddrFrom4([4]byte(ip[12:16]))
	dst = netip.AddrFrom4([4]byte(ip[16:20]))
	udp := ip[ihl:total]
	sport = binary.BigEndian.Uint16(udp[0:2])
	dport = binary.BigEndian.Uint16(udp[2:4])
	ulen := int(binary.BigEndian.Uint16(udp[4:6]))
	if ulen < 8 || ulen > len(udp) {
		ulen = len(udp)
	}
	return src, dst, sport, dport, udp[8:ulen], nil
}

// DecodeDHCP parses a BOOTP payload (the UDP payload, not the frame).
func DecodeDHCP(p []byte) (DHCP, error) {
	const fixed = 236
	if len(p) < fixed+4 {
		return DHCP{}, errNotDHCP
	}
	if binary.BigEndian.Uint32(p[fixed:fixed+4]) != 0x63825363 {
		return DHCP{}, errNotDHCP
	}
	d := DHCP{Op: p[0]}
	if d.Op != 1 && d.Op != 2 {
		return DHCP{}, errNotDHCP
	}
	if p[1] == 1 && p[2] == 6 { // Ethernet, 6-byte addresses
		d.ClientMAC = append(net.HardwareAddr(nil), p[28:34]...)
	}
	d.ClientIP = netip.AddrFrom4([4]byte(p[12:16]))
	d.YourIP = netip.AddrFrom4([4]byte(p[16:20]))

	opts := p[fixed+4:]
	for i := 0; i < len(opts); {
		code := opts[i]
		if code == 0 { // pad
			i++
			continue
		}
		if code == 255 { // end
			break
		}
		if i+1 >= len(opts) {
			break
		}
		l := int(opts[i+1])
		if i+2+l > len(opts) {
			break
		}
		v := opts[i+2 : i+2+l]
		switch code {
		case 53:
			if l >= 1 {
				d.MsgType = v[0]
			}
		case 12:
			d.Hostname = string(v)
		case 60:
			d.VendorClass = string(v)
		case 55:
			d.ParamList = append([]byte(nil), v...)
		case 50:
			if l == 4 {
				d.RequestedIP = netip.AddrFrom4([4]byte(v))
			}
		case 81:
			d.FQDN = decodeFQDNOption(v)
		}
		i += 2 + l
	}
	if d.MsgType == 0 {
		return DHCP{}, errNotDHCP
	}
	return d, nil
}

// decodeFQDNOption decodes RFC 4702 option 81: flags, rcode1, rcode2, name.
// The name is either ASCII or DNS wire format depending on the E flag.
func decodeFQDNOption(v []byte) string {
	if len(v) < 4 {
		return ""
	}
	name := v[3:]
	if v[0]&0x04 == 0 { // E flag clear: deprecated ASCII encoding
		return string(name)
	}
	var labels []string
	for i := 0; i < len(name); {
		l := int(name[i])
		if l == 0 || l > 63 || i+1+l > len(name) {
			break
		}
		labels = append(labels, string(name[i+1:i+1+l]))
		i += 1 + l
	}
	return strings.Join(labels, ".")
}

// ParamListString renders option 55 the way DHCP fingerprint databases do ("1,3,6,15").
func ParamListString(pl []byte) string {
	parts := make([]string, len(pl))
	for i, b := range pl {
		parts[i] = strconv.Itoa(int(b))
	}
	return strings.Join(parts, ",")
}
