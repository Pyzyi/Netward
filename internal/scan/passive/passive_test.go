package passive

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"testing"

	"golang.org/x/net/bpf"

	"github.com/Pyzyi/netzwart/internal/netinfo"
	"github.com/Pyzyi/netzwart/internal/proto"
)

var (
	ourMAC   = net.HardwareAddr{0x02, 0, 0, 0, 0, 1}
	phoneMAC = net.HardwareAddr{0x3c, 0x22, 0xfb, 1, 2, 3}
	iface    = netinfo.Interface{Name: "eth0", MAC: ourMAC, MACStr: ourMAC.String(),
		Addr: netip.MustParseAddr("192.168.1.2"), Prefix: netip.MustParsePrefix("192.168.1.0/24")}
)

func udpFrame(sport, dport uint16, payload []byte, fragment bool) []byte {
	f := make([]byte, 14+20+8)
	copy(f[0:6], proto.Broadcast)
	copy(f[6:12], phoneMAC)
	binary.BigEndian.PutUint16(f[12:14], proto.EtherTypeIPv4)
	ip := f[14:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(20+8+len(payload)))
	if fragment {
		binary.BigEndian.PutUint16(ip[6:8], 0x0001)
	}
	ip[9] = 17
	udp := ip[20:]
	binary.BigEndian.PutUint16(udp[0:2], sport)
	binary.BigEndian.PutUint16(udp[2:4], dport)
	binary.BigEndian.PutUint16(udp[4:6], uint16(8+len(payload)))
	return append(f, payload...)
}

func TestFilter(t *testing.T) {
	prog, err := filter()
	if err != nil {
		t.Fatal(err)
	}
	insns, ok := bpf.Disassemble(prog)
	if !ok {
		t.Fatal("cannot disassemble")
	}
	vm, err := bpf.NewVM(insns)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		frame []byte
		want  bool
	}{
		{"arp", proto.EncodeARPRequest(phoneMAC, netip.MustParseAddr("192.168.1.9"), netip.MustParseAddr("192.168.1.1")), true},
		{"dhcp request", udpFrame(68, 67, make([]byte, 300), false), true},
		{"dhcp reply", udpFrame(67, 68, make([]byte, 300), false), true},
		{"dns", udpFrame(5353, 53, make([]byte, 40), false), false},
		{"fragment", udpFrame(68, 67, make([]byte, 300), true), false},
		{"ipv6", func() []byte { f := udpFrame(68, 67, nil, false); f[12], f[13] = 0x86, 0xdd; return f }(), false},
	}
	for _, c := range cases {
		n, err := vm.Run(c.frame)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := n > 0; got != c.want {
			t.Errorf("%s: accepted=%v, want %v", c.name, got, c.want)
		}
	}
}

func TestHandleARPAndRateLimit(t *testing.T) {
	out := make(chan Sighting, 4)
	l := New(iface, out, slog.New(slog.NewTextHandler(io.Discard, nil)))
	frame := proto.EncodeARPRequest(phoneMAC, netip.MustParseAddr("192.168.1.9"), netip.MustParseAddr("192.168.1.1"))
	l.handle(frame)
	l.handle(frame) // within arpInterval -> suppressed
	if len(out) != 1 {
		t.Fatalf("got %d sightings, want 1", len(out))
	}
	s := <-out
	if s.IP.String() != "192.168.1.9" || s.MAC.String() != phoneMAC.String() {
		t.Fatalf("unexpected sighting %+v", s)
	}
	// Foreign subnet and probes are ignored.
	l.handle(proto.EncodeARPRequest(net.HardwareAddr{0x3c, 0, 0, 0, 0, 9}, netip.MustParseAddr("10.0.0.5"), netip.MustParseAddr("10.0.0.1")))
	l.handle(proto.EncodeARPRequest(net.HardwareAddr{0x3c, 0, 0, 0, 0, 8}, netip.MustParseAddr("0.0.0.0"), netip.MustParseAddr("192.168.1.50")))
	if len(out) != 0 {
		t.Fatal("foreign subnet or ARP probe produced a sighting")
	}
}

func TestHandleDHCP(t *testing.T) {
	out := make(chan Sighting, 4)
	l := New(iface, out, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bootp := make([]byte, 240)
	bootp[0], bootp[1], bootp[2] = 1, 1, 6
	copy(bootp[28:34], phoneMAC)
	binary.BigEndian.PutUint32(bootp[236:240], 0x63825363)
	bootp = append(bootp, 53, 1, proto.DHCPDiscover, 12, 4, 'a', 'n', 'n', 'a', 255)
	l.handle(udpFrame(68, 67, bootp, false))
	if len(out) != 1 {
		t.Fatalf("got %d sightings, want 1", len(out))
	}
	s := <-out
	if s.DHCP == nil || s.DHCP.Hostname != "anna" || s.IP.IsValid() {
		t.Fatalf("unexpected sighting %+v", s)
	}
}
