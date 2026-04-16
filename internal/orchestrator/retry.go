package orchestrator

import (
	"context"
	"errors"
	"math"
	"time"
)

// RetryConfig controls transient-error retry behavior.
//
// Targets Anthropic API hiccups (5xx, overloaded_error, rate_limit_error):
// the autopilot is meant to run unattended, so the default policy retries
// forever with exponential backoff — an API outage should delay progress,
// not halt the run.
type RetryConfig struct {
	// MaxAttempts caps the total number of calls. Zero or negative means
	// "retry forever" — the only exits are success, a non-transient error,
	// or context cancellation. Finite caps exist for tests and for anyone
	// who wants a manual kill-switch.
	MaxAttempts int
	BaseDelay   time.Duration // delay before the second attempt
	MaxDelay    time.Duration // backoff ceiling (clamps exponential growth)
}

// DefaultRetryConfig returns the production defaults: retry forever,
// exponential 30s → 1m → 2m → 4m → 8m → clamp 10m. Only a non-transient
// error (auth, bug) or Ctrl+C stops the loop.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 0, // infinite
		BaseDelay:   30 * time.Second,
		MaxDelay:    10 * time.Minute,
	}
}

// retryLogger is the subset of RunLogger used by retryingExecutor. Kept as
// an interface so tests can inject a no-op logger without touching files.
type retryLogger interface {
	Log(tag, format string, args ...interface{})
}

// retryingExecutor wraps a CommandExecutor with exponential-backoff retry
// on ErrTransientAPI. Auth errors, context cancellation, and all
// non-transient failures propagate immediately.
type retryingExecutor struct {
	inner CommandExecutor
	cfg   RetryConfig
	log   retryLogger
	sleep func(context.Context, time.Duration) error
}

func newRetryingExecutor(inner CommandExecutor, cfg RetryConfig, log retryLogger) *retryingExecutor {
	return &retryingExecutor{
		inner: inner,
		cfg:   cfg,
		log:   log,
		sleep: ctxSleep,
	}
}

func (r *retryingExecutor) Run(ctx context.Context, action Action) (ExecResult, error) {
	var lastResult ExecResult
	var lastErr error

	for attempt := 1; ; attempt++ {
		result, err := r.inner.Run(ctx, action)
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, ErrTransientAPI) {
			return result, err
		}

		lastResult = result
		lastErr = err

		// Finite cap exists for tests and manual kill-switches only.
		// Production default is MaxAttempts=0 → keep going until the API
		// recovers, context is cancelled, or a non-transient error fires.
		if r.cfg.MaxAttempts > 0 && attempt >= r.cfg.MaxAttempts {
			break
		}

		delay := backoffDelay(r.cfg, attempt)
		if r.log != nil {
			r.log.Log("RETRY", "transient API error on attempt %d — sleeping %s before retry: %v",
				attempt, delay.Round(time.Second), err)
		}
		if sleepErr := r.sleep(ctx, delay); sleepErr != nil {
			return result, sleepErr
		}
	}

	return lastResult, lastErr
}

// backoffDelay returns the sleep duration after attempt N just failed,
// before attempt N+1 starts. Exponential: BaseDelay * 2^(attempt-1),
// clamped at MaxDelay.
func backoffDelay(cfg RetryConfig, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exp := math.Pow(2, float64(attempt-1))
	d := time.Duration(float64(cfg.BaseDelay) * exp)
	if d <= 0 || d > cfg.MaxDelay {
		return cfg.MaxDelay
	}
	return d
}

// ctxSleep blocks for d, honoring context cancellation. Returns ctx.Err()
// if the context fires first, nil otherwise.
func ctxSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
