package security

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/software"
	"github.com/Pyzyi/netzwart/internal/store"
)

// Software findings. They are decided from what devices reveal about
// themselves and re-evaluated independently of the (opt-in) security check:
// at start, daily, and shortly after an observation changes.
const (
	CheckSoftwareOutdated = "software_outdated" // no security updates any more
	CheckSoftwareUpdate   = "software_update"   // update waiting or end in sight
)

// maxSingleAlerts: more new findings at once are sent as one summary, e.g.
// on the first evaluation after installing this version.
const maxSingleAlerts = 3

// TriggerSoftware asks for a re-evaluation soon; it never blocks.
func (c *Checker) TriggerSoftware() {
	select {
	case c.swTrigger <- struct{}{}:
	default:
	}
}

// WatchSoftware evaluates software findings until ctx ends.
func (c *Checker) WatchSoftware(ctx context.Context) {
	t := time.NewTimer(90 * time.Second) // let discovery settle after a start
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.swTrigger:
			// Bundle bursts (a discovery round reports many devices at once).
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			t.Reset(30 * time.Second)
			continue
		case <-t.C:
		}
		c.EvaluateSoftware(ctx)
		t.Reset(24 * time.Hour)
	}
}

type swAlert struct {
	f    store.Finding
	name string
	dev  *store.Device
}

// EvaluateSoftware turns the software assessments into findings.
func (c *Checker) EvaluateSoftware(ctx context.Context) {
	// Pruning compares timestamps; two runs within one millisecond must still
	// tell their findings apart.
	runStart := time.Now().UnixMilli()
	c.mu.Lock()
	if runStart <= c.swLastRun {
		runStart = c.swLastRun + 1
	}
	c.swLastRun = runStart
	c.mu.Unlock()
	var checked []int64
	var alerts []swAlert
	for _, d := range c.mon.Devices() {
		checked = append(checked, d.ID)
		a, ok := c.mon.Software(d.ID)
		if !ok {
			continue
		}
		var check, sev string
		switch a.Status {
		case software.StatusOutdated:
			check, sev = CheckSoftwareOutdated, "warning"
		case software.StatusAging, software.StatusUpdate:
			check, sev = CheckSoftwareUpdate, "info"
		default:
			continue
		}
		// The version is not part of the identity: an update within the same
		// verdict must not alert again.
		data, _ := json.Marshal(map[string]string{"product": a.Product, "status": a.Status})
		f := store.Finding{CheckID: check, DeviceID: d.ID, Severity: sev, Data: data, LastSeen: runStart}
		isNew, err := c.st.UpsertFinding(ctx, f)
		if err != nil {
			c.log.Error("store software finding", "err", err)
			continue
		}
		if isNew && sev == "warning" {
			dev := d
			alerts = append(alerts, swAlert{f: f, name: a.Name, dev: &dev})
		}
	}
	if ctx.Err() != nil {
		return
	}
	if err := c.st.PruneFindings(ctx, runStart, checked, []string{CheckSoftwareOutdated, CheckSoftwareUpdate}); err != nil {
		c.log.Error("prune software findings", "err", err)
	}
	c.alertSoftware(ctx, runStart, alerts)
	c.hub.Publish("security", c.Status())
}

func (c *Checker) alertSoftware(ctx context.Context, ts int64, alerts []swAlert) {
	if len(alerts) == 0 {
		return
	}
	if len(alerts) > maxSingleAlerts {
		data := map[string]any{"check": CheckSoftwareOutdated, "count": len(alerts)}
		data["title"], data["detail"] = Describe(CheckSoftwareOutdated, data, "de")
		c.mon.AddEvent(ctx, notify.KindSecurity, "warning", 0, map[string]any{"check": CheckSoftwareOutdated, "count": len(alerts)})
		c.notifier.Notify(notify.Event{Kind: notify.KindSecurity, Severity: "warning", TS: ts, Data: data})
		return
	}
	for _, a := range alerts {
		data := map[string]any{"check": CheckSoftwareOutdated, "software": a.name}
		data["title"], data["detail"] = Describe(CheckSoftwareOutdated, data, "de")
		c.mon.AddEvent(ctx, notify.KindSecurity, "warning", a.f.DeviceID, map[string]any{"check": CheckSoftwareOutdated, "software": a.name})
		c.notifier.Notify(notify.Event{Kind: notify.KindSecurity, Severity: "warning", TS: ts, Device: a.dev, Data: data})
	}
}
