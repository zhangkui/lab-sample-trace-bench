package domain

import (
	"fmt"
	"strings"
	"time"
)

// ScheduleStatus tracks a vessel call against its planned berthing window. The
// yard uses this to sequence loading: only vessels alongside or imminent are
// eligible for staging work, and a departed vessel closes its cut-off.
type ScheduleStatus string

const (
	ScheduleProposed  ScheduleStatus = "proposed"
	ScheduleArriving  ScheduleStatus = "arriving"  // inside ETA window, not yet berthed
	ScheduleAlongside ScheduleStatus = "alongside" // berthed and working
	ScheduleDeparted  ScheduleStatus = "departed"
)

func (s ScheduleStatus) Valid() bool {
	switch s {
	case ScheduleProposed, ScheduleArriving, ScheduleAlongside, ScheduleDeparted:
		return true
	}
	return false
}

// Active reports whether the call can still accept loading work.
func (s ScheduleStatus) Active() bool {
	return s == ScheduleArriving || s == ScheduleAlongside
}

// VesselCall represents a single port call by a ship. It carries the cut-off
// times that gate the loading window and the number of free crane-hours the
// call has been allocated, which the job scheduler draws down as it assigns
// moves.
type VesselCall struct {
	Vessel      string         `json:"vessel"`
	Voyage      string         `json:"voyage"`
	IMO         string         `json:"imo"`
	Status      ScheduleStatus `json:"status"`
	ETA         time.Time      `json:"eta"`
	ETD         time.Time      `json:"etd"`
	CutOffCargo time.Time      `json:"cut_off_cargo"`
	Cranes      int            `json:"cranes"`
	CraneHours  int            `json:"crane_hours"`
}

// Validate checks that the call's identifiers and times are consistent. The
// cut-off must precede departure, which must follow arrival; crane hours must
// be positive only when cranes are allocated.
func (v *VesselCall) Validate() error {
	v.Vessel = strings.TrimSpace(v.Vessel)
	v.Voyage = strings.ToUpper(strings.TrimSpace(v.Voyage))
	v.IMO = strings.TrimSpace(v.IMO)
	if v.Vessel == "" || v.Voyage == "" {
		return fmt.Errorf("vessel name and voyage are required")
	}
	if !v.Status.Valid() {
		return fmt.Errorf("unknown schedule status %q", v.Status)
	}
	if v.ETA.IsZero() {
		return fmt.Errorf("eta is required")
	}
	if !v.ETD.IsZero() && !v.ETD.After(v.ETA) {
		return fmt.Errorf("etd must be after eta")
	}
	if !v.CutOffCargo.IsZero() && !v.ETD.IsZero() && !v.CutOffCargo.Before(v.ETD) {
		return fmt.Errorf("cargo cut-off must precede etd")
	}
	if v.Cranes < 0 {
		return fmt.Errorf("crane allocation cannot be negative")
	}
	if v.CraneHours < 0 {
		return fmt.Errorf("crane hours cannot be negative")
	}
	return nil
}

// LoadOpen reports whether the call is still accepting cargo for loading. A
// departed vessel, or one whose cut-off has passed, is closed.
func (v VesselCall) LoadOpen(now time.Time) bool {
	if !v.Status.Active() {
		return false
	}
	if !v.CutOffCargo.IsZero() && now.After(v.CutOffCargo) {
		return false
	}
	return true
}

// JobKind names the type of yard work a task performs. Each kind has its own
// priority weighting when the scheduler sequences the queue.
type JobKind string

const (
	JobDischarge  JobKind = "discharge"  // unload from vessel to yard
	JobLoad       JobKind = "load"       // move from yard to vessel
	JobGateIn     JobKind = "gate_in"    // receive through gate
	JobGateOut    JobKind = "gate_out"   // release through gate
	JobRestow     JobKind = "restow"     // rehandle within yard
	JobInspection JobKind = "inspection" // survey/inspection move
)

func (k JobKind) Valid() bool {
	switch k {
	case JobDischarge, JobLoad, JobGateIn, JobGateOut, JobRestow, JobInspection:
		return true
	}
	return false
}

// Priority is the urgency of a job on the work queue. Lower numeric values run
// first: critical moves (a vessel cut-off about to lapse, or a safety hold
// release) take precedence over ordinary restows.
type Priority int

const (
	PriorityUrgent   Priority = 1 // safety/regulatory, do before all else
	PriorityVessel   Priority = 2 // driven by an alongside or arriving vessel
	PriorityStandard Priority = 3 // ordinary planned work
	PriorityDeferred Priority = 4 // best-effort reshuffles, idle-time work
)

// Weight maps a job kind and its deadline pressure onto a priority value. The
// scheduler calls this when admitting a job so that kind and time pressure are
// combined into a single comparable number.
func Weight(kind JobKind, deadline time.Time, now time.Time) Priority {
	switch kind {
	case JobDischarge, JobLoad:
		if !deadline.IsZero() && deadline.Sub(now) <= 2*time.Hour {
			return PriorityUrgent
		}
		return PriorityVessel
	case JobGateIn, JobGateOut:
		return PriorityStandard
	case JobRestow, JobInspection:
		return PriorityDeferred
	}
	return PriorityStandard
}

// Crane is a yard resource that executes move jobs. Only one job may be
// assigned to a crane at a time; the scheduler must release a crane before
// re-assigning it.
type Crane struct {
	ID        string    `json:"id"`
	ZoneCode  string    `json:"zone_code"`
	BusyUntil time.Time `json:"busy_until,omitempty"`
}

func (c *Crane) Validate() error {
	c.ID = strings.TrimSpace(c.ID)
	if c.ID == "" {
		return fmt.Errorf("crane id is required")
	}
	return nil
}

// Free reports whether the crane can accept a job at the given time.
func (c Crane) Free(now time.Time) bool {
	return c.BusyUntil.IsZero() || !now.Before(c.BusyUntil)
}

// Task is the scheduler's view of a job: the container to move, the kind of
// move, the crane assigned (if any) and its position on the priority queue.
type Task struct {
	ID         string    `json:"id"`
	Container  string    `json:"container"`
	Kind       JobKind   `json:"kind"`
	Priority   Priority  `json:"priority"`
	VesselCall string    `json:"vessel_call,omitempty"`
	FromSlot   SlotID    `json:"from_slot,omitempty"`
	ToSlot     SlotID    `json:"to_slot,omitempty"`
	Crane      string    `json:"crane,omitempty"`
	Deadline   time.Time `json:"deadline,omitempty"`
	Started    time.Time `json:"started,omitempty"`
	Finished   time.Time `json:"finished,omitempty"`
	State      string    `json:"state"`
}

const (
	TaskQueued   = "queued"
	TaskAssigned = "assigned"
	TaskRunning  = "running"
	TaskDone     = "done"
)
