package orchestrator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var pushEvidencePattern = regexp.MustCompile(`(?im)(^to\s+\S+|everything up-to-date|new branch|set up to track)`)

// ErrAuthExpired signals that the Claude API token has expired.
var ErrAuthExpired = errors.New("authentication token expired")

type ExecResult struct {
	RawOutput        string
	FullStream       string // complete stream-json output (thinking, tool calls, text)
	PushObserved     bool
	UpstreamAdvanced bool
	Published        bool
}

type CommandExecutor interface {
	Run(ctx context.Context, action Action) (ExecResult, error)
}

// streamEvent represents a single event from stream-json output.
type streamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	ContentBlock struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"content_block"`
	Delta struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"delta"`
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
}

// claudeJSONResponse matches the JSON output of `claude -p --output-format json`.
type claudeJSONResponse struct {
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
	IsError   bool   `json:"is_error"`
}

// ClaudeExecutor spawns `claude -p` as a subprocess with BMAD context injection.
type ClaudeExecutor struct {
	workdir       string
	claudeModel   string
	claudeCmd     string
	claudeEffort  string // global override; empty = use per-workflow defaults
	allowedTools  string
}

func NewClaudeExecutor(workdir, claudeModel, claudeCmd, claudeEffort string) *ClaudeExecutor {
	if strings.TrimSpace(claudeCmd) == "" {
		claudeCmd = "claude"
	}
	return &ClaudeExecutor{
		workdir:      workdir,
		claudeModel:  strings.TrimSpace(claudeModel),
		claudeCmd:    claudeCmd,
		claudeEffort: strings.TrimSpace(claudeEffort),
		allowedTools: "Bash,Read,Write,Edit,Glob,Grep,Agent,Skill",
	}
}

func (e *ClaudeExecutor) Run(ctx context.Context, action Action) (ExecResult, error) {
	beforeRef, beforeOK := upstreamRef(ctx, e.workdir)

	args := []string{
		"-p", action.Prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--dangerously-skip-permissions",
	}
	if e.claudeModel != "" {
		args = append(args, "--model", e.claudeModel)
	}
	// Per-action override takes priority over executor default
	allowedTools := e.allowedTools
	if action.AllowedTools != "" {
		allowedTools = action.AllowedTools
	}
	if allowedTools != "" {
		args = append(args, "--allowedTools", allowedTools)
	}

	// Resolve effort level: CLI override > per-workflow default
	effort := e.claudeEffort
	if effort == "" {
		effort = DefaultEffort(action.WorkflowKey)
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}

	// Load and inject BMAD context if workflow key is set
	if action.WorkflowKey != "" {
		bmadCtx, err := LoadBMADContext(e.workdir, action.WorkflowKey)
		if err != nil {
			return ExecResult{}, fmt.Errorf("load BMAD context for %q: %w", action.WorkflowKey, err)
		}
		if bmadCtx != nil && bmadCtx.HasContent() {
			args = append(args, "--append-system-prompt", bmadCtx.SystemPrompt())
		}
	}

	cmd := exec.CommandContext(ctx, e.claudeCmd, args...)
	cmd.Dir = e.workdir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	fullStream := stdout.String()
	rawOutput := extractResultFromStream(fullStream)
	if rawOutput == "" {
		// Fallback: try parsing as single JSON (in case stream-json isn't supported)
		rawOutput = extractClaudeOutput(stdout.Bytes())
	}
	if rawOutput == "" && runErr != nil {
		rawOutput = stderr.String()
	}
	if rawOutput == "" {
		rawOutput = fullStream
	}

	afterRef, afterOK := upstreamRef(ctx, e.workdir)
	headRef, headOK := currentHeadRef(ctx, e.workdir)
	clean, cleanOK := workingTreeClean(ctx, e.workdir)
	ahead, aheadOK := aheadOfUpstream(ctx, e.workdir)

	allOutput := fullStream + "\n" + stderr.String()

	result := ExecResult{
		RawOutput:        rawOutput,
		FullStream:       fullStream,
		PushObserved:     pushEvidencePattern.MatchString(allOutput),
		UpstreamAdvanced: upstreamChanged(beforeRef, beforeOK, afterRef, afterOK),
		Published:        publicationSatisfied(clean, cleanOK, ahead, aheadOK, headRef, headOK, afterRef, afterOK),
	}

	if runErr != nil {
		if isAuthError(allOutput) {
			return result, fmt.Errorf("%w: %v", ErrAuthExpired, runErr)
		}
		return result, fmt.Errorf("claude prompt failed (exit %v): %w", cmd.ProcessState.ExitCode(), runErr)
	}
	return result, nil
}

