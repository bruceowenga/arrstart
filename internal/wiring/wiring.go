// Package wiring defines the shared interface every app wiring client
// implements (PRD §10.5, §12.1): WaitHealthy → Configure → Verify.
// One package per app lives under internal/wiring/<app>. No Buildarr
// dependency; only the narrow set of API calls each app needs.
//
// The framework types below (Step, StepResult, Summary, AppStep, Runner)
// sit on top of the Client interface: a Runner executes WaitHealthy,
// Configure, Verify for every app, isolates failures so one app's malfunction
// does not abort the others (A11), and reports a per-step summary. Runs are
// idempotent because each client's Configure/Verify is itself idempotent
// (A10); re-running the same wiring changes nothing already correct.
package wiring

import (
	"context"
	"errors"
	"time"
)

// Client is the interface every app wiring client must implement.
// Configure receives an app-specific config struct; the concrete type is
// documented on each implementation.
type Client interface {
	// WaitHealthy blocks until the app's API responds (or ctx is cancelled).
	// It does not authenticate — a server that answers with any HTTP status
	// counts as healthy; authentication is handled by Configure.
	WaitHealthy(ctx context.Context) error
	// Configure authenticates and applies the app-specific cfg.
	Configure(ctx context.Context, cfg any) error
	// Verify re-authenticates and checks that the expected state is present.
	Verify(ctx context.Context) error
}

// Step identifies which wiring stage produced a result.
type Step int

const (
	// StepWaitHealthy is the WaitHealthy stage.
	StepWaitHealthy Step = iota
	// StepConfigure is the Configure stage.
	StepConfigure
	// StepVerify is the Verify stage.
	StepVerify
)

// String returns a lower-case, human-readable step name.
func (s Step) String() string {
	switch s {
	case StepWaitHealthy:
		return "waithealthy"
	case StepConfigure:
		return "configure"
	case StepVerify:
		return "verify"
	default:
		return "unknown"
	}
}

// StepResult is the outcome of running one wiring step on one app. Err is
// non-nil exactly when OK is false.
type StepResult struct {
	App      string
	Step     Step
	OK       bool
	Err      error
	Duration time.Duration
}

// Summary collects per-app, per-step results. A11: failures in one step do
// not abort the others — every step that could run is reported here. FailedCount
// is computed from Results, so a Summary built directly (not via Runner.Run)
// still reports failures correctly.
type Summary struct {
	Results []StepResult // one entry per executed step
}

// FailedCount returns the number of failed steps.
func (s Summary) FailedCount() int {
	n := 0
	for _, r := range s.Results {
		if !r.OK {
			n++
		}
	}
	return n
}

// HasFailures reports whether any step failed.
func (s Summary) HasFailures() bool {
	return s.FailedCount() > 0
}

// Error returns a single error summarizing every failure, or nil when the run
// was fully successful.
func (s Summary) Error() error {
	if !s.HasFailures() {
		return nil
	}
	errs := make([]error, 0, s.FailedCount())
	for _, r := range s.Results {
		if !r.OK {
			errs = append(errs, r.Err)
		}
	}
	return errors.Join(errs...)
}

// AppStep pairs a wiring client with the config to apply to it. Name is the
// display name used in results (e.g. "sonarr"). Config may be nil when only
// WaitHealthy and Verify should run for that app.
type AppStep struct {
	Name   string
	Client Client
	Config any
}
