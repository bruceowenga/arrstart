package apply

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/bruceowenga/arrstart/internal/wiring"
)

// Stage is one step of the apply pipeline.
type Stage interface {
	// Name returns a human-readable stage name.
	Name() string
	// Run executes the stage. It must respect ctx for cancellation.
	Run(ctx context.Context) error
}

// Orchestrator runs stages in order, checking context cancellation between
// stages and failing fast on stage errors. A Wire stage's per-app failures are
// isolated by the internal/wiring Runner and reported together.
type Orchestrator struct {
	stages []Stage
}

// NewOrchestrator builds an Orchestrator from the given stages.
func NewOrchestrator(stages ...Stage) *Orchestrator {
	return &Orchestrator{stages: append([]Stage(nil), stages...)}
}

// Run executes all stages in order. Returns the first non-nil stage error.
func (o *Orchestrator) Run(ctx context.Context) error {
	for _, s := range o.stages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.Run(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ResolveStage converts Answers into a Plan.
type ResolveStage struct {
	Answers *Answers
	Plan    *Plan // output
}

func (s *ResolveStage) Name() string { return "resolve" }

func (s *ResolveStage) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := Resolve(s.Answers)
	if err != nil {
		return err
	}
	s.Plan = p
	return nil
}

// GenerateStage generates docker-compose.yml and .env from the Plan.
// Stubbed until WP1.3/WP1.4.
type GenerateStage struct{}

func (s *GenerateStage) Name() string { return "generate" }

func (s *GenerateStage) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("generate stage: not implemented until WP1.4 (compose fragment merging)")
}

// ComposeUpStage shells out to `docker compose up -d` via an injectable exec
// runner (ADR-0007: shell out to docker compose rather than the Docker SDK).
type ComposeUpStage struct {
	ComposeFile string
	Exec        ExecRunner // injectable; defaults to os/exec-based runner
}

func (s *ComposeUpStage) Name() string { return "compose-up" }

func (s *ComposeUpStage) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	exec := s.Exec
	if exec == nil {
		exec = &DefaultExecRunner{}
	}
	args := []string{"compose"}
	if s.ComposeFile != "" {
		args = append(args, "-f", s.ComposeFile)
	}
	args = append(args, "up", "-d", "--pull", "missing")
	return exec.Run(ctx, "docker", args...)
}

// WaitHealthyStage waits for each app's health endpoint.
// Stubbed until WP1.11.
type WaitHealthyStage struct{}

func (s *WaitHealthyStage) Name() string { return "wait-healthy" }

func (s *WaitHealthyStage) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("wait-healthy stage: not implemented until WP1.11 (CI Gate 1)")
}

// WireStage calls the internal/wiring Runner unchanged. Per-app failures are
// isolated by the Runner and reported together; the stage returns an error if
// any app step failed so the orchestrator exits non-zero.
type WireStage struct {
	Runner  *wiring.Runner
	Steps   []wiring.AppStep
	Summary *wiring.Summary // output
}

func (s *WireStage) Name() string { return "wire" }

func (s *WireStage) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Runner == nil {
		return fmt.Errorf("wire stage: no wiring runner configured")
	}
	s.Summary = &wiring.Summary{}
	*s.Summary = s.Runner.Run(ctx, s.Steps)
	if s.Summary.HasFailures() {
		return s.Summary.Error()
	}
	return nil
}

// SummaryStage prints per-app wiring results, redacting secrets.
type SummaryStage struct {
	WiringSummary *wiring.Summary
}

func (s *SummaryStage) Name() string { return "summary" }

func (s *SummaryStage) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.WiringSummary == nil {
		fmt.Println("summary: no wiring results")
		return nil
	}
	fmt.Printf("summary: %d step(s), %d failed\n",
		len(s.WiringSummary.Results), s.WiringSummary.FailedCount())
	for _, r := range s.WiringSummary.Results {
		status := "ok"
		if !r.OK {
			status = "failed"
		}
		fmt.Printf("  %s %s %s\n", status, r.App, r.Step)
		if !r.OK {
			fmt.Printf("    error: %v\n", r.Err)
		}
	}
	return nil
}

// ExecRunner runs an external command. Implementations may be injected for
// testing so no stage ever needs to call Docker or the network directly.
type ExecRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

// DefaultExecRunner runs commands via os/exec, wiring stdout/stderr through.
type DefaultExecRunner struct{}

func (r *DefaultExecRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// FakeExecRunner records invocations for testing.
type FakeExecRunner struct {
	Invocations [][]string
	Err         error
}

func (r *FakeExecRunner) Run(_ context.Context, name string, args ...string) error {
	r.Invocations = append(r.Invocations, append([]string{name}, args...))
	return r.Err
}
