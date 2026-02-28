package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/dlukt/bmad-autopilot/internal/brain"
)

type Config struct {
	StatusFile     string
	Brain          string
	Workdir        string
	CopilotModel   string
	CommandTimeout time.Duration
}

type Runner struct {
	cfg      Config
	brain    brain.Brain
	executor CommandExecutor
}

func New(cfg Config) (*Runner, error) {
	if strings.TrimSpace(cfg.Workdir) == "" {
		cfg.Workdir = "."
	}
	absWorkdir, err := filepath.Abs(cfg.Workdir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir: %w", err)
	}
	cfg.Workdir = absWorkdir

	if strings.TrimSpace(cfg.StatusFile) == "" {
		cfg.StatusFile = "_bmad-output/implementation-artifacts/sprint-status.yaml"
	}
	if !filepath.IsAbs(cfg.StatusFile) {
		cfg.StatusFile = filepath.Join(cfg.Workdir, cfg.StatusFile)
	}

	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = 2 * time.Hour
	}

	selectedBrain, err := brain.New(cfg.Brain)
	if err != nil {
		return nil, err
	}

	return &Runner{
		cfg:      cfg,
		brain:    selectedBrain,
		executor: NewSDKExecutor(cfg.Workdir, cfg.CopilotModel),
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	for {
		sprintStatus, err := LoadSprintStatus(r.cfg.StatusFile)
		if err != nil {
			return err
		}

		story, ok := sprintStatus.NextPendingStory()
		if !ok {
			fmt.Println("DONE: all non-retrospective stories are done")
			return nil
		}

		storyNumber, err := StoryNumberFromKey(story.Key)
		if err != nil {
			return err
		}

		primaryActions, err := PlanPrimaryActions(story.Status, storyNumber)
		if err != nil {
			return err
		}

		for _, action := range primaryActions {
			if _, _, err := r.runStep(ctx, story.Key, action); err != nil {
				return err
			}
		}

		reviewAction := ReviewAction(storyNumber)
		for {
			result, afterStatus, err := r.runStep(ctx, story.Key, reviewAction)
			if err != nil {
				return err
			}
			if !ShouldContinueReview(afterStatus, result.PushObserved || result.UpstreamAdvanced) {
				break
			}
		}
	}
}

func (r *Runner) runStep(ctx context.Context, storyKey string, action Action) (ExecResult, string, error) {
	beforeStatus, err := r.statusForStory(storyKey)
	if err != nil {
		return ExecResult{}, "", err
	}
	fmt.Printf("STORY: %s | STATUS(before): %s | ACTION: %s\n", storyKey, beforeStatus, action.Command)

	commandCtx, cancel := context.WithTimeout(ctx, r.cfg.CommandTimeout)
	defer cancel()

	execResult, execErr := r.executor.Run(commandCtx, action)
	resultLine := r.summarizeResult(commandCtx, action.Command, execResult.RawOutput)
	if execErr != nil {
		if resultLine == "" {
			resultLine = execErr.Error()
		} else {
			resultLine = fmt.Sprintf("%s; error: %v", resultLine, execErr)
		}
	}
	fmt.Printf("RESULT: %s\n", oneLine(resultLine))

	afterStatus, err := r.statusForStory(storyKey)
	if err != nil {
		return execResult, "", err
	}
	fmt.Printf("STATUS(after): %s\n", afterStatus)

	if execErr != nil {
		return execResult, afterStatus, execErr
	}
	return execResult, afterStatus, nil
}

func (r *Runner) statusForStory(storyKey string) (string, error) {
	current, err := LoadSprintStatus(r.cfg.StatusFile)
	if err != nil {
		return "", err
	}

	status, ok := current.StoryStatus(storyKey)
	if !ok {
		return "<missing>", nil
	}
	return status, nil
}

func (r *Runner) summarizeResult(ctx context.Context, command string, rawOutput string) string {
	summary, err := r.brain.Summarize(ctx, command, rawOutput)
	if err != nil || strings.TrimSpace(summary) == "" {
		return fallbackSummary(rawOutput)
	}
	return summary
}

func fallbackSummary(rawOutput string) string {
	lines := strings.Split(rawOutput, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return "no output"
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
