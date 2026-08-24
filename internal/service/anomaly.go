package service

import (
	"fmt"
	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
	"strings"
	"time"
)

func (s *Service) RaiseAnomaly(a domain.Anomaly, actor string) (domain.Anomaly, error) {
	if err := requireActor(actor); err != nil {
		return domain.Anomaly{}, err
	}
	if a.RaisedAt.IsZero() {
		a.RaisedAt = s.now()
	}
	if err := a.Validate(s.now()); err != nil {
		return domain.Anomaly{}, err
	}
	if err := s.repo.SaveAnomaly(a); err != nil {
		return domain.Anomaly{}, err
	}
	if _, err := s.audit.Append(a.ID, "anomaly_raised", actor, map[string]any{"kind": a.Kind, "container": a.Container, "deadline": a.Deadline}); err != nil {
		return domain.Anomaly{}, err
	}
	return a, nil
}
func (s *Service) GetAnomaly(id string) (domain.Anomaly, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Anomaly{}, fmt.Errorf("anomaly id is required")
	}
	return s.repo.LoadAnomaly(id)
}
func (s *Service) AcknowledgeAnomaly(id, actor string) (domain.Anomaly, error) {
	if err := requireActor(actor); err != nil {
		return domain.Anomaly{}, err
	}
	a, err := s.GetAnomaly(id)
	if err != nil {
		return domain.Anomaly{}, err
	}
	if a.AcknowledgedAt.IsZero() {
		a.AcknowledgedAt = s.now()
		if err := s.repo.SaveAnomaly(a); err != nil {
			return domain.Anomaly{}, err
		}
		if _, err := s.audit.Append(id, "anomaly_acknowledged", actor, nil); err != nil {
			return domain.Anomaly{}, err
		}
	}
	return a, nil
}
func (s *Service) EscalateBreached(actor string) ([]domain.Anomaly, error) {
	if err := requireActor(actor); err != nil {
		return nil, err
	}
	open, err := s.repo.OpenAnomalies("")
	if err != nil {
		return nil, err
	}
	changed := []domain.Anomaly{}
	for _, a := range open {
		if !a.Breached(s.now()) {
			continue
		}
		next := domain.NextTier(a.Escalation, a.Severity)
		if next == a.Escalation {
			continue
		}
		a.Escalation = next
		if err := s.repo.SaveAnomaly(a); err != nil {
			return nil, err
		}
		if _, err := s.audit.Append(a.ID, "anomaly_escalated", actor, map[string]any{"tier": next}); err != nil {
			return nil, err
		}
		changed = append(changed, a)
	}
	return changed, nil
}
func (s *Service) ResolveAnomaly(id, actor, resolution string) (domain.Anomaly, error) {
	if err := requireActor(actor); err != nil {
		return domain.Anomaly{}, err
	}
	resolution = strings.TrimSpace(resolution)
	if resolution == "" {
		return domain.Anomaly{}, fmt.Errorf("resolution is required")
	}
	a, err := s.GetAnomaly(id)
	if err != nil {
		return domain.Anomaly{}, err
	}
	if a.ResolvedAt.IsZero() {
		a.ResolvedAt = s.now()
		a.Resolution = resolution
		if err := s.repo.SaveAnomaly(a); err != nil {
			return domain.Anomaly{}, err
		}
		if _, err := s.audit.Append(id, "anomaly_resolved", actor, map[string]any{"resolution": resolution}); err != nil {
			return domain.Anomaly{}, err
		}
	}
	return a, nil
}
func (s *Service) OpenAnomalies(container string) ([]domain.Anomaly, error) {
	return s.repo.OpenAnomalies(strings.TrimSpace(container))
}
func (s *Service) AnomalySummary(resolved bool) (map[domain.Severity]int, error) {
	return s.repo.CountAnomalies(resolved)
}
func IsDue(a domain.Anomaly, now time.Time) bool {
	return a.ResolvedAt.IsZero() && !a.Deadline.After(now)
}
