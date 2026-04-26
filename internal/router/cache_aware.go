package router

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/Coosis/k8s-vllm-router/internal/backend"
	fingerprint "github.com/Coosis/k8s-vllm-router/internal/prefix"
)

var (
	PrefixLengths     = []int{32, 64, 128, 256, 512, 1024}
	MAX_PREFIX_LENGTH = 1024
)

const (
	PREFIX_MATCH_WEIGHT float64 = 150
	INFLIGHT_WEIGHT     float64 = 10
	LATENCY_WEIGHT      float64 = 1.5
	ERROR_WEIGHT        float64 = 50
)

func (r *Router) cacheAwareSelect(
	req *http.Request,
	snapshot *backend.BackendSnapshot,
) (*backend.BackendState, []fingerprint.Fingerprint, error) {
	prints, err := fingerprintsFromRequest(req)
	if err != nil {
		return nil, nil, err
	}

	start := int(r.next.Add(1) - 1)
	fallback, fallbackScore := loadAwareFallback(snapshot, start)
	selected := fallback
	maxScore := fallbackScore

	candidateResult := r.matcher.CandidateMatches(prints)
	if candidateResult.Evictions > 0 {
		r.metric.PrefixEvictions.Add(float64(candidateResult.Evictions))
	}
	for backendID, candidateMatch := range candidateResult.Matches {
		matchLength := strconv.Itoa(candidateMatch.MatchLength)
		r.metric.PrefixCandidateMatches.WithLabelValues(matchLength).Inc()
		r.metric.PrefixMatchWarmth.WithLabelValues(matchLength).Observe(candidateMatch.Warmth)
		r.metric.PrefixMatchAgeSeconds.WithLabelValues(matchLength).Observe(candidateMatch.Age.Seconds())

		endpoint := snapshot.ByID[backendID]
		if !isSelectable(endpoint) {
			continue
		}
		score := scoreBackend(endpoint, float64(candidateMatch.Warmth))
		if score > maxScore {
			maxScore = score
			selected = endpoint
		}
	}

	if selected == nil {
		return nil, nil, ErrNoHealthyBackends
	}
	return selected, prints, nil
}

func loadAwareFallback(snapshot *backend.BackendSnapshot, start int) (*backend.BackendState, float64) {
	maxScore := math.Inf(-1)
	var selected *backend.BackendState
	for i := range snapshot.List {
		endpoint := snapshot.List[(start+i)%len(snapshot.List)]
		if !isSelectable(endpoint) {
			continue
		}
		score := scoreBackend(endpoint, 0)
		if score > maxScore {
			maxScore = score
			selected = endpoint
		}
	}
	return selected, maxScore
}

// score =
//   prefix_match_weight * prefix_match_score
// - inflight_weight     * inflight_requests
// - latency_weight      * latency
// - error_weight        * error

func scoreBackend(endpoint *backend.BackendState, matchScore float64) float64 {
	return PREFIX_MATCH_WEIGHT*matchScore -
		INFLIGHT_WEIGHT*float64(endpoint.Inflight.Load()) -
		LATENCY_WEIGHT*endpoint.LatencyEWMA.Get() -
		ERROR_WEIGHT*endpoint.ErrorEWMA.Get()
}

func fingerprintsFromRequest(req *http.Request) ([]fingerprint.Fingerprint, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))

	var decoded Request
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}

	prompt := decoded.PrefixText(MAX_PREFIX_LENGTH)
	return fingerprint.Fingerprints(prompt, PrefixLengths), nil
}
