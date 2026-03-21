package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"errors"

	"github.com/alexwrite/bmad-autopilot/internal/brain"
)

type Config struct {
	StatusFile           string
	Brain                string
	Workdir              string
	ClaudeModel          string
	ClaudeCommand        string
	CommandTimeout       time.Duration
	DisableCommandOutput bool
}

type Runner struct {
	cfg      Config
	brain    brain.Brain
	executor CommandExecutor
}

const defaultStatusFile = "_bmad-output/implementation-artifacts/sprint-status.yaml"

func New(cfg Config) (*Runner, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve current directory: %w", err)
	}

	statusFile, err := resolveStatusFilePath(cfg.StatusFile, cwd)
	if err != nil {
		return nil, err
	}
	cfg.StatusFile = statusFile

	if strings.TrimSpace(cfg.Workdir) == "" {
		cfg.Workdir = inferWorkdirFromStatusFile(cfg.StatusFile, cwd)
	}

	absWorkdir, err := filepath.Abs(cfg.Workdir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir: %w", err)
	}
	cfg.Workdir = absWorkdir

	selectedBrain, err := brain.New(cfg.Brain)
	if err != nil {
		return nil, err
	}

	return &Runner{
		cfg:      cfg,
		brain:    selectedBrain,
		executor: NewClaudeExecutor(cfg.Workdir, cfg.ClaudeModel, cfg.ClaudeCommand),
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	consecutiveBlocked := 0

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

		// Safety: stop if too many stories blocked in a row
		if consecutiveBlocked >= MaxConsecutiveBlocked {
			fmt.Printf("HALT: %d consecutive stories blocked — stopping autopilot\n", consecutiveBlocked)
			return fmt.Errorf("halted after %d consecutive blocked stories", consecutiveBlocked)
		}

		storyNumber, err := StoryNumberFromKey(story.Key)
		if err != nil {
			return err
		}

		fmt.Printf("\n════════════════════════════════════════\n")
		fmt.Printf("STORY: %s (status: %s)\n", story.Key, story.Status)
		fmt.Printf("════════════════════════════════════════\n")

		outcome, err := r.processStory(ctx, story, storyNumber)
		if err != nil {
			if errors.Is(err, ErrAuthExpired) {
				return err
			}
			return err
		}

		if outcome == "blocked" {
			consecutiveBlocked++
		} else {
			consecutiveBlocked = 0
		}

		// Push any remaining unpushed commits
		pushed, err := EnsurePushed(ctx, r.cfg.Workdir)
		if err != nil {
			return fmt.Errorf("ensure pushed after story %s: %w", story.Key, err)
		}
		if pushed {
			fmt.Printf("PUSH: pushed remaining commits for story %s\n", story.Key)
		}
	}
}

