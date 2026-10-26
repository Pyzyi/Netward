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
	Ident           string `json:"-"`
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
	MACKind         string `json:"macKind"` // universal | random | virtual
	Proxy           bool   `json:"proxy"`
	ViaProxy        bool   `json:"viaProxy"`
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
	// Software is the short software verdict, filled in by the monitor.
	Software *SoftwareSummary `json:"software,omitempty"`
}

// SoftwareSummary is the part of the software assessment lists need.
type SoftwareSummary struct {
	Status  string `json:"status"`  // supported | aging | outdated | update | unknown
	Product string `json:"product"` // "windows"
	Name    string `json:"name"`    // "Windows 10 22H2"
}

const deviceCols = `id, ident, mac, ip, name, hostname, vendor, vendor_official, model, os, type, type_confidence,
	type_override, location, notes, known, favorite, watch, mac_kind, proxy, via_proxy, online, is_gateway, is_self,
	interface, dhcp_fingerprint, dhcp_vendor, first_seen, last_seen, enriched_at, ports_scanned_at`

type scanner interface{ Scan(...any) error }

func scanDevice(r scanner) (Device, error) {
	var d Device
	err := r.Scan(&d.ID, &d.Ident, &d.MAC, &d.IP, &d.Name, &d.Hostname, &d.Vendor, &d.VendorOfficial, &d.Model, &d.OS,
		&d.DetectedType, &d.TypeConfidence, &d.TypeOverride, &d.Location, &d.Notes, &d.Known, &d.Favorite,
		&d.Watch, &d.MACKind, &d.Proxy, &d.ViaProxy, &d.Online, &d.IsGateway, &d.IsSelf, &d.Interface, &d.DHCPFingerprint,
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
	if d.Ident == "" {
		d.Ident = d.MAC
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO devices (ident, mac, ip, hostname, vendor, vendor_official, model, os,
		mac_kind, proxy, via_proxy, online, is_gateway, is_self, interface, dhcp_fingerprint, dhcp_vendor, first_seen,
		last_seen, type, type_confidence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.Ident, d.MAC, d.IP, d.Hostname, d.Vendor, d.VendorOfficial, d.Model, d.OS, nz(d.MACKind, "universal"),
		b2i(d.Proxy), b2i(d.ViaProxy), b2i(d.Online), b2i(d.IsGateway), b2i(d.IsSelf), d.Interface, d.DHCPFingerprint,
		d.DHCPVendor, d.FirstSeen, d.LastSeen, nz(d.DetectedType, "unknown"), d.TypeConfidence)
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
		model = ?, os = ?, type = ?, type_confidence = ?, mac_kind = ?, proxy = ?, via_proxy = ?, online = ?,
		is_gateway = ?, is_self = ?, interface = ?, dhcp_fingerprint = ?, dhcp_vendor = ?, last_seen = MAX(last_seen, ?),
		enriched_at = ? WHERE id = ?`,
		d.IP, d.Hostname, d.Vendor, d.VendorOfficial, d.Model, d.OS, nz(d.DetectedType, "unknown"), d.TypeConfidence,
		nz(d.MACKind, "universal"), b2i(d.Proxy), b2i(d.ViaProxy), b2i(d.Online), b2i(d.IsGateway), b2i(d.IsSelf),
		d.Interface, d.DHCPFingerprint, d.DHCPVendor, d.LastSeen, d.EnrichedAt, d.ID)
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

// Address is one IPv4 address a device has used.
type Address struct {
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
}

// TouchAddress records that a device used ip at ts.
func (s *Store) TouchAddress(ctx context.Context, id int64, ip, iface string, ts int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO device_addresses (device_id, ip, interface, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT (device_id, ip) DO UPDATE SET last_seen = MAX(last_seen, excluded.last_seen),
		interface = excluded.interface`, id, ip, iface, ts, ts)
	return err
}

// ListAddresses returns the addresses a device has used, newest first.
func (s *Store) ListAddresses(ctx context.Context, id int64) ([]Address, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ip, interface, first_seen, last_seen FROM device_addresses
		WHERE device_id = ? ORDER BY last_seen DESC LIMIT 20`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Address{}
	for rows.Next() {
		var a Address
		if err := rows.Scan(&a.IP, &a.Interface, &a.FirstSeen, &a.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// MergeDevices folds device "from" into device "into": history, names, ports
// and user-set fields that "into" lacks move over, then "from" is deleted.
func (s *Store) MergeDevices(ctx context.Context, from, into int64) error {
	if from == into {
		return errors.New("cannot merge a device into itself")
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE id IN (?, ?)`, from, into).Scan(&n); err != nil {
			return err
		}
		if n != 2 {
			return ErrNotFound
		}
		stmts := []string{
			`UPDATE presence SET device_id = ?2 WHERE device_id = ?1 AND ended_at IS NOT NULL`,
			`DELETE FROM presence WHERE device_id = ?1`,
			`UPDATE events SET device_id = ?2 WHERE device_id = ?1`,
			`INSERT OR IGNORE INTO device_addresses (device_id, ip, interface, first_seen, last_seen)
				SELECT ?2, ip, interface, first_seen, last_seen FROM device_addresses WHERE device_id = ?1`,
			`INSERT OR IGNORE INTO device_names (device_id, source, name, updated_at)
				SELECT ?2, source, name, updated_at FROM device_names WHERE device_id = ?1`,
			`INSERT INTO software (device_id, source, product, version, evidence, update_available, seen_at)
				SELECT ?2, source, product, version, evidence, update_available, seen_at FROM software WHERE device_id = ?1 AND true
				ON CONFLICT (device_id, source) DO UPDATE SET product = excluded.product, version = excluded.version,
					evidence = excluded.evidence, update_available = excluded.update_available, seen_at = excluded.seen_at
				WHERE excluded.seen_at > software.seen_at`,
			`UPDATE devices SET
				name = CASE WHEN name = '' THEN (SELECT name FROM devices WHERE id = ?1) ELSE name END,
				notes = CASE WHEN notes = '' THEN (SELECT notes FROM devices WHERE id = ?1) ELSE notes END,
				location = CASE WHEN location = '' THEN (SELECT location FROM devices WHERE id = ?1) ELSE location END,
				type_override = CASE WHEN type_override = '' THEN (SELECT type_override FROM devices WHERE id = ?1) ELSE type_override END,
				known = MAX(known, (SELECT known FROM devices WHERE id = ?1)),
				favorite = MAX(favorite, (SELECT favorite FROM devices WHERE id = ?1)),
				watch = MAX(watch, (SELECT watch FROM devices WHERE id = ?1)),
				first_seen = MIN(first_seen, (SELECT first_seen FROM devices WHERE id = ?1))
				WHERE id = ?2`,
			`DELETE FROM devices WHERE id = ?1`,
		}
		for _, q := range stmts {
			if _, err := tx.ExecContext(ctx, q, from, into); err != nil {
				return err
			}
		}
		return nil
	})
}

