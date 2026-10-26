package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// GetSetting decodes the JSON value of key into v. It returns ErrNotFound if unset.
func (s *Store) GetSetting(ctx context.Context, key string, v any) error {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), v)
}

// SetSetting stores v as JSON under key.
func (s *Store) SetSetting(ctx context.Context, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, string(raw))
	return err
}

// User is an administrator account.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    int64
	UpdatedAt    int64
}

// UserCount returns the number of accounts.
func (s *Store) UserCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// ErrExists is returned when creating something that already exists.
var ErrExists = errors.New("already exists")

// CreateFirstUser creates the initial account only if no account exists yet.
// The check and the insert happen in one immediate transaction, so two
// concurrent setup requests cannot both succeed.
func (s *Store) CreateFirstUser(ctx context.Context, username, hash string) (User, error) {
	var u User
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrExists
		}
		now := Now()
		res, err := tx.ExecContext(ctx, `INSERT INTO users (username, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			username, hash, now, now)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		u = User{ID: id, Username: username, PasswordHash: hash, CreatedAt: now, UpdatedAt: now}
		return err
	})
	return u, err
}

// GetUserByName looks up an account case-insensitively.
func (s *Store) GetUserByName(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, created_at, updated_at FROM users WHERE username = ?`,
		username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// GetUser looks up an account by ID.
func (s *Store) GetUser(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, created_at, updated_at FROM users WHERE id = ?`,
		id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// FirstUser returns the oldest account (used by the CLI reset).
func (s *Store) FirstUser(ctx context.Context) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, created_at, updated_at FROM users ORDER BY id LIMIT 1`).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// SetPassword replaces a password hash and revokes all sessions of the user
// except keepSession (0 revokes all).
func (s *Store) SetPassword(ctx context.Context, userID int64, hash string, keepSession int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, hash, Now(), userID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id != ?`, userID, keepSession)
		return err
	})
}

// Session is a login session. Only the SHA-256 of the token is stored.
type Session struct {
	ID         int64  `json:"id"`
	TokenHash  []byte `json:"-"`
	UserID     int64  `json:"-"`
	CSRFToken  string `json:"-"`
	CreatedAt  int64  `json:"createdAt"`
	LastSeenAt int64  `json:"lastSeenAt"`
	ExpiresAt  int64  `json:"expiresAt"`
	Remember   bool   `json:"remember"`
	UserAgent  string `json:"userAgent"`
	IP         string `json:"ip"`
}

// CreateSession stores a new session and fills in its ID.
func (s *Store) CreateSession(ctx context.Context, se *Session) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, csrf_token, created_at, last_seen_at,
		expires_at, remember, user_agent, ip) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		se.TokenHash, se.UserID, se.CSRFToken, se.CreatedAt, se.LastSeenAt, se.ExpiresAt, b2i(se.Remember), se.UserAgent, se.IP)
	if err != nil {
		return err
	}
	se.ID, err = res.LastInsertId()
	return err
}

// GetSessionByHash returns an unexpired session.
func (s *Store) GetSessionByHash(ctx context.Context, hash []byte, now int64) (Session, error) {
	var se Session
	err := s.db.QueryRowContext(ctx, `SELECT id, token_hash, user_id, csrf_token, created_at, last_seen_at, expires_at,
		remember, user_agent, ip FROM sessions WHERE token_hash = ? AND expires_at > ?`, hash, now).
		Scan(&se.ID, &se.TokenHash, &se.UserID, &se.CSRFToken, &se.CreatedAt, &se.LastSeenAt, &se.ExpiresAt,
			&se.Remember, &se.UserAgent, &se.IP)
	if errors.Is(err, sql.ErrNoRows) {
		return se, ErrNotFound
	}
	return se, err
}

// TouchSession slides the expiry of a session.
func (s *Store) TouchSession(ctx context.Context, id, now, expires int64, ip string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ?, ip = ? WHERE id = ?`, now, expires, ip, id)
	return err
}

