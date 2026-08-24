package ingest

import (
	"context"
	"time"
)

func Retry(ctx context.Context, attempts int, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = fn(); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * time.Millisecond):
		}
	}
	return err
}
