// Package service implements the container-yard business logic that ties the
// domain model to the persistence layer and exposes operations the HTTP API can
// call. Each file in the package owns a coherent slice of the yard's work:
//
//   - service.go:   construction, time source, shared helpers
//   - container.go: container registration, release and search
//   - yard.go:      zones, slot provisioning and stacking policy
//   - move.go:      the gate-in / stack / rehandle / load / gate-out pipeline
//   - schedule.go:  vessel calls, cranes and the task scheduler
//   - anomaly.go:   anomaly raising, acknowledgement, escalation, resolution
//   - report.go:    occupancy, dwell and shift statistics
package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/audit"
	"github.com/zhangkui/lab-sample-trace-bench/internal/store"
)

var ErrNotFound = errors.New("not found")

// Service is the application root. It holds the store, the audit ledger and a
// clock function so tests can pin time. Methods on Service are the surface the
// API layer calls; none of them leak store or audit types to the caller.
type Service struct {
	repo   *store.Store
	audit  *audit.Ledger
	now    func() time.Time
	serial int // monotonic counter for synthetic ids
}

// New constructs a Service backed by repo. The audit ledger is initialised and
// its integrity checked before the service is returned so a corrupted ledger is
// detected at startup rather than mid-operation.
func New(repo *store.Store) (*Service, error) {
	ledger, err := audit.New(repo.DB())
	if err != nil {
		return nil, err
	}
	if idx, err := ledger.Verify(); err != nil {
		return nil, fmt.Errorf("verify audit: %w", err)
	} else if idx != 0 {
		return nil, fmt.Errorf("audit chain broken at entry %d", idx)
	}
	return &Service{
		repo:  repo,
		audit: ledger,
		now:   func() time.Time { return time.Now().UTC() },
	}, nil
}

// SetClock replaces the time source. Tests use it to freeze time so deadlines
// and ordering are deterministic.
func (s *Service) SetClock(f func() time.Time) { s.now = f }

// requireActor validates that an operator identity was supplied. The audit
// chain records the actor on every mutation, so an empty actor would produce
// unverifiable history.
func requireActor(actor string) error {
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("operator is required")
	}
	return nil
}

// id mints a short, monotonically increasing identifier under the given prefix.
// It is not globally unique across restarts, but the audit chain provides the
// authoritative ordering; the id is only a human handle.
func (s *Service) id(prefix string) string {
	s.serial++
	return fmt.Sprintf("%s-%d", prefix, s.serial)
}
