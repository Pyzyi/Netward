package security

import (
	"context"
	"encoding/json"
	"net/netip"
	"sync"

	"github.com/Pyzyi/netzwart/internal/dnscheck"
	"github.com/Pyzyi/netzwart/internal/egress"
	"github.com/Pyzyi/netzwart/internal/store"
)

// DNS findings (docs/10, F2). The server is the identity of a finding; what
// it answered is shown from the stored check, so a server returning changing
// addresses does not raise a new alarm every run.
const (
	CheckDNSManipulated = "dns_manipulated" // known names resolve to other addresses
	CheckDNSRedirect    = "dns_redirect"    // names that do not exist get an address
)

// SetDNSCheck switches the DNS check on or off (settings).
func (c *Checker) SetDNSCheck(on bool) {
	c.mu.Lock()
	c.dnsCheck = on
	c.mu.Unlock()
}

// checkDNS asks every DNS server of the network the probe questions and
// returns the findings (none when the check is switched off).
func (c *Checker) checkDNS(ctx context.Context, runStart int64) (found []store.Finding) {
	c.mu.Lock()
	on, dial := c.dnsCheck, c.dnsDial
	c.mu.Unlock()
	if !on {
		return nil
	}
	if dial == nil {
		dial = egress.DefaultPolicy.DialContext
	}
	servers := c.mon.DNSServers()
	results := make([]dnscheck.Result, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Add(1)
		go func(i int, s netip.Addr) {
			defer wg.Done()
			results[i] = dnscheck.Check(ctx, netip.AddrPortFrom(s, c.dnsPort()), dnscheck.Dialer(dial))
		}(i, s)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil
	}
	c.mon.SetDNSCheck(ctx, results)
	// A server that did not answer this time proves nothing: its findings
	// stay (refreshed, so no second alarm when it answers again).
	var known []store.Finding
	if fs, err := c.st.ListFindings(ctx); err == nil {
		for _, f := range fs {
			if f.CheckID == CheckDNSManipulated || f.CheckID == CheckDNSRedirect {
				known = append(known, f)
			}
		}
	}
	for _, r := range results {
		if r.Verdict == dnscheck.VerdictUnreachable {
			for _, f := range known {
				var d map[string]string
				if json.Unmarshal(f.Data, &d) == nil && d["server"] == r.Server {
					found = append(found, store.Finding{CheckID: f.CheckID, Severity: f.Severity, Data: f.Data, LastSeen: runStart})
				}
			}
			continue
		}
		data, _ := json.Marshal(map[string]string{"server": r.Server})
		switch r.Verdict {
		case dnscheck.VerdictManipulated:
			found = append(found, store.Finding{CheckID: CheckDNSManipulated, Severity: "critical", Data: data, LastSeen: runStart})
		case dnscheck.VerdictRedirects:
			found = append(found, store.Finding{CheckID: CheckDNSRedirect, Severity: "warning", Data: data, LastSeen: runStart})
		}
	}
	return found
}

func (c *Checker) dnsPort() uint16 {
	if c.dnsTestPort != 0 {
		return c.dnsTestPort
	}
	return 53
}
