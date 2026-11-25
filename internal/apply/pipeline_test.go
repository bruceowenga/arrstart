package apply

import (
	"context"
	"errors"
	"testing"

	"github.com/bruceowenga/arrstart/internal/wiring"
)

// fakeStage records invocations and can be programmed to fail.
type fakeStage struct {
	name  string
	fail  error
	calls int
}

// Name returns the stage name.
func (s *fakeStage) Name() string { return s.name }

// Run records a call and returns the programmed error.
func (s *fakeStage) Run(context.Context) error {
	s.calls++
	return s.fail
}

// fakeClient is a test double for wiring.Client.
type fakeClient struct {
	name        string
	waitErr     error
	cfgErr      error
	verifyErr   error
	configured  bool
	verified    bool
	waitHealthy bool
}

func (c *fakeClient) WaitHealthy(ctx context.Context) error {
	c.waitHealthy = true
	return c.waitErr
}

func (c *fakeClient) Configure(ctx context.Context, cfg any) error {
	c.configured = true
	return c.cfgErr
}

func (c *fakeClient) Verify(ctx context.Context) error {
	c.verified = true
	return c.verifyErr
}

func TestOrchestratorRunsStagesInOrder(t *testing.T) {
	var order []string
	stages := []Stage{
		&fakeStage{name: "one"},
		&fakeStage{name: "two"},
		&fakeStage{name: "three"},
	}
	// Wrap to record order.
	recStages := make([]Stage, len(stages))
	for i, s := range stages {
		orig := s
		recStages[i] = &recordingStage{inner: orig, record: &order}
	}

	o := NewOrchestrator(recStages...)
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{"one", "two", "three"}
	for i, name := range want {
		if order[i] != name {
			t.Fatalf("order[%d] = %q, want %q", i, order[i], name)
		}
	}
}

func TestOrchestratorFailFast(t *testing.T) {
	var order []string
	boom := errors.New("stage two exploded")
	stages := []Stage{
		&recordingStage{inner: &fakeStage{name: "one"}, record: &order},
		&recordingStage{inner: &fakeStage{name: "two", fail: boom}, record: &order},
		&recordingStage{inner: &fakeStage{name: "three"}, record: &order},
	}

	o := NewOrchestrator(stages...)
	err := o.Run(context.Background())
	if err == nil {
		t.Fatal("Run: expected error")
	}
	if err.Error() != boom.Error() {
		t.Fatalf("Run: error = %v, want %v", err, boom)
	}
	if len(order) != 2 {
		t.Fatalf("Run: %d stages ran, want 2 (fail-fast)", len(order))
	}
	if order[0] != "one" || order[1] != "two" {
		t.Fatalf("Run: order = %v, want [one two]", order)
	}
}

func TestOrchestratorContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var order []string
	stages := []Stage{
		&recordingStage{inner: &fakeStage{name: "one"}, record: &order},
		&recordingStage{inner: &fakeStage{name: "two"}, record: &order},
	}

	o := NewOrchestrator(stages...)
	err := o.Run(ctx)
	if err == nil {
		t.Fatal("Run: expected context cancelled error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: error = %v, want context.Canceled", err)
	}
	if len(order) != 0 {
		t.Fatalf("Run: %d stages ran before cancellation, want 0", len(order))
	}
}

func TestWireStageIsolatesPerAppFailures(t *testing.T) {
	// App A fails at Configure; app B succeeds all steps.
	clientA := &fakeClient{name: "sonarr", cfgErr: errors.New("auth failed")}
	clientB := &fakeClient{name: "radarr"}

	runner := &wiring.Runner{
		OnResult: func(s wiring.StepResult) {},
	}

	steps := []wiring.AppStep{
		{Name: "sonarr", Client: clientA},
		{Name: "radarr", Client: clientB},
	}

	ws := &WireStage{Runner: runner, Steps: steps}
	err := ws.Run(context.Background())
	if err == nil {
		t.Fatal("WireStage: expected error from sonarr failure")
	}

	if ws.Summary == nil {
		t.Fatal("WireStage: Summary should be populated")
	}
	if ws.Summary.FailedCount() != 1 {
		t.Fatalf("WireStage: %d failures, want 1", ws.Summary.FailedCount())
	}
	if !clientA.configured || !clientA.waitHealthy {
		t.Fatal("WireStage: sonarr steps should have run despite later failure")
	}
	if !clientB.configured || !clientB.verified {
		t.Fatal("WireStage: radarr all steps should run (failure isolation)")
	}
}

func TestWireStageNoRunner(t *testing.T) {
	ws := &WireStage{}
	err := ws.Run(context.Background())
	if err == nil {
		t.Fatal("WireStage: expected error for nil runner")
	}
}

func TestSummaryStagePrintsResults(t *testing.T) {
	summary := &wiring.Summary{
		Results: []wiring.StepResult{
			{App: "sonarr", Step: wiring.StepWaitHealthy, OK: true},
			{App: "sonarr", Step: wiring.StepConfigure, OK: false, Err: errors.New("auth")},
			{App: "radarr", Step: wiring.StepWaitHealthy, OK: true},
		},
	}

	ss := &SummaryStage{WiringSummary: summary}
	if err := ss.Run(context.Background()); err != nil {
		t.Fatalf("SummaryStage: %v", err)
	}

	// Summary counts
	if summary.FailedCount() != 1 {
		t.Fatalf("Summary: %d failures, want 1", summary.FailedCount())
	}
	if !summary.HasFailures() {
		t.Fatal("Summary: should report failures")
	}
	if summary.Error() == nil {
		t.Fatal("Summary: Error() should return joined errors")
	}
}

type recordingStage struct {
	inner  Stage
	record *[]string
}

func (s *recordingStage) Name() string { return s.inner.Name() }

func (s *recordingStage) Run(ctx context.Context) error {
	*s.record = append(*s.record, s.inner.Name())
	return s.inner.Run(ctx)
}
