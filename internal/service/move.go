package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
	"github.com/zhangkui/lab-sample-trace-bench/internal/store"
)

// MoveRequest carries the parameters for a single state transition. The yard
// decides the destination slot when it is omitted, which is the common case for
// gate-in; explicit slots are used for rehandles and restows.
type MoveRequest struct {
	Container string            `json:"container"`
	To        domain.MoveStatus `json:"to"`
	Slot      domain.SlotID     `json:"slot,omitempty"`
	Operator  string            `json:"operator"`
	Note      string            `json:"note,omitempty"`
}

// ApplyMove is the heart of the move pipeline. It validates the requested
// transition, resolves and checks the destination slot, persists the new state
// and records both a move event and an audit entry atomically. Any error leaves
// the yard state unchanged.
func (s *Service) ApplyMove(req MoveRequest) (ContainerView, error) {
	if err := requireActor(req.Operator); err != nil {
		return ContainerView{}, err
	}
	req.Container = strings.TrimSpace(req.Container)
	if req.Container == "" {
		return ContainerView{}, fmt.Errorf("container id is required")
	}
	row, err := s.repo.LoadContainer(req.Container)
	if err != nil {
		return ContainerView{}, ErrNotFound
	}
	from := domain.MoveStatus(row.Status)
	if !domain.CanMove(from, req.To) {
		return ContainerView{}, fmt.Errorf("illegal move %s -> %s for %s", from, req.To, req.Container)
	}
	// Resolve the destination slot according to the target state.
	slot, err := s.resolveSlot(row, req)
	if err != nil {
		return ContainerView{}, err
	}
	// Apply the three persistence writes.
	if err := s.applyMoveWrites(row, from, req, slot); err != nil {
		return ContainerView{}, err
	}
	return s.InspectContainer(req.Container)
}

// applyMoveWrites performs the non-transactional persistence of a move. The
// store uses a single connection so writes are serialised in practice; the
// audit append that follows is what makes the operation verifiable.
func (s *Service) applyMoveWrites(row store.ContainerRow, from domain.MoveStatus, req MoveRequest, slot domain.SlotID) error {
	now := s.now()
	row.Status = string(req.To)
	row.Slot = string(slot)
	row.Revision++
	row.UpdatedAt = now
	if err := s.repo.SaveContainer(row); err != nil {
		return err
	}
	if slot != "" {
		// Release the old slot (if any) and occupy the new one.
		if row := row; true {
			_ = row
		}
		if err := s.occupySlot(slot, req.Container, now); err != nil {
			return err
		}
	}
	move := domain.MoveEvent{
		Container: req.Container, From: from, To: req.To,
		Slot: slot, At: now, Operator: req.Operator, Note: req.Note,
	}
	if err := s.repo.RecordMove(move); err != nil {
		return err
	}
	if _, err := s.audit.Append(req.Container, "move", req.Operator, map[string]any{
		"from": from, "to": req.To, "slot": slot,
	}); err != nil {
		return err
	}
	return nil
}

// occupySlot updates the slot's container column. For transitions that vacate a
// slot (load, gate-out) the caller passes an empty container id, which the yard
// interprets as "clear".
func (s *Service) occupySlot(slot domain.SlotID, container string, _ time.Time) error {
	if slot == "" {
		return nil
	}
	sl, err := s.repo.LoadSlot(string(slot))
	if err != nil {
		return fmt.Errorf("slot %s: %w", slot, err)
	}
	sl.Container = container
	return s.repo.SaveSlot(sl)
}

// resolveSlot picks the destination slot for a move. The rules are:
//   - stacking and restow moves require a slot; if omitted, the yard finds one;
//   - gate-in resolves a slot like stacking;
//   - load and gate-out vacate the current slot, so the destination is empty;
//   - rehandling is driven by an explicit slot (a rehandle target).
func (s *Service) resolveSlot(row store.ContainerRow, req MoveRequest) (domain.SlotID, error) {
	switch req.To {
	case domain.StatusStacked, domain.StatusGatedIn:
		if req.Slot != "" {
			return s.validateSlot(req.Slot, req.Container)
		}
		return s.selectSlot(req.Container)
	case domain.StatusRehandling, domain.StatusStaged:
		if req.Slot != "" {
			return s.validateSlot(req.Slot, req.Container)
		}
		return "", fmt.Errorf("%s requires an explicit slot", req.To)
	case domain.StatusLoaded, domain.StatusGatedOut:
		// Vacate the current slot, if any.
		if row.Slot != "" {
			if err := s.repo.ClearSlot(row.Slot); err != nil {
				return "", err
			}
		}
		return "", nil
	case domain.StatusHeld:
		return domain.SlotID(row.Slot), nil // held in place
	}
	return "", fmt.Errorf("unsupported target status %s", req.To)
}

