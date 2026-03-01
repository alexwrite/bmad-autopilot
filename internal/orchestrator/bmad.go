package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// workflowSpec maps a workflow key to the BMAD files it needs.
type workflowSpec struct {
	agent        string // filename in _bmad/bmm/agents/ (e.g. "sm.md")
	workflowDir  string // dir relative to _bmad/bmm/workflows/ (e.g. "4-implementation/create-story")
	instructFile string // instruction filename inside workflowDir
}

// Workflow registry: which agent + workflow files each action needs.
// Based on BMAD agent menus:
//   - sm.md (Bob) owns: create-story, sprint-planning
//   - dev.md (Amelia) owns: dev-story, code-review
var workflowRegistry = map[string]workflowSpec{
	"create-story": {
		agent:        "sm.md",
		workflowDir:  "4-implementation/create-story",
		instructFile: "instructions.xml",
	},
	"dev-story": {
		agent:        "dev.md",
		workflowDir:  "4-implementation/dev-story",
		instructFile: "instructions.xml",
	},
	"code-review": {
		agent:        "dev.md",
		workflowDir:  "4-implementation/code-review",
		instructFile: "instructions.xml",
	},
}

// BMADContext holds all BMAD file contents needed to execute a workflow.
type BMADContext struct {
	Config       string
	Agent        string
	WorkflowXML  string
	WorkflowYAML string
	Instructions string
	Checklist    string
}

// LoadBMADContext reads the BMAD files for a given workflow key.
// Returns nil (no error) if _bmad/ directory does not exist — the caller
// should fall back to generic prompts.
func LoadBMADContext(workdir, workflowKey string) (*BMADContext, error) {
	bmadRoot := filepath.Join(workdir, "_bmad")

	if _, err := os.Stat(bmadRoot); os.IsNotExist(err) {
		return nil, nil
	}

	spec, ok := workflowRegistry[workflowKey]
	if !ok {
		return nil, fmt.Errorf("unknown BMAD workflow key %q", workflowKey)
	}

	wfDir := filepath.Join(bmadRoot, "bmm", "workflows", spec.workflowDir)

	config := readFileContent(filepath.Join(bmadRoot, "bmm", "config.yaml"))
	agent := readFileContent(filepath.Join(bmadRoot, "bmm", "agents", spec.agent))
	workflowXML := readFileContent(filepath.Join(bmadRoot, "core", "tasks", "workflow.xml"))
	workflowYAML := readFileContent(filepath.Join(wfDir, "workflow.yaml"))
	instructions := readFileContent(filepath.Join(wfDir, spec.instructFile))
	checklist := readFileContent(filepath.Join(wfDir, "checklist.md"))

	// Replace {project-root} with the actual absolute path so Claude
	// can resolve file references during execution.
	replacer := strings.NewReplacer("{project-root}", workdir)

	return &BMADContext{
		Config:       replacer.Replace(config),
		Agent:        replacer.Replace(agent),
		WorkflowXML:  replacer.Replace(workflowXML),
		WorkflowYAML: replacer.Replace(workflowYAML),
		Instructions: replacer.Replace(instructions),
		Checklist:    replacer.Replace(checklist),
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
	sb.WriteString("\nCRITICAL RULES:")
	sb.WriteString("\n- Do NOT wait for user input at any step")
	sb.WriteString("\n- Do NOT display menus or ask questions")
	sb.WriteString("\n- Auto-complete ALL steps as a simulated expert user")
	sb.WriteString("\n- Follow the workflow instructions IN EXACT ORDER")
	sb.WriteString("\n- Save outputs after EACH section")
	sb.WriteString("\n- You have the full BMAD context below — use it\n")

	writeSection("BMAD MODULE CONFIG", ctx.Config)
	writeSection("BMAD AGENT PERSONA", ctx.Agent)
	writeSection("BMAD WORKFLOW ENGINE (workflow.xml)", ctx.WorkflowXML)
	writeSection("WORKFLOW CONFIGURATION (workflow.yaml)", ctx.WorkflowYAML)
	writeSection("WORKFLOW INSTRUCTIONS", ctx.Instructions)
	writeSection("VALIDATION CHECKLIST", ctx.Checklist)

	return sb.String()
}

// HasContent returns true if the context has at least instructions to execute.
func (ctx *BMADContext) HasContent() bool {
	return strings.TrimSpace(ctx.Instructions) != ""
}

func readFileContent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