// HasSessionFromIP reports whether an unexpired session was last used from ip.
func (s *Store) HasSessionFromIP(ctx context.Context, ip string, now int64) bool {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE ip = ? AND expires_at > ? LIMIT 1`, ip, now).Scan(&one)
	return err == nil
}

// ListSessions returns the active sessions of a user.
func (s *Store) ListSessions(ctx context.Context, userID int64) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, last_seen_at, expires_at, remember, user_agent, ip
		FROM sessions WHERE user_id = ? AND expires_at > ? ORDER BY last_seen_at DESC`, userID, Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var se Session
		if err := rows.Scan(&se.ID, &se.CreatedAt, &se.LastSeenAt, &se.ExpiresAt, &se.Remember, &se.UserAgent, &se.IP); err != nil {
			return nil, err
		}
		out = append(out, se)
	}
	return out, rows.Err()
}

// DeleteOtherSessions revokes every session of a user except keep and
// returns how many were removed.
func (s *Store) DeleteOtherSessions(ctx context.Context, userID, keep int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id != ?`, userID, keep)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteSession revokes one session of a user.
func (s *Store) DeleteSession(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Channel is a configured notification target.
type Channel struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Config    json.RawMessage `json:"config"`
	Events    []string        `json:"events"`
	Enabled   bool            `json:"enabled"`
	CreatedAt int64           `json:"createdAt"`
	UpdatedAt int64           `json:"updatedAt"`
}

// ListChannels returns all channels including their secrets (internal use only).
func (s *Store) ListChannels(ctx context.Context) ([]Channel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, kind, config, events, enabled, created_at, updated_at FROM channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Channel{}
	for rows.Next() {
		var c Channel
		var cfg, evs string
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &cfg, &evs, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Config = json.RawMessage(cfg)
		_ = json.Unmarshal([]byte(evs), &c.Events)
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetChannel returns one channel.
func (s *Store) GetChannel(ctx context.Context, id int64) (Channel, error) {
	chs, err := s.ListChannels(ctx)
	if err != nil {
		return Channel{}, err
	}
	for _, c := range chs {
		if c.ID == id {
			return c, nil
		}
	}
	return Channel{}, ErrNotFound
}

// SaveChannel inserts (ID 0) or updates a channel.
func (s *Store) SaveChannel(ctx context.Context, c *Channel) error {
	evs, _ := json.Marshal(c.Events)
	now := Now()
	if c.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO channels (name, kind, config, events, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, c.Name, c.Kind, string(c.Config), string(evs), b2i(c.Enabled), now, now)
		if err != nil {
			return err
		}
		c.ID, err = res.LastInsertId()
		c.CreatedAt, c.UpdatedAt = now, now
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE channels SET name = ?, kind = ?, config = ?, events = ?, enabled = ?, updated_at = ?
		WHERE id = ?`, c.Name, c.Kind, string(c.Config), string(evs), b2i(c.Enabled), now, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	c.UpdatedAt = now
	return nil
}

// DeleteChannel removes a channel.
func (s *Store) DeleteChannel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// FactoryReset removes accounts, sessions, notification channels and
// settings. Devices, history and the activity log are kept. Backups are
// deleted too: they are full copies that still hold the old secrets.
func (s *Store) FactoryReset(ctx context.Context) error {
	if err := s.factoryReset(ctx); err != nil {
		return err
	}
	dir := filepath.Join(filepath.Dir(s.path), "backup")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".db") || strings.HasSuffix(e.Name(), ".tmp") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

func (s *Store) factoryReset(ctx context.Context) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM sessions`,
			`DELETE FROM users`,
			`DELETE FROM channels`,
			`DELETE FROM settings WHERE key NOT IN ('baseline_at', 'heartbeat', 'security_last_run')`,
		} {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		return nil
	})
}
