package service

import (
    "testing"
    "time"
)

func TestBug007EightOClockBelongsToMorningShift(t *testing.T) {
    now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
    if shift := CurrentShift(now); shift.Name != "morning" { t.Fatalf("shift at 08:00 = %s", shift.Name) }
}
