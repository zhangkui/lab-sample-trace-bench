package service

import (
    "testing"
    "time"
    "github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

func TestBug008VesselTaskNeedsCallReference(t *testing.T) {
    app := &Service{now: func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) }}
    admission := app.CheckTaskAdmission(domain.Task{Container:"C1", Kind:domain.JobLoad})
    if admission.Allowed { t.Fatal("load task without vessel call was admitted") }
}
