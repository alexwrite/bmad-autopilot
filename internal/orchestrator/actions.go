package orchestrator

import "fmt"

// MinReviewRounds is the floor of consecutive code-review iterations on a
// single story. Even if the first pass comes back clean, a second fresh
// Claude instance re-audits the code with an empty context — real
// second-pair-of-eyes review instead of the same agent validating its own
// verdict.
const MinReviewRounds = 2

// MaxReviewRounds is the safety limit for consecutive code-review iterations
// on a single story. Prevents infinite loops when reviews keep finding issues.
const MaxReviewRounds = 3

// MaxInvocationsPerStory is the absolute ceiling of Claude calls for a single story
// across all phases (create + dev + review rounds + judge calls don't count).
const MaxInvocationsPerStory = 8

// MaxConsecutiveBlocked stops the autopilot after N stories are blocked in a row.
const MaxConsecutiveBlocked = 2

type Action struct {
	Prompt       string
	Command      string
	WorkflowKey  string // maps to BMAD workflow in workflowRegistry
	AllowedTools string // override default allowed tools (empty = use executor default)
}

// PlanPrimaryActions returns the sequence of BMAD skills to execute for a given
// sprint status. BMAD skills themselves drive status transitions — the autopilot
// only decides which skill to trigger based on the current status snapshot.
//
// Status lifecycle (entirely BMAD-managed):
//   backlog        → create-story transitions to ready-for-dev
//   ready-for-dev  → dev-story transitions to review
//   review         → code-review transitions to done (handled by loop, not here)
//   done / blocked → terminal, autopilot stops
func PlanPrimaryActions(status, storyNumber string) ([]Action, error) {
	switch normalizeStatus(status) {
	case "backlog":
		return []Action{
			createStoryAction(storyNumber),
			devStoryAction(storyNumber),
		}, nil
	case "ready-for-dev", "in-progress":
		return []Action{
			devStoryAction(storyNumber),
		}, nil
	case "review":
		return nil, nil
	case "done", "validated", "blocked":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported story status %q", status)
	}
}

// ShouldContinueReview returns true if the review loop should keep running.
// Stops when BMAD's code-review skill has moved the story to a terminal state.
func ShouldContinueReview(status string) bool {
	s := normalizeStatus(status)
	return s != "done" && s != "blocked" && s != "validated"
}

func ReviewAction(storyNumber string) Action {
	return newAction(
		"code-review",
		fmt.Sprintf(
			`Execute bmad-code-review for story %s. Follow the embedded workflow.

Review mindset: grey-hat security researcher. Focus OWASP Top 10
(injection, broken auth, IDOR, SSRF, crypto, data exposure,
misconfiguration). Every finding ships with a proof-of-concept:
the exact input, request, or scenario that triggers it. Ignore
cosmetic noise (naming, formatting) unless it causes a real bug.

Commit follows the "review(%s): <description>" convention at the
very end. Push.`,
			storyNumber, storyNumber,
		),
	)
}

func createStoryAction(storyNumber string) Action {
	return newAction(
		"create-story",
		fmt.Sprintf(
			`Execute bmad-create-story for story %s. Follow the embedded workflow.
Commits follow the "create(%s): <description>" convention. Push when done.`,
			storyNumber, storyNumber,
		),
	)
}

func devStoryAction(storyNumber string) Action {
	return newAction(
		"dev-story",
		fmt.Sprintf(
			`Execute bmad-dev-story for story %s. Follow the embedded workflow.
Commits follow the "dev(%s): <description>" convention. Push when done.`,
			storyNumber, storyNumber,
		),
	)
}

func newAction(workflowKey, prompt string) Action {
	return Action{
		Prompt:      prompt,
		Command:     fmt.Sprintf("claude -p [bmad-%s skill] --dangerously-skip-permissions --append-system-prompt [BMAD context]", workflowKey),
		WorkflowKey: workflowKey,
	}
}
