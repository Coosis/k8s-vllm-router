package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Coosis/k8s-vllm-router/internal/prefix"
)

func TestSimulateWarmsPrefixCache(t *testing.T) {
	srv := &server{
		backendID: "test-backend",
		cache:     newPrefixCache(time.Minute),
		latency: latencyConfig{
			baseLatency:        0,
			coldPrefillPerWord: time.Millisecond,
			warmPrefillPerWord: 0,
			decodePerToken:     0,
			maxSleep:           time.Minute,
		},
	}

	body := requestBody{
		Messages: []message{{Role: "user", Content: repeatedWords(64)}},
	}

	cold := srv.simulate(body)
	if cold.CacheHit {
		t.Fatal("first request should be cold")
	}
	if cold.TotalSleep <= 0 {
		t.Fatalf("cold sleep = %s, want positive", cold.TotalSleep)
	}

	warm := srv.simulate(body)
	if !warm.CacheHit {
		t.Fatal("second request should hit cache")
	}
	if warm.MatchLength == 0 {
		t.Fatal("warm request should report a matched prefix length")
	}
	if warm.TotalSleep >= cold.TotalSleep {
		t.Fatalf("warm sleep = %s, want less than cold sleep %s", warm.TotalSleep, cold.TotalSleep)
	}
}

func TestPrefixCacheExpires(t *testing.T) {
	cache := newPrefixCache(time.Nanosecond)
	body := requestBody{Messages: []message{{Content: repeatedWords(64)}}}
	prints := prefix.Fingerprints(body.PrefixText(), cachePrefixLengths)

	cache.Remember(prints)
	time.Sleep(time.Millisecond)

	hit, matchLength := cache.Lookup(prints)
	if hit {
		t.Fatalf("expired cache hit = true, match length = %d", matchLength)
	}
}

func repeatedWords(n int) string {
	return strings.Repeat("word ", n)
}
