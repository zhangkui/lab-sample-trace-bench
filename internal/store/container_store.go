package store

import (
	"database/sql"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

// ContainerRow is the persistence projection of a container: it carries the
// derived status/slot/revision fields the in-memory aggregate does not own, so
// that a single round-trip reconstructs the full yard state of a box.
type ContainerRow struct {
	domain.Container
	Status    string
	Slot      string
	Revision  int
	UpdatedAt time.Time
}

// SaveContainer upserts a container profile, persisting its yard state.
func (s *Store) SaveContainer(c ContainerRow) error {
	_, err := s.db.Exec(`INSERT INTO containers
(id,bic,size,cargo_class,un_class,segregation,gross_weight,owner,voyage_in,voyage_out,needs_power,status,slot,revision,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
bic=excluded.bic,size=excluded.size,cargo_class=excluded.cargo_class,un_class=excluded.un_class,
segregation=excluded.segregation,gross_weight=excluded.gross_weight,owner=excluded.owner,voyage_in=excluded.voyage_in,
voyage_out=excluded.voyage_out,needs_power=excluded.needs_power,status=excluded.status,slot=excluded.slot,
revision=excluded.revision,updated_at=excluded.updated_at`,
		c.ID, c.BIC, c.Size, c.CargoClass, c.Danger.UNClass, c.Danger.SegregationGroup,
		c.GrossWeightKg, c.Owner, c.VoyageIn, c.VoyageOut, btoi(c.NeedsPower),
		c.Status, c.Slot, c.Revision, c.CreatedAt.Format(time.RFC3339Nano), nowstamp())
	return err
}

// LoadContainer reads a single container by id. It returns ErrNotFound when the
// id is unknown.
func (s *Store) LoadContainer(id string) (ContainerRow, error) {
	var c ContainerRow
	var un, seg, created, slot string
	var power int
	err := s.db.QueryRow(`SELECT id,bic,size,cargo_class,un_class,segregation,
gross_weight,owner,voyage_in,voyage_out,needs_power,status,slot,revision,created_at,updated_at
FROM containers WHERE id=?`, id).
		Scan(&c.ID, &c.BIC, &c.Size, &c.CargoClass, &un, &seg,
			&c.GrossWeightKg, &c.Owner, &c.VoyageIn, &c.VoyageOut, &power,
			&c.Status, &slot, &c.Revision, &created, &c.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return c, ErrNotFound
		}
		return c, err
	}
	c.Danger = domain.DangerProfile{UNClass: un, SegregationGroup: seg}
	c.NeedsPower = itob(power)
	c.Slot = slot
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return c, nil
}

// ListContainers scans every container and applies fn to each. The scan is the
// backing store for searches and reports that cannot be expressed in a single
// SQL predicate.
func (s *Store) ListContainers(fn func(ContainerRow) error) error {
	rows, err := s.db.Query(`SELECT id,bic,size,cargo_class,un_class,segregation,
gross_weight,owner,voyage_in,voyage_out,needs_power,status,slot,revision,created_at,updated_at
FROM containers ORDER BY updated_at`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c ContainerRow
		var un, seg, created, slot string
		var power int
		if err := rows.Scan(&c.ID, &c.BIC, &c.Size, &c.CargoClass, &un, &seg,
			&c.GrossWeightKg, &c.Owner, &c.VoyageIn, &c.VoyageOut, &power,
			&c.Status, &slot, &c.Revision, &created, &c.UpdatedAt); err != nil {
			return err
		}
		c.Danger = domain.DangerProfile{UNClass: un, SegregationGroup: seg}
		c.NeedsPower = itob(power)
		c.Slot = slot
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if err := fn(c); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ContainersOnSlot returns the ids of containers currently resting on a slot.
// A non-empty result is how the yard detects a physical conflict.
func (s *Store) ContainersOnSlot(slot string) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM containers WHERE slot=? AND status NOT IN ('','gated_out','loaded')`, slot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// btoi returns 1 for true and 0 for false, matching the INTEGER columns used
// for boolean flags.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// itob inverts btoi.
func itob(i int) bool { return i != 0 }
