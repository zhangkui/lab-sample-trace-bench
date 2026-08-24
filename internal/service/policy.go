package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

// CapacityAlert identifies zones that need a gate-in pause or a rehandle plan.
type CapacityAlert struct {
	Zone      string  `json:"zone"`
	Rate      float64 `json:"rate"`
	Threshold float64 `json:"threshold"`
	Action    string  `json:"action"`
}

func (s *Service) CapacityAlerts(threshold float64) ([]CapacityAlert, error) {
	if threshold <= 0 || threshold > 1 {
		return nil, fmt.Errorf("capacity threshold must be between zero and one")
	}
	utilization, err := s.ZoneUtilization()
	if err != nil {
		return nil, err
	}
	alerts := make([]CapacityAlert, 0)
	for _, zone := range utilization {
		if zone.Rate < threshold {
			continue
		}
		action := "pause_gate_in"
		if zone.Rate >= 0.95 {
			action = "urgent_rehandle"
		}
		alerts = append(alerts, CapacityAlert{Zone: zone.Zone, Rate: zone.Rate, Threshold: threshold, Action: action})
	}
	sort.Slice(alerts, func(i, j int) bool { return alerts[i].Rate > alerts[j].Rate })
	return alerts, nil
}

// TaskAdmission explains why a move can or cannot enter the queue. Keeping
// the decision as data makes it visible to API callers and audit reviewers.
type TaskAdmission struct {
	Allowed bool     `json:"allowed"`
	Reasons []string `json:"reasons"`
}

func (s *Service) CheckTaskAdmission(task domain.Task) TaskAdmission {
	result := TaskAdmission{Allowed: true}
	if strings.TrimSpace(task.Container) == "" {
		result.Allowed = false
		result.Reasons = append(result.Reasons, "container is required")
	}
	if !task.Kind.Valid() {
		result.Allowed = false
		result.Reasons = append(result.Reasons, "task kind is invalid")
	}
	if task.Deadline.Before(s.now()) && !task.Deadline.IsZero() {
		result.Allowed = false
		result.Reasons = append(result.Reasons, "deadline has passed")
	}
	if task.VesselCall == "" && (task.Kind == domain.JobLoad || task.Kind == domain.JobDischarge) {
		result.Allowed = false
		result.Reasons = append(result.Reasons, "vessel call is required for vessel work")
	}
	return result
}

// ShiftWindow is used by reports and dispatch policy to make time boundaries
// explicit instead of relying on local machine time.
type ShiftWindow struct {
	Name  string    `json:"name"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func CurrentShift(now time.Time) ShiftWindow {
	now = now.UTC()
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	hour := now.Hour()
	if hour <= 8 {
		return ShiftWindow{Name: "night", Start: base.Add(-8 * time.Hour), End: base}
	}
	if hour < 16 {
		return ShiftWindow{Name: "morning", Start: base.Add(8 * time.Hour), End: base.Add(16 * time.Hour)}
	}
	return ShiftWindow{Name: "evening", Start: base.Add(16 * time.Hour), End: base.Add(24 * time.Hour)}
}

func (s *Service) ShiftSummary(now time.Time) (MoveSummary, ShiftWindow, error) {
	shift := CurrentShift(now)
	summary, err := s.MoveSummary(shift.Start)
	if err != nil {
		return MoveSummary{}, ShiftWindow{}, err
	}
	return summary, shift, nil
}
