package proto

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
)

var (
	macA = net.HardwareAddr{0x02, 0x42, 0xac, 0x11, 0x00, 0x02}
	macB = net.HardwareAddr{0x3c, 0x22, 0xfb, 0x01, 0x02, 0x03}
)

func TestARPRoundTrip(t *testing.T) {
	src := netip.MustParseAddr("192.168.1.10")
	dst := netip.MustParseAddr("192.168.1.20")
	f := EncodeARPRequest(macA, src, dst)
	if len(f) != 60 {
		t.Fatalf("frame length %d, want 60", len(f))
	}
	if !bytes.Equal(f[0:6], Broadcast) || !bytes.Equal(f[6:12], macA) {
		t.Fatal("bad Ethernet header")
	}
	a, err := DecodeARP(f)
	if err != nil {
		t.Fatal(err)
	}
	if a.Op != ARPRequest || a.SenderIP != src || a.TargetIP != dst || !bytes.Equal(a.SenderMAC, macA) {
		t.Fatalf("unexpected decode: %+v", a)
	}
}

func TestDecodeARPRejects(t *testing.T) {
	f := EncodeARPRequest(macA, netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2"))
	bad := append([]byte(nil), f...)
	bad[12], bad[13] = 0x08, 0x00
	if _, err := DecodeARP(bad); err == nil {
		t.Error("IPv4 ethertype accepted as ARP")
	}
	bad = append([]byte(nil), f...)
	bad[14+5] = 16 // IPv6 protocol length
	if _, err := DecodeARP(bad); err == nil {
		t.Error("non-IPv4 ARP accepted")
	}
	if _, err := DecodeARP(f[:41]); err == nil {
		t.Error("truncated frame accepted")
	}
}

func TestValidUnicastMAC(t *testing.T) {
	if ValidUnicastMAC(Broadcast) || ValidUnicastMAC(net.HardwareAddr{0, 0, 0, 0, 0, 0}) || ValidUnicastMAC(net.HardwareAddr{1, 0, 0x5e, 0, 0, 1}) {
		t.Error("broadcast/zero/multicast accepted")
	}
	if !ValidUnicastMAC(macB) {
		t.Error("unicast rejected")
	}
}

// buildDHCPFrame assembles Ethernet/IPv4/UDP/BOOTP with the given options.
func buildDHCPFrame(op byte, chaddr net.HardwareAddr, opts []byte) []byte {
	bootp := make([]byte, 240)
	bootp[0] = op
	bootp[1], bootp[2] = 1, 6
	copy(bootp[28:34], chaddr)
	binary.BigEndian.PutUint32(bootp[236:240], 0x63825363)
	bootp = append(bootp, opts...)
	udp := make([]byte, 8)
	binary.BigEndian.PutUint16(udp[0:2], 68)
	binary.BigEndian.PutUint16(udp[2:4], 67)
	binary.BigEndian.PutUint16(udp[4:6], uint16(8+len(bootp)))
	udp = append(udp, bootp...)
	ip := make([]byte, 20)
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(20+len(udp)))
	ip[8], ip[9] = 64, 17
	copy(ip[16:20], []byte{255, 255, 255, 255})
	ip = append(ip, udp...)
	eth := make([]byte, 14)
	copy(eth[0:6], Broadcast)
	copy(eth[6:12], chaddr)
	binary.BigEndian.PutUint16(eth[12:14], EtherTypeIPv4)
	return append(eth, ip...)
}

func TestDecodeDHCP(t *testing.T) {
	opts := []byte{
		53, 1, DHCPRequest,
		12, 11, 'A', 'n', 'n', 'a', 's', '-', 'P', 'h', 'o', 'n', 'e',
		60, 14, 'a', 'n', 'd', 'r', 'o', 'i', 'd', '-', 'd', 'h', 'c', 'p', '-', '1',
		55, 4, 1, 3, 6, 15,
		50, 4, 192, 168, 1, 42,
		81, 9, 0x04, 0, 0, 4, 'a', 'n', 'n', 'a', 0,
		0, 0, // padding
		255,
	}
	f := buildDHCPFrame(1, macB, opts)
	_, _, sport, dport, payload, err := UDPPayload(f)
	if err != nil || sport != 68 || dport != 67 {
		t.Fatalf("UDPPayload: %v %d %d", err, sport, dport)
	}
	d, err := DecodeDHCP(payload)
	if err != nil {
		t.Fatal(err)
	}
	if d.MsgType != DHCPRequest || d.Hostname != "Annas-Phone" || d.VendorClass != "android-dhcp-1" {
		t.Fatalf("unexpected: %+v", d)
	}
	if ParamListString(d.ParamList) != "1,3,6,15" || d.RequestedIP != netip.MustParseAddr("192.168.1.42") {
		t.Fatalf("unexpected: %+v", d)
	}
	if d.FQDN != "anna" || !bytes.Equal(d.ClientMAC, macB) {
		t.Fatalf("unexpected: %+v", d)
	}
}

func TestDecodeDHCPTruncatedOption(t *testing.T) {
	f := buildDHCPFrame(1, macB, []byte{53, 1, 1, 12, 200, 'x'})
	_, _, _, _, payload, err := UDPPayload(f)
	if err != nil {
		t.Fatal(err)
	}
	d, err := DecodeDHCP(payload)
	if err != nil {
		t.Fatal(err)
	}
	if d.Hostname != "" {
		t.Fatalf("truncated option must be ignored, got %q", d.Hostname)
	}
}

func TestUDPPayloadRejectsFragments(t *testing.T) {
	f := buildDHCPFrame(1, macB, []byte{53, 1, 1, 255})
	f[14+6] = 0x20 // MF flag
	if _, _, _, _, _, err := UDPPayload(f); err == nil {
		t.Fatal("fragment accepted")
	}
}

