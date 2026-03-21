package orchestrator

import (
	"testing"
)

func TestParseJudgeVerdictValid(t *testing.T) {
	input := `{"success": true, "needs_more_work": false, "commit_message": "feat(1-2): add design tokens", "summary": "Tokens added", "recommended_status": "review"}`

	verdict, err := parseJudgeVerdict(input, "dev-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !verdict.Success {
		t.Fatal("expected success=true")
	}
	if verdict.NeedsMoreWork {
		t.Fatal("expected needs_more_work=false")
	}
	if verdict.CommitMessage != "feat(1-2): add design tokens" {
		t.Fatalf("unexpected commit message: %q", verdict.CommitMessage)
	}
	if verdict.RecommendedStatus != "review" {
		t.Fatalf("unexpected status: %q", verdict.RecommendedStatus)
	}
}

func TestParseJudgeVerdictWithWrappedJSON(t *testing.T) {
	input := `Here is my evaluation:
{"success": true, "needs_more_work": false, "commit_message": "feat(1-1): init project", "summary": "Project initialized", "recommended_status": "ready-for-dev"}
That's my verdict.`

	verdict, err := parseJudgeVerdict(input, "create-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.RecommendedStatus != "ready-for-dev" {
		t.Fatalf("expected ready-for-dev, got %q", verdict.RecommendedStatus)
	}
}

func TestParseJudgeVerdictInvalidFallback(t *testing.T) {
	input := "this is not json at all"

	verdict, err := parseJudgeVerdict(input, "dev-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return fallback
	if verdict.RecommendedStatus != "review" {
		t.Fatalf("expected fallback status 'review' for dev-story, got %q", verdict.RecommendedStatus)
	}
}

func TestParseJudgeVerdictEmptyCommitMessage(t *testing.T) {
	input := `{"success": true, "needs_more_work": false, "commit_message": "", "summary": "done", "recommended_status": "done"}`

	verdict, err := parseJudgeVerdict(input, "code-review")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.CommitMessage == "" {
		t.Fatal("expected fallback commit message, got empty")
	}
}

func TestFallbackVerdict(t *testing.T) {
	tests := []struct {
		workflow       string
		expectedStatus string
	}{
		{"create-story", "ready-for-dev"},
		{"dev-story", "review"},
		{"code-review", "done"},
		{"unknown", "in-progress"},
	}

	for _, tt := range tests {
		verdict := fallbackVerdict(tt.workflow)
		if verdict.RecommendedStatus != tt.expectedStatus {
			t.Errorf("fallbackVerdict(%q): expected status %q, got %q", tt.workflow, tt.expectedStatus, verdict.RecommendedStatus)
		}
		if !verdict.Success {
			t.Errorf("fallbackVerdict(%q): expected success=true", tt.workflow)
		}
	}
}

func TestDefaultStatusForWorkflow(t *testing.T) {
	tests := []struct {
		workflow string
		expected string
	}{
		{"create-story", "ready-for-dev"},
		{"dev-story", "review"},
		{"code-review", "done"},
		{"something-else", "in-progress"},
	}

	for _, tt := range tests {
		got := defaultStatusForWorkflow(tt.workflow)
		if got != tt.expected {
			t.Errorf("defaultStatusForWorkflow(%q) = %q, want %q", tt.workflow, got, tt.expected)
		}
	}
}
