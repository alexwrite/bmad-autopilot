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
	ClaudeEffort         string
	CommandTimeout       time.Duration
	DisableCommandOutput bool
	EpicFilter           []int
	StoryFilter          []string // specific story numbers to process (e.g. ["2-1", "2-3"])
}

type Runner struct {
	cfg      Config
	brain    brain.Brain
	executor CommandExecutor
	log      *RunLogger
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

	logger, err := NewRunLogger(cfg.Workdir)
	if err != nil {
		return nil, fmt.Errorf("init logger: %w", err)
	}

	baseExecutor := NewClaudeExecutor(cfg.Workdir, cfg.ClaudeModel, cfg.ClaudeCommand, cfg.ClaudeEffort)

	return &Runner{
		cfg:      cfg,
		brain:    selectedBrain,
		executor: newRetryingExecutor(baseExecutor, DefaultRetryConfig(), logger),
		log:      logger,
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	defer r.log.Close()

	// --story mode: process only the specified stories, then stop
	if len(r.cfg.StoryFilter) > 0 {
		return r.runStoryFilter(ctx)
	}

	return r.runLoop(ctx)
}

// runStoryFilter processes only the explicitly requested stories (--story flag).
func (r *Runner) runStoryFilter(ctx context.Context) error {
	r.log.Log("FILTER", "targeting stories: %v", r.cfg.StoryFilter)

	sprintStatus, err := LoadSprintStatus(r.cfg.StatusFile)
	if err != nil {
		return err
	}

	stories := sprintStatus.FindStoriesByNumbers(r.cfg.StoryFilter)
	if len(stories) == 0 {
		return fmt.Errorf("no stories found matching %v", r.cfg.StoryFilter)
	}

	for _, story := range stories {
		storyNumber, err := StoryNumberFromKey(story.Key)
		if err != nil {
			return err
		}

		r.log.LogSeparator()
		r.log.Log("STORY", "%s (status: %s)", story.Key, story.Status)
		r.log.LogSeparator()

		if err := r.log.SetStory(story.Key); err != nil {
			return fmt.Errorf("init story log dir: %w", err)
		}

		_, err = r.processStory(ctx, story, storyNumber)
		if err != nil {
			if errors.Is(err, ErrAuthExpired) {
				r.log.Log("AUTH", "token expired — stopping")
				return err
			}
			return err
		}

		pushed, err := EnsurePushed(ctx, r.cfg.Workdir)
		if err != nil {
			return fmt.Errorf("ensure pushed after story %s: %w", story.Key, err)
		}
		if pushed {
			r.log.Log("PUSH", "pushed remaining commits for story %s", story.Key)
		}
	}

	r.log.Log("DONE", "all requested stories processed")
	if finalStatus, loadErr := LoadSprintStatus(r.cfg.StatusFile); loadErr == nil {
		r.auditDeferred(finalStatus)
	}
	return nil
}

// runLoop is the original behavior: process stories sequentially until all are done.
func (r *Runner) runLoop(ctx context.Context) error {
	consecutiveBlocked := 0

	if len(r.cfg.EpicFilter) > 0 {
		r.log.Log("FILTER", "targeting epics: %v", r.cfg.EpicFilter)
	}

	for {
		sprintStatus, err := LoadSprintStatus(r.cfg.StatusFile)
		if err != nil {
			return err
		}

		story, ok := sprintStatus.NextPendingStoryInEpics(r.cfg.EpicFilter)
		if !ok {
			if len(r.cfg.EpicFilter) > 0 {
				r.log.Log("DONE", "all stories in selected epics are done")
			} else {
				r.log.Log("DONE", "all non-retrospective stories are done")
			}
			r.auditDeferred(sprintStatus)
			return nil
		}

		// Safety: stop if too many stories blocked in a row
		if consecutiveBlocked >= MaxConsecutiveBlocked {
			r.log.Log("HALT", "%d consecutive stories blocked — stopping autopilot", consecutiveBlocked)
			return fmt.Errorf("halted after %d consecutive blocked stories", consecutiveBlocked)
		}

		storyNumber, err := StoryNumberFromKey(story.Key)
		if err != nil {
			return err
		}

		r.log.LogSeparator()
		r.log.Log("STORY", "%s (status: %s)", story.Key, story.Status)
		r.log.LogSeparator()

		if err := r.log.SetStory(story.Key); err != nil {
			return fmt.Errorf("init story log dir: %w", err)
		}

		outcome, err := r.processStory(ctx, story, storyNumber)
		if err != nil {
			if errors.Is(err, ErrAuthExpired) {
				r.log.Log("AUTH", "token expired — stopping")
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
			r.log.Log("PUSH", "pushed remaining commits for story %s", story.Key)
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

		result, beforeStatus, afterStatus, err := r.runStep(ctx, story.Key, action, 0)
		invocations++
		if err != nil {
			r.log.SaveError(action.WorkflowKey, 0, err.Error())
			if errors.Is(err, ErrAuthExpired) {
				return "", err
			}
			return "", err
		}

		// Save output, stream, and result
		r.log.SaveOutput(action.WorkflowKey, 0, result.RawOutput)
		r.log.SaveStream(action.WorkflowKey, 0, result.FullStream)
		summary := r.summarizeResult(ctx, action.Command, result.RawOutput)
		stepResult := NewStepResult(story.Key, action.WorkflowKey, beforeStatus, afterStatus, summary, result.RawOutput)
		r.log.SaveResult(action.WorkflowKey, 0, stepResult)

		// Check if Claude updated the status itself
		currentStatus, _ := r.statusForStory(story.Key)
		expectedStatus := defaultStatusForWorkflow(action.WorkflowKey)
		if normalizeStatus(currentStatus) == normalizeStatus(expectedStatus) {
			r.log.Log("STATUS", "Claude set %s correctly — no judge needed", expectedStatus)
		} else {
			r.log.Log("GUARD", "Claude left status at %q, autopilot correcting to %q", currentStatus, expectedStatus)
			if err := r.ensureStatus(ctx, story.Key, expectedStatus); err != nil {
				return "", err
			}
		}
	}

	// --- Phase 2: Review loop ---
	reviewAction := ReviewAction(storyNumber)
	for round := 1; round <= MaxReviewRounds; round++ {
		if invocations >= MaxInvocationsPerStory {
			return r.blockStory(ctx, story.Key, "max invocations reached during review")
		}

		r.log.Log("REVIEW", "round %d/%d for %s", round, MaxReviewRounds, story.Key)

		result, beforeStatus, afterStatus, err := r.runStep(ctx, story.Key, reviewAction, round)
		invocations++
		if err != nil {
			r.log.SaveError("code-review", round, err.Error())
			if errors.Is(err, ErrAuthExpired) {
				return "", err
			}
			return "", err
		}

		// Save output, stream, and result
		r.log.SaveOutput("code-review", round, result.RawOutput)
		r.log.SaveStream("code-review", round, result.FullStream)
		summary := r.summarizeResult(ctx, reviewAction.Command, result.RawOutput)
		stepResult := NewStepResult(story.Key, "code-review", beforeStatus, afterStatus, summary, result.RawOutput)
		stepResult.Round = round
		r.log.SaveResult("code-review", round, stepResult)

		// Check if Claude updated the status itself
		currentStatus, _ := r.statusForStory(story.Key)
		if !ShouldContinueReview(currentStatus) {
			if round < MinReviewRounds {
				r.log.Log("REVIEW", "story %s is %s after round %d — forcing fresh-eyes pass (min %d rounds)", story.Key, currentStatus, round, MinReviewRounds)
			} else {
				r.log.Log("REVIEW", "story %s is %s after round %d — no judge needed", story.Key, currentStatus, round)
				return currentStatus, nil
			}
		}

		// Claude didn't set "done" — call judge ONLY NOW to decide
		r.log.Log("JUDGE", "Claude didn't resolve status, calling judge for round %d", round)
		verdict, judgeErr := r.callJudge(ctx, story.Key, "code-review", result.RawOutput)
		if judgeErr != nil {
			r.log.SaveError("judge", round, judgeErr.Error())
			if errors.Is(judgeErr, ErrAuthExpired) {
				return "", judgeErr
			}
			r.log.Log("JUDGE", "evaluation failed for review round %d: %v", round, judgeErr)
		} else {
			r.log.SaveVerdict(round, verdict)
			r.log.Log("JUDGE", "verdict: needs_more_work=%v recommended=%s summary=%s",
				verdict.NeedsMoreWork, verdict.RecommendedStatus, verdict.Summary)
		}

		// Judge says no more work needed — mark done, but only after the
		// fresh-eyes floor has been reached.
		if judgeErr == nil && !verdict.NeedsMoreWork {
			if round < MinReviewRounds {
				r.log.Log("JUDGE", "verdict clean at round %d — forcing fresh-eyes pass (min %d rounds)", round, MinReviewRounds)
			} else {
				r.log.Log("JUDGE", "no more work needed — marking %s as done", story.Key)
				if err := r.ensureStatus(ctx, story.Key, "done"); err != nil {
					return "", err
				}
				return "done", nil
			}
		}

		if round == MaxReviewRounds {
			r.log.Log("REVIEW", "max rounds (%d) reached for %s", MaxReviewRounds, story.Key)
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

// callJudge invokes the judge Claude to evaluate a worker result.
func (r *Runner) callJudge(ctx context.Context, storyKey, workflowKey, rawOutput string) (JudgeVerdict, error) {
	commandCtx := ctx
	cancel := func() {}
	if r.cfg.CommandTimeout > 0 {
		judgeTimeout := r.cfg.CommandTimeout / 3
		if judgeTimeout < 60*time.Second {
			judgeTimeout = 60 * time.Second
		}
		commandCtx, cancel = context.WithTimeout(ctx, judgeTimeout)
	}
	defer cancel()

	return Judge(commandCtx, r.cfg.Workdir, r.cfg.ClaudeCommand, r.cfg.ClaudeModel, r.cfg.ClaudeEffort, storyKey, workflowKey, rawOutput)
}

// ensureStatus updates sprint-status.yaml and commits with [autopilot] prefix.
func (r *Runner) ensureStatus(ctx context.Context, storyKey, newStatus string) error {
	if err := UpdateStoryStatus(r.cfg.StatusFile, storyKey, newStatus); err != nil {
		return fmt.Errorf("update status for %s: %w", storyKey, err)
	}
	if err := GitCommitStatusUpdate(ctx, r.cfg.Workdir, r.cfg.StatusFile, storyKey, newStatus); err != nil {
		return fmt.Errorf("commit status update for %s: %w", storyKey, err)
	}
	r.log.Log("STATUS", "%s → %s [autopilot]", storyKey, newStatus)
	return nil
}

// auditDeferred scans deferred-work.md for open items whose target story
// already reached a terminal state (done/validated). These are either
// reviewer oversights (forgot to close the item) or scope drift (the item
// actually belongs to a different story). Logged as warnings so a human can
// reconcile during the next sprint review — never fatal, since the ledger
// is advisory.
func (r *Runner) auditDeferred(sprintStatus SprintStatus) {
	orphans, err := ScanDeferredOrphans(r.cfg.Workdir, sprintStatus)
	if err != nil {
		r.log.Log("DEFERRED", "audit skipped: %v", err)
		return
	}
	if len(orphans) == 0 {
		return
	}
	r.log.Log("DEFERRED", "%d open item(s) whose target story is already %s — reconcile manually:",
		len(orphans), "done/validated")
	for _, o := range orphans {
		r.log.Log("DEFERRED", "  line %d → story %s (%s): %s",
			o.SourceLine, o.TargetStory, o.TargetStatus, o.ItemText)
	}
}

// blockStory marks a story as "blocked" and logs the reason.
func (r *Runner) blockStory(ctx context.Context, storyKey, reason string) (string, error) {
	r.log.Log("BLOCKED", "%s — %s", storyKey, reason)
	if err := r.ensureStatus(ctx, storyKey, "blocked"); err != nil {
		return "blocked", err
	}
	return "blocked", nil
}

func (r *Runner) runStep(ctx context.Context, storyKey string, action Action, round int) (ExecResult, string, string, error) {
	beforeStatus, err := r.statusForStory(storyKey)
	if err != nil {
		return ExecResult{}, "", "", err
	}
	r.log.Log("ACTION", "%s", action.WorkflowKey)
	r.log.LogRaw("  status(before): %s", beforeStatus)

	commandCtx := ctx
	cancel := func() {}
	if r.cfg.CommandTimeout > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, r.cfg.CommandTimeout)
	}
	defer cancel()

	start := time.Now()
	execResult, execErr := r.executor.Run(commandCtx, action)
	duration := time.Since(start)

	if !r.cfg.DisableCommandOutput && execResult.RawOutput != "" {
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
	r.log.LogRaw("  result: %s", oneLine(resultLine))
	r.log.LogRaw("  duration: %s", duration.Round(time.Second))

	afterStatus, err := r.statusForStory(storyKey)
	if err != nil {
		return execResult, beforeStatus, "", err
	}
	r.log.LogRaw("  status(after): %s", afterStatus)

	if execErr != nil {
		return execResult, beforeStatus, afterStatus, execErr
	}
	return execResult, beforeStatus, afterStatus, nil
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
		r.log.LogRaw("  output: <no output>")
		return
	}
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
