package service

import (
	"context"
	"fmt"
	"time"
)

// QueueDepth exposes bounded queue pressure for the operations dashboard.
func (d *Dispatcher) QueueDepth() int { return len(d.requests) }

// WaitResult waits for one worker outcome while preserving caller cancellation.
func (d *Dispatcher) WaitResult(ctx context.Context) (DispatchResult, error) {
	select {
	case result, ok := <-d.results:
		if !ok {
			return DispatchResult{}, fmt.Errorf("dispatcher is stopped")
		}
		return result, result.Err
	case <-ctx.Done():
		return DispatchResult{}, ctx.Err()
	}
}

// WaitUntilIdle gives a supervisor a bounded way to wait for queued work to
// drain without polling the database in a tight loop.
func (d *Dispatcher) WaitUntilIdle(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if d.QueueDepth() == 0 {
			return nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		case <-d.stop:
			return fmt.Errorf("dispatcher is stopped")
		}
	}
}