// processStory handles a single story through all phases.
// Returns the final outcome: "done" or "blocked".
func (r *Runner) processStory(ctx context.Context, story Story, storyNumber string) (string, error) {
	invocations := 0

	// --- Phase 1: Primary actions (create-story, dev-story) ---
	primaryActions, err := PlanPrimaryActions(story.Status, storyNumber)
	if err != nil {
		return "", err
	}

	for _, action := range primaryActions {
		if invocations >= MaxInvocationsPerStory {
			return r.blockStory(ctx, story.Key, "max invocations reached during primary actions")
		}

		result, _, err := r.runStep(ctx, story.Key, action)
		invocations++
		if err != nil {
			if errors.Is(err, ErrAuthExpired) {
				return "", err
			}
			return "", err
		}

		// Check if Claude updated the status itself
		currentStatus, _ := r.statusForStory(story.Key)
		expectedStatus := defaultStatusForWorkflow(action.WorkflowKey)
		if normalizeStatus(currentStatus) == normalizeStatus(expectedStatus) {
			fmt.Printf("  STATUS: Claude set %s correctly — no judge needed\n", expectedStatus)
		} else {
			// Claude didn't update — apply default transition (no judge, save tokens)
			fmt.Printf("  GUARD: Claude left status at %q, autopilot correcting to %q\n", currentStatus, expectedStatus)
			if err := r.ensureStatus(ctx, story.Key, expectedStatus); err != nil {
				return "", err
			}
		}

		_ = result // raw output not needed when judge is skipped
	}

	// --- Phase 2: Review loop ---
	reviewAction := ReviewAction(storyNumber)
	for round := 1; round <= MaxReviewRounds; round++ {
		if invocations >= MaxInvocationsPerStory {
			return r.blockStory(ctx, story.Key, "max invocations reached during review")
		}

		fmt.Printf("REVIEW: round %d/%d for %s\n", round, MaxReviewRounds, story.Key)

		result, _, err := r.runStep(ctx, story.Key, reviewAction)
		invocations++
		if err != nil {
			if errors.Is(err, ErrAuthExpired) {
				return "", err
			}
			return "", err
		}

		// Check if Claude updated the status itself
		currentStatus, _ := r.statusForStory(story.Key)
		if !ShouldContinueReview(currentStatus) {
			fmt.Printf("REVIEW: story %s is %s after round %d — no judge needed\n", story.Key, currentStatus, round)
			return currentStatus, nil
		}

		// Claude didn't set "done" — call judge ONLY NOW to decide
		fmt.Printf("JUDGE: Claude didn't resolve status, calling judge for round %d\n", round)
		verdict, judgeErr := r.callJudge(ctx, story.Key, "code-review", result.RawOutput)
		if judgeErr != nil {
			if errors.Is(judgeErr, ErrAuthExpired) {
				return "", judgeErr
			}
			fmt.Printf("JUDGE: evaluation failed for review round %d: %v\n", round, judgeErr)
		}

		// Judge says no more work needed — mark done
		if judgeErr == nil && !verdict.NeedsMoreWork {
			fmt.Printf("JUDGE: no more work needed — marking %s as done\n", story.Key)
			if err := r.ensureStatus(ctx, story.Key, "done"); err != nil {
				return "", err
			}
			return "done", nil
		}

		if round == MaxReviewRounds {
			fmt.Printf("REVIEW: max rounds (%d) reached for %s\n", MaxReviewRounds, story.Key)
			// Last chance: judge says done?
			if judgeErr == nil && verdict.RecommendedStatus == "done" {
				if err := r.ensureStatus(ctx, story.Key, "done"); err != nil {
					return "", err
				}
				return "done", nil
			}
			return r.blockStory(ctx, story.Key, fmt.Sprintf("max review rounds (%d) exhausted", MaxReviewRounds))
		}
	}

	return "done", nil
}

// judgeAndTransition calls the judge and ensures the story transitions correctly.
func (r *Runner) judgeAndTransition(ctx context.Context, storyKey, workflowKey, rawOutput string) error {
	verdict, err := r.callJudge(ctx, storyKey, workflowKey, rawOutput)
	if err != nil {
		return err
	}

	fmt.Printf("JUDGE: %s → recommended=%s, success=%v, summary=%s\n",
		workflowKey, verdict.RecommendedStatus, verdict.Success, verdict.Summary)

	// Check if Claude already updated the status
	currentStatus, _ := r.statusForStory(storyKey)
	expectedStatus := verdict.RecommendedStatus

	if normalizeStatus(currentStatus) != normalizeStatus(expectedStatus) {
		// Claude didn't update — autopilot does it
		fmt.Printf("GUARD: Claude left status at %q, autopilot correcting to %q\n", currentStatus, expectedStatus)
		if err := r.ensureStatus(ctx, storyKey, expectedStatus); err != nil {
			return err
		}
	}

	return nil
}

// callJudge invokes the judge Claude to evaluate a worker result.
func (r *Runner) callJudge(ctx context.Context, storyKey, workflowKey, rawOutput string) (JudgeVerdict, error) {
	commandCtx := ctx
	cancel := func() {}
	// Judge gets a shorter timeout than workers
	if r.cfg.CommandTimeout > 0 {
		judgeTimeout := r.cfg.CommandTimeout / 3
		if judgeTimeout < 60*time.Second {
			judgeTimeout = 60 * time.Second
		}
		commandCtx, cancel = context.WithTimeout(ctx, judgeTimeout)
	}
	defer cancel()

	return Judge(commandCtx, r.cfg.Workdir, r.cfg.ClaudeCommand, r.cfg.ClaudeModel, storyKey, workflowKey, rawOutput)
}

