package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadBMADContextNoBmadDir(t *testing.T) {
	dir := t.TempDir()
	ctx, err := LoadBMADContext(dir, "dev-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx != nil {
		t.Fatal("expected nil context when _bmad/ does not exist")
	}
}

func TestLoadBMADContextUnknownKey(t *testing.T) {
	dir := setupTestBMAD(t, "6.3.0")
	_, err := LoadBMADContext(dir, "nonexistent-workflow")
	if err == nil {
		t.Fatal("expected error for unknown workflow key")
	}
}

func TestLoadBMADContextUnsupportedVersion(t *testing.T) {
	dir := setupTestBMAD(t, "6.0.4")
	_, err := LoadBMADContext(dir, "dev-story")
	if err == nil {
		t.Fatal("expected error for unsupported BMAD version")
	}
	if !strings.Contains(err.Error(), "unsupported BMAD version") {
		t.Fatalf("expected unsupported-version error, got: %v", err)
	}
}

func TestLoadBMADContextMissingSkill(t *testing.T) {
	dir := setupTestBMAD(t, "6.3.0")
	// Remove the dev-story skill dir to simulate a partial install.
	os.RemoveAll(filepath.Join(dir, ".claude", "skills", "bmad-dev-story"))

	_, err := LoadBMADContext(dir, "dev-story")
	if err == nil {
		t.Fatal("expected error when skill directory is missing")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected not-installed error, got: %v", err)
	}
}

func TestLoadBMADContextDevStory(t *testing.T) {
	dir := setupTestBMAD(t, "6.3.0")

	ctx, err := LoadBMADContext(dir, "dev-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.Version != "6.3.0" {
		t.Fatalf("expected Version 6.3.0, got %q", ctx.Version)
	}
	if ctx.SkillName != "bmad-dev-story" {
		t.Fatalf("expected SkillName bmad-dev-story, got %q", ctx.SkillName)
	}
	if !strings.Contains(ctx.ModuleCfg, "Alex") {
		t.Fatal("expected module config to contain user name")
	}
	if !strings.Contains(ctx.AgentDoc, "Amelia") {
		t.Fatal("expected dev agent persona (Amelia)")
	}
	if !strings.Contains(ctx.Workflow, "implement") {
		t.Fatal("expected workflow.md content")
	}
	if !strings.Contains(ctx.Checklist, "All tests pass") {
		t.Fatal("expected checklist content")
	}
}

func TestLoadBMADContextCreateStory(t *testing.T) {
	dir := setupTestBMAD(t, "6.3.0")

	ctx, err := LoadBMADContext(dir, "create-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if !strings.Contains(ctx.AgentDoc, "Scrum Master") {
		t.Fatal("expected SM agent persona for create-story")
	}
	if ctx.AgentName != "bmad-agent-sm" {
		t.Fatalf("expected agent name bmad-agent-sm, got %q", ctx.AgentName)
	}
}

func TestLoadBMADContextResolvesProjectRoot(t *testing.T) {
	dir := setupTestBMAD(t, "6.3.0")

	ctx, err := LoadBMADContext(dir, "dev-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(ctx.Workflow, "{project-root}") {
		t.Fatal("expected {project-root} to be resolved")
	}
	if !strings.Contains(ctx.Workflow, dir) {
		t.Fatalf("expected resolved path to contain workdir %q", dir)
	}
}

func TestSystemPromptContainsOverlay(t *testing.T) {
	ctx := &BMADContext{
		Version:   "6.3.0",
		SkillName: "bmad-dev-story",
		Workflow:  "step 1: implement",
	}
	prompt := ctx.SystemPrompt()
	if !strings.Contains(prompt, "autonomously") {
		t.Fatal("expected system prompt to enforce autonomous execution")
	}
	if !strings.Contains(prompt, "<commits>") {
		t.Fatal("expected system prompt to declare the commits overlay")
	}
	if !strings.Contains(prompt, "EXACTLY ONE commit per workflow step") {
		t.Fatal("expected one-commit-per-step rule")
	}
}

func TestSystemPromptContainsAllSections(t *testing.T) {
	ctx := &BMADContext{
		Version:   "6.3.0",
		SkillName: "bmad-dev-story",
		AgentName: "bmad-agent-dev",
		ModuleCfg: "config content",
		AgentDoc:  "agent content",
		SkillDoc:  "skill content",
		Workflow:  "instructions content",
		Checklist: "checklist content",
	}
	prompt := ctx.SystemPrompt()

	sections := []string{
		"BMAD MODULE CONFIG",
		"BMAD AGENT PERSONA (bmad-agent-dev)",
		"SKILL DECLARATION (bmad-dev-story/SKILL.md)",
		"WORKFLOW INSTRUCTIONS (workflow.md)",
		"VALIDATION CHECKLIST (checklist.md)",
	}
	for _, s := range sections {
		if !strings.Contains(prompt, s) {
			t.Fatalf("expected system prompt to contain section %q", s)
		}
	}
}

