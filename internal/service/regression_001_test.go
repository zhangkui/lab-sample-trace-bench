package service

import (
    "testing"
    "github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

func TestBug001UrgentTasksRemainFirst(t *testing.T) {
    tasks := []domain.Task{{ID:"normal", Priority:domain.PriorityStandard}, {ID:"urgent", Priority:domain.PriorityUrgent}}
    SortTasks(tasks)
    if tasks[0].ID != "urgent" { t.Fatalf("urgent task was ordered after normal work: %v", tasks) }
}
