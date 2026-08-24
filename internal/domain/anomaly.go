package domain

import (
	"fmt"
	"strings"
	"time"
)

// AnomalyKind categorises the exceptions the yard raises when an operation
// violates a rule or a condition drifts out of tolerance. Each kind carries a
// default handling deadline and an escalation target.
type AnomalyKind string

const (
	AnomalyConflict    AnomalyKind = "slot_conflict"      // two containers claimed one slot
	AnomalySegregation AnomalyKind = "segregation_breach" // dangerous goods too close
	AnomalyOverweight  AnomalyKind = "overweight"         // exceeds crane safe load
	AnomalyOverstay    AnomalyKind = "overstay"           // dwell beyond free period
	AnomalyPowerLoss   AnomalyKind = "power_loss"         // reefer lost shore power
	AnomalyMisstow     AnomalyKind = "misstow"            // placed on wrong vessel/voyage
	AnomalyDamage      AnomalyKind = "damage"             // physical damage detected
	AnomalySealBroken  AnomalyKind = "seal_broken"        // customs seal compromised
)

func (k AnomalyKind) Valid() bool {
	switch k {
	case AnomalyConflict, AnomalySegregation, AnomalyOverweight, AnomalyOverstay,
		AnomalyPowerLoss, AnomalyMisstow, AnomalyDamage, AnomalySealBroken:
		return true
	}
	return false
}

// Severity ranks how disruptive an anomaly is and drives escalation speed.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

func (s Severity) Valid() bool {
	switch s {
	case SeverityInfo, SeverityWarning, SeverityCritical:
		return true
	}
	return false
}

// defaults returns the handling deadline and escalation target for a kind. The
// yard consults this when creating an anomaly so that every exception has a
// defined time-box from the moment it is raised.
func (k AnomalyKind) defaults() (time.Duration, Severity) {
	switch k {
	case AnomalyConflict, AnomalySegregation, AnomalySealBroken:
		return 1 * time.Hour, SeverityCritical
	case AnomalyPowerLoss, AnomalyDamage:
		return 2 * time.Hour, SeverityCritical
	case AnomalyOverweight, AnomalyMisstow:
		return 4 * time.Hour, SeverityWarning
	case AnomalyOverstay:
		return 24 * time.Hour, SeverityWarning
	}
	return 8 * time.Hour, SeverityWarning
}

// EscalationTier names the level a critical anomaly is promoted to when it
// breaches its handling deadline.
type EscalationTier string

const (
	EscalationOps     EscalationTier = "operations"    // shift supervisor
	EscalationControl EscalationTier = "control_tower" // yard control room
	EscalationManager EscalationTier = "duty_manager"  // duty manager after hours
)

// Anomaly is the record of an exception attached to a container, slot or move.
// Its lifecycle is: raised -> acknowledged -> resolved (or escalated).
type Anomaly struct {
	ID             string         `json:"id"`
	Kind           AnomalyKind    `json:"kind"`
	Severity       Severity       `json:"severity"`
	Container      string         `json:"container,omitempty"`
	Slot           SlotID         `json:"slot,omitempty"`
	Description    string         `json:"description"`
	RaisedAt       time.Time      `json:"raised_at"`
	Deadline       time.Time      `json:"deadline"`
	AcknowledgedAt time.Time      `json:"acknowledged_at,omitempty"`
	ResolvedAt     time.Time      `json:"resolved_at,omitempty"`
	Escalation     EscalationTier `json:"escalation,omitempty"`
	Resolution     string         `json:"resolution,omitempty"`
}

// AnomalyState is the derived lifecycle stage of an anomaly.
type AnomalyState string

const (
	AnomalyOpen         AnomalyState = "open"
	AnomalyAcknowledged AnomalyState = "acknowledged"
	AnomalyResolved     AnomalyState = "resolved"
	AnomalyEscalated    AnomalyState = "escalated"
)

// State derives the lifecycle stage from the timestamps.
func (a Anomaly) State() AnomalyState {
	if !a.ResolvedAt.IsZero() {
		return AnomalyResolved
	}
	if a.Escalation != "" {
		return AnomalyEscalated
	}
	if !a.AcknowledgedAt.IsZero() {
		return AnomalyAcknowledged
	}
	return AnomalyOpen
}

// Breached reports whether the anomaly's handling deadline has passed and it
// has not yet been resolved.
func (a Anomaly) Breached(now time.Time) bool {
	return a.ResolvedAt.IsZero() && !a.Deadline.IsZero() && now.After(a.Deadline)
}

// Validate checks the anomaly is well-formed and, when the kind is known,
// fills in default severity and deadline if the caller left them blank.
func (a *Anomaly) Validate(now time.Time) error {
	a.ID = strings.TrimSpace(a.ID)
	a.Description = strings.TrimSpace(a.Description)
	if a.ID == "" {
		return fmt.Errorf("anomaly id is required")
	}
	if !a.Kind.Valid() {
		return fmt.Errorf("unknown anomaly kind %q", a.Kind)
	}
	if a.Severity == "" {
		ttl, sev := a.Kind.defaults()
		a.Severity = sev
		a.Deadline = now.Add(ttl)
	}
	if !a.Severity.Valid() {
		return fmt.Errorf("unknown severity %q", a.Severity)
	}
	if a.RaisedAt.IsZero() {
		return fmt.Errorf("raised_at is required")
	}
	if a.Deadline.IsZero() {
		ttl, _ := a.Kind.defaults()
		a.Deadline = a.RaisedAt.Add(ttl)
	}
	if !a.Deadline.After(a.RaisedAt) {
		return fmt.Errorf("deadline must be after raised_at")
	}
	return nil
}

// NextTier advances the escalation path for a breached anomaly. Critical
// anomalies escalate to the control tower after the first breach and to the
// duty manager on a second breach; non-critical anomalies stop at operations.
func NextTier(current EscalationTier, severity Severity) EscalationTier {
	if severity != SeverityCritical {
		return EscalationOps
	}
	switch current {
	case "":
		return EscalationControl
	case EscalationControl:
		return EscalationManager
	default:
		return EscalationManager
	}
}
