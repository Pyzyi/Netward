package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Device is one physical or virtual network device, identified by its MAC.
type Device struct {
	ID              int64  `json:"id"`
	MAC             string `json:"mac"`
	IP              string `json:"ip"`
	Name            string `json:"name"`     // set by the user
	Hostname        string `json:"hostname"` // best name discovered on the network
	Vendor          string `json:"vendor"`
	VendorOfficial  string `json:"vendorOfficial"`
	Model           string `json:"model"`
	OS              string `json:"os"`
	Type            string `json:"type"` // effective type (override or detected)
	DetectedType    string `json:"detectedType"`
	TypeConfidence  int    `json:"typeConfidence"`
	TypeOverride    string `json:"typeOverride"`
	Location        string `json:"location"`
	Notes           string `json:"notes"`
	Known           bool   `json:"known"`
	Favorite        bool   `json:"favorite"`
	Watch           bool   `json:"watch"`
	Randomized      bool   `json:"randomized"`
	Online          bool   `json:"online"`
	IsGateway       bool   `json:"isGateway"`
	IsSelf          bool   `json:"isSelf"`
	Interface       string `json:"interface"`
	DHCPFingerprint string `json:"dhcpFingerprint"`
	DHCPVendor      string `json:"dhcpVendor"`
	FirstSeen       int64  `json:"firstSeen"`
	LastSeen        int64  `json:"lastSeen"`
	EnrichedAt      int64  `json:"enrichedAt"`
	PortsScannedAt  int64  `json:"portsScannedAt"`
	New             bool   `json:"new"` // first seen after the learning phase and not yet acknowledged
}

const deviceCols = `id, mac, ip, name, hostname, vendor, vendor_official, model, os, type, type_confidence,
	type_override, location, notes, known, favorite, watch, randomized, online, is_gateway, is_self,
	interface, dhcp_fingerprint, dhcp_vendor, first_seen, last_seen, enriched_at, ports_scanned_at`

type scanner interface{ Scan(...any) error }

func scanDevice(r scanner) (Device, error) {
	var d Device
	err := r.Scan(&d.ID, &d.MAC, &d.IP, &d.Name, &d.Hostname, &d.Vendor, &d.VendorOfficial, &d.Model, &d.OS,
		&d.DetectedType, &d.TypeConfidence, &d.TypeOverride, &d.Location, &d.Notes, &d.Known, &d.Favorite,
		&d.Watch, &d.Randomized, &d.Online, &d.IsGateway, &d.IsSelf, &d.Interface, &d.DHCPFingerprint,
		&d.DHCPVendor, &d.FirstSeen, &d.LastSeen, &d.EnrichedAt, &d.PortsScannedAt)
	d.Type = d.DetectedType
	if d.TypeOverride != "" {
		d.Type = d.TypeOverride
	}
	return d, err
}

// ListDevices returns all devices.
func (s *Store) ListDevices(ctx context.Context) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+deviceCols+` FROM devices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDevice returns a device by ID.
func (s *Store) GetDevice(ctx context.Context, id int64) (Device, error) {
	d, err := scanDevice(s.db.QueryRowContext(ctx, `SELECT `+deviceCols+` FROM devices WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// InsertDevice creates a device and returns its ID.
func (s *Store) InsertDevice(ctx context.Context, d *Device) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO devices (mac, ip, hostname, vendor, vendor_official, randomized,
		online, is_gateway, is_self, interface, first_seen, last_seen, type, type_confidence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.MAC, d.IP, d.Hostname, d.Vendor, d.VendorOfficial, b2i(d.Randomized), b2i(d.Online), b2i(d.IsGateway),
		b2i(d.IsSelf), d.Interface, d.FirstSeen, d.LastSeen, nz(d.DetectedType, "unknown"), d.TypeConfidence)
	if err != nil {
		return err
	}
	d.ID, err = res.LastInsertId()
	return err
}

