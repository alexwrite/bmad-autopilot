package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// stubExecutor returns the queued (result, error) pairs in order, one per
// Run call. Extra calls beyond the queue fail the test — catches runaway
// retry loops.
type stubExecutor struct {
	t     *testing.T
	calls []stubCall
	idx   int
}

type stubCall struct {
	result ExecResult
	err    error
}

func (s *stubExecutor) Run(_ context.Context, _ Action) (ExecResult, error) {
	if s.idx >= len(s.calls) {
		s.t.Fatalf("stubExecutor: unexpected extra call (queue exhausted after %d)", s.idx)
	}
	call := s.calls[s.idx]
	s.idx++
	return call.result, call.err
}

type noopLogger struct{}

func (noopLogger) Log(_, _ string, _ ...interface{}) {}

func newTestRetrying(stub *stubExecutor, cfg RetryConfig) *retryingExecutor {
	r := newRetryingExecutor(stub, cfg, noopLogger{})
	// Replace sleep with an instant no-op — tests shouldn't wait 30s.
	r.sleep = func(_ context.Context, _ time.Duration) error { return nil }
	return r
}

func TestRetry_SucceedsFirstAttempt(t *testing.T) {
	stub := &stubExecutor{
		t:     t,
		calls: []stubCall{{result: ExecResult{RawOutput: "ok"}, err: nil}},
	}
	r := newTestRetrying(stub, DefaultRetryConfig())
	res, err := r.Run(context.Background(), Action{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RawOutput != "ok" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if stub.idx != 1 {
		t.Fatalf("expected 1 call, got %d", stub.idx)
	}
}

func TestRetry_RetriesTransientThenSucceeds(t *testing.T) {
	transient := fmt.Errorf("%w: API Error: 529 overloaded", ErrTransientAPI)
	stub := &stubExecutor{
		t: t,
		calls: []stubCall{
			{err: transient},
			{err: transient},
			{result: ExecResult{RawOutput: "recovered"}},
		},
	}
	r := newTestRetrying(stub, RetryConfig{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	res, err := r.Run(context.Background(), Action{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RawOutput != "recovered" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if stub.idx != 3 {
		t.Fatalf("expected 3 calls, got %d", stub.idx)
	}
}

func TestDefaultRetryConfigIsInfinite(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxAttempts != 0 {
		t.Errorf("default MaxAttempts must be 0 (infinite retry), got %d", cfg.MaxAttempts)
	}
	if cfg.BaseDelay <= 0 || cfg.MaxDelay <= 0 {
		t.Errorf("default delays must be positive, got base=%v max=%v", cfg.BaseDelay, cfg.MaxDelay)
	}
}

func TestRetry_DefaultConfigRetriesPastFiniteCap(t *testing.T) {
	transient := fmt.Errorf("%w: 529", ErrTransientAPI)
	// 20 failures then success — proves MaxAttempts=0 does not stop at any
	// fixed ceiling. The unattended autopilot must ride out long outages.
	calls := make([]stubCall, 20)
	for i := range calls {
		calls[i] = stubCall{err: transient}
	}
	calls = append(calls, stubCall{result: ExecResult{RawOutput: "eventually ok"}})

	stub := &stubExecutor{t: t, calls: calls}
	r := newTestRetrying(stub, DefaultRetryConfig())
	res, err := r.Run(context.Background(), Action{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RawOutput != "eventually ok" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if stub.idx != 21 {
		t.Fatalf("expected 21 calls (20 retries + 1 success), got %d", stub.idx)
	}
}

func TestRetry_FiniteCapStopsAtLimit(t *testing.T) {
	transient := fmt.Errorf("%w: API Error: 500 server down", ErrTransientAPI)
	stub := &stubExecutor{
		t: t,
		calls: []stubCall{
			{err: transient}, {err: transient}, {err: transient},
		},
	}
	r := newTestRetrying(stub, RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	_, err := r.Run(context.Background(), Action{})
	if !errors.Is(err, ErrTransientAPI) {
		t.Fatalf("expected ErrTransientAPI after cap reached, got %v", err)
	}
	if stub.idx != 3 {
		t.Fatalf("expected 3 calls (finite cap), got %d", stub.idx)
	}
}

func TestRetry_AuthErrorNotRetried(t *testing.T) {
	stub := &stubExecutor{
		t:     t,
		calls: []stubCall{{err: fmt.Errorf("%w: 401", ErrAuthExpired)}},
	}
	r := newTestRetrying(stub, DefaultRetryConfig())
	_, err := r.Run(context.Background(), Action{})
	if !errors.Is(err, ErrAuthExpired) {
		t.Fatalf("expected ErrAuthExpired, got %v", err)
	}
	if stub.idx != 1 {
		t.Fatalf("auth error should not retry, got %d calls", stub.idx)
	}
}

func TestRetry_GenericErrorNotRetried(t *testing.T) {
	stub := &stubExecutor{
		t:     t,
		calls: []stubCall{{err: errors.New("something else broke")}},
	}
	r := newTestRetrying(stub, DefaultRetryConfig())
	_, err := r.Run(context.Background(), Action{})
	if err == nil || errors.Is(err, ErrTransientAPI) {
		t.Fatalf("expected non-retry passthrough, got %v", err)
	}
	if stub.idx != 1 {
		t.Fatalf("non-transient error should not retry, got %d calls", stub.idx)
	}
}

func TestRetry_ContextCancelDuringBackoff(t *testing.T) {
	transient := fmt.Errorf("%w: 529", ErrTransientAPI)
	stub := &stubExecutor{
		t:     t,
		calls: []stubCall{{err: transient}, {err: transient}, {err: transient}},
	}
	r := newRetryingExecutor(stub, RetryConfig{MaxAttempts: 5, BaseDelay: time.Minute, MaxDelay: time.Minute}, noopLogger{})
	// real sleep path — test that ctx cancel interrupts it
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Run(ctx, Action{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if stub.idx != 1 {
		t.Fatalf("expected single call before cancel, got %d", stub.idx)
	}
}

func TestBackoffDelay(t *testing.T) {
	cfg := RetryConfig{BaseDelay: 30 * time.Second, MaxDelay: 10 * time.Minute}
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 2 * time.Minute},
		{4, 4 * time.Minute},
		{5, 8 * time.Minute},
		{6, 10 * time.Minute}, // clamped
		{100, 10 * time.Minute},
	}
	for _, tt := range cases {
		got := backoffDelay(cfg, tt.attempt)
		if got != tt.want {
			t.Errorf("backoffDelay(attempt=%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}