func netbiosResponse(txid uint16, names []NetBIOSName) []byte {
	p := binary.BigEndian.AppendUint16(nil, txid)
	p = append(p, 0x84, 0x00, 0, 0, 0, 1, 0, 0, 0, 0)
	p = append(p, 0x20)
	p = append(p, bytes.Repeat([]byte{'C', 'K'}, 1)...)
	p = append(p, bytes.Repeat([]byte{'A'}, 30)...)
	p = append(p, 0)
	p = append(p, 0, 0x21, 0, 1, 0, 0, 0, 0)
	rd := []byte{byte(len(names))}
	for _, n := range names {
		e := make([]byte, 18)
		copy(e, []byte(n.Name + "               ")[:15])
		e[15] = n.Suffix
		if n.Group {
			e[16] = 0x80
		}
		rd = append(rd, e...)
	}
	rd = append(rd, make([]byte, 46)...)
	p = binary.BigEndian.AppendUint16(p, uint16(len(rd)))
	return append(p, rd...)
}

func TestNetBIOS(t *testing.T) {
	q := EncodeNetBIOSStatusQuery(0x1234)
	if len(q) != 50 || q[12] != 0x20 || q[13] != 'C' || q[14] != 'K' {
		t.Fatalf("bad query encoding: % x", q)
	}
	resp := netbiosResponse(0x1234, []NetBIOSName{
		{Name: "WORKGROUP", Suffix: 0, Group: true},
		{Name: "DESKTOP-ANNA", Suffix: 0},
		{Name: "DESKTOP-ANNA", Suffix: 0x20},
	})
	st, err := DecodeNetBIOSStatus(resp, 0x1234)
	if err != nil {
		t.Fatal(err)
	}
	if st.Workstation() != "DESKTOP-ANNA" || st.Workgroup() != "WORKGROUP" {
		t.Fatalf("unexpected names: %+v", st)
	}
	if _, err := DecodeNetBIOSStatus(resp, 0x9999); err == nil {
		t.Error("wrong transaction id accepted")
	}
	if _, err := DecodeNetBIOSStatus(resp[:len(resp)-60], 0x1234); err == nil {
		t.Error("truncated response accepted")
	}
}

func TestSSDP(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=1800\r\nLOCATION: http://192.168.1.30:49152/desc.xml\r\nSERVER: Linux UPnP/1.0 Sonos/80.1\r\nST: upnp:rootdevice\r\nUSN: uuid:RINCON::upnp:rootdevice\r\n\r\n"
	r, err := DecodeSSDPResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if r.Location != "http://192.168.1.30:49152/desc.xml" || r.ST != "upnp:rootdevice" {
		t.Fatalf("unexpected %+v", r)
	}
	from := netip.MustParseAddr("192.168.1.30")
	if _, ok := SafeLocation(r.Location, from); !ok {
		t.Error("valid location rejected")
	}
	for _, bad := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://192.168.1.1:80/admin",
		"https://192.168.1.30/desc.xml",
		"file:///etc/passwd",
		"http://user:pw@192.168.1.30/x",
		"http://evil.example/desc.xml",
	} {
		if _, ok := SafeLocation(bad, from); ok {
			t.Errorf("SafeLocation accepted %q", bad)
		}
	}
}

func TestUPnPDescription(t *testing.T) {
	doc := `<?xml version="1.0"?>
<!DOCTYPE root [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<root xmlns="urn:schemas-upnp-org:device-1-0"><device>
<deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
<friendlyName>Wohnzimmer &xxe;</friendlyName>
<manufacturer>Samsung Electronics</manufacturer><modelName>QE55Q80</modelName>
<deviceList><device><deviceType>urn:dial-multiscreen-org:device:dial:1</deviceType></device></deviceList>
</device></root>`
	d, err := DecodeUPnPDescription([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if d.ModelName != "QE55Q80" || len(d.AllTypes()) != 2 {
		t.Fatalf("unexpected %+v", d)
	}
	if bytes.Contains([]byte(d.FriendlyName), []byte("root:")) {
		t.Fatal("external entity was resolved")
	}
}

func FuzzDecodeARP(f *testing.F) {
	f.Add(EncodeARPRequest(macA, netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2")))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = DecodeARP(b) })
}

func FuzzDecodeDHCPFrame(f *testing.F) {
	f.Add(buildDHCPFrame(1, macB, []byte{53, 1, 3, 12, 3, 'a', 'b', 'c', 81, 5, 4, 0, 0, 1, 'x', 255}))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _, _, p, err := UDPPayload(b)
		if err == nil {
			_, _ = DecodeDHCP(p)
		}
		_, _ = DecodeDHCP(b)
	})
}

func FuzzDecodeNetBIOS(f *testing.F) {
	f.Add(netbiosResponse(7, []NetBIOSName{{Name: "PC", Suffix: 0}}), uint16(7))
	f.Fuzz(func(t *testing.T, b []byte, id uint16) {
		st, err := DecodeNetBIOSStatus(b, id)
		if err == nil {
			_ = st.Workstation()
		}
	})
}

func FuzzDecodeSSDP(f *testing.F) {
	f.Add([]byte("HTTP/1.1 200 OK\r\nLOCATION: http://1.2.3.4/\r\n\r\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		r, err := DecodeSSDPResponse(b)
		if err == nil {
			SafeLocation(r.Location, netip.MustParseAddr("1.2.3.4"))
		}
	})
}

func FuzzDecodeUPnP(f *testing.F) {
	f.Add([]byte(`<root><device><friendlyName>x</friendlyName><deviceList><device/></deviceList></device></root>`))
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := DecodeUPnPDescription(b)
		if err == nil {
			_ = d.AllTypes()
		}
	})
}
