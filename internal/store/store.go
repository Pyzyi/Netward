// Package store persists everything Netzwart knows in a single SQLite file.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// Store wraps the database handle.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (and creates, with mode 0600) the database at path and applies
// pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	q := url.Values{}
	for _, p := range []string{
		"journal_mode(WAL)", "foreign_keys(1)", "busy_timeout(5000)",
		"synchronous(NORMAL)", "temp_store(MEMORY)",
	} {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)
	s := &Store{db: db, path: path}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Size returns the size of the database files in bytes.
func (s *Store) Size() int64 {
	var total int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if fi, err := os.Stat(s.path + suffix); err == nil {
			total += fi.Size()
		}
	}
	return total
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	var current int
	if err := s.db.QueryRowContext(ctx, `SELECT IFNULL(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}
	if current > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this build supports (%d); refusing to downgrade", current, len(migrations))
	}
	for v := current + 1; v <= len(migrations); v++ {
		err := s.tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, migrations[v-1]); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES (?)`, v)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %d: %w", v, err)
		}
	}
	return nil
}

func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Now returns the current time in Unix milliseconds.
func Now() int64 { return time.Now().UnixMilli() }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
