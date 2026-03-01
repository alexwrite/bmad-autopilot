package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type rootOptions struct {
	statusFile        string
	brain             string
	workdir           string
	copilotModel      string
	timeout           time.Duration
	showCommandOutput bool
}

// Execute runs the CLI entrypoint.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	opts := &rootOptions{
		statusFile:        "",
		brain:             "deterministic",
		workdir:           "",
		timeout:           0,
		showCommandOutput: true,
	}

	cmd := &cobra.Command{
		Use:   "bmad-autopilot",
		Short: "Manual loop runner for BMAD sprint stories",
	}

	cmd.PersistentFlags().StringVar(&opts.statusFile, "status-file", opts.statusFile, "Path to sprint-status.yaml (default: <cwd>/_bmad-output/implementation-artifacts/sprint-status.yaml)")
	cmd.PersistentFlags().StringVar(&opts.brain, "brain", opts.brain, "Overseer brain (default: deterministic; options: deterministic, glm-5)")
	cmd.PersistentFlags().StringVar(&opts.workdir, "workdir", opts.workdir, "Working directory for copilot/git operations (default: inferred from status file path)")
	cmd.PersistentFlags().StringVar(&opts.copilotModel, "copilot-model", opts.copilotModel, "Optional Copilot model override")
	cmd.PersistentFlags().DurationVar(&opts.timeout, "timeout", opts.timeout, "Per-command timeout (0 disables timeout)")
	cmd.PersistentFlags().BoolVar(&opts.showCommandOutput, "show-command-output", opts.showCommandOutput, "Print raw Copilot output for each command (default: true)")

	cmd.AddCommand(newRunCmd(opts))
	return cmd
}
