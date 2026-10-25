package netinfo

import (
	"net/netip"
	"strings"
	"testing"
)

func TestHosts(t *testing.T) {
	h := Hosts(netip.MustParsePrefix("192.168.1.0/24"))
	if len(h) != 254 || h[0].String() != "192.168.1.1" || h[253].String() != "192.168.1.254" {
		t.Fatalf("unexpected /24 hosts: %d %v..%v", len(h), h[0], h[len(h)-1])
	}
	h = Hosts(netip.MustParsePrefix("10.0.0.0/30"))
	if len(h) != 2 || h[0].String() != "10.0.0.1" || h[1].String() != "10.0.0.2" {
		t.Fatalf("unexpected /30 hosts: %v", h)
	}
	if HostCount(netip.MustParsePrefix("10.0.0.0/22")) != 1022 {
		t.Fatal("bad /22 host count")
	}
	if Hosts(netip.MustParsePrefix("10.0.0.0/8")) != nil {
		t.Fatal("huge prefix must not be enumerated")
	}
}

func TestParseRoutes(t *testing.T) {
	in := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0101A8C0	0003	0	0	100	00000000	0	0	0
eth0	0001A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
`
	r, err := parseRoutes(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(r) != 1 || r[0].Interface != "eth0" || r[0].Gateway.String() != "192.168.1.1" {
		t.Fatalf("unexpected routes %+v", r)
	}
}

func TestParseARPCache(t *testing.T) {
	in := `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.1      0x1         0x2         3c:a6:2f:11:22:33     *        eth0
192.168.1.77     0x1         0x0         00:00:00:00:00:00     *        eth0
192.168.1.20     0x1         0x2         b8:27:eb:aa:bb:cc     *        eth0
`
	n, err := parseARPCache(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(n) != 2 || n[1].MAC.String() != "b8:27:eb:aa:bb:cc" {
		t.Fatalf("unexpected neighbors %+v", n)
	}
}

func TestParseResolvConf(t *testing.T) {
	got := parseResolvConf(strings.NewReader("# comment\nnameserver 192.168.1.1\nnameserver fe80::1%eth0\nsearch lan\n"))
	if len(got) != 2 || got[0].String() != "192.168.1.1" {
		t.Fatalf("unexpected %v", got)
	}
}

func TestIsVirtual(t *testing.T) {
	for _, n := range []string{"docker0", "veth12ab", "br-1234", "tailscale0", "lo"} {
		if !IsVirtual(n) {
			t.Errorf("%s should be virtual", n)
		}
	}
	for _, n := range []string{"eth0", "enp3s0", "wlan0", "bond0"} {
		if IsVirtual(n) {
			t.Errorf("%s should not be virtual", n)
		}
	}
}