// ExpireRandomDevices deletes unacknowledged devices with a private (random)
// MAC that have been offline since before cutoff and returns their IDs.
func (s *Store) ExpireRandomDevices(ctx context.Context, cutoff int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `DELETE FROM devices WHERE mac_kind = 'random' AND known = 0 AND favorite = 0
		AND watch = 0 AND name = '' AND online = 0 AND last_seen < ? RETURNING id`, cutoff)
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

// Software is one statement about a device's software (see internal/software).
type Software struct {
	DeviceID        int64
	Source          string
	Product         string
	Version         string
	Evidence        string
	UpdateAvailable bool
	SeenAt          int64
}

// UpsertSoftware stores or replaces the row of sw.DeviceID and sw.Source.
func (s *Store) UpsertSoftware(ctx context.Context, sw Software) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO software (device_id, source, product, version, evidence, update_available, seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (device_id, source) DO UPDATE SET product = excluded.product, version = excluded.version,
			evidence = excluded.evidence, update_available = excluded.update_available, seen_at = excluded.seen_at`,
		sw.DeviceID, sw.Source, sw.Product, sw.Version, sw.Evidence, b2i(sw.UpdateAvailable), sw.SeenAt)
	return err
}

// DeleteSoftware removes the row of a device and source.
func (s *Store) DeleteSoftware(ctx context.Context, deviceID int64, source string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM software WHERE device_id = ? AND source = ?`, deviceID, source)
	return err
}

// ListSoftware returns all software rows, grouped by device.
func (s *Store) ListSoftware(ctx context.Context) (map[int64][]Software, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT device_id, source, product, version, evidence, update_available, seen_at FROM software`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]Software{}
	for rows.Next() {
		var sw Software
		if err := rows.Scan(&sw.DeviceID, &sw.Source, &sw.Product, &sw.Version, &sw.Evidence, &sw.UpdateAvailable, &sw.SeenAt); err != nil {
			return nil, err
		}
		out[sw.DeviceID] = append(out[sw.DeviceID], sw)
	}
	return out, rows.Err()
}
