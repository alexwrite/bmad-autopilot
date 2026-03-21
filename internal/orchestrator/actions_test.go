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
	// Verify commit prefix format
	if !strings.Contains(actions[0].Prompt, `"create(1-2): `) {
		t.Fatal("expected create(story) commit prefix in create-story prompt")
	}
	if !strings.Contains(actions[1].Prompt, `"dev(1-2): `) {
		t.Fatal("expected dev(story) commit prefix in dev-story prompt")
	}
	// Verify status update instructions
	if !strings.Contains(actions[0].Prompt, "sprint-status.yaml") {
		t.Fatal("expected status update instruction in create-story prompt")
	}
	if !strings.Contains(actions[1].Prompt, "sprint-status.yaml") {
		t.Fatal("expected status update instruction in dev-story prompt")
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
	if !strings.Contains(action.Prompt, `"review(1-2): `) {
		t.Fatal("expected review(story) commit prefix in code-review prompt")
	}
	if !strings.Contains(action.Prompt, "sprint-status.yaml") {
		t.Fatal("expected status update instruction in code-review prompt")
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
		{"blocked stops", "blocked", false},
		{"Blocked case-insensitive stops", "Blocked", false},
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

func TestMaxInvocationsPerStoryIsPositive(t *testing.T) {
	if MaxInvocationsPerStory < MaxReviewRounds+2 {
		t.Fatalf("MaxInvocationsPerStory (%d) must be > MaxReviewRounds+2 (%d)", MaxInvocationsPerStory, MaxReviewRounds+2)
	}
}

func TestMaxConsecutiveBlockedIsPositive(t *testing.T) {
	if MaxConsecutiveBlocked < 1 {
		t.Fatalf("MaxConsecutiveBlocked must be >= 1, got %d", MaxConsecutiveBlocked)
	}
}

func TestCommitPrefixesAreDistinct(t *testing.T) {
	create := PlanPrimaryActions
	actions, _ := create("backlog", "2-1")
	review := ReviewAction("2-1")

	prefixes := map[string]bool{}
	for _, a := range actions {
		if strings.Contains(a.Prompt, `"create(`) {
			prefixes["create"] = true
		}
		if strings.Contains(a.Prompt, `"dev(`) {
			prefixes["dev"] = true
		}
	}
	if strings.Contains(review.Prompt, `"review(`) {
		prefixes["review"] = true
	}

	if len(prefixes) != 3 {
		t.Fatalf("expected 3 distinct commit prefixes (create, dev, review), got %d", len(prefixes))
	}
}
