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

## Useful flags

- `--status-file <path>`
- `--brain <glm-5|deterministic>`
- `--workdir <path>`
- `--claude-model <model-id>` (e.g. `claude-opus-4-6`, `claude-sonnet-4-6`)
- `--claude-command <path>` (default: `claude`)
- `--show-command-output <true|false>` (default: `true`)
- `--timeout <duration>` (use `0` to disable timeout)
