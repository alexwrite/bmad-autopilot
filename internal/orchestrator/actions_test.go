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
	if actions[0].Command != `copilot --yolo --no-ask-user -s -p "/bmad-bmm-create-story 1-2"` {
		t.Fatalf("unexpected first command: %q", actions[0].Command)
	}
	if actions[1].Command != `copilot --yolo --no-ask-user -s -p "/bmad-bmm-dev-story 1-2"` {
		t.Fatalf("unexpected second command: %q", actions[1].Command)
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
	if actions[0].Command != `copilot --yolo --no-ask-user -s -p "/bmad-bmm-dev-story 3-4"` {
		t.Fatalf("unexpected command: %q", actions[0].Command)
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
