package router

import (
	"sync"

	"github.com/Coosis/k8s-vllm-router/internal/prefix"
)

type Matcher struct {
	prefixToBackends sync.Map
}

func NewMatcher() *Matcher {
	return &Matcher{}
}

// only used for testing
func (m *Matcher) MatchLength(backendID string, prints []prefix.Fingerprint) int {
	best := 0
	for _, print := range prints {
		if m.has(print.Hash, backendID) && print.Length > best {
			best = print.Length
		}
	}
	return best
}

func (m *Matcher) CandidateMatches(prints []prefix.Fingerprint) map[string]int {
	candidates := make(map[string]int)
	for _, print := range prints {
		value, ok := m.prefixToBackends.Load(print.Hash)
		if !ok {
			continue
		}
		backends := value.(*sync.Map)
		backends.Range(func(key, _ any) bool {
			backendID := key.(string)
			if print.Length > candidates[backendID] {
				candidates[backendID] = print.Length
			}
			return true
		})
	}
	return candidates
}

func (m *Matcher) Remember(backendID string, prints []prefix.Fingerprint) {
	for _, print := range prints {
		value, _ := m.prefixToBackends.LoadOrStore(print.Hash, &sync.Map{})
		backends := value.(*sync.Map)
		backends.Store(backendID, struct{}{})
	}
}

func (m *Matcher) has(hash string, backendID string) bool {
	value, ok := m.prefixToBackends.Load(hash)
	if !ok {
		return false
	}
	backends := value.(*sync.Map)
	_, ok = backends.Load(backendID)
	return ok
}
