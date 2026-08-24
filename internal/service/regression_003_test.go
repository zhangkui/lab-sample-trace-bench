package service

import "testing"

func TestBug003ValidInventoryReportIsAccepted(t *testing.T) {
    report := YardReport{TotalContainers: 1, OccupiedSlots: 1, AvailableSlots: 2}
    if err := ValidateReport(report); err != nil { t.Fatalf("valid capacity report rejected: %v", err) }
}
