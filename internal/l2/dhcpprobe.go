package l2

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/Pyzyi/netzwart/internal/proto"
)

// ErrProbeBusy means a DHCP probe is already running on the link.
var ErrProbeBusy = errors.New("l2: DHCP probe already running")

// maxDHCPAnswers bounds what one probe collects; a real network has one or two.
const maxDHCPAnswers = 16

// DHCPAnswer is one server's offer in reply to a probe.
type DHCPAnswer struct {
	MAC  net.HardwareAddr // sender of the offer
	IP   netip.Addr
	DHCP proto.DHCP
}

type dhcpWaiter struct {
	xid    uint32
	chaddr string
	ch     chan DHCPAnswer
}

// ProbeDHCP broadcasts one DHCPDISCOVER for chaddr and collects the offers
// that carry its transaction ID for the duration of wait (docs/09, F1b). No
// REQUEST follows, so no address is leased. The listener of this link must
// be running, it delivers the answers.
func (l *Link) ProbeDHCP(ctx context.Context, chaddr net.HardwareAddr, wait time.Duration) ([]DHCPAnswer, error) {
	var x [4]byte
	if _, err := rand.Read(x[:]); err != nil {
		return nil, err
	}
	w := &dhcpWaiter{xid: binary.BigEndian.Uint32(x[:]), chaddr: string(chaddr), ch: make(chan DHCPAnswer, maxDHCPAnswers)}
	l.pmu.Lock()
	if l.probe != nil {
		l.pmu.Unlock()
		return nil, ErrProbeBusy
	}
	l.probe = w
	l.pmu.Unlock()
	defer func() {
		l.pmu.Lock()
		l.probe = nil
		l.pmu.Unlock()
	}()

	if _, err := l.ip.Write(proto.DHCPDiscoverFrame(l.Interface().MAC, chaddr, w.xid)); err != nil {
		return nil, err
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	var out []DHCPAnswer
	seen := map[netip.Addr]bool{}
	for {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-timer.C:
			return out, nil
		case a := <-w.ch:
			if !seen[a.DHCP.ServerID] && len(out) < maxDHCPAnswers {
				seen[a.DHCP.ServerID] = true
				out = append(out, a)
			}
		}
	}
}

// deliverProbe hands an offer to the probe in flight if it answers it.
func (l *Link) deliverProbe(mac net.HardwareAddr, ip netip.Addr, d proto.DHCP) {
	l.pmu.Lock()
	w := l.probe
	l.pmu.Unlock()
	if w == nil || d.Xid != w.xid || string(d.ClientMAC) != w.chaddr {
		return
	}
	select {
	case w.ch <- DHCPAnswer{MAC: append(net.HardwareAddr(nil), mac...), IP: ip, DHCP: d}:
	default:
	}
}
