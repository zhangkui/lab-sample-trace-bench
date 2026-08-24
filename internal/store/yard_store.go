package store

import (
	"database/sql"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

// SaveZone upserts a yard zone. Zones change rarely, so this is a simple
// replace by primary key.
func (s *Store) SaveZone(z domain.Zone) error {
	_, err := s.db.Exec(`INSERT INTO zones(code,name,type,bay_count,rows_per_bay,tiers_per_row)
VALUES(?,?,?,?,?,?)
ON CONFLICT(code) DO UPDATE SET name=excluded.name,type=excluded.type,
bay_count=excluded.bay_count,rows_per_bay=excluded.rows_per_bay,tiers_per_row=excluded.tiers_per_row`,
		z.Code, z.Name, z.Type, z.BayCount, z.RowsPerBay, z.TiersPerRow)
	return err
}

// LoadZone reads a zone by its code.
func (s *Store) LoadZone(code string) (domain.Zone, error) {
	var z domain.Zone
	err := s.db.QueryRow(`SELECT code,name,type,bay_count,rows_per_bay,tiers_per_row FROM zones WHERE code=?`, code).
		Scan(&z.Code, &z.Name, &z.Type, &z.BayCount, &z.RowsPerBay, &z.TiersPerRow)
	if err == sql.ErrNoRows {
		return z, ErrNotFound
	}
	return z, err
}

// ListZones returns every zone definition.
func (s *Store) ListZones() ([]domain.Zone, error) {
	rows, err := s.db.Query(`SELECT code,name,type,bay_count,rows_per_bay,tiers_per_row FROM zones ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Zone
	for rows.Next() {
		var z domain.Zone
		if err := rows.Scan(&z.Code, &z.Name, &z.Type, &z.BayCount, &z.RowsPerBay, &z.TiersPerRow); err != nil {
			return nil, err
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

// SlotRow is the persisted projection of a stacking position.
type SlotRow struct {
	ID        string
	Zone      string
	Bay       int
	Row       int
	Tier      int
	Powered   bool
	Container string
}

// SaveSlot upserts a slot, including which container currently occupies it.
func (s *Store) SaveSlot(sl SlotRow) error {
	_, err := s.db.Exec(`INSERT INTO slots(id,zone,bay,row,tier,powered,container)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET powered=excluded.powered,container=excluded.container`,
		sl.ID, sl.Zone, sl.Bay, sl.Row, sl.Tier, btoi(sl.Powered), sl.Container)
	return err
}

// LoadSlot reads a single slot by id.
func (s *Store) LoadSlot(id string) (SlotRow, error) {
	var sl SlotRow
	var power int
	err := s.db.QueryRow(`SELECT id,zone,bay,row,tier,powered,container FROM slots WHERE id=?`, id).
		Scan(&sl.ID, &sl.Zone, &sl.Bay, &sl.Row, &sl.Tier, &power, &sl.Container)
	if err == sql.ErrNoRows {
		return sl, ErrNotFound
	}
	sl.Powered = itob(power)
	return sl, err
}

// ClearSlot empties a slot, leaving the position itself defined.
func (s *Store) ClearSlot(id string) error {
	_, err := s.db.Exec(`UPDATE slots SET container='' WHERE id=?`, id)
	return err
}

// SlotsInZone returns every slot belonging to a zone, ordered so that the yard
// service can walk tiers bottom-up when searching for a stacking target.
func (s *Store) SlotsInZone(zone string) ([]SlotRow, error) {
	rows, err := s.db.Query(`SELECT id,zone,bay,row,tier,powered,container FROM slots
WHERE zone=? ORDER BY bay,row,tier`, zone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SlotRow
	for rows.Next() {
		var sl SlotRow
		var power int
		if err := rows.Scan(&sl.ID, &sl.Zone, &sl.Bay, &sl.Row, &sl.Tier, &power, &sl.Container); err != nil {
			return nil, err
		}
		sl.Powered = itob(power)
		out = append(out, sl)
	}
	return out, rows.Err()
}

// CountOccupied counts how many slots in a zone currently hold a container.
func (s *Store) CountOccupied(zone string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM slots WHERE zone=? AND container<>''`, zone).Scan(&n)
	return n, err
}
