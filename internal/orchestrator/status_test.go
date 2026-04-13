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
1-1-story-a: validated
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

func TestNextPendingStoryInEpics(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sprint-status.yaml")
	content := `
1-1-story-a: validated
1-2-story-b: validated
8-7-story-c: review
8-8-story-d: backlog
15-1-story-e: backlog
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	got, err := LoadSprintStatus(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Filter on epic 8 only
	next, ok := got.NextPendingStoryInEpics([]int{8})
	if !ok {
		t.Fatal("expected a story from epic 8")
	}
	if next.Key != "8-7-story-c" {
		t.Fatalf("expected 8-7-story-c, got %s", next.Key)
	}

	// Filter on epic 15
	next, ok = got.NextPendingStoryInEpics([]int{15})
	if !ok {
		t.Fatal("expected a story from epic 15")
	}
	if next.Key != "15-1-story-e" {
		t.Fatalf("expected 15-1-story-e, got %s", next.Key)
	}

	// Filter on epic 1 (all done) → no result
	_, ok = got.NextPendingStoryInEpics([]int{1})
	if ok {
		t.Fatal("expected no pending story in epic 1")
	}

	// No filter → first non-done
	next, ok = got.NextPendingStoryInEpics(nil)
	if !ok || next.Key != "8-7-story-c" {
		t.Fatalf("expected 8-7-story-c with nil filter, got %v %s", ok, next.Key)
	}
}

func TestParseEpicFilter(t *testing.T) {
	tests := []struct {
		input string
		want  []int
		err   bool
	}{
		{"", nil, false},
		{"8", []int{8}, false},
		{"15-21", []int{15, 16, 17, 18, 19, 20, 21}, false},
		{"8,15-17", []int{8, 15, 16, 17}, false},
		{"3, 8, 15-16", []int{3, 8, 15, 16}, false},
		{"abc", nil, true},
		{"5-3", nil, true},
	}
	for _, tt := range tests {
		got, err := ParseEpicFilter(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("ParseEpicFilter(%q): err=%v, wantErr=%v", tt.input, err, tt.err)
			continue
		}
		if err != nil {
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("ParseEpicFilter(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("ParseEpicFilter(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestEpicNumberFromKey(t *testing.T) {
	n, err := EpicNumberFromKey("8-7-suivi-score")
	if err != nil || n != 8 {
		t.Fatalf("expected 8, got %d (err=%v)", n, err)
	}
	n, err = EpicNumberFromKey("15-1-layout-espace")
	if err != nil || n != 15 {
		t.Fatalf("expected 15, got %d (err=%v)", n, err)
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
