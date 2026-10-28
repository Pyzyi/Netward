package discovery

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/Pyzyi/Netward/internal/proto"
)

// fakeResponder answers every datagram with reply(request).
func fakeResponder(t *testing.T, reply func([]byte) []byte) uint16 {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := c.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if r := reply(buf[:n]); r != nil {
				_, _ = c.WriteToUDP(r, from)
			}
		}
	}()
	return uint16(c.LocalAddr().(*net.UDPAddr).Port)
}

func TestNetBIOS(t *testing.T) {
	netbiosPort = fakeResponder(t, func(req []byte) []byte {
		txid := binary.BigEndian.Uint16(req[0:2])
		p := binary.BigEndian.AppendUint16(nil, txid)
		p = append(p, 0x84, 0, 0, 0, 0, 1, 0, 0, 0, 0)
		p = append(p, req[12:12+34]...)
		p = append(p, 0, 0x21, 0, 1, 0, 0, 0, 0)
		names := []byte{2}
		e := []byte("NAS-KELLER     \x00\x04\x00")
		g := []byte("WORKGROUP      \x00\x84\x00")
		names = append(append(names, e...), g...)
		names = append(names, make([]byte, 46)...)
		p = binary.BigEndian.AppendUint16(p, uint16(len(names)))
		return append(p, names...)
	})
	name, wg := NetBIOS(context.Background(), netip.MustParseAddr("127.0.0.1"))
	if name != "NAS-KELLER" || wg != "WORKGROUP" {
		t.Fatalf("got %q %q", name, wg)
	}
}

func TestMDNSUnicast(t *testing.T) {
	ip := netip.MustParseAddr("127.0.0.1")
	mdnsPort = fakeResponder(t, func(req []byte) []byte {
		var p dnsmessage.Parser
		h, err := p.Start(req)
		if err != nil {
			return nil
		}
		b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: h.ID, Response: true})
		_ = b.StartAnswers()
		_ = b.PTRResource(dnsmessage.ResourceHeader{Name: dnsmessage.MustNewName(proto.ReverseName(ip)), Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET},
			dnsmessage.PTRResource{PTR: dnsmessage.MustNewName("Annas-MacBook-Air.local.")})
		out, _ := b.Finish()
		return out
	})
	info := MDNSUnicast(context.Background(), ip)
	if info.Hostname != "Annas-MacBook-Air" {
		t.Fatalf("got %+v", info)
	}
}

func TestTCPAlive(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	if !TCPAlive(context.Background(), netip.MustParseAddr("127.0.0.1"), port, time.Second) {
		t.Fatal("open port not detected")
	}
	ln.Close()
	// A refused connection still proves the host is alive.
	if !TCPAlive(context.Background(), netip.MustParseAddr("127.0.0.1"), port, time.Second) {
		t.Fatal("RST not treated as alive")
	}
}
