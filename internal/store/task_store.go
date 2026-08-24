package store

import (
	"database/sql"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

// SaveTask upserts a scheduler task.
func (s *Store) SaveTask(t domain.Task) error {
	_, err := s.db.Exec(`INSERT INTO tasks
(id,container,kind,priority,vessel_call,from_slot,to_slot,crane,deadline,started,finished,state)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
container=excluded.container,kind=excluded.kind,priority=excluded.priority,
vessel_call=excluded.vessel_call,from_slot=excluded.from_slot,to_slot=excluded.to_slot,
crane=excluded.crane,deadline=excluded.deadline,started=excluded.started,
finished=excluded.finished,state=excluded.state`,
		t.ID, t.Container, t.Kind, t.Priority, t.VesselCall,
		string(t.FromSlot), string(t.ToSlot), t.Crane, stamp(t.Deadline),
		stamp(t.Started), stamp(t.Finished), t.State)
	return err
}

// LoadTask reads a single task by id.
func (s *Store) LoadTask(id string) (domain.Task, error) {
	t, err := scanTask(s.db.QueryRow(`SELECT id,container,kind,priority,vessel_call,from_slot,to_slot,
crane,deadline,started,finished,state FROM tasks WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return t, ErrNotFound
	}
	return t, err
}

// TasksByState returns all tasks in a given state ordered by priority then id,
// which is the order the scheduler dispatches them.
func (s *Store) TasksByState(state string) ([]domain.Task, error) {
	rows, err := s.db.Query(`SELECT id,container,kind,priority,vessel_call,from_slot,to_slot,
crane,deadline,started,finished,state FROM tasks WHERE state=? ORDER BY priority,id`, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// taskScanner is implemented by both *sql.Row and *sql.Rows.
type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(sc taskScanner) (domain.Task, error) {
	var t domain.Task
	var deadline, started, finished, from, to string
	err := sc.Scan(&t.ID, &t.Container, &t.Kind, &t.Priority, &t.VesselCall,
		&from, &to, &t.Crane, &deadline, &started, &finished, &t.State)
	t.FromSlot = domain.SlotID(from)
	t.ToSlot = domain.SlotID(to)
	t.Deadline, _ = time.Parse(time.RFC3339Nano, deadline)
	t.Started, _ = time.Parse(time.RFC3339Nano, started)
	t.Finished, _ = time.Parse(time.RFC3339Nano, finished)
	return t, err
}

// CountTasks reports how many tasks are in a given state.
func (s *Store) CountTasks(state string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE state=?`, state).Scan(&n)
	return n, err
}
