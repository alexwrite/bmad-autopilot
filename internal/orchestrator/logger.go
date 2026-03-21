package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunLogger handles timestamped console output and structured file logging.
type RunLogger struct {
	runDir   string // e.g. _bmad-output/autopilot-logs/2026-03-21T15-30-00
	logFile  *os.File
	storyDir string // current story subdirectory
}

// NewRunLogger creates a new logger for this autopilot run.
// Creates the directory structure under {workdir}/_bmad-output/autopilot-logs/.
func NewRunLogger(workdir string) (*RunLogger, error) {
	ts := time.Now().Format("2006-01-02T15-04-05")
	runDir := filepath.Join(workdir, "_bmad-output", "autopilot-logs", ts)

	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	logFile, err := os.Create(filepath.Join(runDir, "run.log"))
	if err != nil {
		return nil, fmt.Errorf("create run.log: %w", err)
	}

	l := &RunLogger{runDir: runDir, logFile: logFile}
	l.Log("AUTOPILOT", "run started — logs at %s", runDir)
	return l, nil
}

// Close flushes and closes the log file.
func (l *RunLogger) Close() {
	if l.logFile != nil {
		l.logFile.Close()
	}
}

// Log prints a timestamped message to console and writes to run.log.
func (l *RunLogger) Log(tag, format string, args ...interface{}) {
	ts := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] %s: %s", ts, tag, msg)

	fmt.Println(line)

	if l.logFile != nil {
		fullTs := time.Now().Format("2006-01-02T15:04:05")
		fmt.Fprintf(l.logFile, "[%s] %s: %s\n", fullTs, tag, msg)
	}
}

// LogRaw prints a raw line with timestamp prefix.
func (l *RunLogger) LogRaw(format string, args ...interface{}) {
	ts := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] %s", ts, msg)

	fmt.Println(line)

	if l.logFile != nil {
		fullTs := time.Now().Format("2006-01-02T15:04:05")
		fmt.Fprintf(l.logFile, "[%s] %s\n", fullTs, msg)
	}
}

// LogSeparator prints a visual separator.
func (l *RunLogger) LogSeparator() {
	sep := "════════════════════════════════════════"
	fmt.Println(sep)
	if l.logFile != nil {
		fmt.Fprintln(l.logFile, sep)
	}
}

// SetStory configures the current story directory for file logging.
// Creates: {runDir}/{storyKey}/
func (l *RunLogger) SetStory(storyKey string) error {
	l.storyDir = filepath.Join(l.runDir, storyKey)
	return os.MkdirAll(l.storyDir, 0755)
}

// actionDir creates and returns the directory for a specific action.
// e.g. {runDir}/{storyKey}/create-story/ or {runDir}/{storyKey}/code-review-2/
func (l *RunLogger) actionDir(workflowKey string, round int) (string, error) {
	name := workflowKey
	if round > 0 {
		name = fmt.Sprintf("%s-%d", workflowKey, round)
	}
	dir := filepath.Join(l.storyDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// SaveOutput saves the raw Claude output to a file.
func (l *RunLogger) SaveOutput(workflowKey string, round int, rawOutput string) error {
	dir, err := l.actionDir(workflowKey, round)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "output.txt"), []byte(rawOutput), 0644)
}

// SaveStream saves the full stream-json output (thinking, tool calls, text)
// and generates a human-readable markdown version.
func (l *RunLogger) SaveStream(workflowKey string, round int, stream string) error {
	if stream == "" {
		return nil
	}
	dir, err := l.actionDir(workflowKey, round)
	if err != nil {
		return err
	}
	jsonlPath := filepath.Join(dir, "stream.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(stream), 0644); err != nil {
		return err
	}
	// Generate readable version (best-effort, non-fatal)
	_ = FormatStream(jsonlPath)
	return nil
}

// SaveResult saves a structured result as JSON.
func (l *RunLogger) SaveResult(workflowKey string, round int, data interface{}) error {
	dir, err := l.actionDir(workflowKey, round)
	if err != nil {
		return err
	}
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "result.json"), jsonBytes, 0644)
}

// SaveVerdict saves a judge verdict as JSON.
func (l *RunLogger) SaveVerdict(round int, verdict JudgeVerdict) error {
	dir, err := l.actionDir("judge", round)
	if err != nil {
		return err
	}
	jsonBytes, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "verdict.json"), jsonBytes, 0644)
}

// SaveError saves an error to a file.
func (l *RunLogger) SaveError(workflowKey string, round int, errMsg string) error {
	dir, err := l.actionDir(workflowKey, round)
	if err != nil {
		return err
	}
	content := fmt.Sprintf("timestamp: %s\nerror: %s\n", time.Now().Format(time.RFC3339), errMsg)
	return os.WriteFile(filepath.Join(dir, "error.txt"), []byte(content), 0644)
}

// StepResult captures structured info about a completed step.
type StepResult struct {
	StoryKey     string `json:"story_key"`
	WorkflowKey  string `json:"workflow_key"`
	Round        int    `json:"round,omitempty"`
	StatusBefore string `json:"status_before"`
	StatusAfter  string `json:"status_after"`
	Summary      string `json:"summary"`
	Timestamp    string `json:"timestamp"`
	Duration     string `json:"duration,omitempty"`
	HasOutput    bool   `json:"has_output"`
	OutputLines  int    `json:"output_lines"`
}

// NewStepResult creates a StepResult with current timestamp.
func NewStepResult(storyKey, workflowKey, statusBefore, statusAfter, summary, rawOutput string) StepResult {
	lines := 0
	if rawOutput != "" {
		lines = len(strings.Split(rawOutput, "\n"))
	}
	return StepResult{
		StoryKey:     storyKey,
		WorkflowKey:  workflowKey,
		StatusBefore: statusBefore,
		StatusAfter:  statusAfter,
		Summary:      summary,
		Timestamp:    time.Now().Format(time.RFC3339),
		HasOutput:    rawOutput != "",
		OutputLines:  lines,
	}
}
