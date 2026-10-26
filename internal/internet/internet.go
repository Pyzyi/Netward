// Package internet watches the uplink: is the router reachable, does the
// internet answer, how fast, and when were outages.
package internet

import (
	"context"
	"log/slog"
	"math"
	"net"
	"net/netip"
	"slices"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/egress"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/l2"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/store"
)

// States.
const (
	StateUnknown  = "unknown"
	StateOnline   = "online"
	StateDegraded = "degraded" // DNS fails while IP connectivity works
	StateOffline  = "offline"
	StateLANDown  = "lan_down" // the router itself does not answer
	StateDisabled = "disabled"
)

const (
	checkEvery = 30 * time.Second
	failLimit  = 2 // consecutive failed checks before an outage starts
	historyLen = 2880
)

// Sample is one latency measurement (-1 = failed).
type Sample struct {
	TS int64 `json:"t"`
	MS int   `json:"ms"`
}

// Status is shown in the UI and published to MQTT.
type Status struct {
	State     string `json:"state"`
	LatencyMS int    `json:"latencyMs"`
	CheckedAt int64  `json:"checkedAt"`
	Since     int64  `json:"since"`
	Gateway   string `json:"gateway"`
	GatewayOK bool   `json:"gatewayOk"`
}

// Logger records events in the activity log.
type Logger interface {
	AddEvent(ctx context.Context, kind, severity string, deviceID int64, data map[string]any)
}

// Gateways returns the current default gateways.
type Gateways func() []netip.Addr

// Monitor performs the checks.
type Monitor struct {
	st       *store.Store
	hub      *hub.Hub
	log      *slog.Logger
	events   Logger
	notifier notify.Notifier
	pinger   *l2.Pinger
	policy   *egress.Policy
	gateways Gateways

	mu       sync.Mutex
	demo     bool
	enabled  bool
	targets  []string
	status   Status
	fails    int
	outageAt int64
	history  []Sample
	dnsFails int   // checks in a row with working addresses but failing names
	bucket   []int // latencies (ms, -1 = lost) of the current 5-minute bucket
	bucketAt int64 // start of the current bucket

	line       *store.LineSample // last stored connection sample
	lineNow    *store.LineSample // last reported one
	lineUptime int64
}

// latencyBucket is the resolution of the stored latency history.
const latencyBucket = 5 * time.Minute

// New creates the monitor.
func New(st *store.Store, h *hub.Hub, events Logger, n notify.Notifier, pinger *l2.Pinger, policy *egress.Policy, gw Gateways, log *slog.Logger) *Monitor {
	return &Monitor{st: st, hub: h, events: events, notifier: n, pinger: pinger, policy: policy, gateways: gw, log: log,
		status: Status{State: StateUnknown}}
}

// Configure enables/disables checks and sets the targets ("host:port").
func (m *Monitor) Configure(enabled bool, targets []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.demo {
		return
	}
	m.enabled = enabled
	m.targets = append([]string(nil), targets...)
	if !enabled {
		// An outage cannot be observed to end while checks are off: close it now
		// instead of leaving it open forever (and reporting a bogus recovery later).
		if m.outageAt != 0 && m.fails >= failLimit {
			_ = m.st.CloseOutages(context.Background(), time.Now().UnixMilli())
		}
		if m.dnsFails >= failLimit {
			_ = m.st.CloseDNSOutages(context.Background(), time.Now().UnixMilli())
		}
		m.status = Status{State: StateDisabled}
		m.fails, m.outageAt, m.dnsFails = 0, 0, 0
		m.bucket, m.bucketAt = nil, 0
	} else if m.status.State == StateDisabled {
		m.status = Status{State: StateUnknown}
	}
}

