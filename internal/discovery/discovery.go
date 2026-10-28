// Package discovery asks devices what they are called and what they offer:
// reverse DNS, NetBIOS node status, mDNS/DNS-SD and SSDP/UPnP.
//
// All queries are sent from ephemeral ports so Netward never has to bind
// 137, 1900 or 5353 and cannot conflict with Avahi or Samba on the host.
package discovery

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"

	"github.com/Pyzyi/Netward/internal/proto"
	"github.com/Pyzyi/Netward/internal/textutil"
)

// Ports are variables so tests can run fake responders on unprivileged ports.
var (
	netbiosPort uint16 = 137
	mdnsPort    uint16 = 5353
)

// ReverseDNS returns the PTR name of ip using the host's resolver.
func ReverseDNS(ctx context.Context, ip netip.Addr) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip.String())
	if err != nil || len(names) == 0 {
		return ""
	}
	return textutil.Hostname(names[0])
}

func udpExchange(ctx context.Context, dst netip.AddrPort, req []byte, wait time.Duration, handle func([]byte, netip.Addr) bool) error {
	c, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return err
	}
	defer c.Close()
	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = c.SetDeadline(deadline)
	if _, err := c.WriteToUDPAddrPort(req, dst); err != nil {
		return err
	}
	buf := make([]byte, 9000)
	for {
		n, from, err := c.ReadFromUDPAddrPort(buf)
		if err != nil {
			return nil // deadline reached
		}
		if from.Addr().Unmap() != dst.Addr() {
			continue
		}
		if handle(buf[:n], from.Addr().Unmap()) {
			return nil
		}
	}
}

// NetBIOS asks ip for its NetBIOS names (Windows, Samba, many NAS).
func NetBIOS(ctx context.Context, ip netip.Addr) (name, workgroup string) {
	txid := uint16(rand.Uint32())
	_ = udpExchange(ctx, netip.AddrPortFrom(ip, netbiosPort), proto.EncodeNetBIOSStatusQuery(txid), 1500*time.Millisecond,
		func(b []byte, _ netip.Addr) bool {
			st, err := proto.DecodeNetBIOSStatus(b, txid)
			if err != nil {
				return false
			}
			name, workgroup = st.Workstation(), st.Workgroup()
			return true
		})
	return textutil.Clean(name, 64), textutil.Clean(workgroup, 64)
}

// MDNSUnicast sends a direct query to ip:5353 for its reverse name and the
// services it offers. Apple devices and Avahi hosts answer this.
func MDNSUnicast(ctx context.Context, ip netip.Addr) proto.MDNSInfo {
	var merged proto.MDNSInfo
	q, err := proto.BuildMDNSQuery(uint16(rand.Uint32()), dnsmessage.TypePTR, false,
		proto.ReverseName(ip), "_services._dns-sd._udp.local.", "_device-info._tcp.local.")
	if err != nil {
		return merged
	}
	_ = udpExchange(ctx, netip.AddrPortFrom(ip, mdnsPort), q, 1500*time.Millisecond, func(b []byte, from netip.Addr) bool {
		info, err := proto.ParseMDNS(b, from)
		if err == nil {
			Merge(&merged, info)
		}
		return false
	})
	return merged
}

// Merge folds b into a.
func Merge(a *proto.MDNSInfo, b proto.MDNSInfo) {
	if b.Hostname != "" {
		a.Hostname = b.Hostname
	}
	if b.Model != "" && a.Model == "" {
		a.Model = b.Model
	}
	if b.FriendlyName != "" && a.FriendlyName == "" {
		a.FriendlyName = b.FriendlyName
	}
	if b.HomeKitCat != "" && a.HomeKitCat == "" {
		a.HomeKitCat = b.HomeKitCat
	}
	for _, s := range b.ServiceTypes {
		if !contains(a.ServiceTypes, s) && len(a.ServiceTypes) < 64 {
			a.ServiceTypes = append(a.ServiceTypes, s)
		}
	}
	for _, in := range b.Instances {
		dup := false
		for _, x := range a.Instances {
			if x.Service == in.Service && x.Instance == in.Instance {
				dup = true
				break
			}
		}
		if !dup && len(a.Instances) < 64 {
			a.Instances = append(a.Instances, in)
		}
	}
	for _, t := range b.TXT {
		replaced := false
		for i := range a.TXT {
			if a.TXT[i].Service == t.Service {
				a.TXT[i], replaced = t, true
			}
		}
		if !replaced && len(a.TXT) < 8 {
			a.TXT = append(a.TXT, t)
		}
	}
	a.Response = a.Response || b.Response
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// multicastConn returns a UDP socket that sends multicast out of ifi.
func multicastConn(ifi *net.Interface, ttl int) (*net.UDPConn, error) {
	c, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}
	pc := ipv4.NewPacketConn(c)
	if err := pc.SetMulticastInterface(ifi); err != nil {
		c.Close()
		return nil, err
	}
	_ = pc.SetMulticastTTL(ttl)
	_ = pc.SetMulticastLoopback(false)
	return c, nil
}

