package service

import (
	"fmt"
	"strings"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
	"github.com/zhangkui/lab-sample-trace-bench/internal/store"
)

// DefineZone registers a yard zone and materialises every stacking slot it
// contains. Slots are created empty and, for reefer zones, marked powered.
// Re-running DefineZone with the same code is idempotent: existing slots are
// not recreated, which preserves any live placements.
func (s *Service) DefineZone(z domain.Zone, actor string) (domain.Zone, error) {
	if err := requireActor(actor); err != nil {
		return domain.Zone{}, err
	}
	if err := z.Validate(); err != nil {
		return domain.Zone{}, err
	}
	if err := s.repo.SaveZone(z); err != nil {
		return domain.Zone{}, err
	}
	powered := z.Type == domain.ZoneReefer || z.Type == domain.ZoneDanger
	for bay := 1; bay <= z.BayCount; bay++ {
		for row := 1; row <= z.RowsPerBay; row++ {
			for tier := 1; tier <= z.TiersPerRow; tier++ {
				slotID := domain.NewSlot(z.Code, bay, row, tier)
				if err := s.repo.SaveSlot(store.SlotRow{
					ID: string(slotID), Zone: z.Code, Bay: bay, Row: row, Tier: tier,
					Powered: powered,
				}); err != nil {
					return domain.Zone{}, err
				}
			}
		}
	}
	if _, err := s.audit.Append(z.Code, "define_zone", actor, map[string]any{
		"type": z.Type, "capacity": z.BayCount * z.RowsPerBay * z.TiersPerRow,
	}); err != nil {
		return domain.Zone{}, err
	}
	return z, nil
}

// GetZone returns a zone with its current occupancy.
func (s *Service) GetZone(code string) (domain.Zone, domain.Occupancy, error) {
	z, err := s.repo.LoadZone(strings.ToUpper(code))
	if err != nil {
		if err == store.ErrNotFound {
			return domain.Zone{}, domain.Occupancy{}, ErrNotFound
		}
		return domain.Zone{}, domain.Occupancy{}, err
	}
	occ, err := s.OccupancyOf(z)
	if err != nil {
		return domain.Zone{}, domain.Occupancy{}, err
	}
	return z, occ, nil
}

// OccupancyOf computes the live occupancy of a zone by scanning its slots.
// Ground-free counts empty bottom-tier slots, which is what the stacking policy
// needs when looking for a place to put a box that cannot be elevated.
func (s *Service) OccupancyOf(z domain.Zone) (domain.Occupancy, error) {
	slots, err := s.repo.SlotsInZone(z.Code)
	if err != nil {
		return domain.Occupancy{}, err
	}
	occ := domain.Occupancy{Zone: z.Type, Capacity: len(slots)}
	for _, sl := range slots {
		if sl.Container == "" {
			if sl.Tier == 1 {
				occ.GroundFree++
			}
			continue
		}
		occ.Used++
		row, err := s.repo.LoadContainer(sl.Container)
		if err != nil {
			continue
		}
		if row.CargoClass == domain.CargoDangerous {
			occ.Dangerous++
		}
		if row.CargoClass == domain.CargoReefer || row.NeedsPower {
			occ.Reefer++
		}
	}
	return occ, nil
}

// StackingCandidate is a slot the policy is considering, with the support and
// power context the domain StackingRule needs to make a decision.
type StackingCandidate struct {
	Slot    domain.SlotID
	Powered bool
	Below   *domain.Container
}

// findSlot walks a zone bottom-up and returns the first slot that admits the
// container under the stacking rules. The walk prefers the ground tier first
// (to avoid creating unsupported stacks) and, within a tier, prefers powered
// slots for reefer cargo. It returns ErrNoSlot when nothing fits.
var ErrNoSlot = fmt.Errorf("no suitable slot available")

