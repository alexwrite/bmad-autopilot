package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var pushEvidencePattern = regexp.MustCompile(`(?im)(^to\s+\S+|everything up-to-date|new branch|set up to track)`)

type ExecResult struct {
	RawOutput        string
	PushObserved     bool
	UpstreamAdvanced bool
	Published        bool
}

type CommandExecutor interface {
	Run(ctx context.Context, action Action) (ExecResult, error)
}

// claudeJSONResponse matches the JSON output of `claude -p --output-format json`.
type claudeJSONResponse struct {
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
	IsError   bool   `json:"is_error"`
}

// ClaudeExecutor spawns `claude -p` as a subprocess (pattern from OpenClaw).
type ClaudeExecutor struct {
	workdir      string
	claudeModel  string
	claudeCmd    string
	allowedTools string
}

func NewClaudeExecutor(workdir, claudeModel, claudeCmd string) *ClaudeExecutor {
	if strings.TrimSpace(claudeCmd) == "" {
		claudeCmd = "claude"
	}
	return &ClaudeExecutor{
		workdir:      workdir,
		claudeModel:  strings.TrimSpace(claudeModel),
		claudeCmd:    claudeCmd,
		allowedTools: "Bash,Read,Write,Edit,Glob,Grep,Agent,Skill",
	}
}

func (e *ClaudeExecutor) Run(ctx context.Context, action Action) (ExecResult, error) {
	beforeRef, beforeOK := upstreamRef(ctx, e.workdir)

	args := []string{
		"-p", action.Prompt,
		"--output-format", "json",
		"--dangerously-skip-permissions",
	}
	if e.claudeModel != "" {
		args = append(args, "--model", e.claudeModel)
	}
	if e.allowedTools != "" {
		args = append(args, "--allowedTools", e.allowedTools)
	}

	cmd := exec.CommandContext(ctx, e.claudeCmd, args...)
	cmd.Dir = e.workdir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	rawOutput := extractClaudeOutput(stdout.Bytes())
	if rawOutput == "" && runErr != nil {
		rawOutput = stderr.String()
	}
	if rawOutput == "" {
		rawOutput = stdout.String()
	}

	afterRef, afterOK := upstreamRef(ctx, e.workdir)
	headRef, headOK := currentHeadRef(ctx, e.workdir)
	clean, cleanOK := workingTreeClean(ctx, e.workdir)
	ahead, aheadOK := aheadOfUpstream(ctx, e.workdir)

	result := ExecResult{
		RawOutput:        rawOutput,
		PushObserved:     pushEvidencePattern.MatchString(rawOutput),
		UpstreamAdvanced: upstreamChanged(beforeRef, beforeOK, afterRef, afterOK),
		Published:        publicationSatisfied(clean, cleanOK, ahead, aheadOK, headRef, headOK, afterRef, afterOK),
	}

	if runErr != nil {
		return result, fmt.Errorf("claude prompt failed (exit %v): %w", cmd.ProcessState.ExitCode(), runErr)
	}
	return result, nil
}

// extractClaudeOutput parses the JSON response from `claude -p --output-format json`.
func extractClaudeOutput(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}

	var resp claudeJSONResponse
	if err := json.Unmarshal(trimmed, &resp); err != nil {
		// Not JSON — return raw text as-is
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
