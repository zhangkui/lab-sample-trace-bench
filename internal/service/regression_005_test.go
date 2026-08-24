package service

import (
    "testing"
    "github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

func TestBug005DepartedCallCannotBecomeAlongside(t *testing.T) {
    if validScheduleTransition(domain.ScheduleDeparted, domain.ScheduleAlongside) { t.Fatal("departed schedule was allowed to re-enter active state") }
}