func (s *Service) findSlot(z domain.Zone, c domain.Container) (domain.SlotID, error) {
	slots, err := s.repo.SlotsInZone(z.Code)
	if err != nil {
		return "", err
	}
	// Group slots by (bay,row) so we can evaluate tier support per stack.
	type stack struct {
		bay, row int
		tiers    []store.SlotRow
	}
	stacks := map[int]stack{}
	key := func(bay, row int) int { return bay*10000 + row }
	for _, sl := range slots {
		st := stacks[key(sl.Bay, sl.Row)]
		st.bay, st.row = sl.Bay, sl.Row
		st.tiers = append(st.tiers, sl)
		stacks[key(sl.Bay, sl.Row)] = st
	}
	// Evaluate tiers bottom-up; within a tier evaluate powered-first for reefers.
	for tier := 1; tier <= z.TiersPerRow; tier++ {
		for _, st := range stacks {
			row := st.tiers[tier-1]
			if row.Container != "" {
				continue
			}
			var below *domain.Container
			if tier > 1 {
				lower := st.tiers[tier-2]
				if lower.Container == "" {
					continue // no support
				}
				lc, err := s.repo.LoadContainer(lower.Container)
				if err != nil {
					continue
				}
				below = &lc.Container
			}
			rule := domain.StackingRule{
				Slot: domain.SlotID(row.ID), Occupant: nil, Below: below,
				Ground: tier == 1, Powered: row.Powered,
			}
			if err := rule.Admit(c); err == nil {
				return domain.SlotID(row.ID), nil
			}
		}
	}
	return "", ErrNoSlot
}

// isZoneCompatible returns ErrIncompatible when a zone may not receive the
// container at all, regardless of slot availability.
var ErrIncompatible = fmt.Errorf("container incompatible with zone")

func isZoneCompatible(z domain.Zone, c domain.Container) error {
	if !z.Permits(c.CargoClass) {
		return ErrIncompatible
	}
	return nil
}

// RecommendRearrangement proposes a set of rehandle moves that would relieve
// pressure in a zone. It looks for containers stacked above an empty ground slot
// (a hole in the stack) and proposes moving the elevated box down, which both
// lowers the centre of gravity and frees the upper tier.
type Rearrangement struct {
	Container string        `json:"container"`
	From      domain.SlotID `json:"from"`
	To        domain.SlotID `json:"to"`
	Reason    string        `json:"reason"`
}

// ProposeRearrangement scans a zone for the most common inefficiency: a box
// resting on a non-ground tier while a ground slot in the same stack is empty.
// Moving it down is the cheapest rehandle that improves stability.
func (s *Service) ProposeRearrangement(zone string) ([]Rearrangement, error) {
	z, err := s.repo.LoadZone(strings.ToUpper(zone))
	if err != nil {
		return nil, ErrNotFound
	}
	slots, err := s.repo.SlotsInZone(z.Code)
	if err != nil {
		return nil, err
	}
	type stack struct {
		tiers []store.SlotRow
	}
	stacks := map[int]stack{}
	key := func(bay, row int) int { return bay*10000 + row }
	for _, sl := range slots {
		st := stacks[key(sl.Bay, sl.Row)]
		st.tiers = append(st.tiers, sl)
		stacks[key(sl.Bay, sl.Row)] = st
	}
	var out []Rearrangement
	for _, st := range stacks {
		if len(st.tiers) < 2 {
			continue
		}
		if st.tiers[0].Container != "" {
			continue // ground occupied, nothing to gain here
		}
		for i := 1; i < len(st.tiers); i++ {
			if st.tiers[i].Container == "" {
				continue
			}
			// Found an elevated box above an empty ground slot.
			out = append(out, Rearrangement{
				Container: st.tiers[i].Container,
				From:      domain.SlotID(st.tiers[i].ID),
				To:        domain.SlotID(st.tiers[0].ID),
				Reason:    "lower to empty ground slot",
			})
			break // one rehandle per stack is enough for a proposal
		}
	}
	return out, nil
}
