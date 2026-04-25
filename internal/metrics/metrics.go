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
	r.BackendErrorEWMA.WithLabelValues(backend).Set(0)
}
