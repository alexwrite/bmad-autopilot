package orchestrator

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// streamMsg represents the structure of a stream-json event from Claude CLI.
type streamMsg struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`

	// system init
	SessionID string `json:"session_id"`

	// assistant message
	Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
		Model   string          `json:"model"`
	} `json:"message"`

	// task events
	TaskID      string `json:"task_id"`
	Description string `json:"description"`

	// result
	Result string `json:"result"`
}

// contentBlock represents a content block inside a message.
type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
	Name     string `json:"name"`  // tool_use
	ID       string `json:"id"`    // tool_use
	Input    json.RawMessage `json:"input"` // tool_use
	Content  string `json:"content"` // tool_result
}

// FormatStream reads a stream.jsonl file and writes a human-readable markdown version.
func FormatStream(jsonlPath string) error {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return err
	}
	defer f.Close()

	dir := filepath.Dir(jsonlPath)
	outPath := filepath.Join(dir, "stream-readable.md")
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	fmt.Fprintln(out, "# Claude Stream Log")
	fmt.Fprintln(out, "")

	scanner := bufio.NewScanner(f)
	// Increase buffer for large lines
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	msgCount := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var msg streamMsg
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "system":
			formatSystemEvent(out, &msg)

		case "assistant":
			if len(msg.Message.Content) > 0 {
				msgCount++
				formatAssistantMessage(out, &msg, msgCount)
			}
			if msg.Result != "" {
				fmt.Fprintln(out, "---")
				fmt.Fprintln(out, "## Final Result")
				fmt.Fprintln(out, "")
				fmt.Fprintln(out, msg.Result)
				fmt.Fprintln(out, "")
			}

		case "user":
			formatUserMessage(out, &msg)
		}
	}

	return scanner.Err()
}

func formatSystemEvent(out *os.File, msg *streamMsg) {
	switch msg.Subtype {
	case "init":
		fmt.Fprintf(out, "> **Session:** `%s`\n\n", msg.SessionID)
	case "task_started":
		fmt.Fprintf(out, "### 🔧 Task: %s\n\n", msg.Description)
	case "task_completed":
		fmt.Fprintf(out, "> Task completed: `%s`\n\n", msg.TaskID)
	}
}

func formatAssistantMessage(out *os.File, msg *streamMsg, count int) {
	var blocks []contentBlock
	if err := json.Unmarshal(msg.Message.Content, &blocks); err != nil {
		// Not an array — skip
		return
	}

	for _, block := range blocks {
		switch block.Type {
		case "thinking":
			if block.Thinking != "" {
				fmt.Fprintln(out, "<details>")
				fmt.Fprintf(out, "<summary>💭 Thinking (#%d)</summary>\n\n", count)
				fmt.Fprintln(out, "```")
				// Truncate very long thinking blocks for readability
				thinking := block.Thinking
				if len(thinking) > 3000 {
					thinking = thinking[:3000] + "\n... (truncated)"
				}
				fmt.Fprintln(out, thinking)
				fmt.Fprintln(out, "```")
				fmt.Fprintln(out, "</details>")
				fmt.Fprintln(out, "")
			}

		case "text":
			if block.Text != "" {
				fmt.Fprintf(out, "**Assistant:**\n\n%s\n\n", block.Text)
			}

		case "tool_use":
			fmt.Fprintf(out, "#### 🛠 Tool: `%s`\n\n", block.Name)
			if len(block.Input) > 0 {
				// Pretty-print the input, truncated
				var prettyInput interface{}
				if err := json.Unmarshal(block.Input, &prettyInput); err == nil {
					formatted, _ := json.MarshalIndent(prettyInput, "", "  ")
					inputStr := string(formatted)
					if len(inputStr) > 2000 {
						inputStr = inputStr[:2000] + "\n... (truncated)"
					}
					fmt.Fprintf(out, "```json\n%s\n```\n\n", inputStr)
				}
			}

		case "tool_result":
			if block.Content != "" {
				content := block.Content
				if len(content) > 1000 {
					content = content[:1000] + "\n... (truncated)"
				}
				fmt.Fprintf(out, "<details>\n<summary>📋 Tool Result</summary>\n\n```\n%s\n```\n</details>\n\n", content)
			}
		}
	}
}

func formatUserMessage(out *os.File, msg *streamMsg) {
	var blocks []contentBlock
	if err := json.Unmarshal(msg.Message.Content, &blocks); err != nil {
		return
	}

	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			text := block.Text
			if len(text) > 500 {
				text = text[:500] + "\n... (truncated)"
			}
			fmt.Fprintf(out, "> **User/System:** %s\n\n", text)
		}
		if block.Type == "tool_result" && block.Content != "" {
			content := block.Content
			if len(content) > 1000 {
				content = content[:1000] + "\n... (truncated)"
			}
			fmt.Fprintf(out, "<details>\n<summary>📋 Tool Result</summary>\n\n```\n%s\n```\n</details>\n\n", content)
		}
	}
}
