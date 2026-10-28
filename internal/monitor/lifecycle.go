package monitor

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Pyzyi/Netward/internal/classify"
	"github.com/Pyzyi/Netward/internal/notify"
)

// The history of a device (docs/10, F1) keeps its milestones beyond the
// event retention: what changed about it, not every coming and going.
const (
	KindSoftwareChanged = "software_changed" // same product, other version
	KindOutage          = "outage"           // an always-on device was away
)

// lifecycleKinds are the events that also go into the device history.
var lifecycleKinds = map[string]bool{
	KindIPChanged:         true,
	KindIdentityChanged:   true,
	KindPortNew:           true,
	notify.KindSecurity:   true,
	notify.KindIPConflict: true,
}

// outageTypes are devices that are meant to be on all the time. Phones,
// laptops, TVs and consoles come and go; for them an absence is normal.
var outageTypes = map[string]bool{
	classify.Router: true, classify.AccessPoint: true, classify.Switch: true, classify.Server: true,
	classify.NAS: true, classify.Camera: true, classify.SmartHome: true, classify.Plug: true,
}

// minOutage is the shortest absence of an always-on device worth recording.
const minOutage = 30 * time.Minute

func (m *Monitor) logDevice(ctx context.Context, id, ts int64, kind string, data json.RawMessage) {
	if err := m.st.AddDeviceLog(ctx, id, ts, kind, data); err != nil {
		m.log.Error("device history", "id", id, "kind", kind, "err", err)
	}
}

// noteOutage records the absence of an always-on device that just came back.
// Absences overlapping a time when Netward itself was not running are left
// out: nobody knows whether the device was really gone.
func (m *Monitor) noteOutage(ctx context.Context, e *entry, from, to int64) {
	if from <= 0 || e.d.ID == 0 || !outageTypes[e.d.Type] || time.Duration(to-from)*time.Millisecond < minOutage {
		return
	}
	if gaps, err := m.st.ListGaps(ctx, from, to); err != nil || len(gaps) > 0 {
		return
	}
	data, _ := json.Marshal(map[string]int64{"from": from, "to": to})
	m.logDevice(ctx, e.d.ID, to, KindOutage, data)
}
