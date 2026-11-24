// Package wiring (continued): Runner executes WaitHealthy → Configure → Verify
// for a collection of apps, isolating failures so one app's malfunction does not
// abort the others (PRD A11) and reporting a per-step summary. Runs are
// idempotent (A10): each client's Configure/Verify is itself idempotent, so
// re-running changes nothing already correct.
package wiring

import (
	"context"
	"time"

	"github.com/bruceowenga/arrstart/internal/wiring/internal/retry"
)

// Runner executes WaitHealthy, Configure, Verify for every AppStep, one app at
// a time. A failure in one app's step does not abort the remaining apps
// (failure isolation, A11); every step that can run is recorded in the Summary.
type Runner struct {
	// Backoff is the retry timing applied to WaitHealthy (PRD A2: wait for each
	// app's health endpoint before configuring it). Zero uses retry defaults.
	Backoff retry.Backoff

	// OnResult is invoked after each step completes, before Run returns. May be
	// nil.
	OnResult func(StepResult)
}

// runStep times a single step function, records the result, and invokes OnResult.
func (r *Runner) runStep(app string, step Step, fn func() error) StepResult {
	start := time.Now()
	err := fn()
	res := StepResult{
		App:      app,
		Step:     step,
		Duration: time.Since(start),
	}
	if err != nil {
		res.OK = false
		res.Err = err
	} else {
		res.OK = true
	}
	if r.OnResult != nil {
		r.OnResult(res)
	}
	return res
}

// Run executes every AppStep and returns a summary.
func (r *Runner) Run(ctx context.Context, steps []AppStep) Summary {
	summary := Summary{Results: make([]StepResult, 0, len(steps)*3)}

	for _, s := range steps {
		app := s.Name
		client := s.Client

		// WaitHealthy (A2) — retried with backoff until the app answers or ctx
		// is cancelled. App clients also retry internally; this is the
		// framework-level retry hook.
		summary.Results = append(summary.Results, r.runStep(app, StepWaitHealthy, func() error {
			b := r.Backoff
			return b.Do(ctx, func() error {
				return client.WaitHealthy(ctx)
			})
		}))

		// Configure — single attempt; idempotent, so a re-run (rewire, WP3.9)
		// retries transients on the next pass.
		summary.Results = append(summary.Results, r.runStep(app, StepConfigure, func() error {
			return client.Configure(ctx, s.Config)
		}))

		// Verify — single attempt, re-checks the state Configure applied.
		summary.Results = append(summary.Results, r.runStep(app, StepVerify, func() error {
			return client.Verify(ctx)
		}))
	}

	return summary
}
