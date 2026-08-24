package service

import (
    "testing"
    "time"
)

func TestBug006ThirtyMinuteDeadlineIsCritical(t *testing.T) {
    now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
    if got := ClassifyDeadline(now.Add(30*time.Minute), now); got != DeadlineCritical { t.Fatalf("deadline class = %s", got) }
}
