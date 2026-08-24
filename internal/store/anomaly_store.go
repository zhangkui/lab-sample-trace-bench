package store

import (
	"database/sql"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

// SaveAnomaly upserts an anomaly record.
func (s *Store) SaveAnomaly(a domain.Anomaly) error {
	_, err := s.db.Exec(`INSERT INTO anomalies
(id,kind,severity,container,slot,description,raised_at,deadline,acknowledged_at,resolved_at,escalation,resolution)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
kind=excluded.kind,severity=excluded.severity,container=excluded.container,slot=excluded.slot,
description=excluded.description,raised_at=excluded.raised_at,deadline=excluded.deadline,
acknowledged_at=excluded.acknowledged_at,resolved_at=excluded.resolved_at,
escalation=excluded.escalation,resolution=excluded.resolution`,
		a.ID, a.Kind, a.Severity, a.Container, string(a.Slot), a.Description,
		a.RaisedAt.Format(time.RFC3339Nano), a.Deadline.Format(time.RFC3339Nano),
		stamp(a.AcknowledgedAt), stamp(a.ResolvedAt), string(a.Escalation), a.Resolution)
	return err
}

// LoadAnomaly reads a single anomaly by id.
func (s *Store) LoadAnomaly(id string) (domain.Anomaly, error) {
	var a domain.Anomaly
	var raised, deadline, ack, resolved, slot string
	err := s.db.QueryRow(`SELECT id,kind,severity,container,slot,description,raised_at,deadline,
acknowledged_at,resolved_at,escalation,resolution FROM anomalies WHERE id=?`, id).
		Scan(&a.ID, &a.Kind, &a.Severity, &a.Container, &slot, &a.Description,
			&raised, &deadline, &ack, &resolved, &a.Escalation, &a.Resolution)
	if err == sql.ErrNoRows {
		return a, ErrNotFound
	}
	a.Slot = domain.SlotID(slot)
	a.RaisedAt, _ = time.Parse(time.RFC3339Nano, raised)
	a.Deadline, _ = time.Parse(time.RFC3339Nano, deadline)
	a.AcknowledgedAt, _ = time.Parse(time.RFC3339Nano, ack)
	a.ResolvedAt, _ = time.Parse(time.RFC3339Nano, resolved)
	return a, err
}

// OpenAnomalies returns all anomalies that have not been resolved, optionally
// filtered by container. The yard scans these on each tick to detect breaches.
func (s *Store) OpenAnomalies(container string) ([]domain.Anomaly, error) {
	q := `SELECT id,kind,severity,container,slot,description,raised_at,deadline,
acknowledged_at,resolved_at,escalation,resolution FROM anomalies WHERE resolved_at=''`
	var args []any
	if container != "" {
		q += ` AND container=?`
		args = append(args, container)
	}
	q += ` ORDER BY raised_at`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Anomaly
	for rows.Next() {
		var a domain.Anomaly
		var raised, deadline, ack, resolved, slot string
		if err := rows.Scan(&a.ID, &a.Kind, &a.Severity, &a.Container, &slot, &a.Description,
			&raised, &deadline, &ack, &resolved, &a.Escalation, &a.Resolution); err != nil {
			return nil, err
		}
		a.Slot = domain.SlotID(slot)
		a.RaisedAt, _ = time.Parse(time.RFC3339Nano, raised)
		a.Deadline, _ = time.Parse(time.RFC3339Nano, deadline)
		a.AcknowledgedAt, _ = time.Parse(time.RFC3339Nano, ack)
		a.ResolvedAt, _ = time.Parse(time.RFC3339Nano, resolved)
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountAnomalies reports totals by severity for the dashboard.
func (s *Store) CountAnomalies(resolved bool) (map[domain.Severity]int, error) {
	q := `SELECT severity,COUNT(*) FROM anomalies`
	if !resolved {
		q += ` WHERE resolved_at=''`
	}
	q += ` GROUP BY severity`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.Severity]int{}
	for rows.Next() {
		var sev domain.Severity
		var n int
		if err := rows.Scan(&sev, &n); err != nil {
			return nil, err
		}
		out[sev] = n
	}
	return out, rows.Err()
}
