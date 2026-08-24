package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

// OperationalWindow is the normalized interval accepted by report and
// simulation-like planning APIs.
type OperationalWindow struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

func NormalizeWindow(from, to time.Time, maximum time.Duration) (OperationalWindow, error) {
	if from.IsZero() || to.IsZero() {
		return OperationalWindow{}, fmt.Errorf("window endpoints are required")
	}
	from = from.UTC().Truncate(time.Minute)
	to = to.UTC().Truncate(time.Minute)
	if !to.After(from) {
		return OperationalWindow{}, fmt.Errorf("window end must be after start")
	}
	if maximum <= 0 {
		return OperationalWindow{}, fmt.Errorf("maximum duration must be positive")
	}
	if to.Sub(from) > maximum {
		return OperationalWindow{}, fmt.Errorf("window exceeds maximum duration")
	}
	return OperationalWindow{From: from, To: to}, nil
}

func ValidateScheduleWindow(call domain.VesselCall) error {
	if err := call.Validate(); err != nil {
		return err
	}
	if call.ETD.IsZero() {
		return fmt.Errorf("etd is required for an operational call")
	}
	if call.CutOffCargo.IsZero() {
		return fmt.Errorf("cargo cut-off is required for an operational call")
	}
	if call.CutOffCargo.Before(call.ETA) {
		return fmt.Errorf("cargo cut-off cannot precede eta")
	}
	return nil
}

type DeadlineClass string

const (
	DeadlineExpired  DeadlineClass = "expired"
	DeadlineCritical DeadlineClass = "critical"
	DeadlineSoon     DeadlineClass = "soon"
	DeadlineNormal   DeadlineClass = "normal"
	DeadlineUnset    DeadlineClass = "unset"
)

func ClassifyDeadline(deadline, now time.Time) DeadlineClass {
	if deadline.IsZero() {
		return DeadlineUnset
	}
	remaining := deadline.Sub(now)
	if remaining <= 0 {
		return DeadlineExpired
	}
	if remaining <= 30*time.Minute {
		return DeadlineCritical
	}
	if remaining <= 2*time.Hour {
		return DeadlineSoon
	}
	return DeadlineNormal
}

func ValidateTaskReferences(task domain.Task) error {
	if strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("task id is required")
	}
	if strings.TrimSpace(task.Container) == "" {
		return fmt.Errorf("task container is required")
	}
	if !task.Kind.Valid() {
		return fmt.Errorf("task kind is invalid")
	}
	if task.State != domain.TaskQueued && task.State != domain.TaskAssigned && task.State != domain.TaskRunning && task.State != domain.TaskDone {
		return fmt.Errorf("task state is invalid")
	}
	if task.State == domain.TaskDone && task.Finished.IsZero() {
		return fmt.Errorf("completed task requires finished time")
	}
	if task.State == domain.TaskAssigned && strings.TrimSpace(task.Crane) != "" {
		return fmt.Errorf("assigned task requires crane")
	}
	return nil
}
