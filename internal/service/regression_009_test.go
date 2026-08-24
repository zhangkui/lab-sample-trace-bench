package service

import (
    "testing"
    "github.com/zhangkui/lab-sample-trace-bench/internal/domain"
    "github.com/zhangkui/lab-sample-trace-bench/internal/store"
)

func TestBug009CapacityAlertThresholdIsInclusive(t *testing.T) {
    repo, err := store.Open(":memory:"); if err != nil { t.Fatal(err) }; defer repo.Close()
    app, err := New(repo); if err != nil { t.Fatal(err) }
    if _, err = app.DefineZone(domain.Zone{Code:"A", Name:"A", Type:domain.ZoneGeneral, BayCount:1, RowsPerBay:1, TiersPerRow:5}, "ops"); err != nil { t.Fatal(err) }
    for tier := 1; tier <= 4; tier++ { if err := app.repo.SaveSlot(store.SlotRow{ID:string(domain.NewSlot("A",1,1,tier)), Zone:"A", Container:"C"}); err != nil { t.Fatal(err) } }
    alerts, err := app.CapacityAlerts(0.8); if err != nil { t.Fatal(err) }
    if len(alerts) != 1 { t.Fatalf("expected inclusive threshold alert, got %d", len(alerts)) }
}
