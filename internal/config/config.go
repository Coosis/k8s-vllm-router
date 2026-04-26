package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Coosis/k8s-vllm-router/internal/backend"
)

var (
	POLICY_CACHE_AWARE = "cache_aware"
)

type Config struct {
	ListenAddr                    string
	Policy                        string
	Backends                      []backend.Endpoint
	HealthCheckInterval           time.Duration
	BackendHealthTimeout          time.Duration
	BackendHealthFailureThreshold uint64
	BackendHealthPath             string

	Expiry ExpiryConfig

	Discovery DiscoveryConfig
}

type DiscoveryConfig struct {
	Service   string
	Namespace string
	PortName  string
	Scheme    string
	Interval  time.Duration
}

func FromEnv() Config {
	discoveryService := os.Getenv("DISCOVERY_SERVICE")
	return Config{
		ListenAddr:                    getenv("ROUTER_ADDR", ":8080"),
		Policy:                        getenv("ROUTER_POLICY", POLICY_CACHE_AWARE),
		Backends:                      parseBackends(os.Getenv("BACKENDS"), discoveryService == ""),
		HealthCheckInterval:           getenvDuration("HEALTH_CHECK_INTERVAL", 5*time.Second),
		BackendHealthTimeout:          getenvDuration("BACKEND_HEALTH_TIMEOUT", 5*time.Second),
		BackendHealthFailureThreshold: getenvUint64("BACKEND_HEALTH_FAILURE_THRESHOLD", 3),
		BackendHealthPath:             getenv("BACKEND_HEALTH_PATH", "/healthz"),
		Expiry:                        NewExpiryConfig(),
		Discovery: DiscoveryConfig{
			Service:   discoveryService,
			Namespace: getenv("POD_NAMESPACE", "default"),
			PortName:  os.Getenv("DISCOVERY_PORT_NAME"),
			Scheme:    getenv("DISCOVERY_SCHEME", "http"),
			Interval:  getenvDuration("DISCOVERY_INTERVAL", 5*time.Second),
		},
	}
}

func parseBackends(raw string, useDefault bool) []backend.Endpoint {
	if raw == "" {
		if !useDefault {
			return nil
		}
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

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}

func getenvUint64(key string, fallback uint64) uint64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
