package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SupportedBMADMajor is the BMAD major.minor this autopilot targets.
// Pinned to v6.8. Since v6.8 the autopilot delegates workflow execution to
// BMAD's native Claude Code skills — it no longer reads or parses skill
// internals — so this guard is the only remaining version coupling. A new
// BMAD line that changes the skill contract should bump this deliberately
// after a compatibility check, not silently drive an unknown format.
const SupportedBMADMajor = "6.8"

// workflowSpec maps an autopilot action to the BMAD skill it triggers.
// In v6.8 every BMAD workflow IS a native Claude Code skill living under
// .claude/skills/bmad-<skill>/. The autopilot names the skill in the prompt
// and lets Claude load and run it natively. It deliberately does NOT read the
// skill body, resolve its customization TOML, or interpolate its config —
// that is the skill's own job at runtime (it ships a resolver script and its
// own persona). Re-implementing that in Go would duplicate what Claude already
// does natively.
type workflowSpec struct {
	skill  string // skill directory name under .claude/skills/
	effort string // default Claude --effort level (low, medium, high, max)
}

// Workflow registry: every action the autopilot can fire maps to a v6.8 skill.
// Note: BMAD ships no "validate-story" skill — code-review is the final gate.
var workflowRegistry = map[string]workflowSpec{
	"create-story": {skill: "bmad-create-story", effort: "max"},
	"dev-story":    {skill: "bmad-dev-story", effort: "max"},
	"code-review":  {skill: "bmad-code-review", effort: "high"},
}

// DefaultEffort returns the default effort level for a workflow key.
// Returns empty string for unknown workflows.
func DefaultEffort(workflowKey string) string {
	if spec, ok := workflowRegistry[workflowKey]; ok {
		return spec.effort
	}
	return ""
}

// JudgeEffort is the default effort level for judge evaluations.
// Judges perform structured evaluation, not complex reasoning.
const JudgeEffort = "low"

// BMADContext is the minimal handle the autopilot needs to drive a v6.8
// native skill: the detected install version (for logging) and the skill
// name to invoke. The skill body is never read — Claude Code loads it
// natively from .claude/skills/ and resolves its own TOML and config.
type BMADContext struct {
	Version   string // detected BMAD installation version (e.g. "6.8.0")
	SkillName string // skill directory name to invoke (e.g. "bmad-dev-story")
}

// LoadBMADContext validates the BMAD install for a given workflow key.
// Returns nil (no error) if _bmad/ does not exist — the caller falls back to
// generic prompts. Returns an error if the installation is present but at an
// unsupported version, or if the backing skill is not installed, so callers
// can surface a clear message instead of driving a broken workflow.
func LoadBMADContext(workdir, workflowKey string) (*BMADContext, error) {
	bmadRoot := filepath.Join(workdir, "_bmad")

	if _, err := os.Stat(bmadRoot); os.IsNotExist(err) {
		return nil, nil
	}

	version, err := detectBMADVersion(bmadRoot)
	if err != nil {
		return nil, fmt.Errorf("detect BMAD version: %w", err)
	}
	if !isSupportedVersion(version) {
		return nil, fmt.Errorf(
			"unsupported BMAD version %q in %s — autopilot targets v%s.x; "+
				"upgrade with `npx bmad-method install` or pin a matching autopilot release",
			version, bmadRoot, SupportedBMADMajor,
		)
	}

	spec, ok := workflowRegistry[workflowKey]
	if !ok {
		return nil, fmt.Errorf("unknown BMAD workflow key %q", workflowKey)
	}

	skillDir := filepath.Join(workdir, ".claude", "skills", spec.skill)
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		return nil, fmt.Errorf(
			"skill %q not installed at %s — reinstall BMAD with the matching module",
			spec.skill, skillDir,
		)
	}

	return &BMADContext{Version: version, SkillName: spec.skill}, nil
}

// SystemPrompt returns the autopilot overlay injected via
// --append-system-prompt. It is intentionally minimal and version-agnostic:
// it only adds what BMAD's interactive-by-default skills cannot infer on their
// own (autonomy, one commit per step, security-first decisions). Everything
// else — the actual workflow — is delegated to the native BMAD skill the
// prompt names, which Claude loads and runs itself.
func (ctx *BMADContext) SystemPrompt() string {
	return autonomyOverlay
}

const autonomyOverlay = `<mode>
You are running a BMAD workflow autonomously. No human interaction is
possible: every HALT, ASK, or "waiting for your numbered choice" in the
skill must be resolved by making the right call at the right time, not by
defaulting to a fallback option.
</mode>

<decisions>
When the workflow asks for a choice or when you encounter a finding:
- Evaluate the real context (code, ACs, current status, related findings).
- Decide in this priority order: security → maintainability → conformity
  to project patterns.
- Every finding (CRITICAL / MAJOR / MINOR) is fixed properly, never
  worked around or deferred. Apply the best development, security
  (OWASP) and maintainability patterns.
</decisions>

<commits>
- EXACTLY ONE commit per workflow step, created at the very end once all
  work is complete and the story / files are up to date.
  (create-story → 1 commit, dev-story → 1 commit, code-review → 1 commit.)
- Mandatory format: <phase>(<story>): <concrete description>
  where <phase> is one of {create, dev, review}.
- Push immediately after that final commit.
</commits>

<story-file>
Fully document the story as the BMAD workflow mandates: Dev Agent Record
(decisions, completion notes), File List (every touched file, relative
paths), Change Log, Review Findings. A reviewer must be able to
reconstruct WHAT you did, HOW, and WHY from the story file alone.
</story-file>

<bmad>
For everything else — status transitions, File List, DoD, tests, findings,
checklist, test rules — follow the BMAD skill you are running strictly, to
the letter. Execute its embedded <workflow> step by step; run its
customization resolver and load its config exactly as the skill instructs.
</bmad>
`

// detectBMADVersion reads _bmad/_config/manifest.yaml and extracts
// installation.version. Returns "" if the manifest is missing or
// unreadable — callers treat that as "unknown version".
func detectBMADVersion(bmadRoot string) (string, error) {
	manifest, err := os.ReadFile(filepath.Join(bmadRoot, "_config", "manifest.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	// Cheap targeted parse: the manifest is YAML but we only need one
	// key near the top. Avoid pulling in a YAML dep just for this.
	//
	// installation:
	//   version: 6.8.0
	lines := strings.Split(string(manifest), "\n")
	inInstallationBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "installation:") {
			inInstallationBlock = true
			continue
		}
		if inInstallationBlock && strings.HasPrefix(trimmed, "version:") {
			v := strings.TrimSpace(strings.TrimPrefix(trimmed, "version:"))
			return strings.Trim(v, `"'`), nil
		}
		// End of installation block: unindented non-empty line that isn't inside it.
		if inInstallationBlock && len(line) > 0 && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "installation:") {
			break
		}
	}
	return "", nil
}

// isSupportedVersion returns true if the detected BMAD version is one the
// autopilot can drive. Today that's v6.8.x only — bump SupportedBMADMajor
// after checking a new line ships a compatible skill contract.
func isSupportedVersion(version string) bool {
	if version == "" {
		// Unknown version: let the caller proceed optimistically. We'd rather
		// try and fail loudly on a missing skill than block valid installs
		// that happen to ship without a manifest.
		return true
	}
	return strings.HasPrefix(version, SupportedBMADMajor+".") || version == SupportedBMADMajor
}
