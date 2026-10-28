// Package security runs the network security check: services that should
// not be reachable, UPnP port forwarding on the router, IP conflicts and
// devices nobody has confirmed. The port check only opens TCP connections and
// closes them again. Two software probes go further (docs/08): the SSH
// greeting is read without sending anything, and Windows is asked for its
// version through the first step of an SMB logon that is never completed.
package security

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/Pyzyi/Netward/internal/hub"
	"github.com/Pyzyi/Netward/internal/l2"
	"github.com/Pyzyi/Netward/internal/monitor"
	"github.com/Pyzyi/Netward/internal/notify"
	"github.com/Pyzyi/Netward/internal/scanner"
	"github.com/Pyzyi/Netward/internal/software"
	"github.com/Pyzyi/Netward/internal/store"
)

// Check IDs (the UI has texts for each).
const (
	CheckRiskyPort = "risky_port"
	CheckUPnP      = "upnp_igd"
	CheckConflict  = "ip_conflict"
	CheckUnknown   = "unknown_devices"
)

type risk struct {
	severity string
	service  string
}

// riskyPorts are services that are dangerous when reachable on a home network.
var riskyPorts = map[int]risk{
	23:    {"critical", "telnet"},
	2375:  {"critical", "docker"},
	5555:  {"critical", "adb"},
	21:    {"warning", "ftp"},
	5900:  {"warning", "vnc"},
	3389:  {"warning", "rdp"},
	6379:  {"warning", "redis"},
	9200:  {"warning", "elasticsearch"},
	27017: {"warning", "mongodb"},
	11211: {"warning", "memcached"},
	1883:  {"info", "mqtt"},
	445:   {"info", "smb"},
	139:   {"info", "netbios"},
}

const (
	sshPort = 22
	smbPort = 445
)

// smbProbeAllowed skips devices that are known not to run Windows. Their SMB
// servers (Samba on NAS and Linux, macOS file sharing) answer with
// placeholder versions; software.FromNTLMVersion filters those too, this just
// avoids the round trip.
func smbProbeAllowed(d store.Device) bool {
	vendor := strings.ToLower(d.Vendor + " " + d.VendorOfficial)
	for _, v := range []string{"apple", "synology", "qnap", "avm", "raspberry", "western digital", "asustor", "buffalo", "netgear"} {
		if strings.Contains(vendor, v) {
			return false
		}
	}
	switch d.Type {
	case "nas", "router", "access_point", "phone", "tablet", "tv", "printer":
		return false
	}
	switch d.OS {
	case "iOS", "iPadOS", "macOS", "Android", "Linux":
		return false
	}
	return true
}

// Status describes the last run.
type Status struct {
	Running   bool  `json:"running"`
	LastRunAt int64 `json:"lastRunAt"`
	Duration  int64 `json:"duration"`
}

// Checker runs the security check.
type Checker struct {
	st       *store.Store
	mon      *monitor.Monitor
	hub      *hub.Hub
	notifier notify.Notifier
	log      *slog.Logger

	mu        sync.Mutex
	status    Status
	swTrigger chan struct{}
	swLastRun int64
	prober    DHCPProber // nil without raw sockets or in demo mode
	dhcpCheck bool
	dnsCheck  bool
	// Tests only: how DNS servers are reached.
	dnsDial     func(ctx context.Context, network, address string) (net.Conn, error)
	dnsTestPort uint16
}

// DHCPProber sends the active DHCP check (implemented by the scanner).
type DHCPProber interface {
	ProbeDHCP(ctx context.Context, chaddr net.HardwareAddr, wait time.Duration) ([]l2.DHCPAnswer, bool)
}

// dhcpProbeWait is how long offers are collected; servers answer within
// milliseconds, slow ones (mesh repeaters) within a second or two.
const dhcpProbeWait = 3 * time.Second

// SetDHCPProber enables the active DHCP check.
func (c *Checker) SetDHCPProber(p DHCPProber) {
	c.mu.Lock()
	c.prober = p
	c.mu.Unlock()
}

// SetDHCPCheck switches the active DHCP check on or off (settings).
func (c *Checker) SetDHCPCheck(on bool) {
	c.mu.Lock()
	c.dhcpCheck = on
	c.mu.Unlock()
}

// probeDHCP runs the active DHCP check if it is enabled.
func (c *Checker) probeDHCP(ctx context.Context) {
	c.mu.Lock()
	p, on := c.prober, c.dhcpCheck
	c.mu.Unlock()
	if p == nil || !on {
		return
	}
	answers, ok := p.ProbeDHCP(ctx, c.mon.DHCPProbeMAC(), dhcpProbeWait)
	if !ok || ctx.Err() != nil {
		return
	}
	c.log.Info("DHCP check finished", "servers", len(answers))
	c.mon.ObserveDHCPProbe(ctx, answers)
}

