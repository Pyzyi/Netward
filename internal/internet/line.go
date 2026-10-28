package internet

import (
	"context"
	"time"

	"github.com/Pyzyi/Netward/internal/fritzbox"
	"github.com/Pyzyi/Netward/internal/store"
)

// lineHeartbeat: an unchanged line is still stored this often, so the
// report can tell "unchanged" from "not measured".
const lineHeartbeat = time.Hour

// ApplyLine records the connection data the FRITZ!Box reported (docs/09,
// F5): stored when something changes, at least hourly. A connection uptime
// lower than before means the line reconnected.
func (m *Monitor) ApplyLine(ctx context.Context, l fritzbox.Line, at time.Time) {
	s := store.LineSample{TS: at.UnixMilli(), Access: l.Access, Up: l.Up, DownKbps: l.DownKbps, UpKbps: l.UpKbps,
		DownMaxKbps: l.DownMaxKbps, UpMaxKbps: l.UpMaxKbps}
	m.mu.Lock()
	prev, prevUptime := m.line, m.lineUptime
	if l.Uptime > 0 {
		s.Reconnect = prevUptime > 0 && l.Uptime < prevUptime
		m.lineUptime = l.Uptime
	}
	save := prev == nil || s.Reconnect || at.UnixMilli()-prev.TS >= lineHeartbeat.Milliseconds() ||
		prev.Access != s.Access || prev.Up != s.Up || prev.DownKbps != s.DownKbps || prev.UpKbps != s.UpKbps ||
		drift(prev.DownMaxKbps, s.DownMaxKbps) || drift(prev.UpMaxKbps, s.UpMaxKbps)
	if save {
		m.line = &s
	}
	m.lineNow = &s
	m.mu.Unlock()
	if save {
		if err := m.st.AddLineSamples(ctx, s); err != nil {
			m.log.Error("record line data", "err", err)
		}
	}
}

// drift reports a change of the attainable rate by more than 2 %; it
// wanders a little all the time.
func drift(a, b int) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d*50 > max(a, b)
}

// Line returns the last connection data, if the FRITZ!Box reports it.
func (m *Monitor) Line() *store.LineSample {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lineNow == nil {
		return nil
	}
	l := *m.lineNow
	return &l
}
