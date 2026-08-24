package trace

import (
	"fmt"
	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	SampleID string    `json:"sample_id"`
	Sequence int       `json:"sequence"`
	Action   string    `json:"action"`
	At       time.Time `json:"at"`
	Actor    string    `json:"actor"`
	Detail   string    `json:"detail"`
}

func NewEntry(sampleID string, sequence int, action, actor, detail string, at time.Time) (Entry, error) {
	if strings.TrimSpace(sampleID) == "" || strings.TrimSpace(action) == "" || strings.TrimSpace(actor) == "" {
		return Entry{}, fmt.Errorf("sample_id, action and actor are required")
	}
	return Entry{SampleID: sampleID, Sequence: sequence, Action: action, Actor: actor, Detail: detail, At: at.UTC()}, nil
}
func Validate(entries []Entry) error {
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Sequence < sorted[j].Sequence })
	for i, e := range sorted {
		if e.Sequence != i+1 {
			return fmt.Errorf("trace sequence must be contiguous")
		}
		if e.At.IsZero() {
			return fmt.Errorf("trace time is required")
		}
		if i > 0 && e.At.Before(sorted[i-1].At) {
			return fmt.Errorf("trace timestamps must not move backwards")
		}
	}
	return nil
}
func FromTransition(t domain.Transition, sequence int) (Entry, error) {
	return NewEntry(t.SampleID, sequence, "status:"+string(t.To), t.Operator, t.Note, t.At)
}