// New creates a checker.
func New(st *store.Store, mon *monitor.Monitor, h *hub.Hub, n notify.Notifier, log *slog.Logger) *Checker {
	c := &Checker{st: st, mon: mon, hub: h, notifier: n, log: log, swTrigger: make(chan struct{}, 1)}
	var last int64
	if err := st.GetSetting(context.Background(), "security_last_run", &last); err == nil {
		c.status.LastRunAt = last
	}
	return c
}

// Status returns the state of the checker.
func (c *Checker) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// RunAsync starts a check unless one is running; it reports whether it started.
func (c *Checker) RunAsync(ctx context.Context) bool {
	c.mu.Lock()
	if c.status.Running {
		c.mu.Unlock()
		return false
	}
	c.status.Running = true
	c.mu.Unlock()
	c.hub.Publish("security", c.Status())
	go c.run(context.WithoutCancel(ctx))
	return true
}

// Schedule runs the check every days days (0 disables) until ctx ends.
func (c *Checker) Schedule(ctx context.Context, days func() int) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		d := days()
		if d <= 0 {
			continue
		}
		if time.Since(time.UnixMilli(c.Status().LastRunAt)) >= time.Duration(d)*24*time.Hour {
			c.RunAsync(ctx)
		}
	}
}

func (c *Checker) run(ctx context.Context) {
	start := time.Now()
	runStart := start.UnixMilli()
	defer func() {
		c.mu.Lock()
		c.status = Status{Running: false, LastRunAt: time.Now().UnixMilli(), Duration: time.Since(start).Milliseconds()}
		st := c.status
		c.mu.Unlock()
		_ = c.st.SetSetting(ctx, "security_last_run", st.LastRunAt)
		c.hub.Publish("security", st)
	}()
	devs := c.mon.Devices()
	ports := make([]int, 0, len(riskyPorts)+1)
	for p := range riskyPorts {
		ports = append(ports, p)
	}
	ports = append(ports, sshPort) // not risky by itself; read for the software check

	var mu sync.Mutex
	var findings []store.Finding
	add := func(f store.Finding) {
		mu.Lock()
		findings = append(findings, f)
		mu.Unlock()
	}
	var wg sync.WaitGroup
	// Who hands out addresses (F1b): one DISCOVER, answers collected while
	// the ports are scanned. The findings follow from the network evaluation.
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.probeDHCP(ctx)
	}()
	// Whether the network's DNS servers answer honestly (docs/10, F2).
	var dnsFound []store.Finding
	wg.Add(1)
	go func() {
		defer wg.Done()
		dnsFound = c.checkDNS(ctx, runStart)
	}()
	sem := make(chan struct{}, 4)
	var checked []int64
	for _, d := range devs {
		if d.IsGateway {
			checked = append(checked, d.ID) // UPnP finding is decided from stored details
		}
		if !d.Online || d.IsSelf {
			continue
		}
		ip, err := netip.ParseAddr(d.IP)
		if err != nil {
			continue
		}
		if !d.IsGateway {
			checked = append(checked, d.ID)
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d store.Device, ip netip.Addr) {
			defer wg.Done()
			defer func() { <-sem }()
			var obs []software.Observation
			for _, p := range scanner.ScanPorts(ctx, ip, ports) {
				switch {
				case p.Port == sshPort:
					obs = append(obs, software.ProbeSSH(ctx, ip)...)
					continue
				case p.Port == smbPort && smbProbeAllowed(d):
					obs = append(obs, software.ProbeSMB(ctx, ip)...)
				}
				r := riskyPorts[p.Port]
				data, _ := json.Marshal(map[string]any{"port": p.Port, "service": r.service})
				add(store.Finding{CheckID: CheckRiskyPort, DeviceID: d.ID, Severity: r.severity, Data: data, LastSeen: runStart})
			}
			c.mon.AddSoftware(d.ID, obs)
		}(d, ip)
	}
	wg.Wait()
	for _, f := range dnsFound {
		add(f)
	}

	unknown := 0
	for _, d := range devs {
		if !d.Known && !d.IsSelf {
			unknown++
		}
		if det, ok := c.mon.DeviceDetails(d.ID); ok && d.IsGateway {
			for _, t := range det.UPnPTypes {
				if strings.Contains(strings.ToLower(t), "internetgatewaydevice") {
					add(store.Finding{CheckID: CheckUPnP, DeviceID: d.ID, Severity: "warning", Data: json.RawMessage("{}"), LastSeen: runStart})
					break
				}
			}
		}
	}
	if unknown > 0 {
		// Data stays empty so the finding keeps its identity while the count
		// changes; the UI shows the live count from the device list.
		sev := "info"
		if unknown >= 5 {
			sev = "warning"
		}
		add(store.Finding{CheckID: CheckUnknown, Severity: sev, Data: json.RawMessage("{}"), LastSeen: runStart})
	}
	if evs, err := c.st.ListEvents(ctx, store.EventQuery{Kinds: []string{notify.KindIPConflict}, Limit: 20}); err == nil {
		seen := map[string]bool{}
		for _, e := range evs {
			if time.Since(time.UnixMilli(e.TS)) > 7*24*time.Hour {
				continue
			}
			var d map[string]any
			_ = json.Unmarshal(e.Data, &d)
			if g, _ := d["gateway"].(bool); g {
				continue // the router address has its own finding (gateway_mac)
			}
			ip, _ := d["ip"].(string)
			if ip == "" || seen[ip] {
				continue
			}
			seen[ip] = true
			data, _ := json.Marshal(map[string]any{"ip": ip})
			add(store.Finding{CheckID: CheckConflict, Severity: "warning", Data: data, LastSeen: runStart})
		}
	}

	for _, f := range findings {
		isNew, err := c.st.UpsertFinding(ctx, f)
		if err != nil {
			c.log.Error("store finding", "err", err)
			continue
		}
		// New-device alerts already cover unconfirmed devices.
		if isNew && f.CheckID != CheckUnknown && (f.Severity == "critical" || f.Severity == "warning") {
			var dev *store.Device
			if d, ok := c.mon.Device(f.DeviceID); ok {
				dev = &d
			}
			var data map[string]any
			_ = json.Unmarshal(f.Data, &data)
			if data == nil {
				data = map[string]any{}
			}
			data["title"], data["detail"] = Describe(f.CheckID, data, "de")
			c.mon.AddEvent(ctx, notify.KindSecurity, f.Severity, f.DeviceID, map[string]any{"check": f.CheckID, "port": data["port"],
				"service": data["service"], "server": data["server"]})
			c.notifier.Notify(notify.Event{Kind: notify.KindSecurity, Severity: f.Severity, TS: runStart, Device: dev, Data: data})
		}
	}
	if ctx.Err() != nil {
		return // an interrupted run confirms nothing
	}
	if err := c.st.PruneFindings(ctx, runStart, checked, []string{CheckRiskyPort, CheckUPnP, CheckConflict, CheckUnknown}); err != nil {
		c.log.Error("prune findings", "err", err)
	}
	// DNS findings are confirmed by each run; switching the check off clears them.
	if err := c.st.PruneFindings(ctx, runStart, nil, []string{CheckDNSManipulated, CheckDNSRedirect}); err != nil {
		c.log.Error("prune DNS findings", "err", err)
	}
	c.log.Info("security check finished", "findings", len(findings), "took", time.Since(start).Round(time.Millisecond))
}

