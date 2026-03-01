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
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "_bmad"), 0o755)
	_, err := LoadBMADContext(dir, "nonexistent-workflow")
	if err == nil {
		t.Fatal("expected error for unknown workflow key")
	}
}

func TestLoadBMADContextDevStory(t *testing.T) {
	dir := setupTestBMAD(t)

	ctx, err := LoadBMADContext(dir, "dev-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if !strings.Contains(ctx.Config, "Alex") {
		t.Fatal("expected config to contain user name")
	}
	if !strings.Contains(ctx.Agent, "Developer Agent") {
		t.Fatal("expected dev agent persona")
	}
	if !strings.Contains(ctx.WorkflowXML, "Execute Workflow") {
		t.Fatal("expected workflow engine content")
	}
	if !strings.Contains(ctx.WorkflowYAML, "dev-story") {
		t.Fatal("expected workflow yaml content")
	}
	if !strings.Contains(ctx.Instructions, "implement") {
		t.Fatal("expected instructions content")
	}
}

func TestLoadBMADContextCreateStory(t *testing.T) {
	dir := setupTestBMAD(t)

	ctx, err := LoadBMADContext(dir, "create-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if !strings.Contains(ctx.Agent, "Scrum Master") {
		t.Fatal("expected SM agent persona for create-story")
	}
}

func TestLoadBMADContextResolvesProjectRoot(t *testing.T) {
	dir := setupTestBMAD(t)

	ctx, err := LoadBMADContext(dir, "dev-story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(ctx.WorkflowYAML, "{project-root}") {
		t.Fatal("expected {project-root} to be resolved")
	}
	if !strings.Contains(ctx.WorkflowYAML, dir) {
		t.Fatalf("expected resolved path to contain workdir %q", dir)
	}
}

func TestSystemPromptContainsYolo(t *testing.T) {
	ctx := &BMADContext{
		Config:       "user_name: Alex",
		Agent:        "Developer Agent",
		WorkflowXML:  "Execute Workflow",
		WorkflowYAML: "name: dev-story",
		Instructions: "step 1: implement",
	}
	prompt := ctx.SystemPrompt()
	if !strings.Contains(prompt, "#yolo") {
		t.Fatal("expected system prompt to contain yolo mode")
	}
	if !strings.Contains(prompt, "AUTONOMOUS") {
		t.Fatal("expected system prompt to contain autonomous directive")
	}
}

func TestSystemPromptContainsAllSections(t *testing.T) {
	ctx := &BMADContext{
		Config:       "config content",
		Agent:        "agent content",
		WorkflowXML:  "engine content",
		WorkflowYAML: "workflow content",
		Instructions: "instructions content",
		Checklist:    "checklist content",
	}
	prompt := ctx.SystemPrompt()

	sections := []string{
		"BMAD MODULE CONFIG",
		"BMAD AGENT PERSONA",
		"BMAD WORKFLOW ENGINE",
		"WORKFLOW CONFIGURATION",
		"WORKFLOW INSTRUCTIONS",
		"VALIDATION CHECKLIST",
	}
	for _, s := range sections {
		if !strings.Contains(prompt, s) {
			t.Fatalf("expected system prompt to contain section %q", s)
		}
	}
}

func TestSystemPromptSkipsEmptySections(t *testing.T) {
	ctx := &BMADContext{
		Instructions: "step 1: do something",
	}
	prompt := ctx.SystemPrompt()
	if strings.Contains(prompt, "BMAD MODULE CONFIG") {
		t.Fatal("expected empty config section to be skipped")
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

	withInstructions := &BMADContext{Instructions: "step 1"}
	if !withInstructions.HasContent() {
		t.Fatal("expected context with instructions to have content")
	}
}

// setupTestBMAD creates a minimal BMAD file structure for testing.
func setupTestBMAD(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"_bmad/bmm/config.yaml": `user_name: Alex
communication_language: Français
output_folder: "{project-root}/_bmad-output"
`,
		"_bmad/bmm/agents/dev.md": `---
name: "dev"
description: "Developer Agent"
---
<agent name="Amelia" title="Developer Agent">
<persona><role>Senior Software Engineer</role></persona>
</agent>
`,
		"_bmad/bmm/agents/sm.md": `---
name: "sm"
description: "Scrum Master"
---
<agent name="Bob" title="Scrum Master">
<persona><role>Technical Scrum Master</role></persona>
</agent>
`,
		"_bmad/core/tasks/workflow.xml": `<task id="workflow.xml" name="Execute Workflow">
  <objective>Execute given workflow</objective>
  <flow><step n="1">Load and Initialize</step></flow>
</task>
`,
		"_bmad/bmm/workflows/4-implementation/dev-story/workflow.yaml": `name: dev-story
description: "Execute story implementation"
config_source: "{project-root}/_bmad/bmm/config.yaml"
installed_path: "{project-root}/_bmad/bmm/workflows/4-implementation/dev-story"
instructions: "{project-root}/_bmad/bmm/workflows/4-implementation/dev-story/instructions.xml"
`,
		"_bmad/bmm/workflows/4-implementation/dev-story/instructions.xml": `<instructions>
  <step n="1">Read story file and implement all tasks</step>
</instructions>
`,
		"_bmad/bmm/workflows/4-implementation/dev-story/checklist.md":        "- [ ] All tests pass\n",
		"_bmad/bmm/workflows/4-implementation/create-story/workflow.yaml":    "name: create-story\ndescription: Create story file\n",
		"_bmad/bmm/workflows/4-implementation/create-story/instructions.xml": "<instructions><step n=\"1\">Create story</step></instructions>\n",
		"_bmad/bmm/workflows/4-implementation/code-review/workflow.yaml":     "name: code-review\ndescription: Code review\n",
		"_bmad/bmm/workflows/4-implementation/code-review/instructions.xml":  "<instructions><step n=\"1\">Review code</step></instructions>\n",
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
