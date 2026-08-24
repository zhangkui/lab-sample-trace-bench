package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

func (s *Service) RegisterCall(call domain.VesselCall, actor string) (domain.VesselCall, error) {
	if err := requireActor(actor); err != nil {
		return domain.VesselCall{}, err
	}
	if call.Status == "" {
		call.Status = domain.ScheduleProposed
	}
	if err := call.Validate(); err != nil {
		return domain.VesselCall{}, err
	}
	if err := s.repo.SaveCall(call); err != nil {
		return domain.VesselCall{}, err
	}
	if _, err := s.audit.Append(call.Vessel+"/"+call.Voyage, "schedule_registered", actor, map[string]any{"eta": call.ETA, "etd": call.ETD, "status": call.Status}); err != nil {
		return domain.VesselCall{}, err
	}
	return call, nil
}

func (s *Service) GetCall(vessel, voyage string) (domain.VesselCall, error) {
	vessel, voyage = strings.TrimSpace(vessel), strings.ToUpper(strings.TrimSpace(voyage))
	if vessel == "" || voyage == "" {
		return domain.VesselCall{}, fmt.Errorf("vessel and voyage are required")
	}
	return s.repo.LoadCall(vessel, voyage)
}

func (s *Service) AdvanceCall(vessel, voyage string, next domain.ScheduleStatus, actor string) (domain.VesselCall, error) {
	if err := requireActor(actor); err != nil {
		return domain.VesselCall{}, err
	}
	call, err := s.GetCall(vessel, voyage)
	if err != nil {
		return domain.VesselCall{}, err
	}
	if !next.Valid() || !validScheduleTransition(call.Status, next) {
		return domain.VesselCall{}, fmt.Errorf("cannot move schedule from %s to %s", call.Status, next)
	}
	previous := call.Status
	call.Status = next
	if err := s.repo.SaveCall(call); err != nil {
		return domain.VesselCall{}, err
	}
	if _, err := s.audit.Append(call.Vessel+"/"+call.Voyage, "schedule_advanced", actor, map[string]any{"from": previous, "to": next}); err != nil {
		return domain.VesselCall{}, err
	}
	return call, nil
}

func validScheduleTransition(from, to domain.ScheduleStatus) bool {
	switch from {
	case domain.ScheduleProposed:
		return to == domain.ScheduleArriving
	case domain.ScheduleArriving:
		return to == domain.ScheduleAlongside || to == domain.ScheduleDeparted
	case domain.ScheduleAlongside:
		return to == domain.ScheduleDeparted
	default:
		return false
	}
}

func (s *Service) RegisterCrane(crane domain.Crane, actor string) (domain.Crane, error) {
	if err := requireActor(actor); err != nil {
		return domain.Crane{}, err
	}
	if err := crane.Validate(); err != nil {
		return domain.Crane{}, err
	}
	if err := s.repo.SaveCrane(crane); err != nil {
		return domain.Crane{}, err
	}
	if _, err := s.audit.Append(crane.ID, "crane_registered", actor, map[string]any{"zone": crane.ZoneCode}); err != nil {
		return domain.Crane{}, err
	}
	return crane, nil
}

func (s *Service) CreateTask(task domain.Task, actor string) (domain.Task, error) {
	if err := requireActor(actor); err != nil {
		return domain.Task{}, err
	}
	task.ID = strings.TrimSpace(task.ID)
	if task.ID == "" {
		task.ID = s.id("task")
	}
	task.Container = strings.TrimSpace(task.Container)
	if task.Container == "" || !task.Kind.Valid() {
		return domain.Task{}, fmt.Errorf("container and valid task kind are required")
	}
	if task.State == "" {
		task.State = domain.TaskQueued
	}
	if task.State != domain.TaskQueued {
		return domain.Task{}, fmt.Errorf("new task must be queued")
	}
	task.Priority = domain.Weight(task.Kind, task.Deadline, s.now())
	if task.VesselCall != "" {
		parts := strings.SplitN(task.VesselCall, "/", 2)
		if len(parts) != 2 {
			return domain.Task{}, fmt.Errorf("vessel call must be vessel/voyage")
		}
		call, err := s.GetCall(parts[0], parts[1])
		if err != nil {
			return domain.Task{}, err
		}
		if !call.LoadOpen(s.now()) && (task.Kind == domain.JobLoad || task.Kind == domain.JobDischarge) {
			return domain.Task{}, fmt.Errorf("vessel cut-off is closed")
		}
	}
	if err := s.repo.SaveTask(task); err != nil {
		return domain.Task{}, err
	}
	if _, err := s.audit.Append(task.ID, "task_created", actor, map[string]any{"kind": task.Kind, "priority": task.Priority}); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *Service) AssignNextTask(craneID, actor string, duration time.Duration) (domain.Task, error) {
	if err := requireActor(actor); err != nil {
		return domain.Task{}, err
	}
	if duration <= 0 {
		return domain.Task{}, fmt.Errorf("duration must be positive")
	}
	crane, err := s.repo.LoadCrane(strings.TrimSpace(craneID))
	if err != nil {
		return domain.Task{}, err
	}
	now := s.now()
	if !crane.Free(now) {
		return domain.Task{}, fmt.Errorf("crane %s is busy until %s", crane.ID, crane.BusyUntil.Format(time.RFC3339))
	}
	queued, err := s.repo.TasksByState(domain.TaskQueued)
	if err != nil {
		return domain.Task{}, err
	}
	if len(queued) == 0 {
		return domain.Task{}, fmt.Errorf("no queued task")
	}
	SortTasks(queued)
	task := queued[0]
	task.Crane, task.State, task.Started = crane.ID, domain.TaskAssigned, now
	if err := s.repo.SaveTask(task); err != nil {
		return domain.Task{}, err
	}
	crane.BusyUntil = now.Add(duration)
	if err := s.repo.SaveCrane(crane); err != nil {
		return domain.Task{}, err
	}
	if _, err := s.audit.Append(task.ID, "task_assigned", actor, map[string]any{"crane": crane.ID, "busy_until": crane.BusyUntil}); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *Service) CompleteTask(id, actor string) (domain.Task, error) {
	if err := requireActor(actor); err != nil {
		return domain.Task{}, err
	}
	task, err := s.repo.LoadTask(strings.TrimSpace(id))
	if err != nil {
		return domain.Task{}, err
	}
	if task.State == domain.TaskDone {
		return task, nil
	}
	if task.State != domain.TaskAssigned && task.State != domain.TaskRunning {
		return domain.Task{}, fmt.Errorf("task %s is not active", task.ID)
	}
	task.State, task.Finished = domain.TaskDone, s.now()
	if err := s.repo.SaveTask(task); err != nil {
		return domain.Task{}, err
	}
	if task.Crane != "" {
		crane, craneErr := s.repo.LoadCrane(task.Crane)
		if craneErr == nil {
			crane.BusyUntil = s.now()
			if craneErr = s.repo.SaveCrane(crane); craneErr != nil {
				return domain.Task{}, craneErr
			}
		}
	}
	if _, err := s.audit.Append(task.ID, "task_completed", actor, map[string]any{"finished": task.Finished}); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func SortTasks(tasks []domain.Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority < tasks[j].Priority
		}
		if tasks[i].Deadline.IsZero() != tasks[j].Deadline.IsZero() {
			return !tasks[i].Deadline.IsZero()
		}
		if !tasks[i].Deadline.Equal(tasks[j].Deadline) {
			return tasks[i].Deadline.Before(tasks[j].Deadline)
		}
		return tasks[i].ID < tasks[j].ID
	})
}
