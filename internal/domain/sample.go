package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusReceived  Status = "received"
	StatusPrepared  Status = "prepared"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusRejected  Status = "rejected"
)

type Sample struct {
	ID          string    `json:"id"`
	Project     string    `json:"project"`
	Subject     string    `json:"subject"`
	Status      Status    `json:"status"`
	CollectedAt time.Time `json:"collected_at"`
	RetainUntil time.Time `json:"retain_until"`
	Tags        []string  `json:"tags"`
	Revision    int       `json:"revision"`
}
type Transition struct {
	SampleID string    `json:"sample_id"`
	From     Status    `json:"from"`
	To       Status    `json:"to"`
	At       time.Time `json:"at"`
	Operator string    `json:"operator"`
	Note     string    `json:"note"`
}

func (s Sample) Validate(now time.Time) error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("sample id is required")
	}
	if strings.TrimSpace(s.Project) == "" {
		return fmt.Errorf("project is required")
	}
	if strings.TrimSpace(s.Subject) == "" {
		return fmt.Errorf("subject is required")
	}
	if !validStatus(s.Status) {
		return fmt.Errorf("unknown sample status %q", s.Status)
	}
	if s.CollectedAt.IsZero() || s.CollectedAt.After(now.Add(5*time.Minute)) {
		return fmt.Errorf("invalid collected_at")
	}
	if !s.RetainUntil.IsZero() && !s.RetainUntil.After(s.CollectedAt) {
		return fmt.Errorf("retain_until must be after collected_at")
	}
	return nil
}
func (s *Sample) NormalizeTags() {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(s.Tags))
	for _, tag := range s.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	s.Tags = out
}
func CanTransition(from, to Status) bool {
	return map[Status]map[Status]bool{StatusReceived: {StatusPrepared: true, StatusRejected: true}, StatusPrepared: {StatusRunning: true, StatusRejected: true}, StatusRunning: {StatusCompleted: true, StatusRejected: true}}[from][to]
}
func validStatus(s Status) bool {
	return s == StatusReceived || s == StatusPrepared || s == StatusRunning || s == StatusCompleted || s == StatusRejected
}
