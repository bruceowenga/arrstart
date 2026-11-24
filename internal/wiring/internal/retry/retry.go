// Package retry provides a small, dependency-free backoff helper used by the
// wiring framework (and app clients) so every app retries transient failures
// with the same timing instead of each inlining its own loop.
//
// Nothing here is app-specific; it retries whatever func it is given until
// that func returns nil or the context is cancelled.
package retry

import (
	"context"
	"time"
)

// Backoff configures retry timing. Do waits after each failed attempt before
// trying again, doubling the delay each time up to Max. The first attempt is
// immediate.
type Backoff struct {
	// Initial is the delay after the first failure. Zero means a sensible
	// default (500 ms).
	Initial time.Duration
	// Max caps the delay between attempts. Zero means no cap.
	Max time.Duration
}

// Do calls f repeatedly until it returns nil or ctx is cancelled. The first
// call is immediate; subsequent calls are spaced by the backoff delay. The
// returned error is ctx.Err() on cancellation, otherwise the last error from f.
func (b Backoff) Do(ctx context.Context, f func() error) error {
	delay := b.Initial
	if delay <= 0 {
		delay = 500 * time.Millisecond
	}
	for {
		if err := f(); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if b.Max > 0 && delay < b.Max {
			delay *= 2
			if delay > b.Max {
				delay = b.Max
			}
		}
	}
}
