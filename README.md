# bmad-autopilot

Manual loop runner for BMAD sprint stories, implemented as a Go Cobra CLI.

## Run

```bash
go run . run
```

Defaults:

- Status file: `_bmad-output/implementation-artifacts/sprint-status.yaml`
- Brain: `glm-5` (via `github.com/deicod/zai`; falls back to deterministic summaries when `ZAI_API_KEY` is not set)
- Copilot execution: fresh SDK client/session per command, with `--yolo --no-ask-user -s`

## Useful flags

- `--status-file <path>`
- `--brain <glm-5|deterministic>`
- `--workdir <path>`
- `--copilot-model <model-id>`
- `--timeout <duration>`
