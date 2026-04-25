package config

import (
	"os"
	"strings"

	"github.com/Coosis/k8s-vllm-router/internal/backend"
)

var (
	POLICY_CACHE_AWARE = "cache_aware"
)

type Config struct {
	ListenAddr string
	Policy     string
	Backends   []backend.Endpoint
}

func FromEnv() Config {
	return Config{
		ListenAddr: getenv("ROUTER_ADDR", ":8080"),
		Policy:     getenv("ROUTER_POLICY", POLICY_CACHE_AWARE),
		Backends:   parseBackends(os.Getenv("BACKENDS")),
	}
}

func parseBackends(raw string) []backend.Endpoint {
	if raw == "" {
		return []backend.Endpoint{{ID: "mock-1", URL: "http://localhost:8081"}}
	}

	parts := strings.Split(raw, ",")
	backends := make([]backend.Endpoint, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, url, ok := strings.Cut(part, "=")
		if !ok {
			id = part
			url = part
		}
		backends = append(backends, backend.Endpoint{
			ID:  strings.TrimSpace(id),
			URL: strings.TrimRight(strings.TrimSpace(url), "/"),
		})
	}
	return backends
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
