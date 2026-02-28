package brain

import "testing"

func TestNewDefaultIsDeterministic(t *testing.T) {
	b, err := New("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Name() != "deterministic" {
		t.Fatalf("expected deterministic default, got %q", b.Name())
	}
}

func TestNewGLM5(t *testing.T) {
	b, err := New("glm-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Name() != "glm-5" {
		t.Fatalf("expected glm-5, got %q", b.Name())
	}
}
