package cmd

import (
	"context"
	"strings"

	"github.com/alexwrite/bmad-autopilot/internal/orchestrator"

	"github.com/spf13/cobra"
)

func newRunCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run one-story-at-a-time manual loop",
		RunE: func(cmd *cobra.Command, _ []string) error {
			epicFilter, err := orchestrator.ParseEpicFilter(opts.epics)
			if err != nil {
				return err
			}

			// Two-stage Ctrl+C / SIGTERM: first signal stops gracefully at the
			// next step boundary, second aborts the in-flight command.
			runCtx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			stop := orchestrator.NewStopController()
			cleanup := installSignalStop(stop, cancel, cmd.ErrOrStderr())
			defer cleanup()

			runner, err := orchestrator.New(orchestrator.Config{
				StatusFile:           opts.statusFile,
				Brain:                opts.brain,
				Workdir:              opts.workdir,
				ClaudeModel:          opts.claudeModel,
				ClaudeCommand:        opts.claudeCommand,
				ClaudeEffort:         opts.claudeEffort,
				CommandTimeout:       opts.timeout,
				DisableCommandOutput: !opts.showCommandOutput,
				EpicFilter:           epicFilter,
				StoryFilter:          parseStoryFilter(opts.stories),
				StopChecker:          stop,
				SkillOverrides: orchestrator.SkillOverrides{
					CreateStory: opts.createStorySkill,
					DevStory:    opts.devStorySkill,
					CodeReview:  opts.codeReviewSkill,
				},
			})
			if err != nil {
				return err
			}
			return runner.Run(runCtx)
		},
	}
}

// parseStoryFilter splits a comma-separated story list like "2-1,2-3" into individual numbers.
func parseStoryFilter(spec string) []string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
