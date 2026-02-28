package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSprintStatusOrderAndFiltering(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sprint-status.yaml")
	content := `
epic-1: backlog
1-1-story-a: backlog
1-2-retrospective: done
1-3-story-b:
  status: review
2-1-story-c:
  status: done
notes: hello
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	got, err := LoadSprintStatus(path)
	if err != nil {
		t.Fatalf("load sprint status: %v", err)
	}

	if len(got.Stories) != 3 {
		t.Fatalf("expected 3 stories, got %d", len(got.Stories))
	}
	if got.Stories[0].Key != "1-1-story-a" || got.Stories[0].Status != "backlog" {
		t.Fatalf("unexpected first story: %+v", got.Stories[0])
	}
	if got.Stories[1].Key != "1-3-story-b" || got.Stories[1].Status != "review" {
		t.Fatalf("unexpected second story: %+v", got.Stories[1])
	}
	if got.Stories[2].Key != "2-1-story-c" || got.Stories[2].Status != "done" {
		t.Fatalf("unexpected third story: %+v", got.Stories[2])
	}
}

func TestLoadSprintStatusFromDevelopmentStatusMapping(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sprint-status.yaml")
	content := `
generated: 2026-02-17
project: sample
development_status:
  epic-1: in-progress
  1-1-story-a: done
  1-2-story-b: review
  epic-1-retrospective: optional
  2-1-story-c: backlog
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	got, err := LoadSprintStatus(path)
	if err != nil {
		t.Fatalf("load sprint status: %v", err)
	}

	if len(got.Stories) != 3 {
		t.Fatalf("expected 3 stories, got %d", len(got.Stories))
	}
	if got.Stories[0].Key != "1-1-story-a" || got.Stories[0].Status != "done" {
		t.Fatalf("unexpected first story: %+v", got.Stories[0])
	}
	if got.Stories[1].Key != "1-2-story-b" || got.Stories[1].Status != "review" {
		t.Fatalf("unexpected second story: %+v", got.Stories[1])
	}
	if got.Stories[2].Key != "2-1-story-c" || got.Stories[2].Status != "backlog" {
		t.Fatalf("unexpected third story: %+v", got.Stories[2])
	}
}

func TestNextPendingStory(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sprint-status.yaml")
	content := `
1-1-story-a: done
1-2-story-b: in-progress
2-1-story-c: backlog
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	got, err := LoadSprintStatus(path)
	if err != nil {
		t.Fatalf("load sprint status: %v", err)
	}

	next, ok := got.NextPendingStory()
	if !ok {
		t.Fatal("expected a next story")
	}
	if next.Key != "1-2-story-b" {
		t.Fatalf("expected first pending 1-2-story-b, got %s", next.Key)
	}
}

func TestStoryNumberFromKey(t *testing.T) {
	number, err := StoryNumberFromKey("12-3-sample-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if number != "12-3" {
		t.Fatalf("expected 12-3, got %s", number)
	}
}
