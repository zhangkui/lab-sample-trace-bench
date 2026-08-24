package validation

import "time"

func InWindow(t, start, end time.Time) bool                       { return !t.Before(start) && !t.After(end) }
func Recent(t time.Time, limit time.Duration, now time.Time) bool { return now.Sub(t) <= limit }
