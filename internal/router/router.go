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
	Policy   string
	Backends []backend.Endpoint
	Logger   *slog.Logger
	Register prometheus.Registerer
}

type Router struct {
	policy   string
	backends *atomic.Pointer[backend.BackendSnapshot]
	client   backend.Client
	logger   *slog.Logger
	metric   *metrics.Recorder

	matcher *Matcher

	next atomic.Uint64
}

func New(opts Options) (*Router, error) {
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
		policy:   opts.Policy,
		backends: snapshot,
		client:   backend.NewHTTPClient(),
		logger:   logger,
		metric:   recorder,

		matcher: NewMatcher(),
	}, nil
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

func (r *Router) Forward(w http.ResponseWriter, req *http.Request) error {
	endpoint, prints, release, err := r.selectAndReserveBackend(req)
	if err != nil {
		return err
	}
	defer release()
	r.logger.Info("selected backend", "backend", endpoint.Endpoint.ID, "policy", r.policy, "inflight", endpoint.Inflight.Load())

	r.metric.BackendRequests.WithLabelValues(endpoint.Endpoint.ID).Inc()
	r.metric.BackendSelected.WithLabelValues(endpoint.Endpoint.ID).Inc()
	r.metric.BackendInflight.WithLabelValues(endpoint.Endpoint.ID).Set(float64(endpoint.Inflight.Load()))
	r.metric.RoutingDecisions.WithLabelValues(r.policy).Inc()
	start := time.Now()
	err = r.client.Forward(w, req, endpoint.Endpoint)
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
	return err
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

	backend := snapshot.List[(r.next.Add(1)-1)%uint64(len(snapshot.List))]
	return backend, nil, nil
}
