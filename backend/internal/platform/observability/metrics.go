// Package observability provides process-local metrics and tracing assembly.
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Metrics owns an isolated Prometheus registry. No metric is registered in the
// process-global registry, which keeps API and Worker tests and processes independent.
type Metrics struct {
	registry *prometheus.Registry
	service  string

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	httpPanics   *prometheus.CounterVec
	publish      *prometheus.HistogramVec

	outboxEvents             *prometheus.GaugeVec
	outboxOldestAge          prometheus.Gauge
	projectionStates         *prometheus.GaugeVec
	importJobs               *prometheus.GaugeVec
	importJobDurationSum     *prometheus.GaugeVec
	importJobDurationCount   *prometheus.GaugeVec
	importStages             *prometheus.GaugeVec
	importStageDurationSum   *prometheus.GaugeVec
	importStageDurationCount *prometheus.GaugeVec
	aiRequests               *prometheus.GaugeVec
	aiTokens                 *prometheus.GaugeVec
	aiLatencySum             *prometheus.GaugeVec
}

// NewMetrics creates a registry with Go/process collectors and application metrics.
func NewMetrics(service string) *Metrics {
	const namespace = "wiki"
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		service:  service,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "http", Name: "requests_total",
			Help: "HTTP requests partitioned by method, chi route pattern, and status class.",
		}, []string{"service", "method", "route", "status_class"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "http", Name: "request_duration_seconds",
			Help:    "HTTP request latency partitioned by method and chi route pattern.",
			Buckets: prometheus.DefBuckets,
		}, []string{"service", "method", "route"}),
		httpPanics: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "http", Name: "panics_total",
			Help: "Recovered HTTP panics partitioned by chi route pattern.",
		}, []string{"service", "route"}),
		publish: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "page", Name: "publish_duration_seconds",
			Help:    "Revision publish latency partitioned by success or failure.",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"service", "result"}),
		outboxEvents: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "outbox", Name: "events",
			Help: "Current Outbox event count by bounded status.",
		}, []string{"service", "status"}),
		outboxOldestAge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "outbox", Name: "oldest_backlog_age_seconds",
			Help:        "Age of the oldest pending or claimed Outbox event.",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		projectionStates: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "projection", Name: "states",
			Help: "Current Projection state count by bounded health state.",
		}, []string{"service", "state"}),
		importJobs: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "importer", Name: "jobs",
			Help: "Current import job count by bounded status.",
		}, []string{"service", "status"}),
		importJobDurationSum: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "importer", Name: "job_duration_seconds_sum",
			Help: "Cumulative duration of finished import jobs by bounded status.",
		}, []string{"service", "status"}),
		importJobDurationCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "importer", Name: "job_duration_seconds_count",
			Help: "Number of finished import jobs included in the duration sum.",
		}, []string{"service", "status"}),
		importStages: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "importer", Name: "stages",
			Help: "Current import stage run count by bounded stage and status.",
		}, []string{"service", "stage", "status"}),
		importStageDurationSum: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "importer", Name: "stage_duration_seconds_sum",
			Help: "Cumulative duration of finished import stages.",
		}, []string{"service", "stage", "status"}),
		importStageDurationCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "importer", Name: "stage_duration_seconds_count",
			Help: "Number of finished import stages included in the duration sum.",
		}, []string{"service", "stage", "status"}),
		aiRequests: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "ai", Name: "requests",
			Help: "Persisted AI request usage rows by bounded result status.",
		}, []string{"service", "status"}),
		aiTokens: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "ai", Name: "tokens",
			Help: "Persisted AI token usage by direction and bounded result status.",
		}, []string{"service", "direction", "status"}),
		aiLatencySum: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "ai", Name: "latency_seconds_sum",
			Help: "Cumulative persisted AI request latency by bounded result status.",
		}, []string{"service", "status"}),
	}
	m.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.httpRequests, m.httpDuration, m.httpPanics, m.publish,
		m.outboxEvents, m.outboxOldestAge, m.projectionStates,
		m.importJobs, m.importJobDurationSum, m.importJobDurationCount,
		m.importStages, m.importStageDurationSum, m.importStageDurationCount,
		m.aiRequests, m.aiTokens, m.aiLatencySum,
	)
	m.initializeBoundedLabels(service)
	return m
}

func (m *Metrics) initializeBoundedLabels(service string) {
	for _, status := range []string{"pending", "claimed", "retrying", "dead", "backlog"} {
		m.outboxEvents.WithLabelValues(service, status).Set(0)
	}
	for _, state := range []string{"error", "stale"} {
		m.projectionStates.WithLabelValues(service, state).Set(0)
	}
	for _, status := range []string{"queued", "running", "succeeded", "failed", "cancelled"} {
		m.importJobs.WithLabelValues(service, status).Set(0)
		m.importJobDurationSum.WithLabelValues(service, status).Set(0)
		m.importJobDurationCount.WithLabelValues(service, status).Set(0)
	}
	for _, status := range []string{"succeeded", "failed", "timeout", "invalid_output"} {
		m.aiRequests.WithLabelValues(service, status).Set(0)
		m.aiTokens.WithLabelValues(service, "input", status).Set(0)
		m.aiTokens.WithLabelValues(service, "output", status).Set(0)
		m.aiLatencySum.WithLabelValues(service, status).Set(0)
	}
}

// Handler exposes only this Metrics instance's registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// HTTPMiddleware records a bounded chi route pattern rather than the raw path.
func (m *Metrics) HTTPMiddleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := otel.Tracer("github.com/anby/wiki/backend/http").Start(
				ctx, r.Method, trace.WithSpanKind(trace.SpanKindServer),
			)
			defer span.End()
			next.ServeHTTP(ww, r.WithContext(ctx))
			route := RoutePattern(r)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			span.SetName(r.Method + " " + route)
			span.SetAttributes(
				attribute.String("http.request.method", r.Method),
				attribute.String("http.route", route),
				attribute.Int("http.response.status_code", status),
			)
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(status))
			}
			m.httpRequests.WithLabelValues(service, r.Method, route, statusClass(status)).Inc()
			m.httpDuration.WithLabelValues(service, r.Method, route).Observe(time.Since(start).Seconds())
		})
	}
}

// ObservePanic increments the recovered panic counter.
func (m *Metrics) ObservePanic(service string, r *http.Request) {
	m.httpPanics.WithLabelValues(service, RoutePattern(r)).Inc()
}

// ObservePublish implements page.PublishObserver.
func (m *Metrics) ObservePublish(duration time.Duration, err error) {
	result := "success"
	if err != nil {
		result = "failure"
	}
	m.publish.WithLabelValues(m.service, result).Observe(duration.Seconds())
}

// RoutePattern returns a bounded chi route template and never the raw URL.
func RoutePattern(r *http.Request) string {
	if route := chi.RouteContext(r.Context()).RoutePattern(); route != "" {
		return route
	}
	return "unmatched"
}

func statusClass(status int) string {
	return strconv.Itoa(status/100) + "xx"
}
