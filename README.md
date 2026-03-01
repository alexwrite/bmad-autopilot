# bmad-autopilot

Manual loop runner for BMAD sprint stories, implemented as a Go Cobra CLI.

## Run

```bash
go install github.com/dlukt/bmad-autopilot@latest
# cd to your project root
bmad-autopilot
```

Defaults:

- Status file: inferred from current working directory as `<cwd>/_bmad-output/implementation-artifacts/sprint-status.yaml`
- Brain: `deterministic`
- Workdir: inferred from the status file path (the project directory before `_bmad-output/`)
- Timeout: disabled
- Copilot model: unset
- Copilot execution: fresh SDK client/session per command, with `--yolo --no-ask-user -s`
- Logging: each action prints the raw Copilot output block plus a one-line summarized `RESULT` (enabled by default)

## Useful flags

- `--status-file <path>`
- `--brain <glm-5|deterministic>`
- `--workdir <path>`
- `--copilot-model <model-id>`
- `--show-command-output <true|false>` (default: `true`)
- `--timeout <duration>` (use `0` to disable timeout)
