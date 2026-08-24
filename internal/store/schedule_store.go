package store

import (
	"database/sql"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

// SaveCall upserts a vessel call keyed by (vessel, voyage).
func (s *Store) SaveCall(v domain.VesselCall) error {
	_, err := s.db.Exec(`INSERT INTO vessel_calls
(vessel,voyage,imo,status,eta,etd,cut_off,cranes,crane_hours)
VALUES(?,?,?,?,?,?,?,?,?)
ON CONFLICT(vessel,voyage) DO UPDATE SET
imo=excluded.imo,status=excluded.status,eta=excluded.eta,etd=excluded.etd,
cut_off=excluded.cut_off,cranes=excluded.cranes,crane_hours=excluded.crane_hours`,
		v.Vessel, v.Voyage, v.IMO, v.Status, v.ETA.Format(time.RFC3339Nano),
		stamp(v.ETD), stamp(v.CutOffCargo), v.Cranes, v.CraneHours)
	return err
}

// LoadCall reads a single vessel call.
func (s *Store) LoadCall(vessel, voyage string) (domain.VesselCall, error) {
	var v domain.VesselCall
	var eta, etd, cut string
	err := s.db.QueryRow(`SELECT vessel,voyage,imo,status,eta,etd,cut_off,cranes,crane_hours
FROM vessel_calls WHERE vessel=? AND voyage=?`, vessel, voyage).
		Scan(&v.Vessel, &v.Voyage, &v.IMO, &v.Status, &eta, &etd, &cut, &v.Cranes, &v.CraneHours)
	if err == sql.ErrNoRows {
		return v, ErrNotFound
	}
	v.ETA, _ = time.Parse(time.RFC3339Nano, eta)
	v.ETD, _ = time.Parse(time.RFC3339Nano, etd)
	v.CutOffCargo, _ = time.Parse(time.RFC3339Nano, cut)
	return v, err
}

// ListCalls returns all vessel calls ordered by ETA.
func (s *Store) ListCalls() ([]domain.VesselCall, error) {
	rows, err := s.db.Query(`SELECT vessel,voyage,imo,status,eta,etd,cut_off,cranes,crane_hours
FROM vessel_calls ORDER BY eta`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.VesselCall
	for rows.Next() {
		var v domain.VesselCall
		var eta, etd, cut string
		if err := rows.Scan(&v.Vessel, &v.Voyage, &v.IMO, &v.Status, &eta, &etd, &cut, &v.Cranes, &v.CraneHours); err != nil {
			return nil, err
		}
		v.ETA, _ = time.Parse(time.RFC3339Nano, eta)
		v.ETD, _ = time.Parse(time.RFC3339Nano, etd)
		v.CutOffCargo, _ = time.Parse(time.RFC3339Nano, cut)
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveCrane upserts a crane resource.
func (s *Store) SaveCrane(c domain.Crane) error {
	_, err := s.db.Exec(`INSERT INTO cranes(id,zone_code,busy_until) VALUES(?,?,?)
ON CONFLICT(id) DO UPDATE SET zone_code=excluded.zone_code,busy_until=excluded.busy_until`,
		c.ID, c.ZoneCode, stamp(c.BusyUntil))
	return err
}

// LoadCrane reads a crane by id.
func (s *Store) LoadCrane(id string) (domain.Crane, error) {
	var c domain.Crane
	var busy string
	err := s.db.QueryRow(`SELECT id,zone_code,busy_until FROM cranes WHERE id=?`, id).
		Scan(&c.ID, &c.ZoneCode, &busy)
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	c.BusyUntil, _ = time.Parse(time.RFC3339Nano, busy)
	return c, err
}

// ListCranes returns all cranes.
func (s *Store) ListCranes() ([]domain.Crane, error) {
	rows, err := s.db.Query(`SELECT id,zone_code,busy_until FROM cranes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Crane
	for rows.Next() {
		var c domain.Crane
		var busy string
		if err := rows.Scan(&c.ID, &c.ZoneCode, &busy); err != nil {
			return nil, err
		}
		c.BusyUntil, _ = time.Parse(time.RFC3339Nano, busy)
		out = append(out, c)
	}
	return out, rows.Err()
}

// stamp formats a time for storage, returning the empty string for the zero
// time so optional temporal columns read back as "unset".
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}