// extractResultFromStream parses stream-json output and extracts the final result text.
// Also concatenates all assistant text content blocks.
func extractResultFromStream(stream string) string {
	var resultText string
	var textParts []string

	scanner := bufio.NewScanner(strings.NewReader(stream))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event streamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		// Capture the final result field (last message)
		if event.Result != "" {
			resultText = event.Result
		}

		// Accumulate text deltas
		if event.Delta.Text != "" {
			textParts = append(textParts, event.Delta.Text)
		}
	}

	if resultText != "" {
		return strings.TrimSpace(resultText)
	}
	if len(textParts) > 0 {
		return strings.TrimSpace(strings.Join(textParts, ""))
	}
	return ""
}

// isAuthError checks if the output contains an authentication/token expiry error.
func isAuthError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "oauth token has expired") ||
		strings.Contains(lower, "authentication_error") ||
		(strings.Contains(lower, "401") && strings.Contains(lower, "token"))
}

// extractClaudeOutput parses the JSON response from `claude -p --output-format json`.
func extractClaudeOutput(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}

	var resp claudeJSONResponse
	if err := json.Unmarshal(trimmed, &resp); err != nil {
		return string(trimmed)
	}
	return strings.TrimSpace(resp.Result)
}

func upstreamRef(ctx context.Context, workdir string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", "-C", workdir, "rev-parse", "--verify", "@{u}")
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(output)), true
}

func upstreamChanged(before string, beforeOK bool, after string, afterOK bool) bool {
	if !beforeOK && afterOK {
		return true
	}
	return beforeOK && afterOK && before != after
}

func currentHeadRef(ctx context.Context, workdir string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", "-C", workdir, "rev-parse", "--verify", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(output)), true
}

func workingTreeClean(ctx context.Context, workdir string) (bool, bool) {
	cmd := exec.CommandContext(ctx, "git", "-C", workdir, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false, false
	}
	return strings.TrimSpace(string(output)) == "", true
}

func aheadOfUpstream(ctx context.Context, workdir string) (int, bool) {
	cmd := exec.CommandContext(ctx, "git", "-C", workdir, "rev-list", "--count", "@{u}..HEAD")
	output, err := cmd.Output()
	if err != nil {
		return 0, false
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, false
	}

	return count, true
}

// EnsurePushed pushes to the upstream branch if there are unpushed commits.
// Returns true if a push was performed, false if already in sync.
func EnsurePushed(ctx context.Context, workdir string) (bool, error) {
	ahead, ok := aheadOfUpstream(ctx, workdir)
	if !ok || ahead == 0 {
		return false, nil
	}

	cmd := exec.CommandContext(ctx, "git", "-C", workdir, "push")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git push failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return true, nil
}

func publicationSatisfied(
	clean bool,
	cleanOK bool,
	ahead int,
	aheadOK bool,
	headRef string,
	headOK bool,
	upstreamRef string,
	upstreamOK bool,
) bool {
	if !cleanOK || !clean {
		return false
	}
	if !headOK || !upstreamOK {
		return false
	}
	if !aheadOK || ahead != 0 {
		return false
	}
	return headRef == upstreamRef
}
