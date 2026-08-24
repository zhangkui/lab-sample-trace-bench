package domain

import (
	"fmt"
	"time"
)

// MoveStatus is the lifecycle stage of a container within the yard. The states
// form a linear pipeline from arrival, through stacking, optional rehandling,
// to eventual loading onto a vessel or departure through the gate.
type MoveStatus string

const (
	StatusAnnounced  MoveStatus = "announced"  // pre-advised by EDI, not yet on site
	StatusGatedIn    MoveStatus = "gated_in"   // trucked through the gate, awaiting stack
	StatusStacked    MoveStatus = "stacked"    // resting on a yard slot
	StatusRehandling MoveStatus = "rehandling" // crane lifting it to a new slot
	StatusStaged     MoveStatus = "staged"     // positioned at the quay for loading
	StatusLoaded     MoveStatus = "loaded"     // aboard the outbound vessel
	StatusGatedOut   MoveStatus = "gated_out"  // released through the gate
	StatusHeld       MoveStatus = "held"       // customs/HSE hold, no moves allowed
)

func (s MoveStatus) Valid() bool {
	switch s {
	case StatusAnnounced, StatusGatedIn, StatusStacked, StatusRehandling,
		StatusStaged, StatusLoaded, StatusGatedOut, StatusHeld:
		return true
	}
	return false
}

// Onsite reports whether the container is physically present in the yard at the
// given status. Announced and gated-out containers are off-site.
func (s MoveStatus) Onsite() bool {
	switch s {
	case StatusGatedIn, StatusStacked, StatusRehandling, StatusStaged, StatusHeld:
		return true
	}
	return false
}

// Terminal reports whether the status is a final state: once a container is
// loaded aboard or gated out it leaves the active yard population.
func (s MoveStatus) Terminal() bool {
	return s == StatusLoaded || s == StatusGatedOut
}

// moveGraph encodes the legal forward transitions of the state machine. A held
// container can be released back to stacked, and a stacked one can be picked up
// for rehandling or staging before loading.
var moveGraph = map[MoveStatus]map[MoveStatus]bool{
	StatusAnnounced:  {StatusGatedIn: true, StatusHeld: true},
	StatusGatedIn:    {StatusStacked: true, StatusHeld: true},
	StatusStacked:    {StatusRehandling: true, StatusStaged: true, StatusHeld: true, StatusGatedOut: true},
	StatusRehandling: {StatusStacked: true, StatusHeld: true},
	StatusStaged:     {StatusLoaded: true, StatusStacked: true, StatusHeld: true},
	StatusHeld:       {StatusStacked: true, StatusGatedOut: true},
}

// CanMove reports whether a transition from one status to another is permitted
// by the state machine.
func CanMove(from, to MoveStatus) bool {
	allowed, ok := moveGraph[from]
	if !ok {
		return false
	}
	return allowed[to]
}

// MoveEvent records a single state transition applied to a container, together
// with the slot it moved to (if any) and the move that authorised it.
type MoveEvent struct {
	Container string     `json:"container"`
	From      MoveStatus `json:"from"`
	To        MoveStatus `json:"to"`
	Slot      SlotID     `json:"slot,omitempty"`
	MoveID    string     `json:"move_id,omitempty"`
	At        time.Time  `json:"at"`
	Operator  string     `json:"operator"`
	Note      string     `json:"note,omitempty"`
}

// StackingRule enforces the physical constraints that govern whether a
// container may be placed on a given slot. It is a pure function so the yard
// service can consult it both at placement time and when proposing rearrangements.
type StackingRule struct {
	Slot     SlotID
	Occupant *Container // container already on the slot, nil if empty
	Below    *Container // container underneath, nil if ground tier
	Ground   bool
	Powered  bool
}

// Admit decides whether a container may be stacked according to the rule. It
// returns a human-readable reason for rejection. The checks are:
//   - the slot must be empty;
//   - ground slots need no support, but elevated slots require a box beneath;
//   - reefer containers require a powered slot;
//   - the container beneath, if any, must be at least as long so the load bears;
//   - dangerous goods may not be stacked beneath another dangerous box.
func (r StackingRule) Admit(c Container) error {
	if r.Occupant != nil {
		return fmt.Errorf("slot %s already occupied by %s", r.Slot, r.Occupant.ID)
	}
	if !r.Ground {
		if r.Below == nil {
			return fmt.Errorf("slot %s has no support beneath", r.Slot)
		}
		if r.Below.Size.TEU() < c.Size.TEU() {
			return fmt.Errorf("container beneath %s is shorter than %s", r.Slot, c.ID)
		}
		if r.Below.IsDangerous() && c.IsDangerous() {
			return fmt.Errorf("dangerous goods may not stack on dangerous goods at %s", r.Slot)
		}
	}
	if c.RequiresPower() && !r.Powered {
		return fmt.Errorf("slot %s has no power for reefer %s", r.Slot, c.ID)
	}
	return nil
}
