// Package netinfo inspects the host's network configuration: usable
// interfaces and their IPv4 subnets, the default gateway, DNS servers and
// the kernel ARP cache. It only reads; it never changes anything.
package netinfo

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"sort"
	"strings"
)

// Interface is a network interface Netzwart can scan.
type Interface struct {
	Name   string           `json:"name"`
	Index  int              `json:"-"`
	MAC    net.HardwareAddr `json:"-"`
	MACStr string           `json:"mac"`
	Addr   netip.Addr       `json:"addr"`   // our IPv4 address on this interface
	Prefix netip.Prefix     `json:"prefix"` // masked subnet
	Hosts  int              `json:"hosts"`  // number of scannable host addresses
}

// virtualPrefixes are interface name prefixes that never lead to the LAN.
var virtualPrefixes = []string{
	"lo", "docker", "veth", "br-", "virbr", "vnet", "tun", "tap", "wg", "zt",
	"tailscale", "cni", "flannel", "cali", "kube", "lxc", "lxd", "podman", "dummy",
	"ip6tnl", "sit", "gre", "ifb", "nlmon", "vmnet", "vboxnet", "cilium", "weave",
	"hassio", "ham",
}

// IsVirtual reports whether name looks like a container, VPN or loopback interface.
func IsVirtual(name string) bool {
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// HostCount returns the number of usable host addresses in p (IPv4).
func HostCount(p netip.Prefix) int {
	bits := 32 - p.Bits()
	switch {
	case bits <= 0:
		return 1
	case bits == 1:
		return 2
	case bits >= 31:
		return 1 << 30
	}
	return (1 << bits) - 2
}

// Interfaces lists up, broadcast-capable Ethernet-like interfaces with an
// IPv4 address. If include is non-empty only those names are returned and
// the virtual-interface filter is skipped.
func Interfaces(include []string) ([]Interface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range include {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}
	var out []Interface
	for _, ifi := range ifs {
		if len(want) > 0 && !want[ifi.Name] {
			continue
		}
		if len(want) == 0 && IsVirtual(ifi.Name) {
			continue
		}
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagBroadcast == 0 {
			continue
		}
		if len(ifi.HardwareAddr) != 6 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipn.IP)
			if !ok {
				continue
			}
			ip = ip.Unmap()
			if !ip.Is4() || ip.IsLinkLocalUnicast() || ip.IsLoopback() {
				continue
			}
			ones, _ := ipn.Mask.Size()
			p := netip.PrefixFrom(ip, ones).Masked()
			out = append(out, Interface{
				Name: ifi.Name, Index: ifi.Index, MAC: ifi.HardwareAddr, MACStr: ifi.HardwareAddr.String(),
				Addr: ip, Prefix: p, Hosts: HostCount(p),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Hosts enumerates the host addresses of p, skipping the network and
// broadcast addresses for prefixes shorter than /31.
func Hosts(p netip.Prefix) []netip.Addr {
	p = p.Masked()
	if !p.Addr().Is4() {
		return nil
	}
	n := HostCount(p)
	if n > 1<<16 {
		return nil
	}
	base := binary.BigEndian.Uint32(p.Addr().AsSlice())
	out := make([]netip.Addr, 0, n)
	start, end := uint32(1), uint32(n)
	if p.Bits() >= 31 {
		start, end = 0, uint32(n)-1
	}
	for i := start; i <= end; i++ {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], base+i)
		out = append(out, netip.AddrFrom4(b))
	}
	return out
}

// Route is a default route from /proc/net/route.
type Route struct {
	Interface string
	Gateway   netip.Addr
}

// DefaultGateways parses the kernel routing table for default routes.
func DefaultGateways() ([]Route, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseRoutes(f)
}

func parseRoutes(r io.Reader) ([]Route, error) {
	var out []Route
	sc := bufio.NewScanner(r)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		f := strings.Fields(sc.Text())
		if len(f) < 8 || f[1] != "00000000" || f[7] != "00000000" {
			continue
		}
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 4 {
			continue
		}
		// /proc/net/route stores addresses in host (little-endian) order.
		gw := netip.AddrFrom4([4]byte{b[3], b[2], b[1], b[0]})
		if gw.IsUnspecified() {
			continue
		}
		out = append(out, Route{Interface: f[0], Gateway: gw})
	}
	return out, sc.Err()
}

// DNSServers reads nameservers from /etc/resolv.conf.
func DNSServers() []netip.Addr {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	defer f.Close()
	return parseResolvConf(f)
}

func parseResolvConf(r io.Reader) []netip.Addr {
	var out []netip.Addr
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 2 && f[0] == "nameserver" {
			if a, err := netip.ParseAddr(f[1]); err == nil {
				out = append(out, a)
			}
		}
	}
	return out
}

var errFormat = errors.New("unexpected /proc/net/arp format")

// IsOwnAddr reports whether ip belongs to an interface of this host.
func IsOwnAddr(ip netip.Addr) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if x, ok := netip.AddrFromSlice(n.IP); ok && x.Unmap() == ip {
				return true
			}
		}
	}
	return false
}

// OnLink reports whether ip lies in a network one of the host's interfaces
// is part of (e.g. the global IPv6 prefix of the home network).
func OnLink(ip netip.Addr) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP.IsLoopback() {
			continue
		}
		x, ok := netip.AddrFromSlice(n.IP)
		ones, _ := n.Mask.Size()
		if ok && netip.PrefixFrom(x.Unmap(), ones).Masked().Contains(ip) {
			return true
		}
	}
	return false
}
