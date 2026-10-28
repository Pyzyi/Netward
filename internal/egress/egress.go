// Package egress is the only way Netzwart opens outbound connections to
// user-configured or device-supplied addresses (webhooks, notification
// services, MQTT, SMTP, UPnP descriptions, FRITZ!Box).
//
// The container shares the host's network namespace, so 127.0.0.1 is the
// host's loopback where management services (CasaOS/ZimaOS, possibly the
// Docker API) listen and trust local callers. The dialer therefore refuses
// loopback, unspecified, link-local and multicast destinations. The check
// runs on the resolved address at connect time, so neither DNS tricks nor
// redirects can bypass it.
package egress

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"syscall"
	"time"
)

// ErrBlocked is returned when a destination is not allowed.
var ErrBlocked = errors.New("destination not allowed")

// BlockedError explains why an address was refused.
type BlockedError struct{ Addr netip.Addr }

func (e *BlockedError) Error() string {
	return fmt.Sprintf("connection to %s refused: loopback, link-local and multicast addresses are blocked; use the server's LAN address instead", e.Addr)
}

func (e *BlockedError) Unwrap() error { return ErrBlocked }

// Policy decides which destinations are allowed.
type Policy struct {
	mu   sync.RWMutex
	deny []netip.Addr // additional exact addresses (e.g. our own interface IPs)
}

// cloudMetadata are instance-metadata services outside the link-local range
// (AWS over IPv6, Alibaba Cloud). They only exist on cloud VMs, where they
// hand out credentials.
var cloudMetadata = map[netip.Addr]bool{
	netip.MustParseAddr("fd00:ec2::254"):   true,
	netip.MustParseAddr("100.100.100.200"): true,
}

// DefaultPolicy blocks only the generic dangerous ranges.
var DefaultPolicy = &Policy{}

// SetDenied replaces the list of additional blocked addresses.
func (p *Policy) SetDenied(addrs []netip.Addr) {
	p.mu.Lock()
	p.deny = append([]netip.Addr(nil), addrs...)
	p.mu.Unlock()
}

// Check reports whether a connection to addr is permitted.
func (p *Policy) Check(addr netip.Addr) error {
	a := addr.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsUnspecified() || a.IsLinkLocalUnicast() ||
		a.IsLinkLocalMulticast() || a.IsInterfaceLocalMulticast() || a.IsMulticast() {
		return &BlockedError{Addr: a}
	}
	if a.Is4() && a.As4()[0] == 0 { // 0.0.0.0/8 reaches the local host on Linux
		return &BlockedError{Addr: a}
	}
	if a.Is4() && a == netip.AddrFrom4([4]byte{255, 255, 255, 255}) {
		return &BlockedError{Addr: a}
	}
	if cloudMetadata[a] {
		return &BlockedError{Addr: a}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, d := range p.deny {
		if d == a {
			return &BlockedError{Addr: a}
		}
	}
	return nil
}

// Dialer returns a net.Dialer that enforces the policy at connect time.
func (p *Policy) Dialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return &BlockedError{}
			}
			return p.Check(ap.Addr())
		},
	}
}

// DialContext dials with the policy applied.
func (p *Policy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return p.Dialer(10*time.Second).DialContext(ctx, network, address)
}

// HTTPClient returns a client that enforces the policy for every connection,
// including redirect targets, and follows at most maxRedirects redirects.
func (p *Policy) HTTPClient(timeout time.Duration, maxRedirects int) *http.Client {
	tr := &http.Transport{
		Proxy:                  nil, // never route through an environment proxy implicitly
		DialContext:            p.Dialer(timeout).DialContext,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    timeout,
		ResponseHeaderTimeout:  timeout,
		MaxIdleConns:           4,
		IdleConnTimeout:        30 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   timeout * 2,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}
