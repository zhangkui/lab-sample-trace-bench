// Package store provides the SQLite persistence layer for the container yard.
//
// The schema is relational rather than a single JSON blob table: containers,
// yard slots, moves, vessel calls, tasks, cranes, anomalies and audit entries
// each have their own table so that searches and capacity queries can be served
// by indexed SQL instead of scanning and decoding every record in memory.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned by accessors when no row matches the request.
var ErrNotFound = errors.New("record not found")

// Store wraps a sqlite database connection. The connection pool is capped at a
// single writer because modernc/sqlite serialises writes anyway, and a single
// connection keeps the WAL semantics simple for the yard's write-heavy moves.
type Store struct{ db *sql.DB }

// Open creates or opens a database at path, applying migrations that bring the
// schema to the current version. The parent directory is created on demand.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// migrate creates the yard schema. The statements are idempotent so opening an
// existing database is a no-op.
func (s *Store) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS containers (
	id            TEXT PRIMARY KEY,
	bic           TEXT NOT NULL,
	size          TEXT NOT NULL,
	cargo_class   TEXT NOT NULL,
	un_class      TEXT NOT NULL DEFAULT '',
	segregation   TEXT NOT NULL DEFAULT '',
	gross_weight  INTEGER NOT NULL DEFAULT 0,
	owner         TEXT NOT NULL DEFAULT '',
	voyage_in     TEXT NOT NULL DEFAULT '',
	voyage_out    TEXT NOT NULL DEFAULT '',
	needs_power   INTEGER NOT NULL DEFAULT 0,
	status        TEXT NOT NULL DEFAULT '',
	slot          TEXT NOT NULL DEFAULT '',
	revision      INTEGER NOT NULL DEFAULT 0,
	created_at    TEXT NOT NULL,
	updated_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_containers_status ON containers(status);
CREATE INDEX IF NOT EXISTS idx_containers_slot ON containers(slot);
CREATE INDEX IF NOT EXISTS idx_containers_voyage_out ON containers(voyage_out);

CREATE TABLE IF NOT EXISTS zones (
	code        TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	type        TEXT NOT NULL,
	bay_count   INTEGER NOT NULL,
	rows_per_bay INTEGER NOT NULL,
	tiers_per_row INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS slots (
	id        TEXT PRIMARY KEY,
	zone      TEXT NOT NULL REFERENCES zones(code),
	bay       INTEGER NOT NULL,
	row       INTEGER NOT NULL,
	tier      INTEGER NOT NULL,
	powered   INTEGER NOT NULL DEFAULT 0,
	container TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_slots_zone ON slots(zone);
CREATE INDEX IF NOT EXISTS idx_slots_container ON slots(container);

CREATE TABLE IF NOT EXISTS moves (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	container TEXT NOT NULL,
	from_status TEXT NOT NULL,
	to_status   TEXT NOT NULL,
	slot       TEXT NOT NULL DEFAULT '',
	move_id    TEXT NOT NULL DEFAULT '',
	at         TEXT NOT NULL,
	operator   TEXT NOT NULL,
	note       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_moves_container ON moves(container);

CREATE TABLE IF NOT EXISTS vessel_calls (
	vessel     TEXT NOT NULL,
	voyage     TEXT NOT NULL,
	imo        TEXT NOT NULL DEFAULT '',
	status     TEXT NOT NULL,
	eta        TEXT NOT NULL,
	etd        TEXT NOT NULL DEFAULT '',
	cut_off    TEXT NOT NULL DEFAULT '',
	cranes     INTEGER NOT NULL DEFAULT 0,
	crane_hours INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (vessel, voyage)
);

CREATE TABLE IF NOT EXISTS cranes (
	id        TEXT PRIMARY KEY,
	zone_code TEXT NOT NULL DEFAULT '',
	busy_until TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS tasks (
	id           TEXT PRIMARY KEY,
	container    TEXT NOT NULL,
	kind         TEXT NOT NULL,
	priority     INTEGER NOT NULL,
	vessel_call  TEXT NOT NULL DEFAULT '',
	from_slot    TEXT NOT NULL DEFAULT '',
	to_slot      TEXT NOT NULL DEFAULT '',
	crane        TEXT NOT NULL DEFAULT '',
	deadline     TEXT NOT NULL DEFAULT '',
	started      TEXT NOT NULL DEFAULT '',
	finished     TEXT NOT NULL DEFAULT '',
	state        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tasks_state ON tasks(state);
CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority);

CREATE TABLE IF NOT EXISTS anomalies (
	id            TEXT PRIMARY KEY,
	kind          TEXT NOT NULL,
	severity      TEXT NOT NULL,
	container     TEXT NOT NULL DEFAULT '',
	slot          TEXT NOT NULL DEFAULT '',
	description   TEXT NOT NULL,
	raised_at     TEXT NOT NULL,
	deadline      TEXT NOT NULL,
	acknowledged_at TEXT NOT NULL DEFAULT '',
	resolved_at   TEXT NOT NULL DEFAULT '',
	escalation    TEXT NOT NULL DEFAULT '',
	resolution    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_anomalies_resolved ON anomalies(resolved_at);
CREATE INDEX IF NOT EXISTS idx_anomalies_kind ON anomalies(kind);

CREATE TABLE IF NOT EXISTS audit (
	seq       INTEGER PRIMARY KEY AUTOINCREMENT,
	subject   TEXT NOT NULL,
	action    TEXT NOT NULL,
	actor     TEXT NOT NULL,
	detail    TEXT NOT NULL DEFAULT '',
	checksum  INTEGER NOT NULL DEFAULT 0,
	at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_subject ON audit(subject);
`

// Transaction runs fn inside a database transaction, rolling back on error.
// It is the unit of work for moves that must update several tables at once.
func (s *Store) Transaction(fn func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("transaction: %w", err)
	}
	return tx.Commit()
}

// DB exposes the underlying handle for packages that need direct access.
func (s *Store) DB() *sql.DB { return s.db }

// nowstamp formats the current UTC time for storage in TEXT columns.
func nowstamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
