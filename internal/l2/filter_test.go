package l2

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"

	"github.com/Pyzyi/netzwart/internal/netinfo"
	"github.com/Pyzyi/netzwart/internal/proto"
)

var (
	ourMAC   = net.HardwareAddr{0x02, 0, 0, 0, 0, 1}
	phoneMAC = net.HardwareAddr{0x3c, 0x22, 0xfb, 1, 2, 3}
	testIf   = netinfo.Interface{Name: "eth0", Index: 2, MAC: ourMAC, MACStr: ourMAC.String(),
		Addr: netip.MustParseAddr("192.168.1.2"), Prefix: netip.MustParsePrefix("192.168.1.0/24")}
	quiet = slog.New(slog.NewTextHandler(io.Discard, nil))
)

func udpFrame(src net.HardwareAddr, srcIP [4]byte, sport, dport uint16, payload []byte, fragment bool) []byte {
	f := make([]byte, 14+20+8)
	copy(f[0:6], proto.Broadcast)
	copy(f[6:12], src)
	binary.BigEndian.PutUint16(f[12:14], proto.EtherTypeIPv4)
	ip := f[14:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(20+8+len(payload)))
	if fragment {
		binary.BigEndian.PutUint16(ip[6:8], 0x2000)
	}
	ip[9] = 17
	copy(ip[12:16], srcIP[:])
	udp := ip[20:]
	binary.BigEndian.PutUint16(udp[0:2], sport)
	binary.BigEndian.PutUint16(udp[2:4], dport)
	binary.BigEndian.PutUint16(udp[4:6], uint16(8+len(payload)))
	return append(f, payload...)
}

