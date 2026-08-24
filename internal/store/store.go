package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.schema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) schema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS records (kind TEXT NOT NULL, id TEXT NOT NULL, payload BLOB NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(kind,id)); CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT, subject TEXT NOT NULL, action TEXT NOT NULL, created_at TEXT NOT NULL);`)
	return err
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Save(kind, id string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO records(kind,id,payload,updated_at) VALUES(?,?,?,?) ON CONFLICT(kind,id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, kind, id, raw, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) Load(kind, id string, value any) error {
	var raw []byte
	err := s.db.QueryRow(`SELECT payload FROM records WHERE kind=? AND id=?`, kind, id).Scan(&raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, value)
}
func (s *Store) Delete(kind, id string) error {
	_, err := s.db.Exec(`DELETE FROM records WHERE kind=? AND id=?`, kind, id)
	return err
}
func (s *Store) List(kind string, into func([]byte) error) error {
	rows, err := s.db.Query(`SELECT payload FROM records WHERE kind=? ORDER BY updated_at`, kind)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		if err := into(raw); err != nil {
			return err
		}
	}
	return rows.Err()
}
func (s *Store) Event(subject, action string) error {
	_, err := s.db.Exec(`INSERT INTO events(subject,action,created_at) VALUES(?,?,?)`, subject, action, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
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
