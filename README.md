# bmad-autopilot

Manual loop runner for BMAD sprint stories, implemented as a Go Cobra CLI.
Fork adapted for **Claude Code CLI** (original used GitHub Copilot SDK).

## Run

```bash
go install github.com/alexwrite/bmad-autopilot@latest
# cd to your project root
bmad-autopilot run
```

Defaults:

- Status file: inferred from current working directory as `<cwd>/_bmad-output/implementation-artifacts/sprint-status.yaml`
- Brain: `deterministic`
- Workdir: inferred from the status file path (the project directory before `_bmad-output/`)
- Timeout: disabled
- Claude model: unset (uses CLI default)
- Claude execution: fresh subprocess per command, with `--dangerously-skip-permissions --output-format json`
- BMAD context: full workflow chain injected via `--append-system-prompt` in #yolo mode
- Logging: each action prints the raw Claude output block plus a one-line summarized `RESULT` (enabled by default)

## Epic filtering

By default, the autopilot processes **all** stories in order. Use `--epics` to target specific epics:

```bash
# Finish epic 8 only
bmad-autopilot run --epics 8

# Process epics 15 through 21
bmad-autopilot run --epics 15-21

# Mix single epics and ranges
bmad-autopilot run --epics 8,15-21
```

The autopilot stops once all stories in the selected epics are done.

## Useful flags

- `--status-file <path>`
- `--brain <glm-5|deterministic>`
- `--workdir <path>`
- `--epics <spec>` — epic filter (e.g. `8`, `15-21`, `8,15-21`)
- `--claude-model <model-id>` (e.g. `claude-opus-4-6`, `claude-sonnet-4-6`)
- `--claude-command <path>` (default: `claude`)
- `--show-command-output <true|false>` (default: `true`)
- `--timeout <duration>` (use `0` to disable timeout)
