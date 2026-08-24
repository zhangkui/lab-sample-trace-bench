package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/metrics"
	"github.com/zhangkui/lab-sample-trace-bench/internal/store"
	"github.com/zhangkui/lab-sample-trace-bench/internal/validation"
)

var ErrNotFound = errors.New("record not found")

type Record struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Lab struct {
	repo    *store.Store
	metrics *metrics.Registry
}

func NewLab(repo *store.Store) *Lab { return &Lab{repo: repo, metrics: metrics.New()} }
func (l *Lab) Close()               {}
func (l *Lab) Put(r Record) error {
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Kind) == "" {
		return fmt.Errorf("id and kind are required")
	}
	if !validation.InWindow(r.CreatedAt, time.Unix(0, 0), time.Now().Add(time.Minute)) {
		return fmt.Errorf("created_at outside accepted window")
	}
	if !r.ExpiresAt.IsZero() && !r.ExpiresAt.After(r.CreatedAt) {
		return fmt.Errorf("expires_at must be after created_at")
	}
	if err := l.repo.Save(r.Kind, r.ID, r); err != nil {
		return err
	}
	l.metrics.Add("writes", 1)
	return l.repo.Event(r.ID, "put")
}
func (l *Lab) Get(kind, id string) (Record, error) {
	var r Record
	if err := l.repo.Load(kind, id, &r); err != nil {
		return r, ErrNotFound
	}
	return r, nil
}
func (l *Lab) List(kind string) ([]Record, error) {
	out := []Record{}
	err := l.repo.List(kind, func(raw []byte) error {
		var r Record
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		out = append(out, r)
		return nil
	})
	return out, err
}
func (l *Lab) Delete(kind, id string) error {
	if err := l.repo.Delete(kind, id); err != nil {
		return err
	}
	return l.repo.Event(id, "delete")
}
