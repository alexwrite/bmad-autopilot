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
	dir := setupTestBMAD(t, "6.8.0")
	_, err := LoadBMADContext(dir, "nonexistent-workflow")
	if err == nil {
		t.Fatal("expected error for unknown workflow key")
	}
}

func TestLoadBMADContextUnsupportedVersion(t *testing.T) {
	// v6.3 was supported by older autopilot releases; pinned to 6.8 it is
	// now rejected with a clear upgrade message instead of being driven blind.
	dir := setupTestBMAD(t, "6.3.0")
	_, err := LoadBMADContext(dir, "dev-story")
	if err == nil {
		t.Fatal("expected error for unsupported BMAD version")
	}
	if !strings.Contains(err.Error(), "unsupported BMAD version") {
		t.Fatalf("expected unsupported-version error, got: %v", err)
	}
}

func TestLoadBMADContextMissingSkill(t *testing.T) {
	dir := setupTestBMAD(t, "6.8.0")
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
	dir := setupTestBMAD(t, "6.8.0")

	ctx, err := LoadBMADContext(dir, "dev-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.Version != "6.8.0" {
		t.Fatalf("expected Version 6.8.0, got %q", ctx.Version)
	}
	if ctx.SkillName != "bmad-dev-story" {
		t.Fatalf("expected SkillName bmad-dev-story, got %q", ctx.SkillName)
	}
}

func TestLoadBMADContextCreateStory(t *testing.T) {
	dir := setupTestBMAD(t, "6.8.0")

	ctx, err := LoadBMADContext(dir, "create-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.SkillName != "bmad-create-story" {
		t.Fatalf("expected SkillName bmad-create-story, got %q", ctx.SkillName)
	}
}

func TestSystemPromptIsAutonomyOverlayOnly(t *testing.T) {
	ctx := &BMADContext{Version: "6.8.0", SkillName: "bmad-dev-story"}
	prompt := ctx.SystemPrompt()

	// The overlay enforces autonomous, one-commit-per-step execution.
	for _, want := range []string{"autonomously", "<commits>", "EXACTLY ONE commit per workflow step"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected overlay to contain %q", want)
		}
	}

	// It must NOT carry skill internals — those are loaded natively by Claude.
	// The overlay delegates to "the BMAD skill you are running", never inlines
	// a workflow body or persona.
	for _, unwanted := range []string{"WORKFLOW INSTRUCTIONS", "BMAD AGENT PERSONA", "SKILL DECLARATION"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("overlay must not inline skill section %q (delegated to native skill)", unwanted)
		}
	}
}

func TestDetectBMADVersionParsesManifest(t *testing.T) {
	dir := t.TempDir()
	bmadRoot := filepath.Join(dir, "_bmad")
	cfgDir := filepath.Join(bmadRoot, "_config")
	os.MkdirAll(cfgDir, 0o755)

	manifest := `installation:
  version: 6.8.0
  installDate: 2026-06-20T00:00:00.000Z
modules:
  - name: core
    version: 6.8.0
`
	if err := os.WriteFile(filepath.Join(cfgDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := detectBMADVersion(bmadRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "6.8.0" {
		t.Fatalf("expected 6.8.0, got %q", got)
	}
}

func TestIsSupportedVersion(t *testing.T) {
	cases := map[string]bool{
		"":      true, // unknown → optimistic proceed
		"6.8.0": true,
		"6.8.5": true,
		"6.8":   true,
		"6.3.0": false,
		"6.7.0": false,
		"7.0.0": false,
	}
	for v, want := range cases {
		if got := isSupportedVersion(v); got != want {
			t.Errorf("isSupportedVersion(%q) = %v, want %v", v, got, want)
		}
	}
}

// setupTestBMAD builds a minimal v6.8 BMAD layout sufficient for the
// autopilot's validation: a manifest carrying the version, plus each backing
// skill directory with a SKILL.md. The autopilot no longer reads skill
// bodies, customization TOML, config, or agent personas — Claude loads those
// natively — so the fixture only needs what existence checks look at.
func setupTestBMAD(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"_bmad/_config/manifest.yaml":               "installation:\n  version: " + version + "\nmodules:\n  - name: core\n",
		".claude/skills/bmad-dev-story/SKILL.md":    skillStub("bmad-dev-story", "Execute story implementation"),
		".claude/skills/bmad-create-story/SKILL.md": skillStub("bmad-create-story", "Draft a story file"),
		".claude/skills/bmad-code-review/SKILL.md":  skillStub("bmad-code-review", "Review a story implementation"),
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

func skillStub(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\n"
}
