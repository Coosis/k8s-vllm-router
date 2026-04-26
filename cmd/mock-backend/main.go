package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Coosis/k8s-vllm-router/internal/prefix"
)

var cachePrefixLengths = []int{32, 64, 128, 256, 512, 1024}

type server struct {
	backendID string
	logger    *slog.Logger
	cache     *prefixCache
	latency   latencyConfig
}

type prefixCache struct {
	mu     sync.Mutex
	hashes map[string]time.Time
	ttl    time.Duration
}

type latencyConfig struct {
	baseLatency        time.Duration
	coldPrefillPerWord time.Duration
	warmPrefillPerWord time.Duration
	decodePerToken     time.Duration
	maxSleep           time.Duration
}

type requestBody struct {
	Model       string    `json:"model"`
	Prompt      any       `json:"prompt"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	Stream      bool      `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type simulation struct {
	PromptWords int
	MaxTokens   int
	CacheHit    bool
	MatchLength int
	TTFT        time.Duration
	DecodeDelay time.Duration
	TotalSleep  time.Duration
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	addr := getenv("MOCK_BACKEND_ADDR", ":8081")

	srv := &server{
		backendID: getenv("HOSTNAME", "mock-backend"),
		logger:    logger,
		cache:     newPrefixCache(envDuration("MOCK_CACHE_TTL", 10*time.Minute)),
		latency: latencyConfig{
			baseLatency:        envDuration("MOCK_BASE_LATENCY", 20*time.Millisecond),
			coldPrefillPerWord: envDuration("MOCK_COLD_PREFILL_PER_WORD", 2*time.Millisecond),
			warmPrefillPerWord: envDuration("MOCK_WARM_PREFILL_PER_WORD", 200*time.Microsecond),
			decodePerToken:     envDuration("MOCK_DECODE_PER_TOKEN", 4*time.Millisecond),
			maxSleep:           envDuration("MOCK_MAX_SLEEP", 5*time.Second),
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.health)
	mux.HandleFunc("/v1/chat/completions", srv.completion)
	mux.HandleFunc("/v1/completions", srv.completion)

	logger.Info("mock backend listening", "addr", addr, "backend", srv.backendID)
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("mock backend failed", "error", err)
		os.Exit(1)
	}
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *server) completion(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var body requestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sim := s.simulate(body)
	if body.Stream {
		s.streamCompletion(w, sim)
		return
	}

	time.Sleep(sim.TotalSleep)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      "mock-completion",
		"object":  "chat.completion",
		"backend": s.backendID,
		"mock": map[string]any{
			"cache_hit":     sim.CacheHit,
			"match_length":  sim.MatchLength,
			"prompt_words":  sim.PromptWords,
			"max_tokens":    sim.MaxTokens,
			"ttft_ms":       sim.TTFT.Milliseconds(),
			"decode_ms":     sim.DecodeDelay.Milliseconds(),
			"sleep_ms":      sim.TotalSleep.Milliseconds(),
			"cache_entries": s.cache.Len(),
		},
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": fmt.Sprintf("mock response from %s", s.backendID),
				},
				"finish_reason": "stop",
			},
		},
	})
}

func (s *server) streamCompletion(w http.ResponseWriter, sim simulation) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	time.Sleep(sim.TTFT)
	writeSSE(w, map[string]any{
		"id":      "mock-completion",
		"object":  "chat.completion.chunk",
		"backend": s.backendID,
		"mock": map[string]any{
			"cache_hit":     sim.CacheHit,
			"match_length":  sim.MatchLength,
			"prompt_words":  sim.PromptWords,
			"max_tokens":    sim.MaxTokens,
			"ttft_ms":       sim.TTFT.Milliseconds(),
			"decode_ms":     sim.DecodeDelay.Milliseconds(),
			"sleep_ms":      sim.TotalSleep.Milliseconds(),
			"cache_entries": s.cache.Len(),
		},
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]string{
					"role":    "assistant",
					"content": fmt.Sprintf("mock response from %s", s.backendID),
				},
				"finish_reason": nil,
			},
		},
	})
	flusher.Flush()

	if sim.DecodeDelay > 0 {
		time.Sleep(sim.DecodeDelay)
	}
	writeSSE(w, map[string]any{
		"id":      "mock-completion",
		"object":  "chat.completion.chunk",
		"backend": s.backendID,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]string{},
				"finish_reason": "stop",
			},
		},
	})
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (s *server) simulate(body requestBody) simulation {
	promptText := body.PrefixText()
	prints := prefix.Fingerprints(promptText, cachePrefixLengths)
	cacheHit, matchLength := s.cache.Lookup(prints)
	s.cache.Remember(prints)

	promptWords := len(strings.Fields(prefix.Normalize(promptText)))
	maxTokens := body.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 16
	}

	prefillRate := s.latency.coldPrefillPerWord
	if cacheHit {
		prefillRate = s.latency.warmPrefillPerWord
	}

	ttft := max(s.latency.baseLatency+time.Duration(promptWords)*prefillRate, 0)
	decodeDelay := max(time.Duration(maxTokens)*s.latency.decodePerToken, 0)
	totalSleep := max(min(ttft+decodeDelay, s.latency.maxSleep), 0)
	if totalSleep < ttft {
		ttft = totalSleep
		decodeDelay = 0
	}

	if s.logger != nil {
		s.logger.Info(
			"mock completion simulated",
			"backend", s.backendID,
			"cache_hit", cacheHit,
			"match_length", matchLength,
			"prompt_words", promptWords,
			"max_tokens", maxTokens,
			"ttft_ms", ttft.Milliseconds(),
			"decode_ms", decodeDelay.Milliseconds(),
			"sleep_ms", totalSleep.Milliseconds(),
		)
	}

	return simulation{
		PromptWords: promptWords,
		MaxTokens:   maxTokens,
		CacheHit:    cacheHit,
		MatchLength: matchLength,
		TTFT:        ttft,
		DecodeDelay: decodeDelay,
		TotalSleep:  totalSleep,
	}
}

func writeSSE(w http.ResponseWriter, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func newPrefixCache(ttl time.Duration) *prefixCache {
	return &prefixCache{
		hashes: make(map[string]time.Time),
		ttl:    ttl,
	}
}

func (c *prefixCache) Lookup(prints []prefix.Fingerprint) (bool, int) {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	best := 0
	for _, print := range prints {
		seenAt, ok := c.hashes[print.Hash]
		if !ok {
			continue
		}
		if c.ttl > 0 && now.Sub(seenAt) > c.ttl {
			delete(c.hashes, print.Hash)
			continue
		}
		if print.Length > best {
			best = print.Length
		}
	}
	return best > 0, best
}

func (c *prefixCache) Remember(prints []prefix.Fingerprint) {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, print := range prints {
		c.hashes[print.Hash] = now
	}
}

func (c *prefixCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.hashes)
}

func (r requestBody) PrefixText() string {
	if len(r.Messages) > 0 {
		var b strings.Builder
		for _, msg := range r.Messages {
			b.WriteString(msg.Content)
			b.WriteByte('\n')
		}
		return b.String()
	}

	switch prompt := r.Prompt.(type) {
	case string:
		return prompt
	case []any:
		var b strings.Builder
		for _, part := range prompt {
			b.WriteString(fmt.Sprint(part))
			b.WriteByte('\n')
		}
		return b.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(prompt)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	if millis, err := strconv.Atoi(value); err == nil {
		return time.Duration(millis) * time.Millisecond
	}
	return fallback
}
