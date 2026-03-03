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
	// Verify commit instructions
	if !strings.Contains(actions[0].Prompt, "git add") || !strings.Contains(actions[0].Prompt, "commit") {
		t.Fatal("expected git add and commit instruction in create-story prompt")
	}
	if !strings.Contains(actions[1].Prompt, "git add") || !strings.Contains(actions[1].Prompt, "commit") {
		t.Fatal("expected git add and commit instruction in dev-story prompt")
	}
	if strings.Contains(actions[0].Prompt, "push") {
		t.Fatal("create-story prompt should not mention push")
	}
	if strings.Contains(actions[1].Prompt, "push") {
		t.Fatal("dev-story prompt should not mention push")
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
	if !strings.Contains(action.Prompt, "git add") || !strings.Contains(action.Prompt, "commit") {
		t.Fatal("expected git add and commit instruction in code-review prompt")
	}
	if !strings.Contains(action.Prompt, "push") {
		t.Fatal("expected push instruction in code-review prompt")
	}
}

func TestShouldContinueReview(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"review status continues", "review", true},
		{"in-progress continues", "in-progress", true},
		{"backlog continues", "backlog", true},
		{"done stops", "done", false},
		{"Done case-insensitive stops", "Done", false},
		{"DONE uppercase stops", "DONE", false},
	}
	for _, tt := range tests {
		got := ShouldContinueReview(tt.status)
		if got != tt.want {
			t.Errorf("%s: ShouldContinueReview(%q) = %v, want %v", tt.name, tt.status, got, tt.want)
		}
	}
}

func TestMaxReviewRoundsIsPositive(t *testing.T) {
	if MaxReviewRounds < 1 {
		t.Fatalf("MaxReviewRounds must be >= 1, got %d", MaxReviewRounds)
	}
}
