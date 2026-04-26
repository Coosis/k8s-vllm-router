package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Package metrics will own Prometheus collectors for router decisions,
// backend state, TTFT, latency, and fallback reasons.

type Recorder struct {
	BackendInflight *prometheus.GaugeVec
	BackendSelected *prometheus.CounterVec
	BackendRequests *prometheus.CounterVec
	BackendErrors   *prometheus.CounterVec

	BackendLatencyEWMA *prometheus.GaugeVec
	BackendErrorEWMA   *prometheus.GaugeVec

	BackendHealthy *prometheus.GaugeVec

	PrefixCandidateMatches *prometheus.CounterVec
	PrefixMatchWarmth      *prometheus.HistogramVec
	PrefixMatchAgeSeconds  *prometheus.HistogramVec
	PrefixEvictions        prometheus.Counter

	RoutingDecisions *prometheus.CounterVec
}

func NewRecorder(reg prometheus.Registerer) *Recorder {
	return &Recorder{
		BackendInflight: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "router_backend_inflight_requests",
			Help: "Current in-flight requests by backend.",
		}, []string{"backend"}),
		BackendSelected: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "router_backend_selected_total",
			Help: "Total backend selections by backend.",
		}, []string{"backend"}),
		BackendRequests: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "router_backend_requests_total",
			Help: "Total forwarded backend requests by backend.",
		}, []string{"backend"}),
		BackendErrors: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "router_backend_errors_total",
			Help: "Total backend forwarding errors by backend.",
		}, []string{"backend"}),

		BackendLatencyEWMA: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "router_backend_latency_ewma_ms",
			Help: "Router-observed backend latency EWMA in milliseconds.",
		}, []string{"backend"}),
		BackendErrorEWMA: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "router_backend_error_ewma",
			Help: "Router-observed backend error EWMA.",
		}, []string{"backend"}),

		BackendHealthy: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "router_backend_healthy",
			Help: "Whether the backend is currently healthy (1) or unhealthy (0).",
		}, []string{"backend"}),

		PrefixCandidateMatches: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "router_prefix_candidate_matches_total",
			Help: "Total prefix candidate matches by matched prefix length.",
		}, []string{"match_length"}),
		PrefixMatchWarmth: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "router_prefix_match_warmth",
			Help:    "Decay-adjusted prefix match warmth used for routing.",
			Buckets: []float64{1, 8, 16, 32, 64, 128, 256, 512, 1024},
		}, []string{"match_length"}),
		PrefixMatchAgeSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "router_prefix_match_age_seconds",
			Help:    "Age of prefix candidate metadata used for routing.",
			Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600},
		}, []string{"match_length"}),
		PrefixEvictions: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "router_prefix_metadata_evictions_total",
			Help: "Total stale prefix metadata entries evicted opportunistically.",
		}),

		RoutingDecisions: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "router_routing_decisions_total",
			Help: "Total routing decisions by policy.",
		}, []string{"decision"}),
	}
}

func (r *Recorder) InitBackend(backend string) {
	r.BackendInflight.WithLabelValues(backend).Set(0)
	r.BackendSelected.WithLabelValues(backend).Add(0)
	r.BackendRequests.WithLabelValues(backend).Add(0)
	r.BackendErrors.WithLabelValues(backend).Add(0)
	r.BackendLatencyEWMA.WithLabelValues(backend).Set(0)
	r.BackendHealthy.WithLabelValues(backend).Set(0)
	r.BackendErrorEWMA.WithLabelValues(backend).Set(0)
}
