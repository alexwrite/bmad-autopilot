package orchestrator

import "fmt"

type Action struct {
	Prompt  string
	Command string
}

func PlanPrimaryActions(status, storyNumber string) ([]Action, error) {
	switch normalizeStatus(status) {
	case "backlog":
		return []Action{
			newAction(fmt.Sprintf("/bmad-bmm-create-story %s", storyNumber)),
			newAction(fmt.Sprintf("/bmad-bmm-dev-story %s", storyNumber)),
		}, nil
	case "ready-for-dev", "in-progress":
		return []Action{
			newAction(fmt.Sprintf("/bmad-bmm-dev-story %s", storyNumber)),
		}, nil
	case "review", "done":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported story status %q", status)
	}
}

func ReviewAction(storyNumber string) Action {
	return newAction(fmt.Sprintf(
		"/bmad-bmm-code-review %s yolo and fix findings if any, or don't if not. If none are found git commit & push, only if none are found.",
		storyNumber,
	))
}

func ShouldContinueReview(status string, published bool) bool {
	return normalizeStatus(status) != "done" || !published
}

func newAction(prompt string) Action {
	return Action{
		Prompt:  prompt,
		Command: fmt.Sprintf(`copilot --yolo --no-ask-user -s -p %q`, prompt),
	}
}
