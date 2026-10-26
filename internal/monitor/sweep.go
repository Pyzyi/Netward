package monitor

import (
	"context"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/Pyzyi/netzwart/internal/l2"
	"github.com/Pyzyi/netzwart/internal/notify"
)

// proxyThreshold is the number of addresses one MAC must answer for in a
// single sweep before it is treated as a MAC-NAT repeater or proxy-ARP host.
const proxyThreshold = 3

// ApplySweep processes the result of a complete sweep of one interface.
func (m *Monitor) ApplySweep(ctx context.Context, iface string, res l2.SweepResult, at time.Time) {
	m.applySweep(ctx, iface, res, at)
	m.softwareChanged() // re-evaluate findings if the router address changed hands
}

func (m *Monitor) applySweep(ctx context.Context, iface string, res l2.SweepResult, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	perMAC := map[string][]netip.Addr{}
	for ip, macs := range res {
		for _, mac := range macs {
			perMAC[mac.String()] = append(perMAC[mac.String()], ip)
		}
	}
	for mac, ips := range perMAC {
		if len(ips) >= proxyThreshold && !m.proxies[mac] && !m.selfMACs[mac] {
			m.markProxy(ctx, mac, ips)
		}
	}

	ips := make([]netip.Addr, 0, len(res))
	for ip := range res {
		ips = append(ips, ip)
	}
	sort.Slice(ips, func(i, j int) bool { return ips[i].Less(ips[j]) })
	for _, ip := range ips {
		for _, mac := range res[ip] {
			m.observeLocked(ctx, l2.Sighting{MAC: mac, IP: ip, Iface: iface, Source: "sweep", ARP: true, At: at})
		}
	}
	m.detectConflicts(ctx, iface, res)
	m.checkGateways(ctx, res)
	m.lastSweepAt = m.now()
	m.gateOpen = true
	if m.baselineAt == 0 {
		m.baselineAt = at.UnixMilli()
		_ = m.st.SetSetting(ctx, "baseline_at", m.baselineAt)
		m.addEvent(ctx, KindScanBaseline, "info", nil, map[string]any{"count": len(m.byID)})
	}
}

// markProxy flags mac as answering for several addresses. The existing
// record keeps its address; the others become separate "MAC/IP" devices.
func (m *Monitor) markProxy(ctx context.Context, mac string, ips []netip.Addr) {
	m.proxies[mac] = true
	base := m.byIdent[mac]
	if base == nil {
		return
	}
	base.d.Proxy = true
	m.reclassify(base)
	m.persist(ctx, base)
	m.publish(base)
	m.log.Info("device answers ARP for several addresses; treating it as repeater/proxy", "mac", mac, "addresses", len(ips))
}

// detectConflicts reports an IP conflict only when two different MACs answer
// for the same address in two consecutive sweeps. DHCP lease reuse or a
// laptop switching from Wi-Fi to Ethernet produce a single overlap only.
func (m *Monitor) detectConflicts(ctx context.Context, iface string, res l2.SweepResult) {
	current := map[netip.Addr]string{}
	for ip, macs := range res {
		var real []string
		for _, mac := range macs {
			if !m.proxies[mac.String()] {
				real = append(real, mac.String())
			}
		}
		if len(real) < 2 {
			continue
		}
		sort.Strings(real)
		current[ip] = strings.Join(real, ",")
	}
	prev := m.conflicts[iface]
	m.conflicts[iface] = current
	for ip, key := range current {
		if prev[ip] != key {
			continue
		}
		dedup := "conflict/" + ip.String() + "/" + key
		if t, ok := m.reported[dedup]; ok && m.now().Sub(t) < 24*time.Hour {
			continue
		}
		m.reported[dedup] = m.now()
		data := map[string]any{"ip": ip.String(), "macs": strings.Split(key, ",")}
		sev := "warning"
		if m.gateways[ip] {
			// Two devices claiming the router address is how traffic gets
			// redirected (ARP spoofing) – or a misconfigured second router.
			data["gateway"] = true
			sev = "critical"
		}
		dev := m.byIP[ip]
		if dev != nil {
			v := m.view(dev)
			m.addEvent(ctx, notify.KindIPConflict, sev, &v, data)
			m.notify.Notify(notify.Event{Kind: notify.KindIPConflict, Severity: sev, TS: m.nowMS(), Device: &v, Data: data})
		} else {
			m.addEvent(ctx, notify.KindIPConflict, sev, nil, data)
			m.notify.Notify(notify.Event{Kind: notify.KindIPConflict, Severity: sev, TS: m.nowMS(), Data: data})
		}
	}
}

// ObserveSelf records the host Netzwart runs on.
func (m *Monitor) ObserveSelf(ctx context.Context, iface string, ip netip.Addr, mac net.HardwareAddr) {
	m.Observe(ctx, l2.Sighting{MAC: mac, IP: ip, Iface: iface, Source: "self", ARP: true, At: m.now()})
}
