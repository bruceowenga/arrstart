package retry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// countingFunc fails failTimes times, then succeeds. failures counts total
// invocations.
type countingFunc struct {
	failTimes int32
	failures  atomic.Int32
}

func (f *countingFunc) call() error {
	n := f.failures.Add(1)
	if int32(n) <= f.failTimes {
		return errors.New("transient")
	}
	return nil
}

func TestBackoffDoSucceedsFirstTry(t *testing.T) {
	var f countingFunc
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b := Backoff{}
	if err := b.Do(ctx, f.call); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := f.failures.Load(); got != 1 {
		t.Fatalf("invoked %d times, want 1", got)
	}
}

func TestBackoffDoRetriesUntilSuccess(t *testing.T) {
	var f countingFunc
	f.failTimes = 2
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b := Backoff{Initial: 10 * time.Millisecond}
	if err := b.Do(ctx, f.call); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := f.failures.Load(); got != 3 {
		t.Fatalf("invoked %d times, want 3 (2 failures + 1 success)", got)
	}
}

func TestBackoffDoCtxCancelled(t *testing.T) {
	var f countingFunc
	f.failTimes = 1 // first call fails, then the already-cancelled ctx wins
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := Backoff{Initial: 10 * time.Millisecond}
	err := b.Do(ctx, f.call)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Do: got %v, want context.Canceled", err)
	}
}

func TestBackoffDoMaxCap(t *testing.T) {
	// failTimes is high enough that the func always fails within our window,
	// so Do must return ctx.DeadlineExceeded once the ctx expires.
	var f countingFunc
	f.failTimes = 100
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	b := Backoff{Initial: 10 * time.Millisecond, Max: 100 * time.Millisecond}
	err := b.Do(ctx, f.call)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Do: got %v, want context.DeadlineExceeded", err)
	}
}
