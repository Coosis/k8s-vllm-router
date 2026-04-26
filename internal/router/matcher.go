package router

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Coosis/k8s-vllm-router/internal/config"
	"github.com/Coosis/k8s-vllm-router/internal/prefix"
)

type Matcher struct {
	// prefix -> *sync.Map[backendID]*MatchEntry
	prefixToBackends sync.Map

	expiryPolicy string
	decayPolicy  string
	prefixMaxAge time.Duration
	halfLife     time.Duration
}

type MatchEntry struct {
	LastSeenUnixNano atomic.Int64
}

func NewMatcher(cfg config.ExpiryConfig) *Matcher {
	return &Matcher{
		expiryPolicy: cfg.ExpiryPolicy,
		decayPolicy:  cfg.DecayPolicy,
		prefixMaxAge: cfg.PrefixMaxAge,
		halfLife:     cfg.DecayHalfLife,
	}
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

type CandidateMatch struct {
	// main property, used for scoring
	Warmth float64

	// side properties, used for metrics
	MatchLength int
	Age         time.Duration
}

type CandidateMatchResult struct {
	Matches   map[string]CandidateMatch
	Evictions int
}

func (m *Matcher) CandidateMatches(prints []prefix.Fingerprint) CandidateMatchResult {
	now := time.Now()
	evictions := 0
	candidates := make(map[string]CandidateMatch)
	for _, print := range prints {
		value, ok := m.prefixToBackends.Load(print.Hash)
		if !ok {
			continue
		}
		backends := value.(*sync.Map)
		backends.Range(func(key, entry any) bool {
			backend := key.(string)
			matchEntry := entry.(*MatchEntry)
			age := now.Sub(time.Unix(0, matchEntry.LastSeenUnixNano.Load()))
			// hard capping
			if m.isExpired(age) {
				// expired
				backends.Delete(backend)
				evictions++
				return true
			}

			// warmth decays
			warmth := float64(print.Length) * m.decay(age)
			if warmth > candidates[backend].Warmth {
				candidates[backend] = CandidateMatch{
					Warmth:      warmth,
					MatchLength: print.Length,
					Age:         age,
				}
			}
			return true
		})
	}
	return CandidateMatchResult{
		Matches:   candidates,
		Evictions: evictions,
	}
}

func (m *Matcher) Remember(backendID string, prints []prefix.Fingerprint) {
	now := time.Now().UnixNano()
	for _, print := range prints {
		value, _ := m.prefixToBackends.LoadOrStore(print.Hash, &sync.Map{})
		backends := value.(*sync.Map)
		entryVal, _ := backends.LoadOrStore(backendID, &MatchEntry{})
		entry := entryVal.(*MatchEntry)
		entry.LastSeenUnixNano.Store(now)
		backends.Store(backendID, entry)
	}
}

func (m *Matcher) isExpired(age time.Duration) bool {
	return m.expiryPolicy != config.EXPIRY_POLICY_NONE &&
		m.prefixMaxAge > 0 &&
		age > m.prefixMaxAge
}

func (m *Matcher) decay(age time.Duration) float64 {
	if m.expiryPolicy == config.EXPIRY_POLICY_NONE {
		return 1
	}
	if m.halfLife <= 0 {
		return 1
	}
	switch m.decayPolicy {
	case config.DECAY_POLICY_LINEAR:
		if m.prefixMaxAge <= 0 {
			return 1
		}
		return max(0, 1-float64(age)/float64(m.prefixMaxAge))
	case config.DECAY_POLICY_EXPONENTIAL:
		fallthrough
	default:
		return math.Exp(-math.Ln2 * float64(age) / float64(m.halfLife))
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
