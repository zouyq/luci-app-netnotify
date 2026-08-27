//go:build !linux

package neigh

import (
	"context"
)

// StubWatcher is a no-op on non-linux so main builds on Windows.
type StubWatcher struct{}

func NewWatcher() Watcher {
	return &StubWatcher{}
}

func (w *StubWatcher) Watch(ctx context.Context, out chan<- Event) error {
	<-ctx.Done()
	return ctx.Err()
}

func (w *StubWatcher) Dump() ([]Event, error) {
	return nil, nil
}
