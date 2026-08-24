package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
	"github.com/zhangkui/lab-sample-trace-bench/internal/store"
)

// RegisterContainer admits a container profile into the yard registry. The
// container starts in the announced status, meaning EDI has pre-advised it but
// it has not yet arrived through the gate. Registration is the only operation
// that does not require the container to be on site.
func (s *Service) RegisterContainer(c domain.Container, actor string) (domain.Container, error) {
	if err := requireActor(actor); err != nil {
		return domain.Container{}, err
	}
	c.CreatedAt = s.now()
	if err := c.Validate(); err != nil {
		return domain.Container{}, err
	}
	if err := s.repo.SaveContainer(store.ContainerRow{Container: c, Status: string(domain.StatusAnnounced)}); err != nil {
		return domain.Container{}, err
	}
	if _, err := s.audit.Append(c.ID, "register", actor, map[string]any{
		"bic": c.BIC, "size": c.Size, "cargo": c.CargoClass,
	}); err != nil {
		return domain.Container{}, err
	}
	return c, nil
}

// GetContainer loads a container by id.
func (s *Service) GetContainer(id string) (store.ContainerRow, error) {
	row, err := s.repo.LoadContainer(strings.TrimSpace(id))
	if err != nil {
		if err == store.ErrNotFound {
			return store.ContainerRow{}, ErrNotFound
		}
		return store.ContainerRow{}, err
	}
	return row, nil
}

// ContainerView is the read model returned to the API: the profile plus its
// current yard status, slot and move count.
type ContainerView struct {
	store.ContainerRow
	State     domain.MoveStatus `json:"state"`
	Slot      domain.SlotID     `json:"slot"`
	MoveCount int               `json:"move_count"`
}

// InspectContainer returns the full live view of a container, including its
// derived state, slot and the number of moves it has undergone.
func (s *Service) InspectContainer(id string) (ContainerView, error) {
	row, err := s.GetContainer(id)
	if err != nil {
		return ContainerView{}, err
	}
	hist, err := s.repo.MoveHistory(id)
	if err != nil {
		return ContainerView{}, err
	}
	return ContainerView{
		ContainerRow: row,
		State:        domain.MoveStatus(row.Status),
		Slot:         domain.SlotID(row.Slot),
		MoveCount:    len(hist),
	}, nil
}

// ContainerFilter captures the multi-condition search criteria the API accepts.
// Empty fields are treated as wildcards so the same query path serves broad and
// narrow searches.
type ContainerFilter struct {
	Size       domain.Size
	CargoClass domain.CargoClass
	Status     domain.MoveStatus
	VoyageOut  string
	Owner      string
}

// Match reports whether a container satisfies the filter.
func (f ContainerFilter) Match(c store.ContainerRow) bool {
	if f.Size != "" && c.Size != f.Size {
		return false
	}
	if f.CargoClass != "" && c.CargoClass != f.CargoClass {
		return false
	}
	if f.Status != "" && c.Status != string(f.Status) {
		return false
	}
	if f.VoyageOut != "" && c.VoyageOut != f.VoyageOut {
		return false
	}
	if f.Owner != "" && c.Owner != f.Owner {
		return false
	}
	return true
}

// Page is a slice of results with pagination metadata.
type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Size  int `json:"size"`
}

// SearchContainers applies a filter and returns a page of results. Pagination
// is computed in-memory because the filter is applied to the read model; for
// the yard's data volumes this is well within interactive latency.
func (s *Service) SearchContainers(f ContainerFilter, page, size int) (Page[ContainerView], error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 50
	}
	var views []ContainerView
	if err := s.repo.ListContainers(func(c store.ContainerRow) error {
		if !f.Match(c) {
			return nil
		}
		views = append(views, ContainerView{
			ContainerRow: c,
			State:        domain.MoveStatus(c.Status),
			Slot:         domain.SlotID(c.Slot),
		})
		return nil
	}); err != nil {
		return Page[ContainerView]{}, err
	}
	total := len(views)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return Page[ContainerView]{Items: views[start:end], Total: total, Page: page, Size: size}, nil
}

// ReleaseContainer removes a container from the active population after it has
// been gated out or loaded. It is the administrative close that archives the
// profile; the physical departure itself is recorded by the move pipeline.
func (s *Service) ReleaseContainer(id, actor string) error {
	if err := requireActor(actor); err != nil {
		return err
	}
	row, err := s.repo.LoadContainer(id)
	if err != nil {
		return ErrNotFound
	}
	if !domain.MoveStatus(row.Status).Terminal() {
		return fmt.Errorf("container %s has not reached a terminal state", id)
	}
	if _, err := s.audit.Append(id, "release", actor, map[string]any{"final": row.Status}); err != nil {
		return err
	}
	return nil
}

// overdueDwell is the threshold after which an idle container is flagged as
// overstay. It is a yard policy constant rather than a per-call parameter.
const overdueDwell = 7 * 24 * time.Hour

// OverstayedContainers returns boxes that have been stacked longer than the
// free-dwell period without being loaded or gated out, which feeds the
// overstay anomaly sweep.
func (s *Service) OverstayedContainers() ([]store.ContainerRow, error) {
	now := s.now()
	var out []store.ContainerRow
	if err := s.repo.ListContainers(func(c store.ContainerRow) error {
		if c.Status != string(domain.StatusStacked) {
			return nil
		}
		if c.UpdatedAt.IsZero() {
			return nil
		}
		if now.Sub(c.UpdatedAt) >= overdueDwell {
			out = append(out, c)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}
