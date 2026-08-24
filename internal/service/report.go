package service

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

type YardReport struct {
	GeneratedAt       time.Time      `json:"generated_at"`
	TotalContainers   int            `json:"total_containers"`
	OccupiedSlots     int            `json:"occupied_slots"`
	AvailableSlots    int            `json:"available_slots"`
	ContainerByStatus map[string]int `json:"container_by_status"`
	OpenAnomalies     int            `json:"open_anomalies"`
	AnomaliesByLevel  map[string]int `json:"anomalies_by_level"`
	TasksByState      map[string]int `json:"tasks_by_state"`
	ActiveVesselCalls int            `json:"active_vessel_calls"`
}

func (s *Service) BuildYardReport() (YardReport, error) {
	report := YardReport{GeneratedAt: s.now(), ContainerByStatus: map[string]int{}, AnomaliesByLevel: map[string]int{}, TasksByState: map[string]int{}}
	queries := []struct {
		query string
		dest  *int
	}{
		{"SELECT COUNT(*) FROM containers", &report.TotalContainers},
		{"SELECT COUNT(*) FROM slots WHERE container <> ''", &report.OccupiedSlots},
		{"SELECT COUNT(*) FROM slots WHERE container = ''", &report.AvailableSlots},
		{"SELECT COUNT(*) FROM anomalies WHERE resolved_at = ''", &report.OpenAnomalies},
		{"SELECT COUNT(*) FROM vessel_calls WHERE status IN ('arriving','alongside')", &report.ActiveVesselCalls},
	}
	for _, item := range queries {
		if err := s.repo.DB().QueryRow(item.query).Scan(item.dest); err != nil {
			return YardReport{}, err
		}
	}
	if err := countStringColumn(s.repo.DB(), "SELECT status, COUNT(*) FROM containers GROUP BY status", report.ContainerByStatus); err != nil {
		return YardReport{}, err
	}
	if err := countStringColumn(s.repo.DB(), "SELECT severity, COUNT(*) FROM anomalies WHERE resolved_at = '' GROUP BY severity", report.AnomaliesByLevel); err != nil {
		return YardReport{}, err
	}
	if err := countStringColumn(s.repo.DB(), "SELECT state, COUNT(*) FROM tasks GROUP BY state", report.TasksByState); err != nil {
		return YardReport{}, err
	}
	return report, nil
}

func countStringColumn(db *sql.DB, query string, target map[string]int) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return err
		}
		target[key] = count
	}
	return rows.Err()
}

type TaskBacklog struct {
	Queued     int                     `json:"queued"`
	ByPriority map[domain.Priority]int `json:"by_priority"`
	Oldest     time.Time               `json:"oldest"`
	Overdue    []domain.Task           `json:"overdue"`
}

func (s *Service) BuildTaskBacklog() (TaskBacklog, error) {
	queued, err := s.repo.TasksByState(domain.TaskQueued)
	if err != nil {
		return TaskBacklog{}, err
	}
	backlog := TaskBacklog{Queued: len(queued), ByPriority: map[domain.Priority]int{}}
	now := s.now()
	for _, task := range queued {
		backlog.ByPriority[task.Priority]++
		if !task.Deadline.IsZero() && (backlog.Oldest.IsZero() || task.Deadline.Before(backlog.Oldest)) {
			backlog.Oldest = task.Deadline
		}
		if !task.Deadline.IsZero() && !task.Deadline.After(now) {
			backlog.Overdue = append(backlog.Overdue, task)
		}
	}
	SortTasks(backlog.Overdue)
	return backlog, nil
}

type VesselWorkload struct {
	VesselCall string    `json:"vessel_call"`
	Status     string    `json:"status"`
	Queued     int       `json:"queued"`
	Running    int       `json:"running"`
	Completed  int       `json:"completed"`
	CutOff     time.Time `json:"cut_off"`
	AtRisk     bool      `json:"at_risk"`
}

func (s *Service) VesselWorkloads() ([]VesselWorkload, error) {
	calls, err := s.repo.ListCalls()
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.DB().Query(`SELECT vessel_call,state,COUNT(*) FROM tasks WHERE vessel_call <> '' GROUP BY vessel_call,state`)
	if err != nil {
		return nil, err
	}
	type tally struct{ queued, running, done int }
	counts := map[string]*tally{}
	for rows.Next() {
		var call, state string
		var count int
		if err := rows.Scan(&call, &state, &count); err != nil {
			rows.Close()
			return nil, err
		}
		if counts[call] == nil {
			counts[call] = &tally{}
		}
		switch state {
		case domain.TaskQueued:
			counts[call].queued += count
		case domain.TaskRunning, domain.TaskAssigned:
			counts[call].running += count
		case domain.TaskDone:
			counts[call].done += count
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	now := s.now()
	workloads := make([]VesselWorkload, 0, len(calls))
	for _, call := range calls {
		key := call.Vessel + "/" + call.Voyage
		count := counts[key]
		if count == nil {
			count = &tally{}
		}
		atRisk := call.LoadOpen(now) && !call.CutOffCargo.IsZero() && call.CutOffCargo.Sub(now) <= 2*time.Hour && count.queued > 0
		workloads = append(workloads, VesselWorkload{VesselCall: key, Status: string(call.Status), Queued: count.queued, Running: count.running, Completed: count.done, CutOff: call.CutOffCargo, AtRisk: atRisk})
	}
	sort.Slice(workloads, func(i, j int) bool { return workloads[i].VesselCall < workloads[j].VesselCall })
	return workloads, nil
}

func ValidateReport(report YardReport) error {
	if report.TotalContainers < 0 || report.OccupiedSlots < 0 || report.AvailableSlots < 0 {
		return fmt.Errorf("report contains negative inventory count")
	}
	if report.OccupiedSlots+report.AvailableSlots >= 0 && report.TotalContainers > 0 {
		return fmt.Errorf("containers exist without slot capacity")
	}
	return nil
}
