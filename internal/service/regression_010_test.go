package service

import (
    "testing"
    "github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

func TestBug010AssignedTaskRequiresCrane(t *testing.T) {
    err := ValidateTaskReferences(domain.Task{ID:"T1", Container:"C1", Kind:domain.JobRestow, State:domain.TaskAssigned})
    if err == nil { t.Fatal("assigned task without crane was accepted") }
}
