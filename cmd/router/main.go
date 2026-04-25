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
	"github.com/Coosis/k8s-vllm-router/internal/config"
	"github.com/Coosis/k8s-vllm-router/internal/router"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.FromEnv()
	registry := prometheus.NewRegistry()

	rt, err := router.New(router.Options{
		Policy:   cfg.Policy,
		Backends: cfg.Backends,
		Logger:   logger,
		Register: registry,
	})
	if err != nil {
		logger.Error("failed to create router", "error", err)
		os.Exit(1)
	}

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
