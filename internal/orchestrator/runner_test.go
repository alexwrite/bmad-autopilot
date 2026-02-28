package orchestrator

import (
	"path/filepath"
	"testing"
)

func TestResolveStatusFilePath(t *testing.T) {
	cwd := filepath.FromSlash("/tmp/project")

	tests := []struct {
		name      string
		statusArg string
		want      string
	}{
		{
			name:      "default from cwd",
			statusArg: "",
			want:      filepath.Join(cwd, defaultStatusFile),
		},
		{
			name:      "relative path from cwd",
			statusArg: "custom/status.yaml",
			want:      filepath.Join(cwd, "custom/status.yaml"),
		},
		{
			name:      "absolute path untouched",
			statusArg: filepath.FromSlash("/opt/repo/_bmad-output/implementation-artifacts/sprint-status.yaml"),
			want:      filepath.FromSlash("/opt/repo/_bmad-output/implementation-artifacts/sprint-status.yaml"),
		},
	}

	for _, tt := range tests {
		got, err := resolveStatusFilePath(tt.statusArg, cwd)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tt.name, err)
		}
		if got != filepath.Clean(tt.want) {
			t.Fatalf("%s: expected %q, got %q", tt.name, filepath.Clean(tt.want), got)
		}
	}
}

func TestInferWorkdirFromStatusFile(t *testing.T) {
	fallback := filepath.FromSlash("/tmp/fallback")

	got := inferWorkdirFromStatusFile(
		filepath.FromSlash("/tmp/repo/_bmad-output/implementation-artifacts/sprint-status.yaml"),
		fallback,
	)
	if got != filepath.FromSlash("/tmp/repo") {
		t.Fatalf("expected /tmp/repo, got %q", got)
	}

	got = inferWorkdirFromStatusFile(
		filepath.FromSlash("/tmp/repo/custom/status.yaml"),
		fallback,
	)
	if got != fallback {
		t.Fatalf("expected fallback %q, got %q", fallback, got)
	}
}