func TestIPFilter(t *testing.T) {
	prog, err := ipFilter()
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
	src := [4]byte{192, 168, 1, 9}
	cases := []struct {
		name  string
		frame []byte
		want  bool
	}{
		{"dhcp request", udpFrame(phoneMAC, src, 68, 67, make([]byte, 300), false), true},
		{"dhcp reply", udpFrame(phoneMAC, src, 67, 68, make([]byte, 300), false), true},
		{"mdns", udpFrame(phoneMAC, src, 5353, 5353, make([]byte, 40), false), true},
		{"dns", udpFrame(phoneMAC, src, 40000, 53, make([]byte, 40), false), false},
		{"smb-ish tcp", func() []byte { f := udpFrame(phoneMAC, src, 445, 445, nil, false); f[23] = 6; return f }(), false},
		{"fragment", udpFrame(phoneMAC, src, 68, 67, make([]byte, 300), true), false},
		{"arp", proto.EncodeARPRequest(phoneMAC, netip.MustParseAddr("192.168.1.9"), netip.MustParseAddr("192.168.1.1")), false},
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

func TestARPHandle(t *testing.T) {
	out := make(chan Sighting, 8)
	a := NewARP(&Link{iface: testIf}, out, quiet)
	now := time.Now()
	req := proto.EncodeARPRequest(phoneMAC, netip.MustParseAddr("192.168.1.9"), netip.MustParseAddr("192.168.1.1"))
	a.handle(req, now)
	a.handle(req, now.Add(time.Second)) // rate limited
	if len(out) != 1 {
		t.Fatalf("got %d sightings, want 1", len(out))
	}
	s := <-out
	if !s.ARP || s.IP.String() != "192.168.1.9" || s.Source != "arp" {
		t.Fatalf("unexpected sighting %+v", s)
	}
	// ARP probe, foreign subnet and our own address produce nothing.
	a.handle(proto.EncodeARPRequest(net.HardwareAddr{0x3c, 0, 0, 0, 0, 8}, netip.MustParseAddr("0.0.0.0"), netip.MustParseAddr("192.168.1.50")), now)
	a.handle(proto.EncodeARPRequest(net.HardwareAddr{0x3c, 0, 0, 0, 0, 9}, netip.MustParseAddr("10.0.0.5"), netip.MustParseAddr("10.0.0.1")), now)
	if len(out) != 0 {
		t.Fatal("unexpected sighting")
	}
}

func TestARPReplyFeedsSweepAndProbe(t *testing.T) {
	out := make(chan Sighting, 8)
	a := NewARP(&Link{iface: testIf}, out, quiet)
	col := &collector{replies: SweepResult{}}
	a.sweep = col
	ch := make(chan net.HardwareAddr, 1)
	ip := netip.MustParseAddr("192.168.1.30")
	a.probes[ip] = []chan net.HardwareAddr{ch}

	reply := proto.EncodeARPRequest(phoneMAC, ip, testIf.Addr)
	reply[14+7] = proto.ARPReply
	a.handle(reply, time.Now())
	if !col.has(ip) {
		t.Fatal("sweep did not receive the reply")
	}
	select {
	case got := <-ch:
		if got.String() != phoneMAC.String() {
			t.Fatal("wrong MAC delivered to probe")
		}
	default:
		t.Fatal("probe not notified")
	}
	// A second MAC answering for the same IP is recorded (IP conflict).
	other := proto.EncodeARPRequest(net.HardwareAddr{0x3c, 9, 9, 9, 9, 9}, ip, testIf.Addr)
	other[14+7] = proto.ARPReply
	a.handle(other, time.Now())
	if len(col.replies[ip]) != 2 {
		t.Fatalf("got %d MACs for %v, want 2", len(col.replies[ip]), ip)
	}
}

func TestListenerDHCPAndMDNS(t *testing.T) {
	out := make(chan Sighting, 8)
	l := NewListener(&Link{iface: testIf}, out, quiet)
	bootp := make([]byte, 240)
	bootp[0], bootp[1], bootp[2] = 1, 1, 6
	copy(bootp[28:34], phoneMAC)
	binary.BigEndian.PutUint32(bootp[236:240], 0x63825363)
	bootp = append(bootp, 53, 1, proto.DHCPDiscover, 12, 4, 'a', 'n', 'n', 'a', 255)
	l.handle(udpFrame(phoneMAC, [4]byte{}, 68, 67, bootp, false), time.Now())
	// Spoofed: client MAC in BOOTP differs from the frame's source.
	l.handle(udpFrame(net.HardwareAddr{0x3c, 7, 7, 7, 7, 7}, [4]byte{}, 68, 67, bootp, false), time.Now())
	if len(out) != 1 {
		t.Fatalf("got %d sightings, want 1", len(out))
	}
	s := <-out
	if s.DHCP == nil || s.DHCP.Hostname != "anna" || s.IP.IsValid() || s.ARP {
		t.Fatalf("unexpected sighting %+v", s)
	}
}

func TestParseNeighborDump(t *testing.T) {
	msg := func(state uint16, ip [4]byte, mac []byte) []byte {
		body := make([]byte, 12)
		body[0] = unix.AF_INET
		binary.NativeEndian.PutUint32(body[4:8], 2)
		binary.NativeEndian.PutUint16(body[8:10], state)
		attr := func(typ uint16, v []byte) []byte {
			a := make([]byte, 4, 4+len(v)+3)
			binary.NativeEndian.PutUint16(a[0:2], uint16(4+len(v)))
			binary.NativeEndian.PutUint16(a[2:4], typ)
			a = append(a, v...)
			for len(a)%4 != 0 {
				a = append(a, 0)
			}
			return a
		}
		body = append(body, attr(unix.NDA_DST, ip[:])...)
		body = append(body, attr(unix.NDA_LLADDR, mac)...)
		h := make([]byte, unix.NLMSG_HDRLEN)
		binary.NativeEndian.PutUint32(h[0:4], uint32(len(h)+len(body)))
		binary.NativeEndian.PutUint16(h[4:6], unix.RTM_NEWNEIGH)
		return append(h, body...)
	}
	done := make([]byte, unix.NLMSG_HDRLEN+4)
	binary.NativeEndian.PutUint32(done[0:4], uint32(len(done)))
	binary.NativeEndian.PutUint16(done[4:6], unix.NLMSG_DONE)

	b := append(msg(unix.NUD_REACHABLE, [4]byte{192, 168, 1, 7}, phoneMAC), msg(unix.NUD_STALE, [4]byte{192, 168, 1, 8}, phoneMAC)...)
	b = append(b, done...)
	var out []Neighbor
	fin, err := parseNeighborDump(b, &out)
	if err != nil || !fin {
		t.Fatalf("done=%v err=%v", fin, err)
	}
	if len(out) != 2 || !out[0].Reachable() || out[1].Reachable() || out[0].MAC.String() != phoneMAC.String() {
		t.Fatalf("unexpected neighbours %+v", out)
	}
	// Truncated input must not panic.
	var junk []Neighbor
	_, _ = parseNeighborDump(b[:7], &junk)
	_, _ = parseNeighborDump(b[:30], &junk)
}

func FuzzParseNeighborDump(f *testing.F) {
	f.Add([]byte{20, 0, 0, 0, 28, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		var out []Neighbor
		_, _ = parseNeighborDump(b, &out)
	})
}
