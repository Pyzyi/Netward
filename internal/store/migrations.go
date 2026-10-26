package store

// migrations are applied in order, each in its own transaction. Never edit a
// released migration; append a new one instead.
var migrations = []string{
	// 1: initial schema
	`
CREATE TABLE devices (
	id               INTEGER PRIMARY KEY,
	ident            TEXT NOT NULL UNIQUE,  -- MAC, or "MAC/IP" for clients behind a MAC-NAT repeater
	mac              TEXT NOT NULL,
	ip               TEXT NOT NULL DEFAULT '',
	name             TEXT NOT NULL DEFAULT '',
	hostname         TEXT NOT NULL DEFAULT '',
	vendor           TEXT NOT NULL DEFAULT '',
	vendor_official  TEXT NOT NULL DEFAULT '',
	model            TEXT NOT NULL DEFAULT '',
	os               TEXT NOT NULL DEFAULT '',
	type             TEXT NOT NULL DEFAULT 'unknown',
	type_confidence  INTEGER NOT NULL DEFAULT 0,
	type_override    TEXT NOT NULL DEFAULT '',
	location         TEXT NOT NULL DEFAULT '',
	notes            TEXT NOT NULL DEFAULT '',
	known            INTEGER NOT NULL DEFAULT 0,
	favorite         INTEGER NOT NULL DEFAULT 0,
	watch            INTEGER NOT NULL DEFAULT 0,
	mac_kind         TEXT NOT NULL DEFAULT 'universal', -- universal | random | virtual
	proxy            INTEGER NOT NULL DEFAULT 0,          -- answers ARP for several IPs (repeater)
	via_proxy        INTEGER NOT NULL DEFAULT 0,          -- identity is MAC/IP behind a proxy
	online           INTEGER NOT NULL DEFAULT 0,
	is_gateway       INTEGER NOT NULL DEFAULT 0,
	is_self          INTEGER NOT NULL DEFAULT 0,
	interface        TEXT NOT NULL DEFAULT '',
	dhcp_fingerprint TEXT NOT NULL DEFAULT '',
	dhcp_vendor      TEXT NOT NULL DEFAULT '',
	first_seen       INTEGER NOT NULL,
	last_seen        INTEGER NOT NULL,
	enriched_at      INTEGER NOT NULL DEFAULT 0,
	ports_scanned_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX devices_ip ON devices(ip);
CREATE INDEX devices_mac ON devices(mac);
CREATE INDEX devices_online ON devices(online, last_seen);

CREATE TABLE device_names (
	device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
	source     TEXT NOT NULL,
	name       TEXT NOT NULL,
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (device_id, source)
);

CREATE TABLE device_services (
	device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
	source     TEXT NOT NULL,
	service    TEXT NOT NULL,
	detail     TEXT NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (device_id, source, service)
);

CREATE TABLE device_ports (
	device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
	port       INTEGER NOT NULL,
	proto      TEXT NOT NULL DEFAULT 'tcp',
	service    TEXT NOT NULL DEFAULT '',
	first_seen INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL,
	PRIMARY KEY (device_id, port, proto)
);

CREATE TABLE device_addresses (
	device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
	ip         TEXT NOT NULL,
	interface  TEXT NOT NULL DEFAULT '',
	first_seen INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL,
	PRIMARY KEY (device_id, ip)
);

CREATE TABLE presence (
	id         INTEGER PRIMARY KEY,
	device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
	started_at INTEGER NOT NULL,
	ended_at   INTEGER
);
CREATE INDEX presence_device ON presence(device_id, started_at);
CREATE UNIQUE INDEX presence_open ON presence(device_id) WHERE ended_at IS NULL;

CREATE TABLE events (
	id        INTEGER PRIMARY KEY,
	ts        INTEGER NOT NULL,
	kind      TEXT NOT NULL,
	severity  TEXT NOT NULL DEFAULT 'info',
	device_id INTEGER REFERENCES devices(id) ON DELETE SET NULL,
	data      TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX events_ts ON events(ts);
CREATE INDEX events_device ON events(device_id, ts);

CREATE TABLE internet_outages (
	id         INTEGER PRIMARY KEY,
	started_at INTEGER NOT NULL,
	ended_at   INTEGER
);

CREATE TABLE monitor_gaps (
	id         INTEGER PRIMARY KEY,
	started_at INTEGER NOT NULL,
	ended_at   INTEGER NOT NULL
);

CREATE TABLE security_findings (
	id         INTEGER PRIMARY KEY,
	check_id   TEXT NOT NULL,
	device_id  INTEGER REFERENCES devices(id) ON DELETE CASCADE,
	severity   TEXT NOT NULL,
	data       TEXT NOT NULL DEFAULT '{}',
	first_seen INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL,
	dismissed  INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX security_findings_key ON security_findings(check_id, IFNULL(device_id, 0), data);

CREATE TABLE settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE users (
	id            INTEGER PRIMARY KEY,
	username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT NOT NULL,
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
	id           INTEGER PRIMARY KEY,
	token_hash   BLOB NOT NULL UNIQUE,
	user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	csrf_token   TEXT NOT NULL,
	created_at   INTEGER NOT NULL,
	last_seen_at INTEGER NOT NULL,
	expires_at   INTEGER NOT NULL,
	remember     INTEGER NOT NULL DEFAULT 0,
	user_agent   TEXT NOT NULL DEFAULT '',
	ip           TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_expires ON sessions(expires_at);

CREATE TABLE channels (
	id         INTEGER PRIMARY KEY,
	name       TEXT NOT NULL,
	kind       TEXT NOT NULL,
	config     TEXT NOT NULL DEFAULT '{}',
	events     TEXT NOT NULL DEFAULT '[]',
	enabled    INTEGER NOT NULL DEFAULT 1,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
`,
	// 2: the presence strips read every span overlapping the last 24 hours;
	// without this index that is a full scan of the presence table.
	`CREATE INDEX presence_ended ON presence(ended_at);`,
	// 3: what devices reveal about their software (docs/08). One row per
	// device and source; "manual" and "edition" rows are set by the user.
	`
CREATE TABLE software (
	device_id        INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
	source           TEXT NOT NULL,
	product          TEXT NOT NULL,
	version          TEXT NOT NULL DEFAULT '',
	evidence         TEXT NOT NULL DEFAULT '',
	update_available INTEGER NOT NULL DEFAULT 0,
	seen_at          INTEGER NOT NULL,
	PRIMARY KEY (device_id, source)
);
`,
}
