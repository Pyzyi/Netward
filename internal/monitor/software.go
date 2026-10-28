package monitor

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Pyzyi/Netward/internal/software"
	"github.com/Pyzyi/Netward/internal/store"
)

// Rows the user sets; they are not observations of the device.
const (
	swManual  = software.SourceManual
	swEdition = "edition"
)

// refreshSeen: an unchanged observation is written again at most this often,
// which keeps "seen_at" useful for staleness without a write per packet.
const refreshSeen = 6 * time.Hour

// SetSoftwareHook registers fn to run (outside the lock) whenever what a
// device reveals about its software changes.
func (m *Monitor) SetSoftwareHook(fn func()) {
	m.mu.Lock()
	m.swHook = fn
	m.mu.Unlock()
}

// observeSoftware records observations; m.mu must be held.
func (m *Monitor) observeSoftware(e *entry, obs []software.Observation) {
	now := m.nowMS()
	changed := false
	for _, o := range obs {
		if o.Source == "" || o.Source == swManual || o.Source == swEdition || !plausible(e.d, o) {
			continue
		}
		row := store.Software{DeviceID: e.d.ID, Source: o.Source, Product: o.Product, Version: o.Version,
			Evidence: o.Evidence, UpdateAvailable: o.UpdateAvailable, SeenAt: now}
		old, had := e.sw[o.Source]
		same := had && old.Product == row.Product && old.Version == row.Version && old.UpdateAvailable == row.UpdateAvailable
		if same && time.Duration(now-old.SeenAt)*time.Millisecond < refreshSeen {
			continue
		}
		e.sw[o.Source] = row
		if had && e.d.ID != 0 && old.Product == row.Product && old.Version != "" && row.Version != "" && old.Version != row.Version {
			data, _ := json.Marshal(map[string]string{"product": row.Product, "from": old.Version, "to": row.Version})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			m.logDevice(ctx, e.d.ID, now, KindSoftwareChanged, data)
			cancel()
		}
		if e.d.ID != 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := m.st.UpsertSoftware(ctx, row); err != nil {
				m.log.Error("store software", "id", e.d.ID, "err", err)
			}
			cancel()
		}
		if !same {
			changed = true
		}
	}
	if changed {
		m.swChanged = true
	}
}

// plausible rejects announcements that cannot belong to this device: anyone
// on the LAN can send them in another device's name (T-30). Active probes
// and the FRITZ!Box answer are not filtered.
func plausible(d store.Device, o software.Observation) bool {
	vendor := strings.ToLower(d.Vendor + " " + d.VendorOfficial)
	unknownVendor := strings.TrimSpace(vendor) == "" || d.MACKind == "random"
	switch o.Product {
	case software.FritzOS:
		return strings.Contains(vendor, "avm") || d.IsGateway
	case software.DSM:
		return strings.Contains(vendor, "synology") || unknownVendor
	case software.IOS, software.IPadOS, software.MacOS, software.TvOS, software.AudioOS:
		return o.Source == software.SourceSMB || strings.Contains(vendor, "apple") || unknownVendor
	case software.Android:
		return !strings.Contains(vendor, "apple") && !strings.Contains(vendor, "avm") && !strings.Contains(vendor, "synology")
	}
	return true
}

// AddSoftware records observations made outside the monitor (active probes,
// the FRITZ!Box) and publishes the device.
func (m *Monitor) AddSoftware(id int64, obs []software.Observation) {
	if len(obs) == 0 {
		return
	}
	m.mu.Lock()
	e := m.byID[id]
	if e == nil {
		m.mu.Unlock()
		return
	}
	m.observeSoftware(e, obs)
	m.publish(e)
	m.mu.Unlock()
	m.softwareChanged()
}

// softwareChanged runs the hook if an observation changed since the last call.
// It must be called without m.mu held.
func (m *Monitor) softwareChanged() {
	m.mu.Lock()
	fn, changed := m.swHook, m.swChanged || m.netChanged
	m.swChanged, m.netChanged = false, false
	m.mu.Unlock()
	if changed && fn != nil {
		fn()
	}
}

// SetSoftwareManual stores what the user says runs on the device: a product
// and version (e.g. "ios", "18"), or nothing to clear it.
func (m *Monitor) SetSoftwareManual(ctx context.Context, id int64, product, version string) error {
	return m.setUserRow(ctx, id, swManual, product, version)
}

// SetEdition marks a Windows device as Enterprise/LTSC/Server ("extended") or
// back to Home/Pro ("").
func (m *Monitor) SetEdition(ctx context.Context, id int64, edition string) error {
	if edition == "" {
		return m.setUserRow(ctx, id, swEdition, "", "")
	}
	return m.setUserRow(ctx, id, swEdition, software.Windows, edition)
}

func (m *Monitor) setUserRow(ctx context.Context, id int64, source, product, version string) error {
	m.mu.Lock()
	e := m.byID[id]
	if e == nil {
		m.mu.Unlock()
		return store.ErrNotFound
	}
	var err error
	if product == "" {
		delete(e.sw, source)
		err = m.st.DeleteSoftware(ctx, id, source)
	} else {
		row := store.Software{DeviceID: id, Source: source, Product: product, Version: version, Evidence: "", SeenAt: m.nowMS()}
		e.sw[source] = row
		err = m.st.UpsertSoftware(ctx, row)
	}
	m.swChanged = true
	m.publish(e)
	m.mu.Unlock()
	m.softwareChanged()
	return err
}

// assess runs the software assessment for e; m.mu must be held.
func (m *Monitor) assess(e *entry) software.Assessment {
	in := software.Input{Model: e.d.Model, Now: m.now()}
	for src, row := range e.sw {
		if src == swEdition {
			in.Edition = row.Version
			continue
		}
		in.Observations = append(in.Observations, software.Observation{Source: row.Source, Product: row.Product,
			Version: row.Version, Evidence: row.Evidence, SeenAt: row.SeenAt, UpdateAvailable: row.UpdateAvailable})
	}
	return software.Assess(in)
}

// Software returns the full software assessment of a device.
func (m *Monitor) Software(id int64) (software.Assessment, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.byID[id]
	if e == nil {
		return software.Assessment{}, false
	}
	return m.assess(e), true
}
