package cmd

import (
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
			})
			if err != nil {
				return err
			}
			return runner.Run(cmd.Context())
		},
	}
}
