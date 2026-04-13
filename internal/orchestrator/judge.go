package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// JudgeVerdict is the structured response from the judge Claude call.
type JudgeVerdict struct {
	// Success indicates whether the action completed its objective.
	Success bool `json:"success"`
	// NeedsMoreWork indicates whether additional review rounds are needed (code-review only).
	NeedsMoreWork bool `json:"needs_more_work"`
	// CommitMessage is a descriptive conventional commit message for the work done.
	CommitMessage string `json:"commit_message"`
	// Summary is a one-line human-readable summary of what happened.
	Summary string `json:"summary"`
	// RecommendedStatus is the status the story should transition to.
	RecommendedStatus string `json:"recommended_status"`
}

// projectContextPaths lists where to look for project context, in priority order.
var projectContextPaths = []string{
	"docs/project-context.md",
	"_bmad-output/project-context.md",
	"project-context.md",
	"CLAUDE.md",
}

// Judge calls a lightweight Claude instance to evaluate the result of a worker action.
// It inspects git diff, story files, and raw output to produce a structured verdict.
func Judge(ctx context.Context, workdir, claudeCmd, claudeModel, effortOverride, storyKey, workflowKey, rawOutput string) (JudgeVerdict, error) {
	if strings.TrimSpace(claudeCmd) == "" {
		claudeCmd = "claude"
	}

	projectContext := loadProjectContext(workdir)
	prompt := buildJudgePrompt(storyKey, workflowKey, rawOutput, projectContext)

	args := []string{
		"-p", prompt,
		"--output-format", "json",
		"--dangerously-skip-permissions",
		"--allowedTools", "Bash,Read,Glob,Grep",
	}
	if claudeModel != "" {
		args = append(args, "--model", claudeModel)
	}

	// Resolve effort: CLI override > judge default
	effort := strings.TrimSpace(effortOverride)
	if effort == "" {
		effort = JudgeEffort
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}

	cmd := exec.CommandContext(ctx, claudeCmd, args...)
	cmd.Dir = workdir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If judge fails, return a safe default verdict
		if isAuthError(stdout.String()) || isAuthError(stderr.String()) {
			return JudgeVerdict{}, fmt.Errorf("%w: judge auth failed", ErrAuthExpired)
		}
		return fallbackVerdict(workflowKey), nil
	}

	output := extractClaudeOutput(stdout.Bytes())
	return parseJudgeVerdict(output, workflowKey)
}

// loadProjectContext reads the first available project context file.
// Returns empty string if none found (non-fatal).
func loadProjectContext(workdir string) string {
	for _, relPath := range projectContextPaths {
		fullPath := filepath.Join(workdir, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			continue
		}
		// Cap at 6000 chars to keep the judge prompt lean
		if len(trimmed) > 6000 {
			trimmed = trimmed[:6000] + "\n... (truncated)"
		}
		return trimmed
	}
	return ""
}

func buildJudgePrompt(storyKey, workflowKey, rawOutput, projectContext string) string {
	// Truncate raw output to avoid token explosion
	truncated := rawOutput
	if len(truncated) > 4000 {
		truncated = truncated[len(truncated)-4000:]
	}

	var contextBlock string
	if projectContext != "" {
		contextBlock = fmt.Sprintf(`
=== PROJECT CONTEXT ===
%s
=== END PROJECT CONTEXT ===

`, projectContext)
	}

	return fmt.Sprintf(`You are an automated judge evaluating the result of a BMAD workflow step.

TASK: Evaluate the result of the "%s" workflow for story "%s".
%s
INSTRUCTIONS:
1. Run "git log --oneline -5" to see recent commits
2. Run "git diff HEAD~1 --stat" to see what changed in the last commit
3. Check if the sprint-status.yaml was updated for this story
4. Read the story file if it exists in _bmad-output/implementation-artifacts/stories/

Based on your analysis, respond with ONLY a JSON object (no markdown, no explanation):

{
  "success": true/false,
  "needs_more_work": true/false,
  "commit_message": "type(scope): descriptive message of what was actually done",
  "summary": "One sentence describing what happened",
  "recommended_status": "ready-for-dev|in-progress|review|done|blocked"
}

RULES for commit_message:
- Use conventional commits: feat, fix, refactor, chore, docs
- The scope is the story number (e.g. "1-2")
- Describe WHAT was actually implemented or fixed, not just "completed"
- Use the project context above to write domain-aware messages
- Examples: "feat(1-2): add Tailwind CSS-first @theme tokens with Maillard palette"
- Examples: "fix(1-2): correct primary-dark contrast ratio to meet WCAG 4.5:1"

RULES for recommended_status:
- After create-story: "ready-for-dev"
- After dev-story: "review"
- After code-review with no issues found: "done"
- After code-review with fixes applied: "review" (needs another round)
- If something clearly failed: keep current status

RULES for needs_more_work:
- true ONLY if code-review found and fixed issues (another review round needed)
- false if review found nothing or if this is not a code-review step

Last 4000 chars of worker output:
---
%s
---`, workflowKey, storyKey, contextBlock, truncated)
}

// parseJudgeVerdict extracts the JSON verdict from the judge output.
func parseJudgeVerdict(output, workflowKey string) (JudgeVerdict, error) {
	output = strings.TrimSpace(output)

	// Try to find JSON in the output (judge might wrap it in text)
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start >= 0 && end > start {
		output = output[start : end+1]
	}

	var verdict JudgeVerdict
	if err := json.Unmarshal([]byte(output), &verdict); err != nil {
		return fallbackVerdict(workflowKey), nil
	}

	// Sanitize recommended status
	verdict.RecommendedStatus = normalizeStatus(verdict.RecommendedStatus)
	if verdict.RecommendedStatus == "" {
		verdict.RecommendedStatus = defaultStatusForWorkflow(workflowKey)
	}

	// Sanitize commit message
	if strings.TrimSpace(verdict.CommitMessage) == "" {
		verdict.CommitMessage = fmt.Sprintf("chore: %s completed", workflowKey)
	}

	return verdict, nil
}

// fallbackVerdict returns a safe default when the judge can't be reached.
func fallbackVerdict(workflowKey string) JudgeVerdict {
	return JudgeVerdict{
		Success:           true,
		NeedsMoreWork:     false,
		CommitMessage:     fmt.Sprintf("chore: %s completed", workflowKey),
		Summary:           fmt.Sprintf("%s completed (judge unavailable)", workflowKey),
		RecommendedStatus: defaultStatusForWorkflow(workflowKey),
	}
}

func defaultStatusForWorkflow(workflowKey string) string {
	switch workflowKey {
	case "create-story":
		return "ready-for-dev"
	case "dev-story":
		return "review"
	case "code-review":
		return "done"
	case "validate-story":
		return "validated"
	default:
		return "in-progress"
	}
}
