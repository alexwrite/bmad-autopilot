package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	copilot "github.com/github/copilot-sdk/go"
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

type SDKExecutor struct {
	workdir      string
	copilotModel string
}

func NewSDKExecutor(workdir, copilotModel string) *SDKExecutor {
	return &SDKExecutor{
		workdir:      workdir,
		copilotModel: strings.TrimSpace(copilotModel),
	}
}

func (e *SDKExecutor) Run(ctx context.Context, action Action) (ExecResult, error) {
	beforeRef, beforeOK := upstreamRef(ctx, e.workdir)

	client := copilot.NewClient(&copilot.ClientOptions{
		Cwd:      e.workdir,
		LogLevel: "error",
		CLIArgs:  []string{"--yolo", "--no-ask-user", "-s"},
	})
	if err := client.Start(ctx); err != nil {
		return ExecResult{}, fmt.Errorf("start copilot client: %w", err)
	}
	defer client.Stop()

	sessionCfg := &copilot.SessionConfig{
		WorkingDirectory:    e.workdir,
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
	}
	if e.copilotModel != "" {
		sessionCfg.Model = e.copilotModel
	}

	session, err := client.CreateSession(ctx, sessionCfg)
	if err != nil {
		return ExecResult{}, fmt.Errorf("create copilot session: %w", err)
	}
	defer session.Destroy()

	_, sendErr := session.SendAndWait(ctx, copilot.MessageOptions{
		Prompt: action.Prompt,
	})

	events, eventsErr := session.GetMessages(ctx)
	rawOutput := collectOutput(events)
	if rawOutput == "" && sendErr != nil {
		rawOutput = sendErr.Error()
	}
	if rawOutput == "" && eventsErr != nil {
		rawOutput = eventsErr.Error()
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

	if sendErr != nil {
		return result, fmt.Errorf("copilot prompt failed: %w", sendErr)
	}
	if eventsErr != nil {
		return result, fmt.Errorf("read copilot messages: %w", eventsErr)
	}

	return result, nil
}

func collectOutput(events []copilot.SessionEvent) string {
	lines := make([]string, 0)

	appendField := func(value *string) {
		if value == nil {
			return
		}
		text := strings.TrimSpace(*value)
		if text == "" {
			return
		}
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				lines = append(lines, line)
			}
		}
	}

	for _, event := range events {
		appendField(event.Data.Content)
		appendField(event.Data.Message)
		appendField(event.Data.Summary)
		appendField(event.Data.SummaryContent)
		appendField(event.Data.PartialOutput)
		appendField(event.Data.ProgressMessage)
		if event.Data.Result != nil {
			content := event.Data.Result.Content
			appendField(&content)
			appendField(event.Data.Result.DetailedContent)
		}
	}

	return strings.Join(lines, "\n")
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
