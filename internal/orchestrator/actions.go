package orchestrator

import (
	"fmt"
	"strings"
)

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
	WorkflowKey  string // maps to BMAD workflow in workflowRegistry (effort, logging)
	SkillName    string // resolved skill actually invoked (default or overridden), no leading slash
	AllowedTools string // override default allowed tools (empty = use executor default)
}

// SkillOverrides lets an operator point a phase at a different or custom skill
// than the bmad-* default (a renamed skill, a fork, a project-specific
// variant). Empty fields fall back to the registry default. A leading slash
// is accepted and stripped, so "/bmad-dev-story" and "bmad-dev-story" are
// equivalent.
type SkillOverrides struct {
	CreateStory string
	DevStory    string
	CodeReview  string
}

// resolve returns the skill directory name to invoke for a workflow key:
// the override when set, otherwise the registry default.
func (o SkillOverrides) resolve(workflowKey string) string {
	var override string
	switch workflowKey {
	case "create-story":
		override = o.CreateStory
	case "dev-story":
		override = o.DevStory
	case "code-review":
		override = o.CodeReview
	}
	if normalized := normalizeSkillName(override); normalized != "" {
		return normalized
	}
	if spec, ok := workflowRegistry[workflowKey]; ok {
		return spec.skill
	}
	return ""
}

// normalizeSkillName trims spaces and a single leading slash so the skill name
// matches its directory under .claude/skills/.
func normalizeSkillName(name string) string {
	return strings.TrimPrefix(strings.TrimSpace(name), "/")
}

// PlanPrimaryActions returns the sequence of BMAD skills to execute for a given
// sprint status. BMAD skills themselves drive status transitions — the autopilot
// only decides which skill to trigger based on the current status snapshot.
//
// Status lifecycle (entirely BMAD-managed):
//
//	backlog        → create-story transitions to ready-for-dev
//	ready-for-dev  → dev-story transitions to review
//	review         → code-review transitions to done (handled by loop, not here)
//	done / blocked → terminal, autopilot stops
func PlanPrimaryActions(status, storyNumber string, skills SkillOverrides) ([]Action, error) {
	switch normalizeStatus(status) {
	case "backlog":
		return []Action{
			createStoryAction(storyNumber, skills),
			devStoryAction(storyNumber, skills),
		}, nil
	case "ready-for-dev", "in-progress":
		return []Action{
			devStoryAction(storyNumber, skills),
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

func ReviewAction(storyNumber string, skills SkillOverrides) Action {
	skill := skills.resolve("code-review")
	return newAction(
		"code-review",
		skill,
		fmt.Sprintf(
			`Run the /%s skill for story %s. Load it from
.claude/skills/ and execute its embedded <workflow> step by step.

Review mindset: grey-hat security researcher. Focus OWASP Top 10
(injection, broken auth, IDOR, SSRF, crypto, data exposure,
misconfiguration). Every finding ships with a proof-of-concept:
the exact input, request, or scenario that triggers it. Ignore
cosmetic noise (naming, formatting) unless it causes a real bug.

Commit follows the "review(%s): <description>" convention at the
very end. Push.`,
			skill, storyNumber, storyNumber,
		),
	)
}

func createStoryAction(storyNumber string, skills SkillOverrides) Action {
	skill := skills.resolve("create-story")
	return newAction(
		"create-story",
		skill,
		fmt.Sprintf(
			`Run the /%s skill for story %s. Load it from
.claude/skills/ and execute its embedded <workflow> step by step.
Commits follow the "create(%s): <description>" convention. Push when done.`,
			skill, storyNumber, storyNumber,
		),
	)
}

func devStoryAction(storyNumber string, skills SkillOverrides) Action {
	skill := skills.resolve("dev-story")
	return newAction(
		"dev-story",
		skill,
		fmt.Sprintf(
			`Run the /%s skill for story %s. Load it from
.claude/skills/ and execute its embedded <workflow> step by step.
Commits follow the "dev(%s): <description>" convention. Push when done.`,
			skill, storyNumber, storyNumber,
		),
	)
}

func newAction(workflowKey, skill, prompt string) Action {
	return Action{
		Prompt:      prompt,
		Command:     fmt.Sprintf("claude -p [/%s skill] --dangerously-skip-permissions --append-system-prompt [autonomy overlay]", skill),
		WorkflowKey: workflowKey,
		SkillName:   skill,
	}
}