// SeedDemo fills status and history with plausible sample data (demo mode only).
func (m *Monitor) SeedDemo(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.demo = true
	m.history = m.history[:0]
	outStart, outEnd := now.Add(-20*time.Hour), now.Add(-20*time.Hour+23*time.Minute)
	for ts := now.Add(-24 * time.Hour); !ts.After(now); ts = ts.Add(5 * time.Minute) {
		ms := 12 + int(2.5*math.Sin(float64(ts.Unix())/2700)+1.5*math.Sin(float64(ts.Unix())/700))
		if h := ts.Hour(); h >= 19 && h <= 22 {
			ms += 5 // evening streaming peak
		}
		if ts.After(outStart) && ts.Before(outEnd) {
			ms = -1
		}
		m.history = append(m.history, Sample{TS: ts.UnixMilli(), MS: ms})
	}
	m.status = Status{State: StateOnline, LatencyMS: 13, CheckedAt: now.UnixMilli(), Since: outEnd.UnixMilli(),
		Gateway: "192.168.178.1", GatewayOK: true}
	m.lineNow = &store.LineSample{TS: now.UnixMilli(), Access: "dsl", Up: true, DownKbps: 92340, UpKbps: 38012, DownMaxKbps: 118000, UpMaxKbps: 41500}
}

// Status returns the current status.
func (m *Monitor) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// History returns latency samples of the last 24 hours.
func (m *Monitor) History() []Sample {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Sample(nil), m.history...)
}

// Run checks periodically until ctx ends.
func (m *Monitor) Run(ctx context.Context) {
	// Resume an outage that was open when the process stopped.
	// A DNS outage cannot be resumed reliably; the report leaves out the
	// time Netzwart was not running anyway.
	_ = m.st.CloseDNSOutages(ctx, time.Now().UnixMilli())
	if outs, err := m.st.ListOutages(ctx, 0); err == nil {
		for _, o := range outs {
			if o.End == 0 {
				m.outageAt = o.Start
				m.fails = failLimit
			}
		}
	}
	t := time.NewTimer(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		m.Check(ctx)
		t.Reset(checkEvery)
	}
}

func (m *Monitor) dialTime(ctx context.Context, target string) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	start := time.Now()
	c, err := m.policy.Dialer(4*time.Second).DialContext(ctx, "tcp", target)
	if err != nil {
		return 0, err
	}
	c.Close()
	return time.Since(start), nil
}

// Check runs one round of checks and updates the state machine.
func (m *Monitor) Check(ctx context.Context) {
	m.mu.Lock()
	enabled, targets := m.enabled, append([]string(nil), m.targets...)
	m.mu.Unlock()
	if !enabled {
		return
	}
	now := time.Now()

	// Check the first default gateway so a LAN problem is not reported as an internet outage.
	gwOK, gw := true, ""
	if gws := m.gateways(); len(gws) > 0 {
		g := gws[0]
		gw = g.String()
		gwOK = m.pinger != nil && func() bool { _, ok := m.pinger.Ping(ctx, g, 2*time.Second); return ok }()
		if !gwOK {
			for _, p := range []string{"53", "80", "443"} {
				d := net.Dialer{Timeout: 1500 * time.Millisecond}
				if c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(gw, p)); err == nil {
					c.Close()
					gwOK = true
					break
				}
			}
		}
	}

	best := time.Duration(-1)
	ipOK, nameOK, nameTried := false, false, false
	for _, t := range targets {
		host, _, _ := net.SplitHostPort(t)
		isIP := net.ParseIP(host) != nil
		if !isIP {
			nameTried = true
		}
		d, err := m.dialTime(ctx, t)
		if err != nil {
			continue
		}
		if isIP {
			ipOK = true
		} else {
			nameOK = true
		}
		if best < 0 || d < best {
			best = d
		}
	}
	state := StateOffline
	switch {
	case ipOK && nameTried && !nameOK:
		state = StateDegraded
	case ipOK || nameOK:
		state = StateOnline
	case gw != "" && !gwOK:
		state = StateLANDown
	}
	m.record(ctx, now, state, best, gw, gwOK)
}

