// Package dnscheck tells whether a DNS server in the home network answers
// honestly (docs/10, F2). It never reads anyone's traffic: it asks the server
// itself about names whose correct answer is fixed and public, and about a
// name that cannot exist.
//
//   - one.one.one.one, dns.google and dns.quad9.net always point to the
//     anycast addresses of those resolvers. Any other address means the
//     server, or something between it and the internet, rewrites answers.
//   - A random name below example.com does not exist. A server that returns
//     an address for it redirects mistyped names (ad pages, or worse).
//
// Blocking is not manipulation: Pi-hole and AdGuard often block the public
// resolvers on purpose and answer with 0.0.0.0 or "does not exist".
package dnscheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"time"
)

// Status of one question.
const (
	StatusOK       = "ok"       // the expected answer
	StatusBlocked  = "blocked"  // a filter refused the name (0.0.0.0, NXDOMAIN)
	StatusWrong    = "wrong"    // another address than the fixed public one
	StatusNoAnswer = "noanswer" // timeout or server failure
)

// Verdict of a server.
const (
	VerdictOK          = "ok"
	VerdictManipulated = "manipulated" // known names resolve to other addresses
	VerdictRedirects   = "redirects"   // names that do not exist get an address
	VerdictUnreachable = "unreachable" // no usable answer at all
)

// Probe is a name with a fixed, public answer.
type Probe struct {
	Name   string
	Expect []netip.Addr
}

func addrs(s ...string) []netip.Addr {
	out := make([]netip.Addr, len(s))
	for i, v := range s {
		out[i] = netip.MustParseAddr(v)
	}
	return out
}

// Probes are the names asked; their operators publish these addresses.
var Probes = []Probe{
	{Name: "one.one.one.one.", Expect: addrs("1.1.1.1", "1.0.0.1")},
	{Name: "dns.google.", Expect: addrs("8.8.8.8", "8.8.4.4")},
	{Name: "dns.quad9.net.", Expect: addrs("9.9.9.9", "149.112.112.112")},
}

// Answer is the result of one question.
type Answer struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Got    []string `json:"got,omitempty"`
}

// Result is the result for one server.
type Result struct {
	Server  string   `json:"server"`
	Verdict string   `json:"verdict"`
	Answers []Answer `json:"answers"`
	// Missing is the answer for a name that does not exist; Got is set
	// when the server invented an address.
	Missing Answer `json:"missing"`
}

// Dialer opens the connection to the server (the egress policy in Netzwart).
type Dialer func(ctx context.Context, network, address string) (net.Conn, error)

// Check asks server the probe questions. port is 53 except in tests.
func Check(ctx context.Context, server netip.AddrPort, dial Dialer) Result {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dial(ctx, network, server.String())
		},
	}
	res := Result{Server: server.Addr().String()}
	ask := func(name string) ([]netip.Addr, string) {
		qctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		ips, err := r.LookupNetIP(qctx, "ip4", name)
		var dnsErr *net.DNSError
		switch {
		case err == nil:
			return ips, ""
		case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
			return nil, StatusBlocked
		default:
			return nil, StatusNoAnswer
		}
	}

	wrong, answered := false, false
	for _, p := range Probes {
		ips, fail := ask(p.Name)
		a := Answer{Name: p.Name[:len(p.Name)-1], Got: toStrings(ips)}
		switch {
		case fail != "":
			a.Status = fail
		case allSinkholed(ips, server.Addr()):
			a.Status = StatusBlocked
		case subset(ips, p.Expect):
			a.Status = StatusOK
		default:
			a.Status = StatusWrong
			wrong = true
		}
		if a.Status != StatusNoAnswer {
			answered = true
		}
		res.Answers = append(res.Answers, a)
	}

	missing := randomLabel() + ".example.com."
	ips, fail := ask(missing)
	res.Missing = Answer{Name: missing[:len(missing)-1], Status: StatusOK}
	switch {
	case fail == StatusNoAnswer:
		res.Missing.Status = StatusNoAnswer
	case fail == "" && !allSinkholed(ips, server.Addr()):
		res.Missing.Status = StatusWrong
		res.Missing.Got = toStrings(ips)
	}
	if res.Missing.Status != StatusNoAnswer {
		answered = true
	}

	switch {
	case wrong:
		res.Verdict = VerdictManipulated
	case res.Missing.Status == StatusWrong:
		res.Verdict = VerdictRedirects
	case !answered:
		res.Verdict = VerdictUnreachable
	default:
		res.Verdict = VerdictOK
	}
	return res
}

// allSinkholed reports answers a blocker gives instead of the real address:
// 0.0.0.0, loopback, or the DNS server's own address (Pi-hole in IP
// blocking mode answers with itself).
func allSinkholed(ips []netip.Addr, server netip.Addr) bool {
	if len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		ip = ip.Unmap()
		if !ip.IsUnspecified() && !ip.IsLoopback() && ip != server.Unmap() {
			return false
		}
	}
	return true
}

func subset(got, want []netip.Addr) bool {
	if len(got) == 0 {
		return false
	}
	for _, g := range got {
		ok := false
		for _, w := range want {
			if g.Unmap() == w {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func toStrings(ips []netip.Addr) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.Unmap().String())
	}
	return out
}

func randomLabel() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "netzwart-" + hex.EncodeToString(b)
}
