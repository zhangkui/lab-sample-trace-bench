package service

import (
    "testing"
    "time"
)

func TestBug002WindowKeepsMinuteBoundary(t *testing.T) {
    from := time.Date(2026, 8, 24, 12, 34, 0, 0, time.UTC)
    window, err := NormalizeWindow(from, from.Add(30*time.Minute), time.Hour)
    if err != nil { t.Fatal(err) }
    if window.From.Minute() != 34 { t.Fatalf("window start lost minute precision: %s", window.From) }
}
