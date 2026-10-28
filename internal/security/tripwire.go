package security

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/store"
	"github.com/Pyzyi/netzwart/internal/tripwire"
)

// CheckTripwire marks a device that connected to one of Netzwart's decoy
// services (docs/10, F3). One finding per device, whatever ports it tried,
// so a port scan raises one alarm and not twenty.
const CheckTripwire = "tripwire"

// tripwireKeep is how long a tripwire finding stays after the last hit.
const tripwireKeep = 30 * 24 * time.Hour

// maxUnknownTripwire bounds the findings for unknown sources; beyond, one
// finding stands for "many addresses" (someone rotating addresses).
const maxUnknownTripwire = 16

// Tripwire records a connection to a decoy: always as an event, and as a
// critical finding that notifies when it is new. The tripwire manager calls
// it from a single goroutine, rate-limited.
func (c *Checker) Tripwire(ctx context.Context, h tripwire.Hit) {
	data := map[string]any{}
	var dev *store.Device
	if d, ok := c.mon.DeviceByIP(h.IP); ok {
		dev = &d
	} else {
		// Unknown sources are told apart by address, IPv6 by its /64.
		data["ip"] = tripwire.Group(h.IP)
		if c.unknownTripwire(ctx, data["ip"].(string)) >= maxUnknownTripwire {
			data = map[string]any{"many": true}
		}
	}
	raw, _ := json.Marshal(data)
	f := store.Finding{CheckID: CheckTripwire, Severity: "critical", Data: raw, LastSeen: h.At.UnixMilli()}
	if dev != nil {
		f.DeviceID = dev.ID
	}
	isNew, err := c.st.UpsertFinding(ctx, f)
	if err != nil {
		c.log.Error("store tripwire finding", "err", err)
		return
	}
	c.mon.AddEvent(ctx, notify.KindSecurity, "critical", f.DeviceID, map[string]any{"check": CheckTripwire, "ip": h.IP.String(),
		"port": h.Port, "service": h.Service, "repeat": !isNew})
	if !isNew {
		return
	}
	data["ip"], data["port"], data["service"], data["check"] = h.IP.String(), h.Port, h.Service, CheckTripwire
	data["title"], data["detail"] = Describe(CheckTripwire, data, "de")
	c.notifier.Notify(notify.Event{Kind: notify.KindSecurity, Severity: "critical", TS: h.At.UnixMilli(), Device: dev, Data: data})
}

// unknownTripwire counts the tripwire findings of unknown sources other
// than group.
func (c *Checker) unknownTripwire(ctx context.Context, group string) int {
	fs, err := c.st.ListFindings(ctx)
	if err != nil {
		return 0
	}
	n := 0
	for _, f := range fs {
		if f.CheckID != CheckTripwire || f.DeviceID != 0 {
			continue
		}
		var d map[string]any
		_ = json.Unmarshal(f.Data, &d)
		if d["ip"] != group {
			n++
		}
	}
	return n
}

// pruneTripwire removes tripwire findings without a hit for 30 days.
func (c *Checker) pruneTripwire(ctx context.Context) {
	devs := c.mon.Devices()
	ids := make([]int64, 0, len(devs))
	for _, d := range devs {
		ids = append(ids, d.ID)
	}
	if err := c.st.PruneFindings(ctx, time.Now().Add(-tripwireKeep).UnixMilli(), ids, []string{CheckTripwire}); err != nil {
		c.log.Error("prune tripwire findings", "err", err)
	}
}
