// Package security runs the network security check: services that should
// not be reachable, UPnP port forwarding on the router, IP conflicts and
// devices nobody has confirmed. It only ever opens TCP connections and
// closes them again; no payload is sent to any device.
package security

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/scanner"
	"github.com/Pyzyi/netzwart/internal/store"
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

	mu     sync.Mutex
	status Status
}

// New creates a checker.
func New(st *store.Store, mon *monitor.Monitor, h *hub.Hub, n notify.Notifier, log *slog.Logger) *Checker {
	c := &Checker{st: st, mon: mon, hub: h, notifier: n, log: log}
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
	ports := make([]int, 0, len(riskyPorts))
	for p := range riskyPorts {
		ports = append(ports, p)
	}

	var mu sync.Mutex
	var findings []store.Finding
	add := func(f store.Finding) {
		mu.Lock()
		findings = append(findings, f)
		mu.Unlock()
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, d := range devs {
		if !d.Online || d.IsSelf {
			continue
		}
		ip, err := netip.ParseAddr(d.IP)
		if err != nil {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d store.Device, ip netip.Addr) {
			defer wg.Done()
			defer func() { <-sem }()
			for _, p := range scanner.ScanPorts(ctx, ip, ports) {
				r := riskyPorts[p.Port]
				data, _ := json.Marshal(map[string]any{"port": p.Port, "service": r.service})
				add(store.Finding{CheckID: CheckRiskyPort, DeviceID: d.ID, Severity: r.severity, Data: data, LastSeen: runStart})
			}
		}(d, ip)
	}
	wg.Wait()

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
			c.mon.AddEvent(ctx, notify.KindSecurity, f.Severity, f.DeviceID, map[string]any{"check": f.CheckID, "port": data["port"], "service": data["service"]})
			c.notifier.Notify(notify.Event{Kind: notify.KindSecurity, Severity: f.Severity, TS: runStart, Device: dev, Data: data})
		}
	}
	if err := c.st.PruneFindings(ctx, runStart); err != nil {
		c.log.Error("prune findings", "err", err)
	}
	c.log.Info("security check finished", "findings", len(findings), "took", time.Since(start).Round(time.Millisecond))
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
	}
	return check, ""
}
