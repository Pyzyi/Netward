package store

import (
	"context"
	"database/sql"
	"errors"
)

// FloorPlan is an uploaded floor plan for the Wi-Fi map (docs/10, F4).
type FloorPlan struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	CreatedAt int64  `json:"createdAt"`
	Points    int    `json:"points"`
}

// WifiPoint is one measurement on a floor plan.
type WifiPoint struct {
	ID        int64   `json:"id"`
	PlanID    int64   `json:"planId"`
	X         float64 `json:"x"` // 0..1 from the left
	Y         float64 `json:"y"` // 0..1 from the top
	Room      string  `json:"room"`
	DownMbps  float64 `json:"downMbps"`
	UpMbps    float64 `json:"upMbps"`
	LatencyMs float64 `json:"latencyMs"`
	JitterMs  float64 `json:"jitterMs"`
	DeviceID  int64   `json:"deviceId,omitempty"`
	Band      string  `json:"band,omitempty"`
	SignalDBm int     `json:"signalDbm,omitempty"`
	AP        string  `json:"ap,omitempty"`
	TS        int64   `json:"ts"`
}

// MaxFloorPlans bounds the stored plans (one per floor is plenty).
const MaxFloorPlans = 10

// MaxWifiPoints bounds the measurements per plan.
const MaxWifiPoints = 200

// AddFloorPlan stores a plan image.
func (s *Store) AddFloorPlan(ctx context.Context, p FloorPlan, image []byte) (FloorPlan, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM floor_plans`).Scan(&n); err != nil {
		return p, err
	}
	if n >= MaxFloorPlans {
		return p, ErrLimit
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO floor_plans (name, mime, image, width, height, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		p.Name, p.Mime, image, p.Width, p.Height, p.CreatedAt)
	if err != nil {
		return p, err
	}
	p.ID, err = res.LastInsertId()
	return p, err
}

// FloorPlans lists the plans without their images.
func (s *Store) FloorPlans(ctx context.Context) ([]FloorPlan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id, p.name, p.mime, p.width, p.height, p.created_at,
		(SELECT COUNT(*) FROM wifi_points w WHERE w.plan_id = p.id) FROM floor_plans p ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FloorPlan{}
	for rows.Next() {
		var p FloorPlan
		if err := rows.Scan(&p.ID, &p.Name, &p.Mime, &p.Width, &p.Height, &p.CreatedAt, &p.Points); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// FloorPlanImage returns the image of a plan and its media type.
func (s *Store) FloorPlanImage(ctx context.Context, id int64) ([]byte, string, error) {
	var img []byte
	var mime string
	err := s.db.QueryRowContext(ctx, `SELECT image, mime FROM floor_plans WHERE id = ?`, id).Scan(&img, &mime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return img, mime, err
}

// RenameFloorPlan changes the name of a plan.
func (s *Store) RenameFloorPlan(ctx context.Context, id int64, name string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE floor_plans SET name = ? WHERE id = ?`, name, id))
}

// DeleteFloorPlan removes a plan with its measurements.
func (s *Store) DeleteFloorPlan(ctx context.Context, id int64) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM floor_plans WHERE id = ?`, id))
}

// AddWifiPoint stores a measurement.
func (s *Store) AddWifiPoint(ctx context.Context, p WifiPoint) (WifiPoint, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wifi_points WHERE plan_id = ?`, p.PlanID).Scan(&n); err != nil {
		return p, err
	}
	if n >= MaxWifiPoints {
		return p, ErrLimit
	}
	var dev any
	if p.DeviceID != 0 {
		dev = p.DeviceID
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO wifi_points (plan_id, x, y, room, down_mbps, up_mbps, latency_ms, jitter_ms,
		device_id, band, signal_dbm, ap, ts) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.PlanID, p.X, p.Y, p.Room, p.DownMbps, p.UpMbps, p.LatencyMs, p.JitterMs, dev, p.Band, p.SignalDBm, p.AP, p.TS)
	if err != nil {
		return p, err
	}
	p.ID, err = res.LastInsertId()
	return p, err
}

// WifiPoints returns the measurements of a plan, oldest first.
func (s *Store) WifiPoints(ctx context.Context, planID int64) ([]WifiPoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, plan_id, x, y, room, down_mbps, up_mbps, latency_ms, jitter_ms,
		IFNULL(device_id, 0), band, signal_dbm, ap, ts FROM wifi_points WHERE plan_id = ? ORDER BY ts, id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WifiPoint{}
	for rows.Next() {
		var p WifiPoint
		if err := rows.Scan(&p.ID, &p.PlanID, &p.X, &p.Y, &p.Room, &p.DownMbps, &p.UpMbps, &p.LatencyMs, &p.JitterMs,
			&p.DeviceID, &p.Band, &p.SignalDBm, &p.AP, &p.TS); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteWifiPoint removes one measurement of a plan.
func (s *Store) DeleteWifiPoint(ctx context.Context, planID, id int64) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM wifi_points WHERE id = ? AND plan_id = ?`, id, planID))
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
