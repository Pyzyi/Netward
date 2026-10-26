package scanner

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/l2"
)

// ProbeDHCP sends one DHCPDISCOVER on every link and returns the offers
// (docs/09, F1b). ok is false in fallback mode, where there are no raw
// sockets to send it with.
func (s *Scanner) ProbeDHCP(ctx context.Context, chaddr net.HardwareAddr, wait time.Duration) (answers []l2.DHCPAnswer, ok bool) {
	if len(s.links) == 0 {
		return nil, false
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, l := range s.links {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := l.ProbeDHCP(ctx, chaddr, wait)
			if err != nil && ctx.Err() == nil {
				s.log.Warn("DHCP probe failed", "iface", name, "err", err)
			}
			mu.Lock()
			answers = append(answers, a...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return answers, true
}
