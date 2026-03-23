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
	claudeModel       string
	claudeCommand     string
	claudeEffort      string
	timeout           time.Duration
	showCommandOutput bool
	epics             string
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
		claudeCommand:     "claude",
		timeout:           0,
		showCommandOutput: true,
	}

	cmd := &cobra.Command{
		Use:   "bmad-autopilot",
		Short: "Manual loop runner for BMAD sprint stories",
	}

	cmd.PersistentFlags().StringVar(&opts.statusFile, "status-file", opts.statusFile, "Path to sprint-status.yaml (default: <cwd>/_bmad-output/implementation-artifacts/sprint-status.yaml)")
	cmd.PersistentFlags().StringVar(&opts.brain, "brain", opts.brain, "Overseer brain (default: deterministic; options: deterministic, glm-5)")
	cmd.PersistentFlags().StringVar(&opts.workdir, "workdir", opts.workdir, "Working directory for claude/git operations (default: inferred from status file path)")
	cmd.PersistentFlags().StringVar(&opts.claudeModel, "claude-model", opts.claudeModel, "Optional Claude model override (e.g. claude-opus-4-6, claude-sonnet-4-6)")
	cmd.PersistentFlags().StringVar(&opts.claudeCommand, "claude-command", opts.claudeCommand, "Path to the claude CLI binary (default: claude)")
	cmd.PersistentFlags().DurationVar(&opts.timeout, "timeout", opts.timeout, "Per-command timeout (0 disables timeout)")
	cmd.PersistentFlags().BoolVar(&opts.showCommandOutput, "show-command-output", opts.showCommandOutput, "Print raw Claude output for each command (default: true)")
	cmd.PersistentFlags().StringVar(&opts.claudeEffort, "effort", "", `Global effort level override for Claude (low, medium, high, max). Per-workflow defaults: create-story=max, dev-story=max, code-review=high, judge=low`)
	cmd.PersistentFlags().StringVar(&opts.epics, "epics", "", `Epic filter: only process stories from these epics (e.g. "8", "15-21", "8,15-21")`)

	cmd.AddCommand(newRunCmd(opts))
	return cmd
}
