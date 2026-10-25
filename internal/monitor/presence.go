package monitor

import (
	"context"
	"time"

	"github.com/Pyzyi/netzwart/internal/classify"
	"github.com/Pyzyi/netzwart/internal/store"
)

const (
	heartbeatEvery = time.Minute
	// A restart within this window (e.g. an image update) does not count as a gap.
	gapThreshold = 3 * time.Minute
	// The offline sweeper waits for the first complete scan, but not forever.
	gateTimeout = 3 * time.Minute
)

// recoverGap closes presence spans that were open while Netzwart was not
// running and records the gap, so timelines never claim "online" for time
// nobody was watching.
func (m *Monitor) recoverGap(ctx context.Context) error {
	var hb int64
	err := m.st.GetSetting(ctx, "heartbeat", &hb)
	if err == store.ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	now := m.nowMS()
	if time.Duration(now-hb)*time.Millisecond < gapThreshold {
		return nil
	}
	if err := m.st.CloseAllPresence(ctx, hb); err != nil {
		return err
	}
	m.log.Info("monitoring gap detected; closed open presence spans", "since", time.UnixMilli(hb).UTC())
	return m.st.AddGap(ctx, hb, now)
}

// Run drives the periodic work until ctx ends: flushing last_seen, offline
// detection with wake probes and the heartbeat.
func (m *Monitor) Run(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	lastFlush, lastBeat := m.now(), time.Time{}
	for {
		select {
		case <-ctx.Done():
			m.flush(context.Background())
			m.beat(context.Background())
			return
		case <-tick.C:
		}
		now := m.now()
		if now.Sub(lastFlush) >= 15*time.Second {
			m.flush(ctx)
			lastFlush = now
		}
		if now.Sub(lastBeat) >= heartbeatEvery {
			m.beat(ctx)
			lastBeat = now
		}
		m.sweepOffline(ctx)
	}
}

func (m *Monitor) beat(ctx context.Context) {
	if err := m.st.SetSetting(ctx, "heartbeat", m.nowMS()); err != nil {
		m.log.Error("heartbeat", "err", err)
	}
}

// flush writes buffered last_seen timestamps.
func (m *Monitor) flush(ctx context.Context) {
	m.mu.Lock()
	dirty := m.dirty
	m.dirty = map[int64]int64{}
	m.mu.Unlock()
	if err := m.st.TouchLastSeen(ctx, dirty); err != nil {
		m.log.Error("flush last_seen", "err", err)
		m.mu.Lock()
		for id, ts := range dirty {
			if ts > m.dirty[id] {
				m.dirty[id] = ts
			}
		}
		m.mu.Unlock()
	}
}

// SweepOffline evaluates offline candidates once (exported for tests).
func (m *Monitor) SweepOffline(ctx context.Context) { m.sweepOffline(ctx) }

func (m *Monitor) sweepOffline(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if !m.gateOpen {
		if now.Sub(m.startedAt) < gateTimeout {
			return
		}
		m.gateOpen = true
	}
	for _, e := range m.byID {
		if !e.d.Online || e.probing || e.d.IsSelf {
			continue
		}
		limit := m.settings.OfflineAfterDur(classify.Mobile(e.d.Type))
		if now.Sub(time.UnixMilli(e.d.LastSeen)) <= limit {
			continue
		}
		if m.prober == nil {
			m.setOffline(ctx, e)
			continue
		}
		// Ask the device directly before declaring it gone; phones in power
		// save often ignore broadcast ARP but answer unicast probes.
		e.probing = true
		snap := e.d
		prober := m.prober
		go func() {
			m.probeSem <- struct{}{}
			alive := prober.Probe(ctx, snap)
			<-m.probeSem
			m.mu.Lock()
			defer m.mu.Unlock()
			cur := m.byID[snap.ID]
			if cur == nil {
				return
			}
			cur.probing = false
			if alive {
				ts := m.nowMS()
				if ts > cur.d.LastSeen {
					cur.d.LastSeen = ts
					m.dirty[cur.d.ID] = ts
				}
				return
			}
			// A sighting may have arrived while probing.
			if cur.d.Online && m.now().Sub(time.UnixMilli(cur.d.LastSeen)) > limit {
				m.setOffline(ctx, cur)
			}
		}()
	}
}

// LastSweep returns when the last complete scan finished.
func (m *Monitor) LastSweep() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastSweepAt
}

// Maintain runs daily housekeeping: retention and expiry of private MACs.
func (m *Monitor) Maintain(ctx context.Context) {
	m.mu.Lock()
	s := m.settings
	m.mu.Unlock()
	now := m.now()
	if err := m.st.Prune(ctx, now.AddDate(0, 0, -s.RetentionDays).UnixMilli()); err != nil {
		m.log.Error("prune", "err", err)
	}
	if s.ExpireRandomDays <= 0 {
		return
	}
	ids, err := m.st.ExpireRandomDevices(ctx, now.AddDate(0, 0, -s.ExpireRandomDays).UnixMilli())
	if err != nil {
		m.log.Error("expire random devices", "err", err)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if e := m.byID[id]; e != nil {
			m.unindex(e)
			m.hub.Publish("device_removed", map[string]int64{"id": id})
		}
	}
	if len(ids) > 0 {
		m.log.Info("expired unacknowledged devices with private MAC addresses", "count", len(ids))
	}
}
