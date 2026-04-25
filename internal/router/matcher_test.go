package router

import (
	"testing"

	"github.com/Coosis/k8s-vllm-router/internal/prefix"
)

func TestMatcherReadDoesNotMutate(t *testing.T) {
	matcher := NewMatcher()
	prints := []prefix.Fingerprint{{Length: 128, Hash: "abc"}}

	if got := matcher.MatchLength("pod-a", prints); got != 0 {
		t.Fatalf("cold match length = %d, want 0", got)
	}
	if got := matcher.MatchLength("pod-a", prints); got != 0 {
		t.Fatalf("second cold match length = %d, want 0", got)
	}

	matcher.Remember("pod-a", prints)

	if got := matcher.MatchLength("pod-a", prints); got != 128 {
		t.Fatalf("warm match length = %d, want 128", got)
	}
	if got := matcher.MatchLength("pod-b", prints); got != 0 {
		t.Fatalf("other backend match length = %d, want 0", got)
	}
}