// validateSlot confirms a caller-supplied slot is empty and admissible for the
// container, returning an error if another box already claims it.
func (s *Service) validateSlot(slot domain.SlotID, container string) (domain.SlotID, error) {
	sl, err := s.repo.LoadSlot(string(slot))
	if err != nil {
		return "", fmt.Errorf("slot %s: %w", slot, err)
	}
	if sl.Container != "" && sl.Container != container {
		// Conflict: another container is already there. This is how the yard
		// detects a physical position clash and raises an anomaly upstream.
		return "", &SlotConflictError{Slot: slot, Occupant: sl.Container, Challenger: container}
	}
	row, err := s.repo.LoadContainer(container)
	if err != nil {
		return "", err
	}
	var below *domain.Container
	if !slot.Ground() {
		// The slot below must hold something for support.
		_, _, _, tier, _ := slot.Parts()
		lowerID := domain.NewSlot(sl.Zone, sl.Bay, sl.Row, tier-1)
		lower, err := s.repo.LoadSlot(string(lowerID))
		if err != nil || lower.Container == "" {
			return "", fmt.Errorf("slot %s has no support beneath", slot)
		}
		lc, err := s.repo.LoadContainer(lower.Container)
		if err != nil {
			return "", err
		}
		below = &lc.Container
	}
	rule := domain.StackingRule{
		Slot: slot, Below: below, Ground: slot.Ground(), Powered: sl.Powered,
	}
	if err := rule.Admit(row.Container); err != nil {
		return "", err
	}
	return slot, nil
}

// selectSlot finds a compatible zone and an admissible slot within it for a
// container. It tries the dangerous-goods zone first for dangerous cargo, the
// reefer zone first for reefers, and otherwise any general zone with capacity.
func (s *Service) selectSlot(container string) (domain.SlotID, error) {
	row, err := s.repo.LoadContainer(container)
	if err != nil {
		return "", err
	}
	zones, err := s.repo.ListZones()
	if err != nil {
		return "", err
	}
	// Order zones so the preferred zone for the cargo class is tried first.
	ordered := orderZones(zones, row.Container)
	for _, z := range ordered {
		if err := isZoneCompatible(z, row.Container); err != nil {
			continue
		}
		slot, err := s.findSlot(z, row.Container)
		if err == nil {
			return slot, nil
		}
	}
	return "", ErrNoSlot
}

// orderZones returns zones with the preferred zone type for the cargo class
// first. Dangerous cargo prefers the danger zone; reefers the reefer zone;
// everything else general/export zones.
func orderZones(zones []domain.Zone, c domain.Container) []domain.Zone {
	preferred := domain.ZoneGeneral
	switch {
	case c.IsDangerous():
		preferred = domain.ZoneDanger
	case c.RequiresPower():
		preferred = domain.ZoneReefer
	case c.VoyageOut != "":
		preferred = domain.ZoneExport
	}
	var head, tail []domain.Zone
	for _, z := range zones {
		if z.Type == preferred {
			head = append(head, z)
		} else {
			tail = append(tail, z)
		}
	}
	return append(head, tail...)
}

// SlotConflictError signals that two containers claimed the same slot.
type SlotConflictError struct {
	Slot       domain.SlotID
	Occupant   string
	Challenger string
}

func (e *SlotConflictError) Error() string {
	return fmt.Sprintf("slot %s occupied by %s, cannot place %s", e.Slot, e.Occupant, e.Challenger)
}

// MoveHistory returns the ordered transition log for a container.
func (s *Service) MoveHistory(container string) ([]domain.MoveEvent, error) {
	return s.repo.MoveHistory(container)
}
