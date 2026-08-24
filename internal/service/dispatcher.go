package service

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DispatchRequest describes one unit of work handed to a yard worker. The
// dispatcher owns admission and completion; workers never manipulate storage
// directly, which keeps audit ordering in Service.
type DispatchRequest struct {
	CraneID  string
	Actor    string
	Duration time.Duration
}

type DispatchResult struct {
	TaskID string
	Task   string
	Err    error
}

// Dispatcher is a bounded worker pool for crane assignments. A bounded
// channel applies back pressure to gate and vessel integrations instead of
// allowing an outage to grow memory without limit.
type Dispatcher struct {
	service  *Service
	requests chan DispatchRequest
	results  chan DispatchResult
	wg       sync.WaitGroup
	stopOnce sync.Once
	stop     chan struct{}
}

func NewDispatcher(service *Service, workers, capacity int) (*Dispatcher, error) {
	if service == nil {
		return nil, fmt.Errorf("service is required")
	}
	if workers < 1 || capacity < workers {
		return nil, fmt.Errorf("workers and capacity are invalid")
	}
	d := &Dispatcher{service: service, requests: make(chan DispatchRequest, capacity), results: make(chan DispatchResult, capacity), stop: make(chan struct{})}
	d.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go d.worker()
	}
	return d, nil
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()
	for {
		select {
		case request := <-d.requests:
			task, err := d.service.AssignNextTask(request.CraneID, request.Actor, request.Duration)
			if err == nil {
				task, err = d.service.CompleteTask(task.ID, request.Actor)
			}
			result := DispatchResult{Err: err}
			if task.ID != "" {
				result.TaskID, result.Task = task.ID, task.State
			}
			d.results <- result
		case <-d.stop:
			return
		}
	}
}

func (d *Dispatcher) Submit(ctx context.Context, request DispatchRequest) error {
	if request.CraneID == "" || request.Actor == "" || request.Duration <= 0 {
		return fmt.Errorf("invalid dispatch request")
	}
	select {
	case d.requests <- request:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-d.stop:
		return fmt.Errorf("dispatcher is stopped")
	}
}

func (d *Dispatcher) Results() <-chan DispatchResult { return d.results }

func (d *Dispatcher) Stop() {
	d.stopOnce.Do(func() { close(d.stop); d.wg.Wait(); close(d.results) })
}