func TestSystemPromptSkipsEmptySections(t *testing.T) {
	ctx := &BMADContext{
		SkillName: "bmad-dev-story",
		Workflow:  "step 1: do something",
	}
	prompt := ctx.SystemPrompt()
	if strings.Contains(prompt, "BMAD MODULE CONFIG") {
		t.Fatal("expected empty module config section to be skipped")
	}
	if strings.Contains(prompt, "BMAD AGENT PERSONA") {
		t.Fatal("expected empty agent persona section to be skipped")
	}
	if !strings.Contains(prompt, "WORKFLOW INSTRUCTIONS") {
		t.Fatal("expected instructions section to be present")
	}
}

func TestHasContent(t *testing.T) {
	empty := &BMADContext{}
	if empty.HasContent() {
		t.Fatal("expected empty context to have no content")
	}

	withWorkflow := &BMADContext{Workflow: "step 1"}
	if !withWorkflow.HasContent() {
		t.Fatal("expected context with workflow to have content")
	}
}

func TestDetectBMADVersionParsesManifest(t *testing.T) {
	dir := t.TempDir()
	bmadRoot := filepath.Join(dir, "_bmad")
	cfgDir := filepath.Join(bmadRoot, "_config")
	os.MkdirAll(cfgDir, 0o755)

	manifest := `installation:
  version: 6.3.0
  installDate: 2026-04-15T00:00:00.000Z
modules:
  - name: core
    version: 6.3.0
`
	if err := os.WriteFile(filepath.Join(cfgDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := detectBMADVersion(bmadRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "6.3.0" {
		t.Fatalf("expected 6.3.0, got %q", got)
	}
}

func TestIsSupportedVersion(t *testing.T) {
	cases := map[string]bool{
		"":       true, // unknown → optimistic proceed
		"6.3.0":  true,
		"6.3.5":  true,
		"6.3":    true,
		"6.0.4":  false,
		"6.2.0":  false,
		"7.0.0":  false,
	}
	for v, want := range cases {
		if got := isSupportedVersion(v); got != want {
			t.Errorf("isSupportedVersion(%q) = %v, want %v", v, got, want)
		}
	}
}

// setupTestBMAD builds a minimal v6.3 BMAD layout:
//   - _bmad/_config/manifest.yaml (version)
//   - _bmad/bmm/config.yaml       (user-level settings)
//   - .claude/skills/bmad-<workflow>/{SKILL.md, workflow.md, checklist.md}
//   - .claude/skills/bmad-agent-<role>/SKILL.md
func setupTestBMAD(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()

	manifest := "installation:\n  version: " + version + "\nmodules:\n  - name: core\n"

	files := map[string]string{
		"_bmad/_config/manifest.yaml": manifest,
		"_bmad/bmm/config.yaml": `user_name: Alex
communication_language: Français
output_folder: "{project-root}/_bmad-output"
`,
		".claude/skills/bmad-agent-dev/SKILL.md": `---
name: bmad-agent-dev
description: Developer agent
---
# Amelia
Senior software engineer.
`,
		".claude/skills/bmad-agent-sm/SKILL.md": `---
name: bmad-agent-sm
description: Scrum Master
---
# Bob
Technical Scrum Master.
`,
		".claude/skills/bmad-agent-qa/SKILL.md": `---
name: bmad-agent-qa
description: QA agent
---
# Murat
QA test architect.
`,
		".claude/skills/bmad-dev-story/SKILL.md": `---
name: bmad-dev-story
description: Execute story implementation
---
Follow ./workflow.md.
`,
		".claude/skills/bmad-dev-story/workflow.md":  "Read the story at {project-root}/story.md and implement all tasks.\n",
		".claude/skills/bmad-dev-story/checklist.md": "- [ ] All tests pass\n",
		".claude/skills/bmad-create-story/SKILL.md": `---
name: bmad-create-story
description: Draft a story file
---
Follow ./workflow.md.
`,
		".claude/skills/bmad-create-story/workflow.md": "Create the story spec.\n",
		".claude/skills/bmad-code-review/SKILL.md": `---
name: bmad-code-review
description: Review a story implementation
---
Follow ./workflow.md.
`,
		".claude/skills/bmad-code-review/workflow.md": "Review the implementation.\n",
	}

	for relPath, content := range files {
		absPath := filepath.Join(dir, relPath)
		os.MkdirAll(filepath.Dir(absPath), 0o755)
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}
	return dir
}