func nz(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// SaveDiscovered persists the fields that discovery and the presence logic own.
func (s *Store) SaveDiscovered(ctx context.Context, d *Device) error {
	_, err := s.db.ExecContext(ctx, `UPDATE devices SET ip = ?, hostname = ?, vendor = ?, vendor_official = ?,
		model = ?, os = ?, type = ?, type_confidence = ?, randomized = ?, online = ?, is_gateway = ?, is_self = ?,
		interface = ?, dhcp_fingerprint = ?, dhcp_vendor = ?, last_seen = MAX(last_seen, ?), enriched_at = ?,
		ports_scanned_at = ? WHERE id = ?`,
		d.IP, d.Hostname, d.Vendor, d.VendorOfficial, d.Model, d.OS, nz(d.DetectedType, "unknown"), d.TypeConfidence,
		b2i(d.Randomized), b2i(d.Online), b2i(d.IsGateway), b2i(d.IsSelf), d.Interface, d.DHCPFingerprint,
		d.DHCPVendor, d.LastSeen, d.EnrichedAt, d.PortsScannedAt, d.ID)
	return err
}

// TouchLastSeen updates last_seen for many devices in one transaction.
func (s *Store) TouchLastSeen(ctx context.Context, seen map[int64]int64) error {
	if len(seen) == 0 {
		return nil
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		st, err := tx.PrepareContext(ctx, `UPDATE devices SET last_seen = MAX(last_seen, ?) WHERE id = ?`)
		if err != nil {
			return err
		}
		defer st.Close()
		for id, ts := range seen {
			if _, err := st.ExecContext(ctx, ts, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// DevicePatch holds user-editable fields; nil means "unchanged".
type DevicePatch struct {
	Name         *string `json:"name"`
	TypeOverride *string `json:"type"`
	Location     *string `json:"location"`
	Notes        *string `json:"notes"`
	Known        *bool   `json:"known"`
	Favorite     *bool   `json:"favorite"`
	Watch        *bool   `json:"watch"`
}

// PatchDevice applies user edits.
func (s *Store) PatchDevice(ctx context.Context, id int64, p DevicePatch) error {
	var sets []string
	var args []any
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if p.Name != nil {
		add("name", *p.Name)
	}
	if p.TypeOverride != nil {
		add("type_override", *p.TypeOverride)
	}
	if p.Location != nil {
		add("location", *p.Location)
	}
	if p.Notes != nil {
		add("notes", *p.Notes)
	}
	if p.Known != nil {
		add("known", b2i(*p.Known))
	}
	if p.Favorite != nil {
		add("favorite", b2i(*p.Favorite))
	}
	if p.Watch != nil {
		add("watch", b2i(*p.Watch))
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	res, err := s.db.ExecContext(ctx, `UPDATE devices SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AcknowledgeAll marks every device as known and returns the affected IDs.
func (s *Store) AcknowledgeAll(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `UPDATE devices SET known = 1 WHERE known = 0 RETURNING id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteDevice removes a device and everything attached to it.
func (s *Store) DeleteDevice(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM devices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Name is a name a device announced through one discovery source.
type Name struct {
	Source    string `json:"source"`
	Name      string `json:"name"`
	UpdatedAt int64  `json:"updatedAt"`
}

// UpsertName stores the name a source reported; an empty name deletes it.
func (s *Store) UpsertName(ctx context.Context, id int64, source, name string) error {
	if name == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM device_names WHERE device_id = ? AND source = ?`, id, source)
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO device_names (device_id, source, name, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (device_id, source) DO UPDATE SET name = excluded.name, updated_at = excluded.updated_at`,
		id, source, name, Now())
	return err
}

// ListNames returns all discovered names of a device.
func (s *Store) ListNames(ctx context.Context, id int64) ([]Name, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source, name, updated_at FROM device_names WHERE device_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Name
	for rows.Next() {
		var n Name
		if err := rows.Scan(&n.Source, &n.Name, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Service is something a device announced via mDNS or SSDP.
type Service struct {
	Source  string `json:"source"`
	Service string `json:"service"`
	Detail  string `json:"detail"`
}

// ReplaceServices replaces all services of one source for a device.
func (s *Store) ReplaceServices(ctx context.Context, id int64, source string, svcs []Service) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM device_services WHERE device_id = ? AND source = ?`, id, source); err != nil {
			return err
		}
		now := Now()
		for _, sv := range svcs {
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO device_services (device_id, source, service, detail, updated_at)
				VALUES (?, ?, ?, ?, ?)`, id, source, sv.Service, sv.Detail, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListServices returns all services of a device.
func (s *Store) ListServices(ctx context.Context, id int64) ([]Service, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source, service, detail FROM device_services WHERE device_id = ? ORDER BY source, service`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Service
	for rows.Next() {
		var sv Service
		if err := rows.Scan(&sv.Source, &sv.Service, &sv.Detail); err != nil {
			return nil, err
		}
		out = append(out, sv)
	}
	return out, rows.Err()
}

// Port is an open TCP port.
type Port struct {
	Port      int    `json:"port"`
	Proto     string `json:"proto"`
	Service   string `json:"service"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
}

// ReplacePorts stores the result of a full port scan.
func (s *Store) ReplacePorts(ctx context.Context, id int64, ports []Port, scannedAt int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		// Upsert first so ports that stay open keep their first_seen, then
		// drop everything this scan did not confirm.
		for _, p := range ports {
			if _, err := tx.ExecContext(ctx, `INSERT INTO device_ports (device_id, port, proto, service, first_seen, last_seen)
				VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (device_id, port, proto) DO UPDATE SET last_seen = excluded.last_seen,
				service = excluded.service`, id, p.Port, nz(p.Proto, "tcp"), p.Service, scannedAt, scannedAt); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM device_ports WHERE device_id = ? AND last_seen < ?`, id, scannedAt)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE devices SET ports_scanned_at = ? WHERE id = ?`, scannedAt, id)
		return err
	})
}

// ListPorts returns open ports of a device, or of all devices when id is 0.
func (s *Store) ListPorts(ctx context.Context, id int64) (map[int64][]Port, error) {
	q := `SELECT device_id, port, proto, service, first_seen, last_seen FROM device_ports`
	var args []any
	if id != 0 {
		q += ` WHERE device_id = ?`
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY port`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]Port{}
	for rows.Next() {
		var did int64
		var p Port
		if err := rows.Scan(&did, &p.Port, &p.Proto, &p.Service, &p.FirstSeen, &p.LastSeen); err != nil {
			return nil, err
		}
		out[did] = append(out[did], p)
	}
	return out, rows.Err()
}

// IPRecord is one address a device has used.
type IPRecord struct {
	IP        string `json:"ip"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
}

// TouchIP records that a device used ip at ts.
func (s *Store) TouchIP(ctx context.Context, id int64, ip string, ts int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO ip_history (device_id, ip, first_seen, last_seen) VALUES (?, ?, ?, ?)
		ON CONFLICT (device_id, ip) DO UPDATE SET last_seen = MAX(last_seen, excluded.last_seen)`, id, ip, ts, ts)
	return err
}

// ListIPHistory returns the addresses a device has used, newest first.
func (s *Store) ListIPHistory(ctx context.Context, id int64) ([]IPRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ip, first_seen, last_seen FROM ip_history WHERE device_id = ? ORDER BY last_seen DESC LIMIT 20`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IPRecord
	for rows.Next() {
		var r IPRecord
		if err := rows.Scan(&r.IP, &r.FirstSeen, &r.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
