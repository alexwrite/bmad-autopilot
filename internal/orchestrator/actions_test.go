package orchestrator

import "testing"

func TestPlanPrimaryActionsBacklog(t *testing.T) {
	actions, err := PlanPrimaryActions("backlog", "1-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0].Prompt == "" {
		t.Fatal("expected non-empty prompt for create-story action")
	}
	if actions[1].Prompt == "" {
		t.Fatal("expected non-empty prompt for dev-story action")
	}
}

func TestPlanPrimaryActionsReadyForDev(t *testing.T) {
	actions, err := PlanPrimaryActions("ready-for-dev", "3-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Prompt == "" {
		t.Fatal("expected non-empty prompt for dev-story action")
	}
}

func TestPlanPrimaryActionsDone(t *testing.T) {
	actions, err := PlanPrimaryActions("done", "1-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("expected 0 actions for done, got %d", len(actions))
	}
}

func TestShouldContinueReview(t *testing.T) {
	if !ShouldContinueReview("review", false) {
		t.Fatal("expected review status to continue")
	}
	if !ShouldContinueReview("done", false) {
		t.Fatal("expected done without push evidence to continue")
	}
	if ShouldContinueReview("done", true) {
		t.Fatal("expected done with push evidence to stop")
	}
}
