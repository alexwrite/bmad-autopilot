package orchestrator

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// deferredStoryRefPattern extracts the target story number from bullet
// items like "... scoped to story 1-3 (BaseLayout) ...". Only the number
// part (epic-story) is captured; the optional parenthesised comment is
// ignored so edits to the descriptive label don't break the linkage.
var deferredStoryRefPattern = regexp.MustCompile(`(?i)scoped to story (\d+-\d+)`)

// deferredPath is the canonical location of the deferred-work ledger
// maintained by BMAD skills (create-story, code-review).
const deferredPath = "_bmad-output/implementation-artifacts/deferred-work.md"

// DeferredOrphan is a deferred-work.md item that is still open (no
// **CLOSED marker) even though its target story already reached a terminal
// state. Either the reviewer of the target forgot to close it, the item
// was scoped to the wrong story, or the implementation silently skipped
// the follow-up — a human eye is needed.
type DeferredOrphan struct {
	ItemText     string // first line of the bullet, trimmed and truncated
	TargetStory  string // story number part only, e.g. "1-3"
	TargetStatus string // normalised status of the target story
	SourceLine   int    // 1-based line number in deferred-work.md
}

// ScanDeferredOrphans reads deferred-work.md under workdir and returns the
// open items whose scoped target is already done/validated. Absence of the
// file is not an error — projects without deferred work legitimately skip
// the ledger.
func ScanDeferredOrphans(workdir string, status SprintStatus) ([]DeferredOrphan, error) {
	path := filepath.Join(workdir, deferredPath)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", deferredPath, err)
	}
	defer f.Close()

	var orphans []DeferredOrphan
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		if strings.Contains(line, "**CLOSED") {
			continue
		}

		match := deferredStoryRefPattern.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		target := match[1]

		targetStatus, hit := findStatusByStoryNumber(status, target)
		if !hit {
			continue
		}
		if targetStatus != "done" && targetStatus != "validated" {
			continue
		}

		orphans = append(orphans, DeferredOrphan{
			ItemText:     truncateRunes(strings.TrimPrefix(trimmed, "- "), 140),
			TargetStory:  target,
			TargetStatus: targetStatus,
			SourceLine:   lineNum,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", deferredPath, err)
	}
	return orphans, nil
}

// findStatusByStoryNumber looks up a story by its number part (e.g. "1-3")
// and returns its normalised status. Full story keys in sprint-status are
// "1-3-baselayout-..." — the number is the stable join key with deferred
// annotations.
func findStatusByStoryNumber(status SprintStatus, number string) (string, bool) {
	for _, story := range status.Stories {
		n, err := StoryNumberFromKey(story.Key)
		if err != nil {
			continue
		}
		if n == number {
			return normalizeStatus(story.Status), true
		}
	}
	return "", false
}

// truncateRunes shortens a string to at most n runes, appending an
// ellipsis. Runes (not bytes) are the unit so that multi-byte characters
// common in the French ledger aren't split in the middle.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
