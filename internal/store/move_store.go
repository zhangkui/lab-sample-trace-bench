package store

import (
	"database/sql"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

// RecordMove appends a state-transition event to the move history. The history
// is append-only: moves are never edited or deleted, which is what makes the
// audit chain tamper-evident.
func (s *Store) RecordMove(m domain.MoveEvent) error {
	_, err := s.db.Exec(`INSERT INTO moves(container,from_status,to_status,slot,move_id,at,operator,note)
VALUES(?,?,?,?,?,?,?,?)`,
		m.Container, m.From, m.To, string(m.Slot), m.MoveID, m.At.Format(time.RFC3339Nano), m.Operator, m.Note)
	return err
}

// MoveHistory returns the full ordered transition log for a container.
func (s *Store) MoveHistory(container string) ([]domain.MoveEvent, error) {
	rows, err := s.db.Query(`SELECT container,from_status,to_status,slot,move_id,at,operator,note
FROM moves WHERE container=? ORDER BY id`, container)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MoveEvent
	for rows.Next() {
		var m domain.MoveEvent
		var slot, at string
		if err := rows.Scan(&m.Container, &m.From, &m.To, &slot, &m.MoveID, &at, &m.Operator, &m.Note); err != nil {
			return nil, err
		}
		m.Slot = domain.SlotID(slot)
		m.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountMoves returns the number of recorded transitions, used by reports.
func (s *Store) CountMoves() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM moves`).Scan(&n)
	return n, err
}

// MovesBetween returns the transitions recorded in a half-open time range,
// used by the shift report.
func (s *Store) MovesBetween(from, to time.Time) ([]domain.MoveEvent, error) {
	rows, err := s.db.Query(`SELECT container,from_status,to_status,slot,move_id,at,operator,note
FROM moves WHERE at>=? AND at<? ORDER BY id`, from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MoveEvent
	for rows.Next() {
		var m domain.MoveEvent
		var slot, at string
		if err := rows.Scan(&m.Container, &m.From, &m.To, &slot, &m.MoveID, &at, &m.Operator, &m.Note); err != nil {
			return nil, err
		}
		m.Slot = domain.SlotID(slot)
		m.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, m)
	}
	return out, rows.Err()
}

// LastMoveOfKind returns the most recent transition into a given status for a
// container, or ErrNotFound when none exists.
func (s *Store) LastMoveOfKind(container string, to domain.MoveStatus) (domain.MoveEvent, error) {
	var m domain.MoveEvent
	var slot, at string
	err := s.db.QueryRow(`SELECT container,from_status,to_status,slot,move_id,at,operator,note
FROM moves WHERE container=? AND to_status=? ORDER BY id DESC LIMIT 1`, container, to).
		Scan(&m.Container, &m.From, &m.To, &slot, &m.MoveID, &at, &m.Operator, &m.Note)
	if err == sql.ErrNoRows {
		return m, ErrNotFound
	}
	m.Slot = domain.SlotID(slot)
	m.At, _ = time.Parse(time.RFC3339Nano, at)
	return m, err
}
