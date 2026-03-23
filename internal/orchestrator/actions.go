package orchestrator

import "fmt"

// MaxReviewRounds is the safety limit for consecutive code-review iterations
// on a single story. Prevents infinite loops when reviews keep finding issues.
const MaxReviewRounds = 3

// MaxInvocationsPerStory is the absolute ceiling of Claude calls for a single story
// across all phases (create + dev + review rounds + judge calls don't count).
const MaxInvocationsPerStory = 8

// MaxConsecutiveBlocked stops the autopilot after N stories are blocked in a row.
const MaxConsecutiveBlocked = 2

type Action struct {
	Prompt      string
	Command     string
	WorkflowKey string // maps to BMAD workflow in workflowRegistry
}

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
	case "review", "done":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported story status %q", status)
	}
}

func ReviewAction(storyNumber string) Action {
	return newAction(
		"code-review",
		fmt.Sprintf(`Execute the code-review workflow for story %s in #yolo mode.
Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions.
Review all changed files and fix any findings.

COMMIT RULES:
- ALL commit messages MUST start with "review(%s): " followed by a description.
- If you made fixes, git add and commit with a DESCRIPTIVE message.
  Example: "review(%s): fix contrast ratio for primary-dark token"
  Example: "review(%s): extract menu card into reusable component"
- Do NOT use generic messages like "code-review completed".
- Describe WHAT you actually changed, not that you reviewed.
- If no changes were needed, do NOT create an empty commit.

STATUS UPDATE:
- After review, update sprint-status.yaml for this story:
  - If no issues found or all fixes applied: set status to "done"
  - If you found issues but could not fix them: set status to "blocked"
- Commit the status update separately: "review(%s): update status to [new-status]"
- Then push all commits.`, storyNumber, storyNumber, storyNumber, storyNumber, storyNumber),
	)
}

// ShouldContinueReview returns true if the review loop should keep running.
// Stops when status reaches "done" or "blocked".
func ShouldContinueReview(status string) bool {
	s := normalizeStatus(status)
	return s != "done" && s != "blocked"
}

func createStoryAction(storyNumber string) Action {
	return newAction(
		"create-story",
		fmt.Sprintf(`Execute the create-story workflow for story %s in #yolo mode.
Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions.
Auto-complete all steps autonomously as an expert Scrum Master.

TEST-FIRST MANDATE:
- Design the story with TESTABILITY as the primary constraint.
- The "Test Strategy" section MUST be written BEFORE the "Tasks / Subtasks" section.
- Each acceptance criterion must have a corresponding test scenario (unit or functional).
- Tasks must be ordered: test infrastructure first, then implementation, then integration tests.
- If a task cannot be tested in isolation, split it until it can.
- Specify exactly WHAT to test and HOW (test class, method pattern, assertions).

COMMIT RULES:
- ALL commit messages MUST start with "create(%s): " followed by a description.
  Example: "create(%s): define story spec with 6 acceptance criteria and test-first task breakdown"
- Do NOT use generic messages like "create-story completed".
- Describe what the story spec contains.

STATUS UPDATE:
- Update sprint-status.yaml: set this story's status to "ready-for-dev"
- Commit the status update separately: "create(%s): update status to ready-for-dev"`, storyNumber, storyNumber, storyNumber, storyNumber),
	)
}

func devStoryAction(storyNumber string) Action {
	return newAction(
		"dev-story",
		fmt.Sprintf(`Execute the dev-story workflow for story %s in #yolo mode.
Read the story file, implement ALL tasks and subtasks IN ORDER.
Write tests for each task. Mark tasks [x] only when tests pass.
Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions.

TEST EXECUTION RULES:
- ONLY run YOUR tests — the tests you wrote or modified for this story.
- Use targeted test commands (single file or single method), NEVER the full test suite.
- Assume the existing test suite is green before you started. Your job is to keep it green.
- Run the full suite ONCE at the very end, just before setting status to "review", as a final sanity check.
- If a pre-existing test breaks, fix the regression — but do NOT re-run the full suite after every small change.

COMMIT RULES:
- ALL commit messages MUST start with "dev(%s): " followed by a description.
- Commit after each logical unit of work (not one giant commit at the end).
  Example: "dev(%s): implement BaseLayout with header, main and footer landmarks"
  Example: "dev(%s): add responsive navigation with glassmorphism effect"
- Do NOT use generic messages like "dev-story completed".
- Describe WHAT you implemented.

STATUS UPDATE:
- When all tasks are done, update sprint-status.yaml: set this story's status to "review"
- Commit the status update separately: "dev(%s): update status to review"`, storyNumber, storyNumber, storyNumber, storyNumber, storyNumber),
	)
}

func newAction(workflowKey, prompt string) Action {
	return Action{
		Prompt:      prompt,
		Command:     fmt.Sprintf("claude -p [%s] --dangerously-skip-permissions --append-system-prompt [BMAD context]", workflowKey),
		WorkflowKey: workflowKey,
	}
}
