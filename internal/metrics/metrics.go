// Package metrics exposes what the process is doing in a form Prometheus can
// scrape. Nothing in the layers below knows this package exists: it either
// receives numbers through a narrow interface or reads counters those layers
// already keep.
package metrics

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns a registry of its own rather than the library's package level
// default. Two tests can then each build a clean set without colliding, and
// nothing can register into this one from somewhere unrelated.
type Metrics struct {
	registry *prometheus.Registry

	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
	failures *prometheus.CounterVec
}

func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),

		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Requests answered, by route and status code.",
		}, []string{"method", "route", "status"}),

		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "How long requests took to answer.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),

		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Requests being answered right now.",
		}),

		// Status alone would put every refusal of a seat in with every other
		// conflict. The code is what separates losing a race for a seat from
		// replaying an idempotency key.
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "api_failures_total",
			Help: "Failed requests, by the error code the API answered with.",
		}, []string{"code"}),
	}

	m.registry.MustRegister(
		m.requests, m.duration, m.inFlight, m.failures,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return m
}

// Handler serves the scrape endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// RecordRequest is called once per request, after it has been answered. The
// route is the pattern the mux matched, never the path: one series per hold id
// is how a metrics store is brought down.
func (m *Metrics) RecordRequest(method, route string, status int, took time.Duration) {
	m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(method, route).Observe(took.Seconds())
}

// RecordFailure counts one refusal by the code the caller was given.
func (m *Metrics) RecordFailure(code string) {
	m.failures.WithLabelValues(code).Inc()
}

func (m *Metrics) RequestStarted() {
	m.inFlight.Inc()
}

func (m *Metrics) RequestFinished() {
	m.inFlight.Dec()
}

// StreamSource is a broker that already counts its own work, declared here for
// the consumer so the event package never learns what Prometheus is.
type StreamSource interface {
	Subscribers() int
	Dropped() int64
}

// ObserveStreams reads the broker's numbers at scrape time rather than having
// the broker push them. The broker already keeps both, and a value read on
// demand cannot go stale between updates.
func (m *Metrics) ObserveStreams(source StreamSource) error {
	return errors.Join(
		m.registry.Register(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "stream_subscribers",
			Help: "Clients watching an event stream right now.",
		}, func() float64 { return float64(source.Subscribers()) })),

		m.registry.Register(prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "stream_notices_dropped_total",
			Help: "Notices thrown away because a subscriber was not keeping up.",
		}, func() float64 { return float64(source.Dropped()) })),
	)
}

// HandoffSource is the seat handoff, which counts the offers it had to throw
// away.
type HandoffSource interface {
	Waiting() int
	Dropped() int64
}

// ObserveHandoff exposes how the waiting list handoff is coping. A dropped
// offer means a freed seat was never offered to the queue, which is the number
// that says the buffer or the worker count is wrong.
func (m *Metrics) ObserveHandoff(source HandoffSource) error {
	return errors.Join(
		m.registry.Register(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "handoff_seats_waiting",
			Help: "Freed seats queued up to be offered to the waiting list.",
		}, func() float64 { return float64(source.Waiting()) })),

		m.registry.Register(prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "handoff_offers_dropped_total",
			Help: "Freed seats that were never offered because the queue was full.",
		}, func() float64 { return float64(source.Dropped()) })),
	)
}