// applyDefaultTransition sets the expected status when judge is unavailable.
func (r *Runner) applyDefaultTransition(ctx context.Context, storyKey, workflowKey string) {
	expected := defaultStatusForWorkflow(workflowKey)
	currentStatus, _ := r.statusForStory(storyKey)
	if normalizeStatus(currentStatus) != normalizeStatus(expected) {
		_ = r.ensureStatus(ctx, storyKey, expected)
	}
}

// ensureStatus updates sprint-status.yaml and commits with [autopilot] prefix.
func (r *Runner) ensureStatus(ctx context.Context, storyKey, newStatus string) error {
	if err := UpdateStoryStatus(r.cfg.StatusFile, storyKey, newStatus); err != nil {
		return fmt.Errorf("update status for %s: %w", storyKey, err)
	}
	if err := GitCommitStatusUpdate(ctx, r.cfg.Workdir, r.cfg.StatusFile, storyKey, newStatus); err != nil {
		return fmt.Errorf("commit status update for %s: %w", storyKey, err)
	}
	fmt.Printf("STATUS: %s → %s [autopilot]\n", storyKey, newStatus)
	return nil
}

// blockStory marks a story as "blocked" and logs the reason.
func (r *Runner) blockStory(ctx context.Context, storyKey, reason string) (string, error) {
	fmt.Printf("BLOCKED: %s — %s\n", storyKey, reason)
	if err := r.ensureStatus(ctx, storyKey, "blocked"); err != nil {
		return "blocked", err
	}
	return "blocked", nil
}

func (r *Runner) runStep(ctx context.Context, storyKey string, action Action) (ExecResult, string, error) {
	beforeStatus, err := r.statusForStory(storyKey)
	if err != nil {
		return ExecResult{}, "", err
	}
	fmt.Printf("  ACTION: %s\n", action.WorkflowKey)
	fmt.Printf("  STATUS(before): %s\n", beforeStatus)

	commandCtx := ctx
	cancel := func() {}
	if r.cfg.CommandTimeout > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, r.cfg.CommandTimeout)
	}
	defer cancel()

	execResult, execErr := r.executor.Run(commandCtx, action)
	if !r.cfg.DisableCommandOutput {
		r.printRawOutput(execResult.RawOutput)
	}

	resultLine := r.summarizeResult(commandCtx, action.Command, execResult.RawOutput)
	if execErr != nil {
		if resultLine == "" {
			resultLine = execErr.Error()
		} else {
			resultLine = fmt.Sprintf("%s; error: %v", resultLine, execErr)
		}
	}
	fmt.Printf("  RESULT: %s\n", oneLine(resultLine))

	afterStatus, err := r.statusForStory(storyKey)
	if err != nil {
		return execResult, "", err
	}
	fmt.Printf("  STATUS(after): %s\n", afterStatus)

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

func (r *Runner) printRawOutput(rawOutput string) {
	trimmed := strings.TrimSpace(rawOutput)
	if trimmed == "" {
		fmt.Println("  OUTPUT: <no output>")
		return
	}
	fmt.Println("  OUTPUT:")
	fmt.Println(trimmed)
}

func resolveStatusFilePath(statusFile, cwd string) (string, error) {
	statusFile = strings.TrimSpace(statusFile)
	if statusFile == "" {
		statusFile = defaultStatusFile
	}
	if filepath.IsAbs(statusFile) {
		return filepath.Clean(statusFile), nil
	}
	absolute, err := filepath.Abs(filepath.Join(cwd, statusFile))
	if err != nil {
		return "", fmt.Errorf("resolve status file path: %w", err)
	}
	return absolute, nil
}

func inferWorkdirFromStatusFile(statusFile, fallback string) string {
	clean := filepath.Clean(statusFile)
	marker := string(filepath.Separator) + "_bmad-output" + string(filepath.Separator)
	switch idx := strings.Index(clean, marker); {
	case idx > 0:
		return clean[:idx]
	case idx == 0:
		return string(filepath.Separator)
	default:
		return fallback
	}
}
