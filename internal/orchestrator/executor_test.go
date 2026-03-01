package orchestrator

import "testing"

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
