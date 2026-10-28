package l2

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/Pyzyi/Netward/internal/proto"
)

// Listener turns DHCP and mDNS traffic on a link into sightings.
type Listener struct {
	link *Link
	out  chan<- Sighting
	log  *slog.Logger
}

// NewListener returns the IPv4 listener for link.
func NewListener(link *Link, out chan<- Sighting, log *slog.Logger) *Listener {
	return &Listener{link: link, out: out, log: log}
}

// Run reads the socket until ctx ends.
func (l *Listener) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = l.link.ip.SetReadDeadline(time.Now())
	}()
	buf := make([]byte, 1600)
	for {
		n, err := l.link.ip.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			return err
		}
		l.handle(buf[:n], time.Now())
	}
}

func (l *Listener) handle(frame []byte, now time.Time) {
	defer func() {
		if r := recover(); r != nil { // parsers are fuzzed; this is the last line of defence
			l.log.Error("l2: recovered from panic while parsing", "panic", r)
		}
	}()
	if len(frame) < 14 {
		return
	}
	iface := l.link.Interface()
	srcMAC := net.HardwareAddr(frame[6:12])
	if string(srcMAC) == string(iface.MAC) || !proto.ValidUnicastMAC(srcMAC) {
		return
	}
	src, _, sport, dport, payload, err := proto.UDPPayload(frame)
	if err != nil {
		return
	}
	switch {
	case (sport == 68 && dport == 67) || (sport == 67 && dport == 68):
		d, err := proto.DecodeDHCP(payload)
		if err != nil || !proto.ValidUnicastMAC(d.ClientMAC) {
			return
		}
		s := Sighting{MAC: d.ClientMAC, Iface: iface.Name, Source: "dhcp", DHCP: &d, At: now}
		if d.Op == 2 && d.MsgType == proto.DHCPOffer && d.ServerID.IsValid() && sport == 67 {
			l.link.deliverProbe(srcMAC, src, d)
		}
		if d.Op == 2 && (d.MsgType == proto.DHCPOffer || d.MsgType == proto.DHCPAck) && d.ServerID.IsValid() && sport == 67 {
			// Who hands out addresses here (F1): reported separately and never
			// turned into a device by itself.
			l.emit(Sighting{MAC: append(net.HardwareAddr(nil), srcMAC...), IP: src, Iface: iface.Name, Source: "dhcp-server", DHCP: &d, At: now})
		}
		if d.Op == 2 {
			// A server ACK proves the lease, not that the client is present yet.
			if d.MsgType != proto.DHCPAck || !iface.Prefix.Contains(d.YourIP) {
				return
			}
			s.IP = d.YourIP
			s.Source = "dhcp-ack"
		} else if string(d.ClientMAC) != string(srcMAC) {
			return // relayed or spoofed: the client MAC must match the frame
		} else if d.ClientIP.IsValid() && !d.ClientIP.IsUnspecified() && iface.Prefix.Contains(d.ClientIP) {
			s.IP = d.ClientIP
		}
		l.emit(s)
	case sport == 5353:
		if !iface.Prefix.Contains(src) || src == iface.Addr {
			return
		}
		info, err := proto.ParseMDNS(payload, src)
		if err != nil || !info.Response {
			return
		}
		l.emit(Sighting{MAC: append(net.HardwareAddr(nil), srcMAC...), IP: src, Iface: iface.Name, Source: "mdns", MDNS: &info, At: now})
	}
}

func (l *Listener) emit(s Sighting) {
	select {
	case l.out <- s:
	default:
	}
}
