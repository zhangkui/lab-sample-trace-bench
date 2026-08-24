package service

import (
    "context"
    "testing"
)

func TestBug004StoppedDispatcherReturnsError(t *testing.T) {
    dispatcher := &Dispatcher{results: make(chan DispatchResult), stop: make(chan struct{})}
    close(dispatcher.results)
    result, err := dispatcher.WaitResult(context.Background())
    if err == nil || result.TaskID != "" { t.Fatalf("closed result stream was treated as a successful result: %#v %v", result, err) }
}
