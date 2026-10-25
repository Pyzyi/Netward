package scanner

import (
	"context"
	"net"
	"net/netip"
	"testing"
)

func TestMagicPacket(t *testing.T) {
	mac := net.HardwareAddr{1, 2, 3, 4, 5, 6}
	p := MagicPacket(mac)
	if len(p) != 102 {
		t.Fatalf("len %d", len(p))
	}
	for i := 0; i < 6; i++ {
		if p[i] != 0xff {
			t.Fatal("bad sync stream")
		}
	}
	for i := 0; i < 16; i++ {
		if string(p[6+i*6:12+i*6]) != string(mac) {
			t.Fatal("bad repetition")
		}
	}
}

func TestBroadcastAddr(t *testing.T) {
	cases := map[string]string{"192.168.1.0/24": "192.168.1.255", "10.0.0.0/22": "10.0.3.255", "172.16.5.4/30": "172.16.5.7"}
	for in, want := range cases {
		if got := broadcastAddr(netip.MustParsePrefix(in)).String(); got != want {
			t.Errorf("broadcast(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestScanPorts(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	got := ScanPorts(context.Background(), netip.MustParseAddr("127.0.0.1"), []int{port, 1})
	if len(got) != 1 || got[0].Port != port {
		t.Fatalf("got %+v", got)
	}
}
