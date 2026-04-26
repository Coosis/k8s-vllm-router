package router

import (
	"sync"
	"testing"
	"time"

	"github.com/Coosis/k8s-vllm-router/internal/config"
	"github.com/Coosis/k8s-vllm-router/internal/prefix"
)

func TestMatcherReadDoesNotMutate(t *testing.T) {
	cfg := config.NewExpiryConfig()
	matcher := NewMatcher(cfg)
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

func TestMatcherExponentialDecay(t *testing.T) {
	matcher := NewMatcher(config.ExpiryConfig{
		PrefixMaxAge:  time.Hour,
		ExpiryPolicy:  config.EXPIRY_POLICY_DECAY,
		DecayPolicy:   config.DECAY_POLICY_EXPONENTIAL,
		DecayHalfLife: time.Minute,
	})
	prints := []prefix.Fingerprint{{Length: 128, Hash: "abc"}}
	matcher.Remember("pod-a", prints)
	setLastSeen(t, matcher, "abc", "pod-a", time.Now().Add(-time.Minute))

	matches := matcher.CandidateMatches(prints).Matches
	match, ok := matches["pod-a"]
	if !ok {
		t.Fatal("expected pod-a candidate")
	}
	if match.Warmth < 63 || match.Warmth > 65 {
		t.Fatalf("warmth = %f, want about 64", match.Warmth)
	}
}

func TestMatcherDeletesExpiredCandidate(t *testing.T) {
	matcher := NewMatcher(config.ExpiryConfig{
		PrefixMaxAge:  time.Minute,
		ExpiryPolicy:  config.EXPIRY_POLICY_DECAY,
		DecayPolicy:   config.DECAY_POLICY_EXPONENTIAL,
		DecayHalfLife: time.Minute,
	})
	prints := []prefix.Fingerprint{{Length: 128, Hash: "abc"}}
	matcher.Remember("pod-a", prints)
	setLastSeen(t, matcher, "abc", "pod-a", time.Now().Add(-2*time.Minute))

	result := matcher.CandidateMatches(prints)
	matches := result.Matches
	if _, ok := matches["pod-a"]; ok {
		t.Fatal("expired candidate should not match")
	}
	if result.Evictions != 1 {
		t.Fatalf("evictions = %d, want 1", result.Evictions)
	}
	if matcher.MatchLength("pod-a", prints) != 0 {
		t.Fatal("expired candidate should be deleted")
	}
}

func setLastSeen(t *testing.T, matcher *Matcher, hash string, backendID string, seenAt time.Time) {
	t.Helper()

	value, ok := matcher.prefixToBackends.Load(hash)
	if !ok {
		t.Fatalf("missing prefix hash %q", hash)
	}
	backends := value.(*sync.Map)
	entryValue, ok := backends.Load(backendID)
	if !ok {
		t.Fatalf("missing backend %q", backendID)
	}
	entry := entryValue.(*MatchEntry)
	entry.LastSeenUnixNano.Store(seenAt.UnixNano())
}