func collect(c *net.UDPConn, until time.Time, prefix netip.Prefix, fn func([]byte, netip.Addr)) {
	_ = c.SetReadDeadline(until)
	buf := make([]byte, 9000)
	for {
		n, from, err := c.ReadFromUDPAddrPort(buf)
		if err != nil {
			return
		}
		src := from.Addr().Unmap()
		if prefix.Contains(src) {
			fn(buf[:n], src)
		}
	}
}

var mdnsGroup = netip.AddrPortFrom(netip.AddrFrom4([4]byte{224, 0, 0, 251}), 5353)

// MDNSBrowse discovers DNS-SD services on the segment of ifi. Queries come
// from an ephemeral port, so responders answer by unicast ("legacy unicast").
func MDNSBrowse(ctx context.Context, ifi *net.Interface, prefix netip.Prefix) map[netip.Addr]*proto.MDNSInfo {
	out := map[netip.Addr]*proto.MDNSInfo{}
	c, err := multicastConn(ifi, 255)
	if err != nil {
		return out
	}
	defer c.Close()
	add := func(b []byte, src netip.Addr) {
		info, err := proto.ParseMDNS(b, src)
		if err != nil || !info.Response {
			return
		}
		m := out[src]
		if m == nil {
			if len(out) >= 1024 {
				return
			}
			m = &proto.MDNSInfo{}
			out[src] = m
		}
		Merge(m, info)
	}
	q, _ := proto.BuildMDNSQuery(0, dnsmessage.TypePTR, false, "_services._dns-sd._udp.local.")
	_, _ = c.WriteToUDPAddrPort(q, mdnsGroup)
	collect(c, minTime(ctx, time.Now().Add(2*time.Second)), prefix, add)

	types := map[string]bool{}
	for _, m := range out {
		for _, s := range m.ServiceTypes {
			types[s] = true
		}
	}
	// Commonly announced types that some devices do not list under _services.
	for _, s := range []string{"_airplay._tcp", "_googlecast._tcp", "_ipp._tcp", "_printer._tcp", "_hap._tcp",
		"_raop._tcp", "_spotify-connect._tcp", "_smb._tcp", "_device-info._tcp", "_companion-link._tcp",
		"_sonos._tcp", "_hue._tcp", "_esphomelib._tcp", "_shelly._tcp", "_home-assistant._tcp", "_amzn-wplay._tcp",
		"_http._tcp"} {
		types[s] = true
	}
	var names []string
	for s := range types {
		if len(names) >= 40 {
			break
		}
		names = append(names, s+".local.")
	}
	// Several smaller queries keep each packet well below the MTU.
	for i := 0; i < len(names); i += 10 {
		j := min(i+10, len(names))
		q, err := proto.BuildMDNSQuery(0, dnsmessage.TypePTR, false, names[i:j]...)
		if err == nil {
			_, _ = c.WriteToUDPAddrPort(q, mdnsGroup)
		}
	}
	collect(c, minTime(ctx, time.Now().Add(3*time.Second)), prefix, add)
	return out
}

var ssdpGroup = netip.AddrPortFrom(netip.AddrFrom4([4]byte{239, 255, 255, 250}), 1900)

// SSDPSearch sends M-SEARCH on ifi and returns the responses per sender.
func SSDPSearch(ctx context.Context, ifi *net.Interface, prefix netip.Prefix) map[netip.Addr][]proto.SSDPResponse {
	out := map[netip.Addr][]proto.SSDPResponse{}
	c, err := multicastConn(ifi, 2)
	if err != nil {
		return out
	}
	defer c.Close()
	_, _ = c.WriteToUDPAddrPort(proto.SSDPSearch, ssdpGroup)
	time.AfterFunc(500*time.Millisecond, func() { _, _ = c.WriteToUDPAddrPort(proto.SSDPSearch, ssdpGroup) })
	collect(c, minTime(ctx, time.Now().Add(3500*time.Millisecond)), prefix, func(b []byte, src netip.Addr) {
		r, err := proto.DecodeSSDPResponse(b)
		if err != nil || len(out[src]) >= 16 {
			return
		}
		if len(out) >= 1024 && out[src] == nil {
			return
		}
		out[src] = append(out[src], r)
	})
	return out
}

func minTime(ctx context.Context, t time.Time) time.Time {
	if d, ok := ctx.Deadline(); ok && d.Before(t) {
		return d
	}
	return t
}

// FetchDescription downloads and parses a UPnP device description. The URL
// must have passed proto.SafeLocation; client must be an egress client.
func FetchDescription(ctx context.Context, client *http.Client, u *url.URL) (proto.UPnPDevice, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return proto.UPnPDevice{}, err
	}
	req.Header.Set("User-Agent", "Netward/1 UPnP/1.1")
	resp, err := client.Do(req)
	if err != nil {
		return proto.UPnPDevice{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return proto.UPnPDevice{}, errors.New(resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, proto.MaxDescription+1))
	if err != nil {
		return proto.UPnPDevice{}, err
	}
	return proto.DecodeUPnPDescription(b)
}

// TCPAlive reports whether ip responds on port at all: an accepted
// connection or an immediate refusal (RST) both prove the host is up.
func TCPAlive(ctx context.Context, ip netip.Addr, port uint16, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	c, err := d.DialContext(ctx, "tcp4", netip.AddrPortFrom(ip, port).String())
	if err == nil {
		c.Close()
		return true
	}
	return errors.Is(err, syscall.ECONNREFUSED)
}
