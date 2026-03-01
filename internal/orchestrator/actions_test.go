package orchestrator

import (
	"strings"
	"testing"
)

func TestPlanPrimaryActionsBacklog(t *testing.T) {
	actions, err := PlanPrimaryActions("backlog", "1-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0].WorkflowKey != "create-story" {
		t.Fatalf("expected first action workflow key create-story, got %q", actions[0].WorkflowKey)
	}
	if actions[1].WorkflowKey != "dev-story" {
		t.Fatalf("expected second action workflow key dev-story, got %q", actions[1].WorkflowKey)
	}
	if !strings.Contains(actions[0].Prompt, "#yolo") {
		t.Fatal("expected yolo mode in create-story prompt")
	}
	if !strings.Contains(actions[1].Prompt, "#yolo") {
		t.Fatal("expected yolo mode in dev-story prompt")
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
	if actions[0].WorkflowKey != "dev-story" {
		t.Fatalf("expected workflow key dev-story, got %q", actions[0].WorkflowKey)
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

func TestReviewActionWorkflowKey(t *testing.T) {
	action := ReviewAction("1-2")
	if action.WorkflowKey != "code-review" {
		t.Fatalf("expected workflow key code-review, got %q", action.WorkflowKey)
	}
	if !strings.Contains(action.Prompt, "#yolo") {
		t.Fatal("expected yolo mode in review prompt")
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
