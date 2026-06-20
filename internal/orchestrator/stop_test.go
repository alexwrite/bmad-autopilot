package orchestrator

import (
	"context"
	"testing"
)

func TestStopControllerLifecycle(t *testing.T) {
	c := NewStopController()
	if c.StopRequested() {
		t.Fatal("fresh controller must not report a stop")
	}

	c.RequestStop()
	if !c.StopRequested() {
		t.Fatal("expected stop after RequestStop")
	}

	c.CancelStop()
	if c.StopRequested() {
		t.Fatal("expected no stop after CancelStop")
	}
}

func TestWithDefaultStopChecker(t *testing.T) {
	if withDefaultStopChecker(nil).StopRequested() {
		t.Fatal("nil checker must default to never-stops")
	}

	c := NewStopController()
	c.RequestStop()
	if !withDefaultStopChecker(c).StopRequested() {
		t.Fatal("expected the provided controller to be used")
	}
}

// TestRunLoopHonorsStopBeforeWork verifies the runner exits cleanly (nil) at
// the top of the loop when a stop is already pending — without touching the
// status file or running any step.
func TestRunLoopHonorsStopBeforeWork(t *testing.T) {
	logger, err := NewRunLogger(t.TempDir())
	if err != nil {
		t.Fatalf("init logger: %v", err)
	}
	defer logger.Close()

	stop := NewStopController()
	stop.RequestStop()

	r := &Runner{
		cfg:  Config{StatusFile: "/nonexistent/sprint-status.yaml"},
		log:  logger,
		stop: stop,
	}

	if err := r.runLoop(context.Background()); err != nil {
		t.Fatalf("expected clean nil exit on pending stop, got: %v", err)
	}
}
