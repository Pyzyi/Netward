package store

import (
	"context"
	"database/sql"
	"errors"
)

// Person groups devices of one member of the household (docs/09, F4).
type Person struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	TrackTime bool           `json:"trackTime"`     // time at home per day (opt-in)
	ConsentAt int64          `json:"consentAt"`     // when the user confirmed the person knows
	HA        bool           `json:"homeAssistant"` // device_tracker in Home Assistant (opt-in, needs consent too)
	CreatedAt int64          `json:"createdAt"`
	Devices   []PersonDevice `json:"devices"`
}

// PersonDevice assigns a device to a person.
type PersonDevice struct {
	DeviceID int64 `json:"deviceId"`
	Presence bool  `json:"presence"` // counts for "at home"
}

// MaxPersons bounds the household.
const MaxPersons = 50

// ListPersons returns all persons with their devices, oldest first.
func (s *Store) ListPersons(ctx context.Context) ([]Person, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, track_time, consent_at, home_assist, created_at FROM persons ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Person{}
	idx := map[int64]int{}
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name, &p.TrackTime, &p.ConsentAt, &p.HA, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Devices = []PersonDevice{}
		idx[p.ID] = len(out)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	drows, err := s.db.QueryContext(ctx, `SELECT person_id, device_id, presence FROM person_devices ORDER BY device_id`)
	if err != nil {
		return nil, err
	}
	defer drows.Close()
	for drows.Next() {
		var pid int64
		var d PersonDevice
		if err := drows.Scan(&pid, &d.DeviceID, &d.Presence); err != nil {
			return nil, err
		}
		if i, ok := idx[pid]; ok {
			out[i].Devices = append(out[i].Devices, d)
		}
	}
	return out, drows.Err()
}

// ErrLimit means a bound was reached.
var ErrLimit = errors.New("store: limit reached")

// CreatePerson adds a person.
func (s *Store) CreatePerson(ctx context.Context, name string) (Person, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM persons`).Scan(&n); err != nil {
		return Person{}, err
	}
	if n >= MaxPersons {
		return Person{}, ErrLimit
	}
	p := Person{Name: name, CreatedAt: Now(), Devices: []PersonDevice{}}
	res, err := s.db.ExecContext(ctx, `INSERT INTO persons (name, created_at) VALUES (?, ?)`, name, p.CreatedAt)
	if err != nil {
		return Person{}, err
	}
	p.ID, err = res.LastInsertId()
	return p, err
}

// UpdatePerson stores name and options.
func (s *Store) UpdatePerson(ctx context.Context, p Person) error {
	res, err := s.db.ExecContext(ctx, `UPDATE persons SET name = ?, track_time = ?, consent_at = ?, home_assist = ? WHERE id = ?`,
		p.Name, p.TrackTime, p.ConsentAt, p.HA, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePerson removes a person and its assignments.
func (s *Store) DeletePerson(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM persons WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPersonDevices replaces the devices of a person. A device that belonged
// to someone else moves to this person.
func (s *Store) SetPersonDevices(ctx context.Context, personID int64, devs []PersonDevice) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var one int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM persons WHERE id = ?`, personID).Scan(&one); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM person_devices WHERE person_id = ?`, personID); err != nil {
			return err
		}
		for _, d := range devs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO person_devices (device_id, person_id, presence) VALUES (?, ?, ?)
				ON CONFLICT (device_id) DO UPDATE SET person_id = excluded.person_id, presence = excluded.presence`,
				d.DeviceID, personID, d.Presence); err != nil {
				return err
			}
		}
		return nil
	})
}
