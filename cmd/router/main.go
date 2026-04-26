package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Coosis/k8s-vllm-router/internal/api"
	"github.com/Coosis/k8s-vllm-router/internal/backend"
	"github.com/Coosis/k8s-vllm-router/internal/config"
	"github.com/Coosis/k8s-vllm-router/internal/discovery"
	"github.com/Coosis/k8s-vllm-router/internal/router"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.FromEnv()
	registry := prometheus.NewRegistry()
	matcher := router.NewMatcher(cfg.Expiry)

	rt, err := router.New(router.Options{
		Policy:                        cfg.Policy,
		Backends:                      cfg.Backends,
		BackendHealthPath:             cfg.BackendHealthPath,
		BackendHealthTimeout:          cfg.BackendHealthTimeout,
		BackendHealthFailureThreshold: cfg.BackendHealthFailureThreshold,
		Logger:                        logger,
		Register:                      registry,
	}, matcher)
	if err != nil {
		logger.Error("failed to create router", "error", err)
		os.Exit(1)
	}
	if cfg.Discovery.Service != "" {
		discoverer, err := discovery.NewEndpointSliceDiscoverer(discovery.EndpointSliceOptions{
			Namespace: cfg.Discovery.Namespace,
			Service:   cfg.Discovery.Service,
			PortName:  cfg.Discovery.PortName,
			Scheme:    cfg.Discovery.Scheme,
			Logger:    logger,
		})
		if err != nil {
			logger.Error("failed to create endpointslice discoverer", "error", err)
			os.Exit(1)
		}
		startDiscovery(ctx, discoverer, rt, cfg.Discovery.Interval, logger)
	}
	rt.StartHealthCheck(ctx, cfg.HealthCheckInterval)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.NewHandler(rt, registry, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("router listening", "addr", cfg.ListenAddr, "policy", cfg.Policy, "backends", cfg.Backends)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("router failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("router shutdown failed", "error", err)
	}
}

type backendDiscoverer interface {
	Discover(context.Context) ([]backend.Endpoint, error)
}

func startDiscovery(
	ctx context.Context,
	discoverer backendDiscoverer,
	rt *router.Router,
	interval time.Duration,
	logger *slog.Logger,
) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		discoverOnce(ctx, discoverer, rt, logger)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				discoverOnce(ctx, discoverer, rt, logger)
			}
		}
	}()
}

func discoverOnce(
	ctx context.Context,
	discoverer backendDiscoverer,
	rt *router.Router,
	logger *slog.Logger,
) {
	endpoints, err := discoverer.Discover(ctx)
	if err != nil {
		logger.Error("backend discovery failed", "error", err)
		return
	}
	rt.UpdateBackends(endpoints)
}
