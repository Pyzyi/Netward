package scanner

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/Pyzyi/Netward/internal/netinfo"
	"github.com/Pyzyi/Netward/internal/store"
)

// Services maps well-known TCP ports to short service names.
var Services = map[int]string{
	21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp", 53: "dns", 80: "http", 81: "http-alt", 88: "kerberos",
	110: "pop3", 111: "rpcbind", 135: "msrpc", 139: "netbios", 143: "imap", 389: "ldap", 443: "https",
	445: "smb", 515: "lpd", 548: "afp", 554: "rtsp", 587: "submission", 631: "ipp", 873: "rsync",
	993: "imaps", 995: "pop3s", 1080: "socks", 1400: "sonos", 1433: "mssql", 1521: "oracle", 1723: "pptp",
	1883: "mqtt", 2049: "nfs", 2375: "docker", 2376: "docker-tls", 3000: "http-dev", 3306: "mysql",
	3389: "rdp", 3689: "daap", 5000: "upnp/dsm", 5001: "dsm-https", 5060: "sip", 5357: "wsd",
	5432: "postgres", 5555: "adb", 5900: "vnc", 5985: "winrm", 6379: "redis", 6443: "kubernetes",
	7000: "airplay", 7100: "airplay-alt", 8000: "http-alt", 8006: "proxmox", 8008: "cast", 8009: "cast-tls",
	8060: "roku", 8080: "http-proxy", 8081: "http-alt", 8088: "http-alt", 8123: "home-assistant",
	8443: "https-alt", 8554: "rtsp-alt", 8883: "mqtts", 8888: "http-alt", 9000: "http-alt", 9090: "http-alt",
	9100: "jetdirect", 9200: "elasticsearch", 9443: "https-alt", 10000: "webmin", 11211: "memcached",
	27017: "mongodb", 32400: "plex", 49000: "tr-064", 49152: "upnp", 62078: "apple-lockdown",
}

// TopPorts is the list scanned by a device port scan.
func TopPorts() []int {
	out := make([]int, 0, len(Services))
	for p := range Services {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// ErrBusy is returned when a scan for the device is already running.
var ErrBusy = errors.New("scan already running")

var portScanSlots = make(chan struct{}, 2)

// ScanPorts runs a TCP connect scan (no payload is ever sent) against ports.
func ScanPorts(ctx context.Context, ip netip.Addr, ports []int) []store.Port {
	const parallel = 32
	var (
		mu   sync.Mutex
		open []store.Port
		wg   sync.WaitGroup
		sem  = make(chan struct{}, parallel)
	)
	d := net.Dialer{Timeout: 800 * time.Millisecond}
	for _, p := range ports {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			defer func() { <-sem }()
			c, err := d.DialContext(ctx, "tcp4", netip.AddrPortFrom(ip, uint16(p)).String())
			if err != nil {
				return
			}
			c.Close()
			mu.Lock()
			open = append(open, store.Port{Port: p, Proto: "tcp", Service: Services[p]})
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	sort.Slice(open, func(i, j int) bool { return open[i].Port < open[j].Port })
	return open
}

// PortScan scans a device and stores the result.
func (s *Scanner) PortScan(ctx context.Context, id int64) ([]store.Port, error) {
	d, ok := s.mon.Device(id)
	if !ok {
		return nil, store.ErrNotFound
	}
	ip, err := netip.ParseAddr(d.IP)
	if err != nil {
		return nil, store.ErrNotFound
	}
	s.mu.Lock()
	if s.portScans[id] {
		s.mu.Unlock()
		return nil, ErrBusy
	}
	s.portScans[id] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.portScans, id)
		s.mu.Unlock()
	}()
	select {
	case portScanSlots <- struct{}{}:
		defer func() { <-portScanSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// The scan outlives the request (the user may navigate away) but has its
	// own deadline. An incomplete scan is discarded: storing it would delete
	// the ports the device is known to have.
	scanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	s.hub.Publish("portscan", map[string]any{"id": id, "running": true})
	defer s.hub.Publish("portscan", map[string]any{"id": id, "running": false})
	ports := ScanPorts(scanCtx, ip, TopPorts())
	if err := scanCtx.Err(); err != nil {
		return nil, err
	}
	if err := s.mon.SetPorts(scanCtx, id, ports, time.Now().UnixMilli(), "top"); err != nil {
		return nil, err
	}
	return ports, nil
}

// MagicPacket builds a Wake-on-LAN payload.
func MagicPacket(mac net.HardwareAddr) []byte {
	p := make([]byte, 0, 102)
	for i := 0; i < 6; i++ {
		p = append(p, 0xff)
	}
	for i := 0; i < 16; i++ {
		p = append(p, mac...)
	}
	return p
}

// Wake sends a Wake-on-LAN magic packet as UDP broadcast on the device's subnet.
func (s *Scanner) Wake(ctx context.Context, id int64) error {
	d, ok := s.mon.Device(id)
	if !ok {
		return store.ErrNotFound
	}
	mac, err := net.ParseMAC(d.MAC)
	if err != nil || len(mac) != 6 {
		return store.ErrNotFound
	}
	s.mu.Lock()
	ifaces := append([]netinfo.Interface(nil), s.ifaces...)
	s.mu.Unlock()
	payload := MagicPacket(mac)
	sent := false
	for _, i := range ifaces {
		if d.Interface != "" && d.Interface != i.Name {
			continue
		}
		bcast := broadcastAddr(i.Prefix)
		lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
			var serr error
			_ = c.Control(func(fd uintptr) { serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1) })
			return serr
		}}
		pc, err := lc.ListenPacket(ctx, "udp4", netip.AddrPortFrom(i.Addr, 0).String())
		if err != nil {
			continue
		}
		for _, port := range []uint16{9, 7} {
			if _, err := pc.WriteTo(payload, net.UDPAddrFromAddrPort(netip.AddrPortFrom(bcast, port))); err == nil {
				sent = true
			}
		}
		pc.Close()
	}
	if !sent {
		return errors.New("no interface available to send the wake-up packet")
	}
	return nil
}

func broadcastAddr(p netip.Prefix) netip.Addr {
	b := p.Masked().Addr().As4()
	hostBits := 32 - p.Bits()
	v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	v |= (1 << hostBits) - 1
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}
