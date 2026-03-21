package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	storyKeyPattern    = regexp.MustCompile(`^[0-9]+-[0-9]+-`)
	storyNumberPattern = regexp.MustCompile(`^([0-9]+-[0-9]+)-`)
)

type Story struct {
	Key    string
	Status string
}

type SprintStatus struct {
	Stories []Story
	byKey   map[string]string
}

func LoadSprintStatus(path string) (SprintStatus, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return SprintStatus{}, fmt.Errorf("read sprint status %q: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return SprintStatus{}, fmt.Errorf("parse sprint status yaml: %w", err)
	}

	root, err := mappingRoot(&doc)
	if err != nil {
		return SprintStatus{}, err
	}
	statusMap := statusMapping(root)

	result := SprintStatus{
		Stories: make([]Story, 0),
		byKey:   make(map[string]string),
	}

	for i := 0; i+1 < len(statusMap.Content); i += 2 {
		key := strings.TrimSpace(statusMap.Content[i].Value)
		if !eligibleStoryKey(key) {
			continue
		}

		status := normalizeStatus(extractStatus(statusMap.Content[i+1]))
		if status == "" {
			status = "unknown"
		}

		story := Story{Key: key, Status: status}
		result.Stories = append(result.Stories, story)
		result.byKey[key] = status
	}

	return result, nil
}

func (s SprintStatus) NextPendingStory() (Story, bool) {
	for _, story := range s.Stories {
		if normalizeStatus(story.Status) != "done" {
			return story, true
		}
	}
	return Story{}, false
}

func (s SprintStatus) StoryStatus(key string) (string, bool) {
	status, ok := s.byKey[key]
	return status, ok
}

func StoryNumberFromKey(storyKey string) (string, error) {
	match := storyNumberPattern.FindStringSubmatch(storyKey)
	if len(match) != 2 {
		return "", fmt.Errorf("invalid story key %q", storyKey)
	}
	return match[1], nil
}

func mappingRoot(doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected top-level mapping in sprint status yaml")
	}
	return doc, nil
}

func statusMapping(root *yaml.Node) *yaml.Node {
	root = resolveAlias(root)
	if root == nil || root.Kind != yaml.MappingNode {
		return root
	}

	for i := 0; i+1 < len(root.Content); i += 2 {
		key := strings.TrimSpace(root.Content[i].Value)
		if strings.EqualFold(key, "development_status") {
			child := resolveAlias(root.Content[i+1])
			if child != nil && child.Kind == yaml.MappingNode {
				return child
			}
			return root
		}
	}

	return root
}

func eligibleStoryKey(key string) bool {
	lower := strings.ToLower(key)
	if strings.HasPrefix(lower, "epic-") || strings.Contains(lower, "retrospective") {
		return false
	}
	return storyKeyPattern.MatchString(key)
}

func extractStatus(node *yaml.Node) string {
	node = resolveAlias(node)
	if node == nil {
		return ""
	}
	if node.Kind == yaml.ScalarNode {
		return strings.TrimSpace(node.Value)
	}
	if node.Kind != yaml.MappingNode {
		return ""
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if strings.EqualFold(strings.TrimSpace(node.Content[i].Value), "status") {
			return strings.TrimSpace(node.Content[i+1].Value)
		}
	}

	return ""
}

func normalizeStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

// UpdateStoryStatus updates a single story's status in the sprint-status.yaml file.
// It reads, modifies, and writes back the YAML preserving comments and structure.
func UpdateStoryStatus(path, storyKey, newStatus string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read sprint status for update: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Match lines like "  1-2-story-name: ready-for-dev"
		if strings.HasPrefix(trimmed, storyKey+":") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = fmt.Sprintf("%s%s: %s", indent, storyKey, newStatus)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("story key %q not found in sprint status file", storyKey)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// GitCommitStatusUpdate stages and commits the sprint-status.yaml change
// with an [autopilot] prefixed message.
func GitCommitStatusUpdate(ctx context.Context, workdir, statusFile, storyKey, newStatus string) error {
	// Stage the status file
	stageCmd := exec.CommandContext(ctx, "git", "-C", workdir, "add", statusFile)
	if out, err := stageCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add status file: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Check if there's actually something to commit
	diffCmd := exec.CommandContext(ctx, "git", "-C", workdir, "diff", "--cached", "--quiet")
	if err := diffCmd.Run(); err == nil {
		// Exit code 0 means no diff — nothing to commit
		return nil
	}

	msg := fmt.Sprintf("[autopilot] status: %s → %s", storyKey, newStatus)
	commitCmd := exec.CommandContext(ctx, "git", "-C", workdir, "commit", "-m", msg)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit status update: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func resolveAlias(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}
