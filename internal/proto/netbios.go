package proto

import (
	"encoding/binary"
	"errors"
	"strings"
)

// NetBIOSName is one entry of a node status response.
type NetBIOSName struct {
	Name   string
	Suffix byte
	Group  bool
}

// NetBIOSStatus is the decoded answer to a node status (NBSTAT) query.
type NetBIOSStatus struct {
	Names []NetBIOSName
}

var errNetBIOS = errors.New("invalid NetBIOS node status response")

// EncodeNetBIOSStatusQuery builds a node status request for the wildcard name "*".
func EncodeNetBIOSStatusQuery(txid uint16) []byte {
	q := make([]byte, 0, 50)
	q = binary.BigEndian.AppendUint16(q, txid)
	q = append(q, 0x00, 0x00) // flags: query
	q = append(q, 0x00, 0x01) // QDCOUNT
	q = append(q, 0, 0, 0, 0, 0, 0)
	q = append(q, 0x20)
	name := [16]byte{'*'}
	for _, b := range name {
		q = append(q, 'A'+(b>>4), 'A'+(b&0x0f))
	}
	q = append(q, 0x00)
	q = append(q, 0x00, 0x21) // NBSTAT
	q = append(q, 0x00, 0x01) // IN
	return q
}

// DecodeNetBIOSStatus parses a node status response for transaction txid.
func DecodeNetBIOSStatus(p []byte, txid uint16) (NetBIOSStatus, error) {
	if len(p) < 12 || binary.BigEndian.Uint16(p[0:2]) != txid || p[2]&0x80 == 0 {
		return NetBIOSStatus{}, errNetBIOS
	}
	if binary.BigEndian.Uint16(p[6:8]) == 0 { // ANCOUNT
		return NetBIOSStatus{}, errNetBIOS
	}
	i := 12
	// Skip the owner name: either a compression pointer or labels.
	for {
		if i >= len(p) {
			return NetBIOSStatus{}, errNetBIOS
		}
		l := int(p[i])
		if l&0xc0 == 0xc0 {
			i += 2
			break
		}
		i++
		if l == 0 {
			break
		}
		i += l
	}
	// type(2) class(2) ttl(4) rdlength(2)
	if i+10 > len(p) || binary.BigEndian.Uint16(p[i:i+2]) != 0x21 {
		return NetBIOSStatus{}, errNetBIOS
	}
	rdlen := int(binary.BigEndian.Uint16(p[i+8 : i+10]))
	i += 10
	if i+rdlen > len(p) || rdlen < 1 {
		return NetBIOSStatus{}, errNetBIOS
	}
	rd := p[i : i+rdlen]
	n := int(rd[0])
	if 1+n*18 > len(rd) {
		return NetBIOSStatus{}, errNetBIOS
	}
	st := NetBIOSStatus{Names: make([]NetBIOSName, 0, n)}
	for k := 0; k < n; k++ {
		e := rd[1+k*18 : 1+(k+1)*18]
		st.Names = append(st.Names, NetBIOSName{
			Name:   strings.TrimRight(string(e[0:15]), " \x00"),
			Suffix: e[15],
			Group:  binary.BigEndian.Uint16(e[16:18])&0x8000 != 0,
		})
	}
	return st, nil
}

// Workstation returns the unique workstation name (suffix 0x00), if any.
func (s NetBIOSStatus) Workstation() string {
	for _, n := range s.Names {
		if n.Suffix == 0x00 && !n.Group && n.Name != "" {
			return n.Name
		}
	}
	return ""
}

// Workgroup returns the group name with suffix 0x00 (domain or workgroup).
func (s NetBIOSStatus) Workgroup() string {
	for _, n := range s.Names {
		if n.Suffix == 0x00 && n.Group && n.Name != "" {
			return n.Name
		}
	}
	return ""
}
