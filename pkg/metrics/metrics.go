package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP Metrics
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Current number of HTTP requests being processed",
		},
	)

	// Chat Service Metrics
	MessagesQueued = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "chat_messages_queued_total",
			Help: "Total number of messages queued for delivery",
		},
	)

	MessagesSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "chat_messages_sent_total",
			Help: "Total number of messages successfully sent to Kafka",
		},
	)

	MessagesFailed = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "chat_messages_failed_total",
			Help: "Total number of messages that failed to send",
		},
	)

	MessagesDropped = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "chat_messages_dropped_total",
			Help: "Total number of messages dropped due to buffer overflow",
		},
	)

	MessageBufferSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "chat_message_buffer_size",
			Help: "Current number of messages in the buffer",
		},
	)

	MessageDeliveryLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "chat_message_delivery_latency_seconds",
			Help:    "Time from message creation to Kafka delivery",
			Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
	)

	KafkaBatchSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "chat_kafka_batch_size",
			Help:    "Number of messages in each Kafka batch",
			Buckets: []float64{1, 5, 10, 25, 50, 100},
		},
	)

	// SSE Connection Metrics
	SSEConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "sse_connections_active",
			Help: "Current number of active SSE connections",
		},
	)

	SSEConnectionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "sse_connections_total",
			Help: "Total number of SSE connections established",
		},
	)

	SSEConnectionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "sse_connection_duration_seconds",
			Help:    "Duration of SSE connections",
			Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600},
		},
	)

	SSEReconnections = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "sse_reconnections_total",
			Help: "Total number of SSE reconnection attempts",
		},
	)

	// Session Metrics
	SessionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "sessions_active",
			Help: "Current number of active sessions",
		},
	)

	SessionsCreated = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "sessions_created_total",
			Help: "Total number of sessions created",
		},
	)

	SessionsExpired = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "sessions_expired_total",
			Help: "Total number of sessions that expired",
		},
	)

	SessionRenewalsFailed = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "session_renewals_failed_total",
			Help: "Total number of failed session renewal attempts",
		},
	)

	// Friend Service Metrics
	FriendRequestsSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "friend_requests_sent_total",
			Help: "Total number of friend requests sent",
		},
	)

	FriendRequestsAccepted = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "friend_requests_accepted_total",
			Help: "Total number of friend requests accepted",
		},
	)

	FriendRequestsRejected = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "friend_requests_rejected_total",
			Help: "Total number of friend requests rejected",
		},
	)

	// Database Metrics
	DatabaseQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "database_query_duration_seconds",
			Help:    "Database query execution time",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2},
		},
		[]string{"query", "status"},
	)

	DatabaseConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "database_connections_active",
			Help: "Current number of active database connections",
		},
	)

	DatabaseConnectionsIdle = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "database_connections_idle",
			Help: "Current number of idle database connections",
		},
	)

	// Redis Metrics
	RedisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redis_operation_duration_seconds",
			Help:    "Redis operation execution time",
			Buckets: []float64{.0001, .0005, .001, .005, .01, .025, .05, .1},
		},
		[]string{"operation", "status"},
	)

	RedisConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "redis_connections_active",
			Help: "Current number of active Redis connections",
		},
	)

	RedisCacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "redis_cache_hits_total",
			Help: "Total number of Redis cache hits",
		},
	)

	RedisCacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "redis_cache_misses_total",
			Help: "Total number of Redis cache misses",
		},
	)

	// Rate Limiting Metrics
	RateLimitExceeded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_exceeded_total",
			Help: "Total number of rate limit violations",
		},
		[]string{"endpoint"},
	)

	// Authentication Metrics
	LoginAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "login_attempts_total",
			Help: "Total number of login attempts",
		},
		[]string{"status"}, // success, failed
	)

	RegistrationsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registrations_total",
			Help: "Total number of user registrations",
		},
	)

	// Error Metrics
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "errors_total",
			Help: "Total number of errors by type",
		},
		[]string{"type", "code"},
	)

	// System Metrics
	SystemInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "system_info",
			Help: "System information",
		},
		[]string{"version", "go_version", "start_time"},
	)
)

// IncrementMessagesQueued increments the messages queued counter
func IncrementMessagesQueued() {
	MessagesQueued.Inc()
}

// IncrementMessagesSent increments the messages sent counter
func IncrementMessagesSent() {
	MessagesSent.Inc()
}

// IncrementMessagesFailed increments the messages failed counter
func IncrementMessagesFailed() {
	MessagesFailed.Inc()
}

// IncrementMessagesDropped increments the messages dropped counter
func IncrementMessagesDropped() {
	MessagesDropped.Inc()
}

// SetMessageBufferSize sets the current message buffer size
func SetMessageBufferSize(size int) {
	MessageBufferSize.Set(float64(size))
}

// RecordMessageDeliveryLatency records message delivery latency
func RecordMessageDeliveryLatency(seconds float64) {
	MessageDeliveryLatency.Observe(seconds)
}

// RecordKafkaBatchSize records Kafka batch size
func RecordKafkaBatchSize(size int) {
	KafkaBatchSize.Observe(float64(size))
}

// SSE Connection Helpers
func IncrementSSEConnections() {
	SSEConnectionsActive.Inc()
	SSEConnectionsTotal.Inc()
}

func DecrementSSEConnections() {
	SSEConnectionsActive.Dec()
}

func RecordSSEConnectionDuration(seconds float64) {
	SSEConnectionDuration.Observe(seconds)
}

func IncrementSSEReconnections() {
	SSEReconnections.Inc()
}

// Session Helpers
func SetSessionsActive(count int) {
	SessionsActive.Set(float64(count))
}

func IncrementSessionsCreated() {
	SessionsCreated.Inc()
}

func IncrementSessionsExpired() {
	SessionsExpired.Inc()
}

func IncrementSessionRenewalsFailed() {
	SessionRenewalsFailed.Inc()
}

// Friend Service Helpers
func IncrementFriendRequestsSent() {
	FriendRequestsSent.Inc()
}

func IncrementFriendRequestsAccepted() {
	FriendRequestsAccepted.Inc()
}

func IncrementFriendRequestsRejected() {
	FriendRequestsRejected.Inc()
}

// Database Helpers
func RecordDatabaseQuery(query string, duration float64, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	DatabaseQueryDuration.WithLabelValues(query, status).Observe(duration)
}

// Redis Helpers
func RecordRedisOperation(operation string, duration float64, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	RedisOperationDuration.WithLabelValues(operation, status).Observe(duration)
}

func IncrementRedisCacheHits() {
	RedisCacheHits.Inc()
}

func IncrementRedisCacheMisses() {
	RedisCacheMisses.Inc()
}

// Rate Limiting Helpers
func IncrementRateLimitExceeded(endpoint string) {
	RateLimitExceeded.WithLabelValues(endpoint).Inc()
}

// Auth Helpers
func RecordLoginAttempt(success bool) {
	status := "success"
	if !success {
		status = "failed"
	}
	LoginAttemptsTotal.WithLabelValues(status).Inc()
}

func IncrementRegistrations() {
	RegistrationsTotal.Inc()
}

// Error Helpers
func RecordError(errorType, errorCode string) {
	ErrorsTotal.WithLabelValues(errorType, errorCode).Inc()
}
