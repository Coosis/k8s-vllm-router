package router

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Coosis/k8s-vllm-router/internal/backend"
	"github.com/Coosis/k8s-vllm-router/internal/config"
	"github.com/Coosis/k8s-vllm-router/internal/metrics"
	"github.com/Coosis/k8s-vllm-router/internal/prefix"
	"github.com/prometheus/client_golang/prometheus"
)

type Options struct {
	Policy                        string
	Backends                      []backend.Endpoint
	BackendHealthPath             string
	BackendHealthTimeout          time.Duration
	BackendHealthFailureThreshold uint64
	Logger                        *slog.Logger
	Register                      prometheus.Registerer
}

type Router struct {
	policy                 string
	backends               *atomic.Pointer[backend.BackendSnapshot]
	client                 backend.Client
	logger                 *slog.Logger
	metric                 *metrics.Recorder
	healthTimeout          time.Duration
	healthFailureThreshold uint64

	matcher *Matcher

	next atomic.Uint64
}

var ErrNoHealthyBackends = errors.New("no healthy backends")

func New(opts Options, matcher *Matcher) (*Router, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	backends := make([]*backend.BackendState, len(opts.Backends))
	for i, endpoint := range opts.Backends {
		backends[i] = backend.NewBackendState(endpoint)
	}

	var snapshot *atomic.Pointer[backend.BackendSnapshot]
	snapshot = &atomic.Pointer[backend.BackendSnapshot]{}
	snapshot.Store(backend.NewBackendSnapshot(backends))
	registerer := opts.Register
	if registerer == nil {
		return nil, errors.New("prometheus registerer is required")
	}
	recorder := metrics.NewRecorder(registerer)
	for _, state := range backends {
		recorder.InitBackend(state.Endpoint.ID)
	}

	return &Router{
		policy:                 opts.Policy,
		backends:               snapshot,
		client:                 backend.NewHTTPClient(opts.BackendHealthPath),
		logger:                 logger,
		metric:                 recorder,
		healthTimeout:          normalizedDuration(opts.BackendHealthTimeout, 5*time.Second),
		healthFailureThreshold: normalizedUint64(opts.BackendHealthFailureThreshold, 3),

		matcher: matcher,
	}, nil
}

func (r *Router) UpdateBackends(endpoints []backend.Endpoint) {
	current := r.backends.Load()
	nextStates := make([]*backend.BackendState, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if existing := current.ByID[endpoint.ID]; existing != nil && existing.Endpoint == endpoint {
			nextStates = append(nextStates, existing)
		} else {
			nextStates = append(nextStates, backend.NewBackendState(endpoint))
			r.metric.InitBackend(endpoint.ID)
		}
	}
	r.backends.Store(backend.NewBackendSnapshot(nextStates))
	r.logger.Info("backend snapshot updated", "count", len(nextStates))
}

func (r *Router) Ready(ctx context.Context) bool {
	snapshot := r.backends.Load()
	for _, backend := range snapshot.List {
		if err := r.client.Health(ctx, backend.Endpoint); err == nil {
			return true
		}
	}
	return false
}

// Forward returns whether a response has already been written.
func (r *Router) Forward(w http.ResponseWriter, req *http.Request) (bool, error) {
	endpoint, prints, release, err := r.selectAndReserveBackend(req)
	if err != nil {
		return false, err
	}
	defer release()
	r.logger.Info("selected backend", "backend", endpoint.Endpoint.ID, "policy", r.policy, "inflight", endpoint.Inflight.Load())

	r.metric.BackendRequests.WithLabelValues(endpoint.Endpoint.ID).Inc()
	r.metric.BackendSelected.WithLabelValues(endpoint.Endpoint.ID).Inc()
	r.metric.BackendInflight.WithLabelValues(endpoint.Endpoint.ID).Set(float64(endpoint.Inflight.Load()))
	r.metric.RoutingDecisions.WithLabelValues(r.policy).Inc()
	start := time.Now()
	written, err := r.client.Forward(w, req, endpoint.Endpoint)
	elapsed := time.Since(start)
	endpoint.RequestsTotal.Add(1)
	endpoint.LatencyEWMA.Observe(float64(elapsed.Milliseconds()))
	if err == nil {
		endpoint.ErrorEWMA.Observe(0)
		r.matcher.Remember(endpoint.Endpoint.ID, prints)
	} else {
		endpoint.ErrorsTotal.Add(1)
		endpoint.ErrorEWMA.Observe(1)
		r.metric.BackendErrors.WithLabelValues(endpoint.Endpoint.ID).Inc()
	}
	r.metric.BackendErrorEWMA.WithLabelValues(endpoint.Endpoint.ID).Set(endpoint.ErrorEWMA.Get())
	r.metric.BackendLatencyEWMA.WithLabelValues(endpoint.Endpoint.ID).Set(endpoint.LatencyEWMA.Get())
	return written, err
}

func (r *Router) selectAndReserveBackend(req *http.Request) (*backend.BackendState, []prefix.Fingerprint, func(), error) {
	endpoint, prints, err := r.selectBackend(req)
	if err != nil {
		return nil, nil, nil, err
	}
	counter := endpoint.Inflight
	counter.Add(1)
	endpoint.SelectedTotal.Add(1)
	r.metric.BackendInflight.WithLabelValues(endpoint.Endpoint.ID).Inc()
	return endpoint, prints, func() {
		counter.Add(-1)
		r.metric.BackendInflight.WithLabelValues(endpoint.Endpoint.ID).Dec()
	}, nil
}

func (r *Router) selectBackend(req *http.Request) (*backend.BackendState, []prefix.Fingerprint, error) {
	snapshot := r.backends.Load()
	if len(snapshot.List) == 0 {
		return nil, nil, errors.New("no backends configured")
	}

	if r.policy == config.POLICY_CACHE_AWARE {
		return r.cacheAwareSelect(req, snapshot)
	}

	start := int(r.next.Add(1) - 1)
	for i := range snapshot.List {
		backend := snapshot.List[(start+i)%len(snapshot.List)]
		if isSelectable(backend) {
			return backend, nil, nil
		}
	}
	return nil, nil, ErrNoHealthyBackends
}

func (r *Router) StartHealthCheck(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		r.checkBackendsHealth(ctx)
		for {
			select {
			case <-ticker.C:
				r.checkBackendsHealth(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (r *Router) checkBackendsHealth(ctx context.Context) {
	snapshot := r.backends.Load()
	for _, backend := range snapshot.List {
		ckctx, cancel := context.WithTimeout(ctx, r.healthTimeout)
		err := r.client.Health(ckctx, backend.Endpoint)
		cancel()

		if err == nil {
			backend.ConsecutiveHealthFailures.Store(0)
			backend.Healthy.Store(true)
			r.metric.BackendHealthy.WithLabelValues(backend.Endpoint.ID).Set(1)
			continue
		}

		failures := backend.ConsecutiveHealthFailures.Add(1)
		r.logger.Warn(
			"backend health check failed",
			"backend", backend.Endpoint.ID,
			"url", backend.Endpoint.URL,
			"failures", failures,
			"threshold", r.healthFailureThreshold,
			"error", err,
		)
		if failures >= r.healthFailureThreshold {
			backend.Healthy.Store(false)
			r.metric.BackendHealthy.WithLabelValues(backend.Endpoint.ID).Set(0)
		}
	}
}

func isSelectable(backend *backend.BackendState) bool {
	return backend != nil && backend.Healthy.Load()
}

func normalizedDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func normalizedUint64(value uint64, fallback uint64) uint64 {
	if value == 0 {
		return fallback
	}
	return value
}
