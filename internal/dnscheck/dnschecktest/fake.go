// Package dnschecktest provides a tiny DNS server for tests.
package dnschecktest

import (
	"net"
	"net/netip"
	"strings"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

// Honest answers the probe names like the real resolvers do.
var Honest = map[string]string{"one.one.one.one": "1.1.1.1", "dns.google": "8.8.8.8", "dns.quad9.net": "9.9.9.9"}

// Server answers A questions from table; other names get NXDOMAIN, or the
// wildcard address if set (a server that redirects unknown names).
func Server(t testing.TB, table map[string]string, wildcard string) netip.AddrPort {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			var p dnsmessage.Parser
			h, err := p.Start(buf[:n])
			if err != nil {
				continue
			}
			q, err := p.Question()
			if err != nil {
				continue
			}
			name := strings.TrimSuffix(q.Name.String(), ".")
			ip, ok := table[name]
			if !ok && wildcard != "" {
				ip, ok = wildcard, true
			}
			rh := dnsmessage.Header{ID: h.ID, Response: true, RecursionAvailable: true, RecursionDesired: h.RecursionDesired}
			if !ok {
				rh.RCode = dnsmessage.RCodeNameError
			}
			b := dnsmessage.NewBuilder(nil, rh)
			_ = b.StartQuestions()
			_ = b.Question(q)
			_ = b.StartAnswers()
			if ok && q.Type == dnsmessage.TypeA {
				_ = b.AResource(dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: 60},
					dnsmessage.AResource{A: netip.MustParseAddr(ip).As4()})
			}
			msg, _ := b.Finish()
			_, _ = pc.WriteTo(msg, from)
		}
	}()
	return netip.MustParseAddrPort(pc.LocalAddr().String())
}
