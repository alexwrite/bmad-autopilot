package orchestrator

import "sync/atomic"

// StopChecker reports whether a graceful stop has been requested. The runner
// consults it only at safe boundaries (between stories, between workflow
// steps) so an operator can stop the loop without killing an in-flight
// `claude` call mid-commit and leaving the working tree half-done.
type StopChecker interface {
	StopRequested() bool
}

// StopController is a concurrency-safe StopChecker that an external trigger
// (a signal handler, a test) flips. The zero value is ready to use and
// reports no stop. It is safe to read from the runner goroutine while a
// signal goroutine writes to it.
type StopController struct {
	requested atomic.Bool
}

// NewStopController returns a controller with no stop requested.
func NewStopController() *StopController {
	return &StopController{}
}

// RequestStop asks the runner to stop once the current step completes.
func (c *StopController) RequestStop() {
	c.requested.Store(true)
}

// CancelStop withdraws a pending stop request (the loop keeps running).
func (c *StopController) CancelStop() {
	c.requested.Store(false)
}

// StopRequested reports whether a stop is currently requested.
func (c *StopController) StopRequested() bool {
	return c.requested.Load()
}

// noopStopChecker is the default when no controller is wired (tests, library
// use). It never requests a stop.
type noopStopChecker struct{}

func (noopStopChecker) StopRequested() bool { return false }

// withDefaultStopChecker returns c, or a no-op checker when c is nil, so the
// runner can call StopRequested() unconditionally.
func withDefaultStopChecker(c StopChecker) StopChecker {
	if c == nil {
		return noopStopChecker{}
	}
	return c
}
