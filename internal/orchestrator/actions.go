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
			newAction(fmt.Sprintf("Run the BMAD create-story workflow for story %s. Read the sprint plan and epics, then create a complete story file with acceptance criteria, technical context, and implementation tasks.", storyNumber)),
			newAction(fmt.Sprintf("Run the BMAD dev-story workflow for story %s. Read the story file, implement all tasks following the acceptance criteria, write tests, and ensure the code compiles.", storyNumber)),
		}, nil
	case "ready-for-dev", "in-progress":
		return []Action{
			newAction(fmt.Sprintf("Run the BMAD dev-story workflow for story %s. Read the story file, implement all tasks following the acceptance criteria, write tests, and ensure the code compiles.", storyNumber)),
		}, nil
	case "review", "done":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported story status %q", status)
	}
}

func ReviewAction(storyNumber string) Action {
	return newAction(fmt.Sprintf(
		"Run a code review for story %s. Review all changed files for bugs, security issues, and code quality. Fix any findings. If no issues are found, git commit and push.",
		storyNumber,
	))
}

func ShouldContinueReview(status string, published bool) bool {
	return normalizeStatus(status) != "done" || !published
}

func newAction(prompt string) Action {
	return Action{
		Prompt:  prompt,
		Command: fmt.Sprintf(`claude -p %q --dangerously-skip-permissions`, prompt),
	}
}
