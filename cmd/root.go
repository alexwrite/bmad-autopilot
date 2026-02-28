package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type rootOptions struct {
	statusFile   string
	brain        string
	workdir      string
	copilotModel string
	timeout      time.Duration
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
		statusFile: "_bmad-output/implementation-artifacts/sprint-status.yaml",
		brain:      "glm-5",
		workdir:    ".",
		timeout:    2 * time.Hour,
	}

	cmd := &cobra.Command{
		Use:   "bmad-autopilot",
		Short: "Manual loop runner for BMAD sprint stories",
	}

	cmd.PersistentFlags().StringVar(&opts.statusFile, "status-file", opts.statusFile, "Path to sprint-status.yaml")
	cmd.PersistentFlags().StringVar(&opts.brain, "brain", opts.brain, "Overseer brain (glm-5, deterministic)")
	cmd.PersistentFlags().StringVar(&opts.workdir, "workdir", opts.workdir, "Working directory for copilot/git operations")
	cmd.PersistentFlags().StringVar(&opts.copilotModel, "copilot-model", opts.copilotModel, "Optional Copilot model override")
	cmd.PersistentFlags().DurationVar(&opts.timeout, "timeout", opts.timeout, "Per-command timeout")

	cmd.AddCommand(newRunCmd(opts))
	return cmd
}
