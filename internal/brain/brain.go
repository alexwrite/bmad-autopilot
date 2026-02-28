package brain

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/deicod/zai"
)

// Brain summarizes command outcomes without controlling deterministic flow rules.
type Brain interface {
	Name() string
	Summarize(ctx context.Context, command, rawOutput string) (string, error)
}

// New creates a brain by name.
func New(name string) (Brain, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "", "glm-5", "glm5":
		return &GLM5Brain{fallback: DeterministicBrain{}}, nil
	case "deterministic", "none":
		return DeterministicBrain{}, nil
	default:
		return nil, fmt.Errorf("unsupported brain %q", name)
	}
}

// DeterministicBrain uses local output-only summarization.
type DeterministicBrain struct{}

func (DeterministicBrain) Name() string {
	return "deterministic"
}

func (DeterministicBrain) Summarize(_ context.Context, _ string, rawOutput string) (string, error) {
	lines := strings.Split(rawOutput, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return oneLine(line), nil
		}
	}
	return "no output", nil
}

// GLM5Brain uses z.ai glm-5 when ZAI_API_KEY is available.
type GLM5Brain struct {
	fallback Brain
}

func (b *GLM5Brain) Name() string {
	return "glm-5"
}

func (b *GLM5Brain) Summarize(ctx context.Context, command, rawOutput string) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("ZAI_API_KEY"))
	if apiKey == "" {
		return b.fallbackSummary(ctx, command, rawOutput)
	}

	client, err := zai.NewClient(apiKey)
	if err != nil {
		return b.fallbackSummary(ctx, command, rawOutput)
	}

	resp, err := client.ChatCompletions(ctx, &zai.ChatCompletionRequest{
		Model: "glm-5",
		Messages: []zai.Message{
			{
				Role:    zai.RoleSystem,
				Content: "Summarize the command result in exactly one concise sentence.",
			},
			{
				Role: zai.RoleUser,
				Content: fmt.Sprintf("Command:\n%s\n\nOutput:\n%s",
					command,
					rawOutput,
				),
			},
		},
	})
	if err != nil || len(resp.Choices) == 0 {
		return b.fallbackSummary(ctx, command, rawOutput)
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		return b.fallbackSummary(ctx, command, rawOutput)
	}

	return oneLine(content), nil
}

func (b *GLM5Brain) fallbackSummary(ctx context.Context, command, rawOutput string) (string, error) {
	if b.fallback == nil {
		return DeterministicBrain{}.Summarize(ctx, command, rawOutput)
	}
	return b.fallback.Summarize(ctx, command, rawOutput)
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
