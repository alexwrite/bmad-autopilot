package orchestrator

import "fmt"

// MaxReviewRounds is the safety limit for consecutive code-review iterations
// on a single story. Prevents infinite loops when reviews keep finding issues.
const MaxReviewRounds = 3

type Action struct {
	Prompt      string
	Command     string
	WorkflowKey string // maps to BMAD workflow in workflowRegistry
}

func PlanPrimaryActions(status, storyNumber string) ([]Action, error) {
	switch normalizeStatus(status) {
	case "backlog":
		return []Action{
			newAction(
				"create-story",
				fmt.Sprintf("Execute the create-story workflow for story %s in #yolo mode. Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions. Auto-complete all steps autonomously as an expert Scrum Master. When done, git add all changed files and commit with message 'chore(%s): create-story completed'.", storyNumber, storyNumber),
			),
			newAction(
				"dev-story",
				fmt.Sprintf("Execute the dev-story workflow for story %s in #yolo mode. Read the story file, implement ALL tasks and subtasks IN ORDER. Write tests for each task. Mark tasks [x] only when tests pass. Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions. When done, git add all changed files and commit with message 'chore(%s): dev-story completed'.", storyNumber, storyNumber),
			),
		}, nil
	case "ready-for-dev", "in-progress":
		return []Action{
			newAction(
				"dev-story",
				fmt.Sprintf("Execute the dev-story workflow for story %s in #yolo mode. Read the story file, implement ALL tasks and subtasks IN ORDER. Write tests for each task. Mark tasks [x] only when tests pass. Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions. When done, git add all changed files and commit with message 'chore(%s): dev-story completed'.", storyNumber, storyNumber),
			),
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
		fmt.Sprintf("Execute the code-review workflow for story %s in #yolo mode. Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions. Review all changed files, fix any findings. When done, git add all changed files and commit with message 'chore(%s): code-review completed', then push.", storyNumber, storyNumber),
	)
}

// ShouldContinueReview returns true if the review loop should keep running.
// Once the story status reaches "done", the loop stops unconditionally.
// The published flag is no longer used as exit criterion because the BMAD
// workflow commits the status YAML update after the push, leaving the repo
// 1 commit ahead of upstream — which made the old check loop forever.
func ShouldContinueReview(status string) bool {
	return normalizeStatus(status) != "done"
}

func newAction(workflowKey, prompt string) Action {
	return Action{
		Prompt:      prompt,
		Command:     fmt.Sprintf("claude -p [%s] --dangerously-skip-permissions --append-system-prompt [BMAD context]", workflowKey),
		WorkflowKey: workflowKey,
	}
}
