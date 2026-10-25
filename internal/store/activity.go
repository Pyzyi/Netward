package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

// Outage is a period without internet connectivity.
type Outage struct {
	ID    int64 `json:"id"`
	Start int64 `json:"start"`
	End   int64 `json:"end"` // 0 = ongoing
}

// OpenOutage records the start of an internet outage.
func (s *Store) OpenOutage(ctx context.Context, ts int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO internet_outages (started_at) VALUES (?)`, ts)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CloseOutages ends every open outage at ts.
func (s *Store) CloseOutages(ctx context.Context, ts int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE internet_outages SET ended_at = ? WHERE ended_at IS NULL`, ts)
	return err
}

// ListOutages returns outages that started after since, newest first.
func (s *Store) ListOutages(ctx context.Context, since int64) ([]Outage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, started_at, IFNULL(ended_at, 0) FROM internet_outages
		WHERE started_at >= ? OR ended_at IS NULL ORDER BY started_at DESC LIMIT 200`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Outage{}
	for rows.Next() {
		var o Outage
		if err := rows.Scan(&o.ID, &o.Start, &o.End); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Gap is a period during which Netzwart was not running.
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
func (s *Store) CloseAllPresence(ctx context.Context, ts int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE presence SET ended_at = MAX(started_at,
		MIN(?, (SELECT last_seen FROM devices WHERE devices.id = presence.device_id))) WHERE ended_at IS NULL`, ts)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE devices SET online = 0 WHERE online = 1`)
	return err
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
	if len(f.Data) == 0 {
		f.Data = json.RawMessage("{}")
	}
	var dev any
	if f.DeviceID != 0 {
		dev = f.DeviceID
	}
	var existing int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM security_findings WHERE check_id = ? AND IFNULL(device_id, 0) = ? AND data = ?`,
		f.CheckID, f.DeviceID, string(f.Data)).Scan(&existing)
	switch {
	case err == nil:
		_, err = s.db.ExecContext(ctx, `UPDATE security_findings SET last_seen = ?, severity = ? WHERE id = ?`, f.LastSeen, f.Severity, existing)
		return false, err
	case errors.Is(err, sql.ErrNoRows):
		_, err = s.db.ExecContext(ctx, `INSERT INTO security_findings (check_id, device_id, severity, data, first_seen, last_seen)
			VALUES (?, ?, ?, ?, ?, ?)`, f.CheckID, dev, f.Severity, string(f.Data), f.LastSeen, f.LastSeen)
		return err == nil, err
	default:
		return false, err
	}
}

// PruneFindings deletes findings not confirmed by the run that started at runStart.
func (s *Store) PruneFindings(ctx context.Context, runStart int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM security_findings WHERE last_seen < ?`, runStart)
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
