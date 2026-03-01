package orchestrator

import "fmt"

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
				fmt.Sprintf("Execute the create-story workflow for story %s in #yolo mode. Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions. Auto-complete all steps autonomously as an expert Scrum Master.", storyNumber),
			),
			newAction(
				"dev-story",
				fmt.Sprintf("Execute the dev-story workflow for story %s in #yolo mode. Read the story file, implement ALL tasks and subtasks IN ORDER. Write tests for each task. Mark tasks [x] only when tests pass. Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions.", storyNumber),
			),
		}, nil
	case "ready-for-dev", "in-progress":
		return []Action{
			newAction(
				"dev-story",
				fmt.Sprintf("Execute the dev-story workflow for story %s in #yolo mode. Read the story file, implement ALL tasks and subtasks IN ORDER. Write tests for each task. Mark tasks [x] only when tests pass. Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions.", storyNumber),
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
		fmt.Sprintf("Execute the code-review workflow for story %s in #yolo mode. Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions. Review all changed files, fix any findings. If no issues are found, git commit and push.", storyNumber),
	)
}

func ShouldContinueReview(status string, published bool) bool {
	return normalizeStatus(status) != "done" || !published
}

func newAction(workflowKey, prompt string) Action {
	return Action{
		Prompt:      prompt,
		Command:     fmt.Sprintf("claude -p [%s] --dangerously-skip-permissions --append-system-prompt [BMAD context]", workflowKey),
		WorkflowKey: workflowKey,
	}
}
