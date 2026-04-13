package cmd

import (
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
			})
			if err != nil {
				return err
			}
			return runner.Run(cmd.Context())
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
