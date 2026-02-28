# bmad-autopilot

Manual loop runner for BMAD sprint stories, implemented as a Go Cobra CLI.

## Run

```bash
go run . run
```

Defaults:

- Status file: inferred from current working directory as `<cwd>/_bmad-output/implementation-artifacts/sprint-status.yaml`
- Brain: `deterministic`
- Workdir: inferred from the status file path (the project directory before `_bmad-output/`)
- Timeout: disabled
- Copilot model: unset
- Copilot execution: fresh SDK client/session per command, with `--yolo --no-ask-user -s`

## Useful flags

- `--status-file <path>`
- `--brain <glm-5|deterministic>`
- `--workdir <path>`
- `--copilot-model <model-id>`
- `--timeout <duration>` (use `0` to disable timeout)