func suffix(s string) string {
	if s == "" {
		return ""
	}
	return ": " + s
}

// Describe returns a short title and detail text for notifications.
func Describe(check string, data map[string]any, lang string) (string, string) {
	tr := func(de, en string) string {
		if lang == "en" {
			return en
		}
		return de
	}
	switch check {
	case CheckRiskyPort:
		svc, _ := data["service"].(string)
		return tr("Riskanter Dienst erreichbar: ", "Risky service reachable: ") + strings.ToUpper(svc),
			tr("Prüfe, ob der Dienst gebraucht wird, und schalte ihn sonst ab.", "Check whether the service is needed and turn it off otherwise.")
	case CheckUPnP:
		return tr("UPnP am Router aktiv", "UPnP enabled on the router"),
			tr("Geräte können selbstständig Ports ins Internet öffnen.", "Devices can open ports to the internet on their own.")
	case CheckConflict:
		ip, _ := data["ip"].(string)
		return tr("IP-Konflikt bei ", "IP conflict on ") + ip, ""
	case CheckUnknown:
		return tr("Unbestätigte Geräte im Netz", "Unconfirmed devices on the network"), ""
	case CheckGatewayMAC:
		ip, _ := data["ip"].(string)
		if k, _ := data["kind"].(string); k == "conflict" {
			return tr("Zwei Geräte beanspruchen die Router-Adresse ", "Two devices claim the router address ") + ip,
				tr("So wird Datenverkehr umgeleitet (ARP-Spoofing) – oder ein zweiter Router ist falsch eingerichtet.",
					"This is how traffic gets redirected (ARP spoofing) – or a second router is misconfigured.")
		}
		return tr("Router-Adresse gehört jetzt einem anderen Gerät: ", "The router address now belongs to another device: ") + ip,
			tr("Wenn du keinen neuen Router angeschlossen hast, prüfe das sofort.", "If you did not connect a new router, check this right away.")
	case CheckDHCPRogue:
		srv, _ := data["server"].(string)
		return tr("Zwei Geräte verteilen Adressen im Netz: ", "Two devices hand out addresses on the network: ") + srv,
			tr("Ein zweiter DHCP-Server kann Geräte auf einen falschen Router oder DNS-Server lenken.",
				"A second DHCP server can point devices to a wrong router or DNS server.")
	case CheckIdentity:
		from, _ := data["from"].(string)
		to, _ := data["to"].(string)
		return fmt.Sprintf(tr("Gerät meldet sich als anderes System (%s statt %s)", "Device reports as another system (%s instead of %s)"), family(to, lang), family(from, lang)),
			tr("Ein bekanntes Gerät fragt plötzlich wie ein anderes Betriebssystem nach einer Adresse. Wurde es neu eingerichtet? Sonst könnte ein anderes Gerät seine Kennung nutzen.",
				"A known device suddenly asks for an address like a different operating system. Was it set up anew? Otherwise another device may be using its identity.")
	case CheckDNSManipulated:
		srv, _ := data["server"].(string)
		return tr("DNS-Server liefert falsche Adressen: ", "DNS server returns wrong addresses: ") + srv,
			tr("Bekannte Namen zeigen auf fremde Adressen. So lassen sich Anmeldeseiten fälschen. Prüfe die DNS-Einstellungen im Router.",
				"Well-known names point to foreign addresses. This is how login pages get faked. Check the DNS settings of your router.")
	case CheckDNSRedirect:
		srv, _ := data["server"].(string)
		return tr("DNS-Server leitet Tippfehler um: ", "DNS server redirects mistyped names: ") + srv,
			tr("Namen, die es nicht gibt, bekommen trotzdem eine Adresse – oft eine Werbeseite des Anbieters.",
				"Names that do not exist still get an address – often an ad page of the provider.")
	case CheckTripwire:
		ip, _ := data["ip"].(string)
		if many, _ := data["many"].(bool); many {
			ip = tr("viele Adressen", "many addresses")
		}
		svc, _ := data["service"].(string)
		return tr("Gerät hat den Stolperdraht ausgelöst: ", "Device tripped the tripwire: ") + ip,
			fmt.Sprintf(tr("Es hat sich mit dem Köder-Dienst %s verbunden, den kein normales Gerät benutzt. "+
				"Häufige Ursache ist Schadsoftware, die das Netz absucht. Prüfe das Gerät oder nimm es vom Netz.",
				"It connected to the decoy service %s that no normal device uses. A common cause is malware scanning the network. "+
					"Check the device or take it off the network."), strings.ToUpper(svc))
	case CheckKnownExploited:
		sw, _ := data["software"].(string)
		fixed, _ := data["fixed"].(string)
		return tr("Aktiv ausgenutzte Sicherheitslücken", "Actively exploited vulnerabilities") + suffix(sw),
			fmt.Sprintf(tr("Für diese Version sind Lücken bekannt, die bereits für Angriffe genutzt werden. Behoben in Version %s.",
				"Holes in this version are known to be used in attacks. Fixed in version %s."), fixed)
	case CheckSoftwareOutdated:
		if n, ok := data["count"].(int); ok {
			return fmt.Sprintf(tr("%d Geräte bekommen keine Sicherheitsupdates mehr", "%d devices no longer get security updates"), n),
				tr("Details in Netzwart unter Sicherheit.", "See Security in Netzwart for details.")
		}
		sw, _ := data["software"].(string)
		return tr("Keine Sicherheitsupdates mehr", "No more security updates") + suffix(sw),
			tr("Das System dieses Geräts wird vom Hersteller nicht mehr gepflegt. Netzwart zeigt, was du tun kannst.",
				"The maker no longer maintains this device's system. Netzwart shows what you can do.")
	}
	return check, ""
}

// family names an operating system family for messages.
func family(f, lang string) string {
	names := map[string]string{"apple": "Apple", "windows": "Windows", "android": "Android", "linux": "Linux"}
	if n, ok := names[f]; ok {
		return n
	}
	if lang == "en" {
		return "unknown"
	}
	return "unbekannt"
}
