package router

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Coosis/k8s-vllm-router/internal/backend"
	"github.com/Coosis/k8s-vllm-router/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

func TestBackendStateInflightCounter(t *testing.T) {
	state := backend.NewBackendState(backend.Endpoint{ID: "pod-a", URL: "http://pod-a"})
	counter := state.Inflight

	if got := state.Inflight.Load(); got != 0 {
		t.Fatalf("initial inflight = %d, want 0", got)
	}

	counter.Add(1)
	counter.Add(1)
	if got := state.Inflight.Load(); got != 2 {
		t.Fatalf("incremented inflight = %d, want 2", got)
	}

	counter.Add(-1)
	if got := state.Inflight.Load(); got != 1 {
		t.Fatalf("decremented inflight = %d, want 1", got)
	}
}

func TestCacheAwareColdSelectionsRotateAcrossBackends(t *testing.T) {
	matcher := NewMatcher(config.NewExpiryConfig())
	r, err := New(Options{
		Policy: "cache_aware",
		Backends: []backend.Endpoint{
			{ID: "pod-a", URL: "http://pod-a"},
			{ID: "pod-b", URL: "http://pod-b"},
			{ID: "pod-c", URL: "http://pod-c"},
		},
		Register: prometheus.NewRegistry(),
	}, matcher)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range r.backends.Load().List {
		state.Healthy.Store(true)
	}

	seen := map[string]bool{}
	for range 3 {
		req := newJSONRequest(`{"messages":[{"role":"user","content":"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty twenty-one twenty-two twenty-three twenty-four twenty-five twenty-six twenty-seven twenty-eight twenty-nine thirty thirty-one thirty-two"}],"max_tokens":1}`)
		endpoint, _, release, err := r.selectAndReserveBackend(req)
		if err != nil {
			t.Fatal(err)
		}
		seen[endpoint.Endpoint.ID] = true
		release()
	}

	if len(seen) != 3 {
		t.Fatalf("cold selections hit %d backends, want 3: %#v", len(seen), seen)
	}
}

func newJSONRequest(body string) *http.Request {
	req, err := http.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		panic(err)
	}
	return req
}
