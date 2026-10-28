package store

import (
	"context"
	"encoding/json"
)

// LogEntry is a milestone in the history of a device (docs/10, F1).
type LogEntry struct {
	ID   int64           `json:"id"`
	TS   int64           `json:"ts"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// deviceLogKeep bounds the history per device; a flapping device must not
// grow the database without limit.
const deviceLogKeep = 500

// AddDeviceLog appends a milestone to the history of a device.
func (s *Store) AddDeviceLog(ctx context.Context, deviceID, ts int64, kind string, data json.RawMessage) error {
	if len(data) == 0 {
		data = json.RawMessage("{}")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO device_log (device_id, ts, kind, data) VALUES (?, ?, ?, ?)`,
		deviceID, ts, kind, string(data))
	return err
}

// DeviceLog returns the history of a device, newest first.
func (s *Store) DeviceLog(ctx context.Context, deviceID int64, limit int) ([]LogEntry, error) {
	if limit <= 0 || limit > deviceLogKeep {
		limit = deviceLogKeep
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, kind, data FROM device_log WHERE device_id = ?
		ORDER BY ts DESC, id DESC LIMIT ?`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogEntry{}
	for rows.Next() {
		var e LogEntry
		var data string
		if err := rows.Scan(&e.ID, &e.TS, &e.Kind, &data); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	return out, rows.Err()
}

// TrimDeviceLog keeps the newest deviceLogKeep entries per device.
func (s *Store) TrimDeviceLog(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM device_log WHERE id IN (
		SELECT id FROM (SELECT id, ROW_NUMBER() OVER (PARTITION BY device_id ORDER BY ts DESC, id DESC) AS n FROM device_log)
		WHERE n > ?)`, deviceLogKeep)
	return err
}
