package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// PresenceSpan is a continuous period during which a device was online.
type PresenceSpan struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"` // 0 = still online
}

// OpenPresence starts a presence span unless one is already open.
func (s *Store) OpenPresence(ctx context.Context, deviceID, ts int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO presence (device_id, started_at) VALUES (?, ?) ON CONFLICT DO NOTHING`, deviceID, ts)
	return err
}

// ListAllPresence returns the presence spans of every device overlapping [from, to].
func (s *Store) ListAllPresence(ctx context.Context, from, to int64) (map[int64][]PresenceSpan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT device_id, started_at, IFNULL(ended_at, 0) FROM presence
		WHERE started_at <= ? AND (ended_at IS NULL OR ended_at >= ?)`, to, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]PresenceSpan{}
	for rows.Next() {
		var id int64
		var p PresenceSpan
		if err := rows.Scan(&id, &p.Start, &p.End); err != nil {
			return nil, err
		}
		out[id] = append(out[id], p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Sorted here rather than in SQL: an ORDER BY makes SQLite pick the
	// device index and scan the whole table instead of using presence_ended.
	for _, spans := range out {
		sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	}
	return out, nil
}

// ClosePresence ends the open span of a device at ts.
func (s *Store) ClosePresence(ctx context.Context, deviceID, ts int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE presence SET ended_at = MAX(started_at, ?) WHERE device_id = ? AND ended_at IS NULL`, ts, deviceID)
	return err
}

// ListPresence returns spans overlapping [from, to].
func (s *Store) ListPresence(ctx context.Context, deviceID, from, to int64) ([]PresenceSpan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT started_at, IFNULL(ended_at, 0) FROM presence
		WHERE device_id = ? AND started_at <= ? AND (ended_at IS NULL OR ended_at >= ?) ORDER BY started_at`, deviceID, to, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PresenceSpan
	for rows.Next() {
		var p PresenceSpan
		if err := rows.Scan(&p.Start, &p.End); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Event is an entry of the activity log.
type Event struct {
	ID       int64           `json:"id"`
	TS       int64           `json:"ts"`
	Kind     string          `json:"kind"`
	Severity string          `json:"severity"`
	DeviceID int64           `json:"deviceId,omitempty"`
	Data     json.RawMessage `json:"data"`
}

// AddEvent appends to the activity log and fills in e.ID.
func (s *Store) AddEvent(ctx context.Context, e *Event) error {
	if len(e.Data) == 0 {
		e.Data = json.RawMessage("{}")
	}
	if e.Severity == "" {
		e.Severity = "info"
	}
	var dev any
	if e.DeviceID != 0 {
		dev = e.DeviceID
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO events (ts, kind, severity, device_id, data) VALUES (?, ?, ?, ?, ?)`,
		e.TS, e.Kind, e.Severity, dev, string(e.Data))
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

// EventQuery filters the activity log.
type EventQuery struct {
	Before   int64 // cursor: return events with id < Before
	Limit    int
	Kinds    []string
	DeviceID int64
}

// ListEvents returns events newest first.
func (s *Store) ListEvents(ctx context.Context, q EventQuery) ([]Event, error) {
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	var where []string
	var args []any
	if q.Before > 0 {
		where = append(where, "id < ?")
		args = append(args, q.Before)
	}
	if q.DeviceID > 0 {
		where = append(where, "device_id = ?")
		args = append(args, q.DeviceID)
	}
	if len(q.Kinds) > 0 {
		where = append(where, "kind IN (?"+strings.Repeat(",?", len(q.Kinds)-1)+")")
		for _, k := range q.Kinds {
			args = append(args, k)
		}
	}
	sqlq := `SELECT id, ts, kind, severity, IFNULL(device_id, 0), data FROM events`
	if len(where) > 0 {
		sqlq += " WHERE " + strings.Join(where, " AND ")
	}
	sqlq += " ORDER BY id DESC LIMIT ?"
	args = append(args, q.Limit)
	rows, err := s.db.QueryContext(ctx, sqlq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var data string
		if err := rows.Scan(&e.ID, &e.TS, &e.Kind, &e.Severity, &e.DeviceID, &data); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Outage kinds.
const (
	OutageInternet = "internet" // router answers, the internet does not
	OutageRouter   = "router"   // the router itself does not answer
	OutageDNS      = "dns"      // addresses work, name resolution does not
)

// Outage is a period without internet connectivity.
type Outage struct {
	ID    int64  `json:"id"`
	Start int64  `json:"start"`
	End   int64  `json:"end"` // 0 = ongoing
	Kind  string `json:"kind"`
}

// OpenOutage records the start of an outage of the given kind.
func (s *Store) OpenOutage(ctx context.Context, ts int64, kind string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO internet_outages (started_at, kind) VALUES (?, ?)`, ts, kind)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CloseOutages ends every open connectivity outage (router or internet) at ts.
func (s *Store) CloseOutages(ctx context.Context, ts int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE internet_outages SET ended_at = ? WHERE ended_at IS NULL AND kind <> 'dns'`, ts)
	return err
}

// CloseDNSOutages ends every open DNS outage at ts.
func (s *Store) CloseDNSOutages(ctx context.Context, ts int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE internet_outages SET ended_at = ? WHERE ended_at IS NULL AND kind = 'dns'`, ts)
	return err
}

// ListOutages returns connectivity outages (no DNS) that started after
// since, newest first.
func (s *Store) ListOutages(ctx context.Context, since int64) ([]Outage, error) {
	return s.queryOutages(ctx, `SELECT id, started_at, IFNULL(ended_at, 0), kind FROM internet_outages
		WHERE kind <> 'dns' AND (started_at >= ? OR ended_at IS NULL) ORDER BY started_at DESC LIMIT 200`, since)
}

// OutagesBetween returns every outage of any kind overlapping [from, to],
// oldest first (provider report).
func (s *Store) OutagesBetween(ctx context.Context, from, to int64) ([]Outage, error) {
	return s.queryOutages(ctx, `SELECT id, started_at, IFNULL(ended_at, 0), kind FROM internet_outages
		WHERE started_at <= ? AND (ended_at IS NULL OR ended_at >= ?) ORDER BY started_at LIMIT 10000`, to, from)
}

func (s *Store) queryOutages(ctx context.Context, q string, args ...any) ([]Outage, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Outage{}
	for rows.Next() {
		var o Outage
		if err := rows.Scan(&o.ID, &o.Start, &o.End, &o.Kind); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Latency is one 5-minute bucket of internet checks.
type Latency struct {
	TS      int64 `json:"t"`      // bucket start
	Median  int   `json:"median"` // ms of the successful checks, -1 if none
	Max     int   `json:"max"`    // ms, -1 if none
	Lost    int   `json:"lost"`   // failed checks while the router answered
	Samples int   `json:"samples"`
}

// AddLatency stores buckets (replacing a bucket with the same start).
func (s *Store) AddLatency(ctx context.Context, ls ...Latency) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, l := range ls {
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO internet_latency (ts, median_ms, max_ms, lost, samples)
				VALUES (?, ?, ?, ?, ?)`, l.TS, l.Median, l.Max, l.Lost, l.Samples); err != nil {
				return err
			}
		}
		return nil
	})
}

// LatencyBetween returns the buckets in [from, to), oldest first.
func (s *Store) LatencyBetween(ctx context.Context, from, to int64) ([]Latency, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts, median_ms, max_ms, lost, samples FROM internet_latency
		WHERE ts >= ? AND ts < ? ORDER BY ts`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Latency{}
	for rows.Next() {
		var l Latency
		if err := rows.Scan(&l.TS, &l.Median, &l.Max, &l.Lost, &l.Samples); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Gap is a period during which Netward was not running.
type Gap struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// AddGap records a monitoring gap.
func (s *Store) AddGap(ctx context.Context, start, end int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO monitor_gaps (started_at, ended_at) VALUES (?, ?)`, start, end)
	return err
}

// ListGaps returns gaps overlapping [from, to].
func (s *Store) ListGaps(ctx context.Context, from, to int64) ([]Gap, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT started_at, ended_at FROM monitor_gaps WHERE started_at <= ? AND ended_at >= ?
		ORDER BY started_at`, to, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Gap{}
	for rows.Next() {
		var g Gap
		if err := rows.Scan(&g.Start, &g.End); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// CloseAllPresence ends every open presence span at the device's last_seen
// (capped at ts). Used after a monitoring gap.
// It returns the devices that were online, so the caller can avoid treating
// their return as news.
func (s *Store) CloseAllPresence(ctx context.Context, ts int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM devices WHERE online = 1`)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if _, err := s.db.ExecContext(ctx, `UPDATE presence SET ended_at = MAX(started_at,
		MIN(?, (SELECT last_seen FROM devices WHERE devices.id = presence.device_id))) WHERE ended_at IS NULL`, ts); err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE devices SET online = 0 WHERE online = 1`)
	return ids, err
}

// Backup writes a consistent copy of the database to path (VACUUM INTO).
func (s *Store) Backup(ctx context.Context, path string) error {
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}

// Finding is a result of the security check.
type Finding struct {
	ID        int64           `json:"id"`
	CheckID   string          `json:"check"`
	DeviceID  int64           `json:"deviceId,omitempty"`
	Severity  string          `json:"severity"`
	Data      json.RawMessage `json:"data"`
	FirstSeen int64           `json:"firstSeen"`
	LastSeen  int64           `json:"lastSeen"`
	Dismissed bool            `json:"dismissed"`
}

// UpsertFinding inserts or refreshes a finding and reports whether it is new.
func (s *Store) UpsertFinding(ctx context.Context, f Finding) (bool, error) {
	prev, err := s.UpsertFindingPrev(ctx, f)
	return err == nil && prev == "", err
}

// UpsertFindingPrev is UpsertFinding returning the severity the finding had
// before ("" when it is new), so callers can alert when it got worse.
func (s *Store) UpsertFindingPrev(ctx context.Context, f Finding) (string, error) {
	if len(f.Data) == 0 {
		f.Data = json.RawMessage("{}")
	}
	var dev any
	if f.DeviceID != 0 {
		dev = f.DeviceID
	}
	var existing int64
	var prev string
	err := s.db.QueryRowContext(ctx, `SELECT id, severity FROM security_findings WHERE check_id = ? AND IFNULL(device_id, 0) = ? AND data = ?`,
		f.CheckID, f.DeviceID, string(f.Data)).Scan(&existing, &prev)
	switch {
	case err == nil:
		_, err = s.db.ExecContext(ctx, `UPDATE security_findings SET last_seen = ?, severity = ? WHERE id = ?`, f.LastSeen, f.Severity, existing)
		if prev == "" {
			prev = "info"
		}
		return prev, err
	case errors.Is(err, sql.ErrNoRows):
		_, err = s.db.ExecContext(ctx, `INSERT INTO security_findings (check_id, device_id, severity, data, first_seen, last_seen)
			VALUES (?, ?, ?, ?, ?, ?)`, f.CheckID, dev, f.Severity, string(f.Data), f.LastSeen, f.LastSeen)
		return "", err
	default:
		return "", err
	}
}

// PruneFindings deletes findings the run that started at runStart did not
// confirm. Device findings are only pruned for devices the run checked (an
// offline device proves nothing), and dismissed findings are kept for 30 days
// so a port that is briefly unreachable does not resurrect them. Only the
// given check IDs are touched: each job prunes its own findings.
func (s *Store) PruneFindings(ctx context.Context, runStart int64, checked []int64, checks []string) error {
	ids, _ := json.Marshal(checked)
	cids, _ := json.Marshal(checks)
	_, err := s.db.ExecContext(ctx, `DELETE FROM security_findings
		WHERE last_seen < ?
		  AND check_id IN (SELECT value FROM json_each(?))
		  AND (device_id IS NULL OR device_id IN (SELECT value FROM json_each(?)))
		  AND (dismissed = 0 OR last_seen < ?)`,
		runStart, string(cids), string(ids), runStart-30*24*3600*1000)
	return err
}

// ListFindings returns current findings.
func (s *Store) ListFindings(ctx context.Context) ([]Finding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, check_id, IFNULL(device_id, 0), severity, data, first_seen, last_seen, dismissed
		FROM security_findings ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, first_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Finding{}
	for rows.Next() {
		var f Finding
		var data string
		if err := rows.Scan(&f.ID, &f.CheckID, &f.DeviceID, &f.Severity, &data, &f.FirstSeen, &f.LastSeen, &f.Dismissed); err != nil {
			return nil, err
		}
		f.Data = json.RawMessage(data)
		out = append(out, f)
	}
	return out, rows.Err()
}

// SetFindingDismissed hides or shows a finding.
func (s *Store) SetFindingDismissed(ctx context.Context, id int64, dismissed bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE security_findings SET dismissed = ? WHERE id = ?`, b2i(dismissed), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Prune deletes history older than the retention cut-off (Unix ms).
func (s *Store) Prune(ctx context.Context, before int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM events WHERE ts < ?`,
			`DELETE FROM presence WHERE ended_at IS NOT NULL AND ended_at < ?`,
			`DELETE FROM internet_outages WHERE ended_at IS NOT NULL AND ended_at < ?`,
			`DELETE FROM monitor_gaps WHERE ended_at < ?`,
			`DELETE FROM internet_latency WHERE ts < ?`,
			`DELETE FROM line_samples WHERE ts < ?`,
			`DELETE FROM sessions WHERE expires_at < ?`,
		} {
			arg := before
			if strings.Contains(q, "sessions") {
				arg = Now()
			}
			if _, err := tx.ExecContext(ctx, q, arg); err != nil {
				return err
			}
		}
		return nil
	})
}

// LineSample is the connection state at one moment (stored on change).
type LineSample struct {
	TS          int64  `json:"t"`
	Access      string `json:"access"`
	Up          bool   `json:"up"`
	DownKbps    int    `json:"down"`
	UpKbps      int    `json:"upstream"`
	DownMaxKbps int    `json:"downMax,omitempty"`
	UpMaxKbps   int    `json:"upMax,omitempty"`
	Reconnect   bool   `json:"reconnect,omitempty"`
}

// AddLineSamples stores connection samples.
func (s *Store) AddLineSamples(ctx context.Context, ls ...LineSample) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, l := range ls {
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO line_samples (ts, access, link_up, down_kbps, up_kbps,
				down_max_kbps, up_max_kbps, reconnect) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				l.TS, l.Access, l.Up, l.DownKbps, l.UpKbps, l.DownMaxKbps, l.UpMaxKbps, l.Reconnect); err != nil {
				return err
			}
		}
		return nil
	})
}

// LineSamplesBetween returns the samples in [from, to] plus the last one
// before from (it describes the state at from), oldest first.
func (s *Store) LineSamplesBetween(ctx context.Context, from, to int64) ([]LineSample, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts, access, link_up, down_kbps, up_kbps, down_max_kbps, up_max_kbps, reconnect
		FROM line_samples WHERE ts >= (SELECT IFNULL(MAX(ts), 0) FROM line_samples WHERE ts <= ?) AND ts <= ? ORDER BY ts`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LineSample{}
	for rows.Next() {
		var l LineSample
		if err := rows.Scan(&l.TS, &l.Access, &l.Up, &l.DownKbps, &l.UpKbps, &l.DownMaxKbps, &l.UpMaxKbps, &l.Reconnect); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
