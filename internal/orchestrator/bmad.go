package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SupportedBMADMajor is the BMAD major.minor this autopilot targets.
// Today we support v6.3 exclusively. To add a new major or minor, extend
// isSupportedVersion() and provide a dedicated loader if the skill layout
// ever diverges from the current "<project>/.claude/skills/bmad-*/" pattern.
const SupportedBMADMajor = "6.3"

// workflowSpec maps an action to the BMAD skill it should invoke.
// In v6.3, every BMAD workflow is backed by a Claude Code skill whose
// instructions live under .claude/skills/bmad-<skill>/. The autopilot
// injects those instructions verbatim so Claude executes the workflow
// without relying on its own skill auto-discovery.
type workflowSpec struct {
	skill      string // skill directory name under .claude/skills/
	agentSkill string // companion agent skill (persona) — empty if none
	effort     string // default Claude --effort level (low, medium, high, max)
}

// Workflow registry: every action the autopilot can fire maps to a v6.3 skill.
var workflowRegistry = map[string]workflowSpec{
	"create-story": {
		skill:      "bmad-create-story",
		agentSkill: "bmad-agent-sm",
		effort:     "max",
	},
	"dev-story": {
		skill:      "bmad-dev-story",
		agentSkill: "bmad-agent-dev",
		effort:     "max",
	},
	"code-review": {
		skill:      "bmad-code-review",
		agentSkill: "bmad-agent-dev",
		effort:     "high",
	},
	"validate-story": {
		skill:      "bmad-validate-story",
		agentSkill: "bmad-agent-qa",
		effort:     "max",
	},
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

// BMADContext carries the v6.3 skill material needed to drive a workflow.
type BMADContext struct {
	Version     string // detected BMAD installation version (e.g. "6.3.0")
	SkillName   string // fully qualified skill name (e.g. "bmad-dev-story")
	SkillDoc    string // SKILL.md frontmatter + pointer
	Workflow    string // workflow.md body (the real instructions)
	Checklist   string // checklist.md body, if present
	AgentName   string // fully qualified agent skill (e.g. "bmad-agent-dev")
	AgentDoc    string // agent SKILL.md body — the persona
	ModuleCfg   string // _bmad/bmm/config.yaml — user-level BMAD settings
}

// LoadBMADContext reads the BMAD skill files for a given workflow key.
// Returns nil (no error) if _bmad/ does not exist — caller falls back to
// generic prompts. Returns an error if the installation is present but at
// an unsupported version, so callers can surface a clear upgrade message
// instead of silently producing broken prompts.
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
				"upgrade with `npx bmad-method install` or pin an older autopilot release",
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

	moduleCfg := readFileContent(filepath.Join(bmadRoot, "bmm", "config.yaml"))
	skillDoc := readFileContent(filepath.Join(skillDir, "SKILL.md"))
	workflow := readFileContent(filepath.Join(skillDir, "workflow.md"))
	checklist := readFileContent(filepath.Join(skillDir, "checklist.md"))

	var agentName, agentDoc string
	if spec.agentSkill != "" {
		agentDir := filepath.Join(workdir, ".claude", "skills", spec.agentSkill)
		if doc := readFileContent(filepath.Join(agentDir, "SKILL.md")); doc != "" {
			agentName = spec.agentSkill
			agentDoc = doc
		}
	}

	// Replace {project-root} with the actual absolute path so Claude
	// can resolve file references during execution.
	replacer := strings.NewReplacer("{project-root}", workdir)

	return &BMADContext{
		Version:   version,
		SkillName: spec.skill,
		SkillDoc:  replacer.Replace(skillDoc),
		Workflow:  replacer.Replace(workflow),
		Checklist: replacer.Replace(checklist),
		AgentName: agentName,
		AgentDoc:  replacer.Replace(agentDoc),
		ModuleCfg: replacer.Replace(moduleCfg),
	}, nil
}

// SystemPrompt builds the full system prompt to inject via --append-system-prompt.
func (ctx *BMADContext) SystemPrompt() string {
	var sb strings.Builder

	writeSection := func(title, content string) {
		if strings.TrimSpace(content) == "" {
			return
		}
		sb.WriteString("\n=== ")
		sb.WriteString(title)
		sb.WriteString(" ===\n")
		sb.WriteString(content)
		sb.WriteString("\n")
	}

	sb.WriteString("You are executing a BMAD workflow in AUTONOMOUS #yolo mode.")
	if ctx.Version != "" {
		sb.WriteString(fmt.Sprintf("\nBMAD installation: v%s. Skill: %s.", ctx.Version, ctx.SkillName))
	}
	sb.WriteString("\nCRITICAL RULES:")
	sb.WriteString("\n- Do NOT wait for user input at any step")
	sb.WriteString("\n- Do NOT display menus or ask questions")
	sb.WriteString("\n- Auto-complete ALL steps as a simulated expert user")
	sb.WriteString("\n- Follow the workflow instructions below IN EXACT ORDER")
	sb.WriteString("\n- Save outputs after EACH section")
	sb.WriteString("\n- You have the full skill context below — use it")
	sb.WriteString("\n\nTEST EXECUTION POLICY (GLOBAL — overrides everything):")
	sb.WriteString("\n- NEVER run the full test suite. Running 'php bin/phpunit' without explicit file paths is FORBIDDEN.")
	sb.WriteString("\n- NEVER use 'composer test' (Composer 300s timeout + runs full suite).")
	sb.WriteString("\n- ALWAYS specify test file paths: 'php bin/phpunit tests/Unit/A.php tests/Functional/B.php'")
	sb.WriteString("\n- Before testing: list modified files → identify their tests + dependent tests → run ONLY those.")
	sb.WriteString("\n- Pre-existing test failures are NOT your problem. Do NOT investigate or mention them.")
	sb.WriteString("\n\nBROWSER TESTING POLICY (for validate-story workflows):")
	sb.WriteString("\n- When browser testing is required, use MCP Chrome DevTools tools (mcp__chrome-devtools__*).")
	sb.WriteString("\n- You ARE the browser tester. Navigate pages, click elements, fill forms, take screenshots.")
	sb.WriteString("\n- Test responsive (resize to mobile/tablet/desktop), dark mode (toggle data-theme), i18n (switch locale).")
	sb.WriteString("\n- Record PASS/FAIL for each Acceptance Criterion based on your browser observations.\n")

	writeSection("BMAD MODULE CONFIG", ctx.ModuleCfg)
	if ctx.AgentDoc != "" {
		writeSection(fmt.Sprintf("BMAD AGENT PERSONA (%s)", ctx.AgentName), ctx.AgentDoc)
	}
	writeSection(fmt.Sprintf("SKILL DECLARATION (%s/SKILL.md)", ctx.SkillName), ctx.SkillDoc)
	writeSection("WORKFLOW INSTRUCTIONS (workflow.md)", ctx.Workflow)
	writeSection("VALIDATION CHECKLIST (checklist.md)", ctx.Checklist)

	return sb.String()
}

// HasContent returns true if the context has at least workflow instructions to execute.
func (ctx *BMADContext) HasContent() bool {
	return strings.TrimSpace(ctx.Workflow) != ""
}

func readFileContent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

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
	//   version: 6.3.0
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
// autopilot can drive. Today that's v6.3.x only — extend here when a new
// supported line is added.
func isSupportedVersion(version string) bool {
	if version == "" {
		// Unknown version: let the caller proceed optimistically. We'd rather
		// try and fail loudly on a missing skill than block valid installs
		// that happen to ship without a manifest.
		return true
	}
	return strings.HasPrefix(version, SupportedBMADMajor+".") || version == SupportedBMADMajor
}