func (m *Monitor) record(ctx context.Context, now time.Time, state string, latency time.Duration, gw string, gwOK bool) {
	m.mu.Lock()
	ms := -1
	if latency >= 0 {
		ms = int(latency.Milliseconds())
	}
	m.history = append(m.history, Sample{TS: now.UnixMilli(), MS: ms})
	if len(m.history) > historyLen {
		m.history = m.history[len(m.history)-historyLen:]
	}
	flush := m.addToBucket(now, ms, gwOK)
	prev := m.status.State
	up := state == StateOnline || state == StateDegraded
	var startOutage, endOutage, startDNS, endDNS bool
	var outageStart int64
	if state == StateDegraded {
		m.dnsFails++
		startDNS = m.dnsFails == failLimit
	} else {
		endDNS = m.dnsFails >= failLimit
		m.dnsFails = 0
	}
	if up {
		if m.fails >= failLimit && m.outageAt != 0 {
			endOutage, outageStart = true, m.outageAt
		}
		m.fails, m.outageAt = 0, 0
	} else {
		m.fails++
		if m.fails == 1 {
			m.outageAt = now.UnixMilli()
		}
		if m.fails == failLimit {
			startOutage = true
		}
	}
	since := m.status.Since
	if prev != state {
		since = now.UnixMilli()
	}
	m.status = Status{State: state, LatencyMS: ms, CheckedAt: now.UnixMilli(), Since: since, Gateway: gw, GatewayOK: gwOK}
	st := m.status
	outageAt := m.outageAt
	m.mu.Unlock()

	m.hub.Publish("internet", st)
	if flush != nil {
		if err := m.st.AddLatency(ctx, *flush); err != nil {
			m.log.Error("record latency", "err", err)
		}
	}
	// DNS outages only go into the provider report: no event, no message.
	if startDNS {
		_, _ = m.st.OpenOutage(ctx, now.Add(-time.Duration(failLimit-1)*checkEvery).UnixMilli(), store.OutageDNS)
	}
	if endDNS {
		_ = m.st.CloseDNSOutages(ctx, now.UnixMilli())
	}
	if startOutage {
		// The kind is decided at the start: an outage that begins with the
		// router not answering (a reboot, a power cut) is not the provider's.
		kind := store.OutageInternet
		if state == StateLANDown {
			kind = store.OutageRouter
		}
		if _, err := m.st.OpenOutage(ctx, outageAt, kind); err != nil {
			m.log.Error("record outage", "err", err)
		}
		data := map[string]any{"state": state}
		m.events.AddEvent(ctx, notify.KindInternetDown, "warning", 0, data)
		m.notifier.Notify(notify.Event{Kind: notify.KindInternetDown, Severity: "warning", TS: outageAt, Data: data})
		m.log.Warn("internet outage started", "state", state)
	}
	if endOutage {
		_ = m.st.CloseOutages(ctx, now.UnixMilli())
		dur := now.UnixMilli() - outageStart
		data := map[string]any{"durationMs": dur, "since": outageStart}
		m.events.AddEvent(ctx, notify.KindInternetUp, "info", 0, data)
		m.notifier.Notify(notify.Event{Kind: notify.KindInternetUp, Severity: "info", TS: now.UnixMilli(), Data: data})
		m.log.Info("internet outage ended", "duration", time.Duration(dur)*time.Millisecond)
	}
}

// addToBucket adds one check to the current 5-minute bucket and returns the
// previous bucket once a new one starts; m.mu must be held. Failures while
// the router itself does not answer are not counted as lost: that is a
// problem at home, not on the line.
func (m *Monitor) addToBucket(now time.Time, ms int, gwOK bool) *store.Latency {
	start := now.Truncate(latencyBucket).UnixMilli()
	var out *store.Latency
	if m.bucketAt != 0 && start != m.bucketAt && len(m.bucket) > 0 {
		l := summarize(m.bucketAt, m.bucket)
		out = &l
		m.bucket = m.bucket[:0]
	}
	m.bucketAt = start
	if ms >= 0 || gwOK {
		m.bucket = append(m.bucket, ms)
	}
	return out
}

func summarize(at int64, vals []int) store.Latency {
	l := store.Latency{TS: at, Median: -1, Max: -1, Samples: len(vals)}
	var ok []int
	for _, v := range vals {
		if v < 0 {
			l.Lost++
			continue
		}
		ok = append(ok, v)
	}
	if len(ok) > 0 {
		slices.Sort(ok)
		l.Median, l.Max = ok[len(ok)/2], ok[len(ok)-1]
	}
	return l
}
