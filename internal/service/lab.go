package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
	"github.com/zhangkui/lab-sample-trace-bench/internal/store"
	"github.com/zhangkui/lab-sample-trace-bench/internal/trace"
	"sort"
	"strings"
	"time"
)

type SampleService struct {
	repo *store.Store
	now  func() time.Time
}

func NewSampleService(repo *store.Store) *SampleService {
	return &SampleService{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}
func (s *SampleService) Create(sample domain.Sample, actor string) (domain.Sample, error) {
	sample.NormalizeTags()
	if sample.Status == "" {
		sample.Status = domain.StatusReceived
	}
	if err := sample.Validate(s.now()); err != nil {
		return domain.Sample{}, err
	}
	if err := s.repo.Save("sample", sample.ID, sample); err != nil {
		return domain.Sample{}, err
	}
	entry, err := trace.NewEntry(sample.ID, 1, "received", actor, "sample registered", s.now())
	if err != nil {
		return domain.Sample{}, err
	}
	if err = s.repo.Save("trace", sample.ID, []trace.Entry{entry}); err != nil {
		return domain.Sample{}, err
	}
	return sample, nil
}
func (s *SampleService) Get(id string) (domain.Sample, error) {
	var sample domain.Sample
	if err := s.repo.Load("sample", id, &sample); err != nil {
		return domain.Sample{}, ErrNotFound
	}
	return sample, nil
}
func (s *SampleService) Transition(id string, to domain.Status, actor, note string) (domain.Sample, error) {
	sample, err := s.Get(id)
	if err != nil {
		return domain.Sample{}, err
	}
	if !domain.CanTransition(sample.Status, to) {
		return domain.Sample{}, fmt.Errorf("cannot transition %s to %s", sample.Status, to)
	}
	var entries []trace.Entry
	if err = s.repo.Load("trace", id, &entries); err != nil && err != sql.ErrNoRows {
		return domain.Sample{}, err
	}
	transition := domain.Transition{SampleID: id, From: sample.Status, To: to, At: s.now(), Operator: actor, Note: note}
	entry, err := trace.FromTransition(transition, len(entries)+1)
	if err != nil {
		return domain.Sample{}, err
	}
	entries = append(entries, entry)
	if err = trace.Validate(entries); err != nil {
		return domain.Sample{}, err
	}
	sample.Status = to
	sample.Revision++
	if err = s.repo.Save("sample", id, sample); err != nil {
		return domain.Sample{}, err
	}
	if err = s.repo.Save("trace", id, entries); err != nil {
		return domain.Sample{}, err
	}
	return sample, nil
}
func (s *SampleService) Search(project, status string, asOf time.Time) ([]domain.Sample, error) {
	var samples []domain.Sample
	err := s.repo.List("sample", func(raw []byte) error {
		var sample domain.Sample
		if err := json.Unmarshal(raw, &sample); err != nil {
			return err
		}
		if project != "" && sample.Project != project {
			return nil
		}
		if status != "" && string(sample.Status) != status {
			return nil
		}
		if !asOf.IsZero() && sample.CollectedAt.After(asOf) {
			return nil
		}
		samples = append(samples, sample)
		return nil
	})
	sort.Slice(samples, func(i, j int) bool { return samples[i].CollectedAt.Before(samples[j].CollectedAt) })
	return samples, err
}
func (s *SampleService) Expired(asOf time.Time) ([]domain.Sample, error) {
	return s.Search("", "", asOf)
}
func validActor(actor string) error {
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("operator is required")
	}
	return nil
}
