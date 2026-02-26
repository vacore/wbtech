package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP layer
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "order_service",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests by method, path, and status code.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "order_service",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency in seconds.",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"method", "path"},
	)

	// Kafka layer
	KafkaMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "order_service",
			Name:      "kafka_messages_total",
			Help:      "Kafka messages by outcome: processed, failed, dlq.",
		},
		[]string{"status"},
	)

	KafkaProcessDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "order_service",
			Name:      "kafka_process_duration_seconds",
			Help:      "Time to process a single Kafka message.",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2},
		},
	)

	// Cache layer
	CacheOps = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "order_service",
			Name:      "cache_operations_total",
			Help:      "Cache operations: hit, miss, eviction, expiration.",
		},
		[]string{"op"},
	)

	CacheSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "order_service",
			Name:      "cache_current_size",
			Help:      "Current number of items in cache.",
		},
	)

	// Database layer
	DBDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "order_service",
			Name:      "db_operation_duration_seconds",
			Help:      "Database operation latency.",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2},
		},
		[]string{"operation"},
	)
)

func RecordHTTP(method, path string, status int, duration time.Duration) {
	HTTPRequestsTotal.WithLabelValues(method, path, strconv.Itoa(status)).Inc()
	HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

func ObserveDB(operation string, start time.Time) {
	DBDuration.WithLabelValues(operation).Observe(time.Since(start).Seconds())
}
