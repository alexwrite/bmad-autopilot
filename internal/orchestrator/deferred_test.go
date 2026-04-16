package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDeferredFile(t *testing.T, workdir, content string) {
	t.Helper()
	dir := filepath.Join(workdir, "_bmad-output", "implementation-artifacts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deferred-work.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestScanDeferredOrphans_NoFileReturnsNil(t *testing.T) {
	orphans, err := ScanDeferredOrphans(t.TempDir(), SprintStatus{})
	if err != nil {
		t.Fatalf("unexpected error on missing file: %v", err)
	}
	if orphans != nil {
		t.Errorf("expected nil orphans when file absent, got %+v", orphans)
	}
}

func TestScanDeferredOrphans_ClassifiesItemsCorrectly(t *testing.T) {
	dir := t.TempDir()
	md := `# Deferred Work

## Deferred from review of 1-1

- ~~Already-closed item — scoped to story 1-3.~~ **CLOSED by story 1-3 (2026-04-16):** shipped.
- Still-pending item — scoped to story 1-5 (Footer). Target not done yet, should not flag.
- Orphaned item — scoped to story 1-7 (Nav). Target already done, reviewer forgot to close.
- Validated-target orphan — scoped to story 2-1 (Hero). Terminal state "validated" also counts.
- Untargeted concern — no specific story scope mentioned here. Should be ignored.
- Not story 1-1 scope (pre-existing) — this "scope" word should not trigger the regex.
`
	writeDeferredFile(t, dir, md)

	status := SprintStatus{
		Stories: []Story{
			{Key: "1-3-baselayout", Status: "done"},
			{Key: "1-5-footer", Status: "review"},
			{Key: "1-7-nav", Status: "done"},
			{Key: "2-1-hero", Status: "validated"},
		},
	}

	orphans, err := ScanDeferredOrphans(dir, status)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(orphans) != 2 {
		t.Fatalf("expected 2 orphans (1-7 done, 2-1 validated), got %d: %+v", len(orphans), orphans)
	}

	seen := make(map[string]DeferredOrphan, len(orphans))
	for _, o := range orphans {
		seen[o.TargetStory] = o
	}
	if _, ok := seen["1-7"]; !ok {
		t.Errorf("missing orphan for story 1-7")
	}
	if _, ok := seen["2-1"]; !ok {
		t.Errorf("missing orphan for story 2-1")
	}
	if seen["1-7"].TargetStatus != "done" {
		t.Errorf("1-7 target status = %q, want done", seen["1-7"].TargetStatus)
	}
	if seen["2-1"].TargetStatus != "validated" {
		t.Errorf("2-1 target status = %q, want validated", seen["2-1"].TargetStatus)
	}
}

func TestScanDeferredOrphans_TargetUnknownIgnored(t *testing.T) {
	dir := t.TempDir()
	md := `- Orphan pointing to nonexistent story — scoped to story 9-9 (Ghost).`
	writeDeferredFile(t, dir, md)

	// Story 9-9 is not in the sprint status — cannot be an orphan because we
	// don't know whether it was planned at all. Ignore silently.
	orphans, err := ScanDeferredOrphans(dir, SprintStatus{
		Stories: []Story{{Key: "1-1-init", Status: "done"}},
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("expected 0 orphans for unknown target, got %+v", orphans)
	}
}

func TestScanDeferredOrphans_AllClosedIsClean(t *testing.T) {
	dir := t.TempDir()
	md := `- ~~A — scoped to story 1-3.~~ **CLOSED by 1-3:** ok.
- ~~B — scoped to story 1-5.~~ **CLOSED by 1-5:** ok.`
	writeDeferredFile(t, dir, md)

	orphans, err := ScanDeferredOrphans(dir, SprintStatus{
		Stories: []Story{
			{Key: "1-3-base", Status: "done"},
			{Key: "1-5-foot", Status: "done"},
		},
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("expected 0 orphans when everything closed, got %+v", orphans)
	}
}

func TestTruncateRunes_PreservesUTF8(t *testing.T) {
	// Emoji-free, but multi-byte French characters present in the real ledger.
	s := "Déférer à la story 1-3 — accessibilité WCAG"
	got := truncateRunes(s, 10)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix on truncation, got %q", got)
	}
	if len([]rune(got)) != 11 { // 10 runes + ellipsis
		t.Errorf("expected 11 runes, got %d (%q)", len([]rune(got)), got)
	}

	short := "Déjà court"
	if got := truncateRunes(short, 100); got != short {
		t.Errorf("expected passthrough on short input, got %q", got)
	}
}
