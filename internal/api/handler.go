package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Coosis/k8s-vllm-router/internal/router"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewHandler(rt *router.Router, reg *prometheus.Registry, logger *slog.Logger) http.Handler {
	reg.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if !rt.Ready(ctx) {
			http.Error(w, "no healthy backends", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/v1/chat/completions", forward(rt, logger))
	mux.HandleFunc("/v1/completions", forward(rt, logger))
	return mux
}

func forward(rt *router.Router, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if written, err := rt.Forward(w, r); err != nil {
			logger.Error("request forwarding failed", "error", err)
			if !written {
				http.Error(w, err.Error(), http.StatusBadGateway)
			}
		}
	}
}
