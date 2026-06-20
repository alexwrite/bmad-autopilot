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
- BMAD context: the workflow itself runs as a native Claude Code skill (`.claude/skills/bmad-*/`); the autopilot only injects a small autonomy overlay via `--append-system-prompt` in #yolo mode
- Logging: each action prints the raw Claude output block plus a one-line summarized `RESULT` (enabled by default)

## Stopping a run

The autopilot is built to run unattended, so stopping is two-stage and never
leaves a half-finished step:

- **First `Ctrl+C` / `SIGTERM`**: graceful stop. The current `claude` step is
  allowed to finish (commit + push intact), then the loop exits cleanly at the
  next step boundary.
- **Second `Ctrl+C` / `SIGTERM`**: hard abort. Cancels the in-flight command
  immediately.

This works headless (no TTY required): send the signal with `kill`, a process
manager, or `docker stop`.

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

## BMAD version compatibility

Targets **BMAD v6.8.x** exclusively. Since v6.8, BMAD workflows are native
Claude Code skills: the whole workflow lives inside each
`.claude/skills/bmad-*/SKILL.md`, and the skill resolves its own
customization (TOML) and config at runtime. The autopilot therefore
**delegates** execution — it names the skill (e.g. `/bmad-dev-story`) and lets
Claude load and run it natively, instead of reading and injecting the skill
body itself. It only adds a small autonomy overlay (one commit per step,
resolve HALT/ASK autonomously, security-first decisions).

It detects the install version via `_bmad/_config/manifest.yaml` and aborts
with a clear error on any non-6.8 install rather than driving an unknown
skill contract. To support a future BMAD line, verify the skill contract is
compatible, then bump `SupportedBMADMajor` in
`internal/orchestrator/bmad.go`.

## Useful flags

- `--status-file <path>`
- `--brain <glm-5|deterministic>`
- `--workdir <path>`
- `--epics <spec>` — epic filter (e.g. `8`, `15-21`, `8,15-21`)
- `--claude-model <model-id>` (e.g. `claude-opus-4-6`, `claude-sonnet-4-6`)
- `--claude-command <path>` (default: `claude`)
- `--show-command-output <true|false>` (default: `true`)
- `--timeout <duration>` (use `0` to disable timeout)
- `--create-story-skill <name>` — override the create-story skill (default: `bmad-create-story`)
- `--dev-story-skill <name>` — override the dev-story skill (default: `bmad-dev-story`)
- `--code-review-skill <name>` — override the code-review skill (default: `bmad-code-review`)

The `--*-skill` overrides point a phase at a renamed, forked, or
project-specific skill. A leading slash is optional (`/bmad-dev-story` ==
`bmad-dev-story`). The overridden skill must be installed under
`.claude/skills/<name>/`, or the run aborts with a clear error.
