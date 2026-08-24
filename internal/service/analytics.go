package service

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
)

// ContainerAging describes dwell-time distribution for shift planning.
type ContainerAging struct {
	Container string        `json:"container"`
	Status    string        `json:"status"`
	Slot      string        `json:"slot"`
	Age       time.Duration `json:"age"`
	Overdue   bool          `json:"overdue"`
}

// ContainerAgingReport groups boxes into operational dwell bands. The query
// reads the persisted update timestamp so it remains useful after a restart.
func (s *Service) ContainerAgingReport() ([]ContainerAging, error) {
	rows, err := s.repo.DB().Query(`SELECT id,status,slot,updated_at FROM containers ORDER BY updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := s.now()
	items := make([]ContainerAging, 0)
	for rows.Next() {
		var item ContainerAging
		var stamp string
		if err := rows.Scan(&item.Container, &item.Status, &item.Slot, &stamp); err != nil {
			return nil, err
		}
		updated, parseErr := time.Parse(time.RFC3339Nano, stamp)
		if parseErr != nil {
			return nil, fmt.Errorf("container %s has invalid updated_at: %w", item.Container, parseErr)
		}
		item.Age = now.Sub(updated)
		if item.Age < 0 {
			item.Age = 0
		}
		item.Overdue = item.Age >= overdueDwell && item.Status == string(domain.StatusStacked)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Age > items[j].Age })
	return items, nil
}

// ZoneUtilization is the capacity view used by planners before accepting
// additional gate-in work.
type ZoneUtilization struct {
	Zone      string  `json:"zone"`
	Name      string  `json:"name"`
	Capacity  int     `json:"capacity"`
	Occupied  int     `json:"occupied"`
	Available int     `json:"available"`
	Rate      float64 `json:"rate"`
}

func (s *Service) ZoneUtilization() ([]ZoneUtilization, error) {
	rows, err := s.repo.DB().Query(`SELECT z.code,z.name,
z.bay_count*z.rows_per_bay*z.tiers_per_row,
COALESCE(SUM(CASE WHEN sl.container <> '' THEN 1 ELSE 0 END),0)
FROM zones z LEFT JOIN slots sl ON sl.zone=z.code
GROUP BY z.code,z.name,z.bay_count,z.rows_per_bay,z.tiers_per_row ORDER BY z.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ZoneUtilization, 0)
	for rows.Next() {
		var item ZoneUtilization
		if err := rows.Scan(&item.Zone, &item.Name, &item.Capacity, &item.Occupied); err != nil {
			return nil, err
		}
		item.Available = item.Capacity - item.Occupied
		if item.Available < 0 {
			item.Available = 0
		}
		if item.Capacity > 0 {
			item.Rate = float64(item.Occupied) / float64(item.Capacity)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// MoveSummary is a compact throughput metric for the current reporting day.
type MoveSummary struct {
	Day        time.Time      `json:"day"`
	Total      int            `json:"total"`
	ByStatus   map[string]int `json:"by_status"`
	ByOperator map[string]int `json:"by_operator"`
}

func (s *Service) MoveSummary(day time.Time) (MoveSummary, error) {
	day = day.UTC()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	summary := MoveSummary{Day: start, ByStatus: map[string]int{}, ByOperator: map[string]int{}}
	if err := s.repo.DB().QueryRow(`SELECT COUNT(*) FROM moves WHERE at>=? AND at<?`, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano)).Scan(&summary.Total); err != nil {
		return MoveSummary{}, err
	}
	if err := countMoves(s.repo.DB(), `SELECT to_status,COUNT(*) FROM moves WHERE at>=? AND at<? GROUP BY to_status`, start, end, summary.ByStatus); err != nil {
		return MoveSummary{}, err
	}
	if err := countMoves(s.repo.DB(), `SELECT operator,COUNT(*) FROM moves WHERE at>=? AND at<? GROUP BY operator`, start, end, summary.ByOperator); err != nil {
		return MoveSummary{}, err
	}
	return summary, nil
}

func countMoves(db *sql.DB, query string, start, end time.Time, target map[string]int) error {
	rows, err := db.Query(query, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
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

// AnomalyWindow summarizes unresolved risk in a time window.
type AnomalyWindow struct {
	From       time.Time      `json:"from"`
	To         time.Time      `json:"to"`
	Total      int            `json:"total"`
	Breached   int            `json:"breached"`
	ByKind     map[string]int `json:"by_kind"`
	BySeverity map[string]int `json:"by_severity"`
}

func (s *Service) AnomalyWindow(from, to time.Time) (AnomalyWindow, error) {
	if from.IsZero() || to.IsZero() || !to.After(from) {
		return AnomalyWindow{}, fmt.Errorf("anomaly window must be non-empty")
	}
	window := AnomalyWindow{From: from, To: to, ByKind: map[string]int{}, BySeverity: map[string]int{}}
	query := `SELECT COUNT(*) FROM anomalies WHERE raised_at>=? AND raised_at<?`
	if err := s.repo.DB().QueryRow(query, from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano)).Scan(&window.Total); err != nil {
		return AnomalyWindow{}, err
	}
	if err := s.repo.DB().QueryRow(`SELECT COUNT(*) FROM anomalies WHERE raised_at>=? AND raised_at<? AND deadline<=? AND resolved_at=''`, from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano), s.now().Format(time.RFC3339Nano)).Scan(&window.Breached); err != nil {
		return AnomalyWindow{}, err
	}
	if err := countAnomalies(s.repo.DB(), `SELECT kind,COUNT(*) FROM anomalies WHERE raised_at>=? AND raised_at<? GROUP BY kind`, from, to, window.ByKind); err != nil {
		return AnomalyWindow{}, err
	}
	if err := countAnomalies(s.repo.DB(), `SELECT severity,COUNT(*) FROM anomalies WHERE raised_at>=? AND raised_at<? GROUP BY severity`, from, to, window.BySeverity); err != nil {
		return AnomalyWindow{}, err
	}
	return window, nil
}

func countAnomalies(db *sql.DB, query string, from, to time.Time, target map[string]int) error {
	rows, err := db.Query(query, from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano))
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

// SearchAudit subjects is intentionally narrow: reports may filter by actor
// or action without exposing arbitrary SQL fragments to callers.
func (s *Service) SearchAudit(actor, action string, limit int) ([]map[string]any, error) {
	actor = strings.TrimSpace(actor)
	action = strings.TrimSpace(action)
	if limit <= 0 || limit > 500 {
		return nil, fmt.Errorf("limit must be between 1 and 500")
	}
	rows, err := s.repo.DB().Query(`SELECT seq,subject,action,actor,detail,at FROM audit
WHERE (?='' OR actor=?) AND (?='' OR action=?) ORDER BY seq DESC LIMIT ?`, actor, actor, action, action, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var seq int
		var subject, storedAction, storedActor, detail, at string
		if err := rows.Scan(&seq, &subject, &storedAction, &storedActor, &detail, &at); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"seq": seq, "subject": subject, "action": storedAction, "actor": storedActor, "detail": detail, "at": at})
	}
	return result, rows.Err()
}
