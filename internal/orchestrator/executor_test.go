package orchestrator

import (
	"strings"
	"testing"
)

func TestPushEvidencePattern(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "push target line",
			text: "To github.com:org/repo.git\n   abc..def  main -> main",
			want: true,
		},
		{
			name: "up-to-date line",
			text: "Everything up-to-date",
			want: true,
		},
		{
			name: "no push evidence",
			text: "No findings detected.",
			want: false,
		},
	}

	for _, tt := range cases {
		got := pushEvidencePattern.MatchString(tt.text)
		if got != tt.want {
			t.Fatalf("%s: expected %v, got %v", tt.name, tt.want, got)
		}
	}
}

func TestUpstreamChanged(t *testing.T) {
	if !upstreamChanged("", false, "abc", true) {
		t.Fatal("expected upstream creation to count as changed")
	}
	if !upstreamChanged("abc", true, "def", true) {
		t.Fatal("expected changed sha to count as changed")
	}
	if upstreamChanged("abc", true, "abc", true) {
		t.Fatal("expected identical sha to not count as changed")
	}
	if upstreamChanged("", false, "", false) {
		t.Fatal("expected both missing upstream refs to not count as changed")
	}
}

func TestPublicationSatisfied(t *testing.T) {
	cases := []struct {
		name       string
		clean      bool
		cleanOK    bool
		ahead      int
		aheadOK    bool
		head       string
		headOK     bool
		upstream   string
		upstreamOK bool
		want       bool
	}{
		{
			name:       "published state",
			clean:      true,
			cleanOK:    true,
			ahead:      0,
			aheadOK:    true,
			head:       "abc",
			headOK:     true,
			upstream:   "abc",
			upstreamOK: true,
			want:       true,
		},
		{
			name:       "dirty tree",
			clean:      false,
			cleanOK:    true,
			ahead:      0,
			aheadOK:    true,
			head:       "abc",
			headOK:     true,
			upstream:   "abc",
			upstreamOK: true,
			want:       false,
		},
		{
			name:       "ahead of upstream",
			clean:      true,
			cleanOK:    true,
			ahead:      2,
			aheadOK:    true,
			head:       "abc",
			headOK:     true,
			upstream:   "def",
			upstreamOK: true,
			want:       false,
		},
		{
			name:       "missing upstream",
			clean:      true,
			cleanOK:    true,
			ahead:      0,
			aheadOK:    false,
			head:       "abc",
			headOK:     true,
			upstream:   "",
			upstreamOK: false,
			want:       false,
		},
	}

	for _, tc := range cases {
		got := publicationSatisfied(
			tc.clean,
			tc.cleanOK,
			tc.ahead,
			tc.aheadOK,
			tc.head,
			tc.headOK,
			tc.upstream,
			tc.upstreamOK,
		)
		if got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestExtractClaudeOutputJSON(t *testing.T) {
	input := []byte(`{"result":"Story 1-2 implemented successfully.","session_id":"abc-123","is_error":false}`)
	got := extractClaudeOutput(input)
	if got != "Story 1-2 implemented successfully." {
		t.Fatalf("expected result text, got %q", got)
	}
}

func TestExtractClaudeOutputPlainText(t *testing.T) {
	input := []byte("just plain text output\n")
	got := extractClaudeOutput(input)
	if got != "just plain text output" {
		t.Fatalf("expected trimmed plain text, got %q", got)
	}
}

func TestExtractClaudeOutputEmpty(t *testing.T) {
	got := extractClaudeOutput([]byte(""))
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestExtractClaudeOutputError(t *testing.T) {
	input := []byte(`{"result":"Something went wrong","session_id":"xyz","is_error":true}`)
	got := extractClaudeOutput(input)
	if got != "Something went wrong" {
		t.Fatalf("expected error result text, got %q", got)
	}
}

func TestIsAuthError(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "oauth token expired",
			output: `Failed to authenticate. API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"OAuth token has expired."}}`,
			want:   true,
		},
		{
			name:   "authentication_error keyword",
			output: `{"type":"error","error":{"type":"authentication_error","message":"invalid token"}}`,
			want:   true,
		},
		{
			name:   "401 with token mention",
			output: `Error: 401 Unauthorized - token invalid`,
			want:   true,
		},
		{
			name:   "normal error no auth issue",
			output: `Error: command failed with exit code 1`,
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
	}

	for _, tt := range cases {
		got := isAuthError(tt.output)
		if got != tt.want {
			t.Errorf("%s: isAuthError(%q) = %v, want %v", tt.name, tt.output, got, tt.want)
		}
	}
}

func TestIsTransientAPIError(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "529 overloaded",
			output: `API Error: 529 {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
			want:   true,
		},
		{
			name:   "500 server error",
			output: `API Error: 500 {"type":"error","error":{"type":"api_error","message":"Internal server error"}}`,
			want:   true,
		},
		{
			name:   "503 service unavailable",
			output: `API Error: 503 upstream unavailable`,
			want:   true,
		},
		{
			name:   "rate_limit_error type",
			output: `{"type":"error","error":{"type":"rate_limit_error","message":"Too many requests"}}`,
			want:   true,
		},
		{
			name:   "401 auth takes precedence over transient detection",
			output: `Failed to authenticate. API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"OAuth token has expired."}}`,
			want:   false,
		},
		{
			name:   "400 bad request — not transient",
			output: `API Error: 400 invalid request`,
			want:   false,
		},
		{
			name:   "plain exit error — not transient",
			output: `claude: command exited with status 1`,
			want:   false,
		},
	}

	for _, tt := range cases {
		got := isTransientAPIError(tt.output)
		if got != tt.want {
			t.Errorf("%s: isTransientAPIError(...) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestFirstLine(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "no output"},
		{"   \n   \n", "no output"},
		{"first\nsecond\n", "first"},
		{"   leading spaces\ntail\n", "leading spaces"},
	}
	for _, tt := range cases {
		got := firstLine(tt.in)
		if got != tt.want {
			t.Errorf("firstLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	// Truncation above 240 chars
	long := strings.Repeat("z", 500)
	got := firstLine(long)
	if !strings.HasSuffix(got, "…") || len(got) > 260 {
		t.Errorf("expected truncated single-line output with ellipsis, got len=%d", len(got))
	}
}
