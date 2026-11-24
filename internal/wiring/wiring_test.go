package wiring

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruceowenga/arrstart/internal/wiring/internal/retry"
)

// fakeClient is a programmable wiring.Client for framework tests.
type fakeClient struct {
	waitHealthyErr atomic.Value // error
	configureErr   atomic.Value
	verifyErr      atomic.Value
	waitCalls      atomic.Int32
	configCalls    atomic.Int32
	verifyCalls    atomic.Int32
}

func (f *fakeClient) WaitHealthy(ctx context.Context) error {
	f.waitCalls.Add(1)
	return f.err(&f.waitHealthyErr)
}
func (f *fakeClient) Configure(ctx context.Context, cfg any) error {
	f.configCalls.Add(1)
	return f.err(&f.configureErr)
}
func (f *fakeClient) Verify(ctx context.Context) error {
	f.verifyCalls.Add(1)
	return f.err(&f.verifyErr)
}

func (f *fakeClient) err(v *atomic.Value) error {
	if e := v.Load(); e != nil {
		return e.(error)
	}
	return nil
}

func (f *fakeClient) setConfigureErr(err error) { f.configureErr.Store(err) }
func (f *fakeClient) setVerifyErr(err error)    { f.verifyErr.Store(err) }

func TestStepString(t *testing.T) {
	cases := map[Step]string{
		StepWaitHealthy: "waithealthy",
		StepConfigure:   "configure",
		StepVerify:      "verify",
		Step(99):        "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("Step(%d).String() = %q, want %q", s, got, want)
		}
	}
}

func TestSummary(t *testing.T) {
	okSum := Summary{Results: []StepResult{{App: "a", Step: StepConfigure, OK: true}}}
	if okSum.HasFailures() || okSum.Error() != nil {
		t.Fatal("empty-failure summary should report no failures")
	}

	fail := errors.New("boom")
	sum := Summary{Results: []StepResult{
		{App: "a", Step: StepConfigure, OK: true},
		{App: "b", Step: StepConfigure, OK: false, Err: fail},
		{App: "b", Step: StepVerify, OK: false, Err: fail},
	}}
	if !sum.HasFailures() || sum.FailedCount() != 2 {
		t.Fatalf("summary: HasFailures=%v FailedCount=%d, want true 2", sum.HasFailures(), sum.FailedCount())
	}
	err := sum.Error()
	if err == nil || !errors.Is(err, fail) {
		t.Fatalf("summary.Error() = %v, want wrapped %v", err, fail)
	}
}

func TestRunnerRunAllSteps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sonarr := &fakeClient{}
	radarr := &fakeClient{}
	runner := &Runner{Backoff: retry.Backoff{Initial: 5 * time.Millisecond}}

	summary := runner.Run(ctx, []AppStep{
		{Name: "sonarr", Client: sonarr},
		{Name: "radarr", Client: radarr},
	})
	if summary.HasFailures() || summary.FailedCount() != 0 {
		t.Fatalf("expected fully healthy run, got FailedCount=%d: %v", summary.FailedCount(), summary.Error())
	}
	if got := len(summary.Results); got != 6 {
		t.Fatalf("expected 6 step results (2 apps × 3 steps), got %d", got)
	}
	if sonarr.waitCalls.Load() != 1 || sonarr.configCalls.Load() != 1 || sonarr.verifyCalls.Load() != 1 {
		t.Fatalf("sonarr: wait=%d configure=%d verify=%d, want 1 1 1",
			sonarr.waitCalls.Load(), sonarr.configCalls.Load(), sonarr.verifyCalls.Load())
	}
}

func TestRunnerIsolatesFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sonarr := &fakeClient{}
	sonarr.setConfigureErr(errors.New("configure rejected"))

	radarr := &fakeClient{}
	radarr.setVerifyErr(errors.New("verify mismatch"))

	jellyfin := &fakeClient{} // healthy

	seen := make(map[string]int)
	runner := &Runner{
		Backoff: retry.Backoff{Initial: 5 * time.Millisecond},
		OnResult: func(res StepResult) {
			seen[res.App+":"+res.Step.String()]++
		},
	}
	summary := runner.Run(ctx, []AppStep{
		{Name: "sonarr", Client: sonarr},
		{Name: "radarr", Client: radarr},
		{Name: "jellyfin", Client: jellyfin},
	})

	// Failure isolation: every app still had all three steps attempted.
	if sonarr.waitCalls.Load() != 1 || sonarr.configCalls.Load() != 1 || sonarr.verifyCalls.Load() != 1 {
		t.Fatal("sonarr steps not all attempted despite configure failure")
	}
	if jellyfin.waitCalls.Load() != 1 || jellyfin.verifyCalls.Load() != 1 {
		t.Fatal("jellyfin steps not run despite other apps failing")
	}

	if !summary.HasFailures() || summary.FailedCount() != 2 {
		t.Fatalf("expected 2 failures, got %d", summary.FailedCount())
	}
	err := summary.Error()
	if err == nil {
		t.Fatal("summary.Error() should be non-nil with failures")
	}
	if !seenContains(seen, "sonarr:configure") || !seenContains(seen, "radarr:verify") {
		t.Fatal("expected OnResult for sonarr:configure and radarr:verify failures")
	}
}

func seenContains(seen map[string]int, key string) bool {
	_, ok := seen[key]
	return ok
}
