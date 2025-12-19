package metrics

import (
	"context"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// DatabaseStatsCollector collects database connection pool statistics
type DatabaseStatsCollector struct {
	db *sql.DB

	maxOpenConnections *prometheus.Desc
	openConnections    *prometheus.Desc
	inUse              *prometheus.Desc
	idle               *prometheus.Desc
	waitCount          *prometheus.Desc
	waitDuration       *prometheus.Desc
	maxIdleClosed      *prometheus.Desc
	maxLifetimeClosed  *prometheus.Desc
}

// NewDatabaseStatsCollector creates a new database stats collector
func NewDatabaseStatsCollector(db *sql.DB) *DatabaseStatsCollector {
	return &DatabaseStatsCollector{
		db: db,
		maxOpenConnections: prometheus.NewDesc(
			"database_max_open_connections",
			"Maximum number of open connections to the database",
			nil, nil,
		),
		openConnections: prometheus.NewDesc(
			"database_open_connections",
			"The number of established connections both in use and idle",
			nil, nil,
		),
		inUse: prometheus.NewDesc(
			"database_connections_in_use",
			"The number of connections currently in use",
			nil, nil,
		),
		idle: prometheus.NewDesc(
			"database_connections_idle",
			"The number of idle connections",
			nil, nil,
		),
		waitCount: prometheus.NewDesc(
			"database_wait_count_total",
			"The total number of connections waited for",
			nil, nil,
		),
		waitDuration: prometheus.NewDesc(
			"database_wait_duration_seconds_total",
			"The total time blocked waiting for a new connection",
			nil, nil,
		),
		maxIdleClosed: prometheus.NewDesc(
			"database_max_idle_closed_total",
			"The total number of connections closed due to SetMaxIdleConns",
			nil, nil,
		),
		maxLifetimeClosed: prometheus.NewDesc(
			"database_max_lifetime_closed_total",
			"The total number of connections closed due to SetConnMaxLifetime",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector
func (c *DatabaseStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.maxOpenConnections
	ch <- c.openConnections
	ch <- c.inUse
	ch <- c.idle
	ch <- c.waitCount
	ch <- c.waitDuration
	ch <- c.maxIdleClosed
	ch <- c.maxLifetimeClosed
}

// Collect implements prometheus.Collector
func (c *DatabaseStatsCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.db.Stats()

	ch <- prometheus.MustNewConstMetric(
		c.maxOpenConnections,
		prometheus.GaugeValue,
		float64(stats.MaxOpenConnections),
	)
	ch <- prometheus.MustNewConstMetric(
		c.openConnections,
		prometheus.GaugeValue,
		float64(stats.OpenConnections),
	)
	ch <- prometheus.MustNewConstMetric(
		c.inUse,
		prometheus.GaugeValue,
		float64(stats.InUse),
	)
	ch <- prometheus.MustNewConstMetric(
		c.idle,
		prometheus.GaugeValue,
		float64(stats.Idle),
	)
	ch <- prometheus.MustNewConstMetric(
		c.waitCount,
		prometheus.CounterValue,
		float64(stats.WaitCount),
	)
	ch <- prometheus.MustNewConstMetric(
		c.waitDuration,
		prometheus.CounterValue,
		stats.WaitDuration.Seconds(),
	)
	ch <- prometheus.MustNewConstMetric(
		c.maxIdleClosed,
		prometheus.CounterValue,
		float64(stats.MaxIdleClosed),
	)
	ch <- prometheus.MustNewConstMetric(
		c.maxLifetimeClosed,
		prometheus.CounterValue,
		float64(stats.MaxLifetimeClosed),
	)
}

// RedisStatsCollector collects Redis statistics
type RedisStatsCollector struct {
	client *redis.Client

	poolHits       *prometheus.Desc
	poolMisses     *prometheus.Desc
	poolTimeouts   *prometheus.Desc
	poolTotalConns *prometheus.Desc
	poolIdleConns  *prometheus.Desc
	poolStaleConns *prometheus.Desc
}

// NewRedisStatsCollector creates a new Redis stats collector
func NewRedisStatsCollector(client *redis.Client) *RedisStatsCollector {
	return &RedisStatsCollector{
		client: client,
		poolHits: prometheus.NewDesc(
			"redis_pool_hits_total",
			"Number of times free connection was found in the pool",
			nil, nil,
		),
		poolMisses: prometheus.NewDesc(
			"redis_pool_misses_total",
			"Number of times free connection was NOT found in the pool",
			nil, nil,
		),
		poolTimeouts: prometheus.NewDesc(
			"redis_pool_timeouts_total",
			"Number of times a wait timeout occurred",
			nil, nil,
		),
		poolTotalConns: prometheus.NewDesc(
			"redis_pool_total_connections",
			"Number of total connections in the pool",
			nil, nil,
		),
		poolIdleConns: prometheus.NewDesc(
			"redis_pool_idle_connections",
			"Number of idle connections in the pool",
			nil, nil,
		),
		poolStaleConns: prometheus.NewDesc(
			"redis_pool_stale_connections_total",
			"Number of stale connections removed from the pool",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector
func (c *RedisStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.poolHits
	ch <- c.poolMisses
	ch <- c.poolTimeouts
	ch <- c.poolTotalConns
	ch <- c.poolIdleConns
	ch <- c.poolStaleConns
}

// Collect implements prometheus.Collector
func (c *RedisStatsCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.client.PoolStats()

	ch <- prometheus.MustNewConstMetric(
		c.poolHits,
		prometheus.CounterValue,
		float64(stats.Hits),
	)
	ch <- prometheus.MustNewConstMetric(
		c.poolMisses,
		prometheus.CounterValue,
		float64(stats.Misses),
	)
	ch <- prometheus.MustNewConstMetric(
		c.poolTimeouts,
		prometheus.CounterValue,
		float64(stats.Timeouts),
	)
	ch <- prometheus.MustNewConstMetric(
		c.poolTotalConns,
		prometheus.GaugeValue,
		float64(stats.TotalConns),
	)
	ch <- prometheus.MustNewConstMetric(
		c.poolIdleConns,
		prometheus.GaugeValue,
		float64(stats.IdleConns),
	)
	ch <- prometheus.MustNewConstMetric(
		c.poolStaleConns,
		prometheus.CounterValue,
		float64(stats.StaleConns),
	)
}

// ChatServiceStatsCollector collects chat service internal metrics
type ChatServiceStatsCollector struct {
	getMetrics func() map[string]int64

	messagesQueued  *prometheus.Desc
	messagesSent    *prometheus.Desc
	messagesFailed  *prometheus.Desc
	messagesDropped *prometheus.Desc
}

// NewChatServiceStatsCollector creates a new chat service stats collector
func NewChatServiceStatsCollector(getMetrics func() map[string]int64) *ChatServiceStatsCollector {
	return &ChatServiceStatsCollector{
		getMetrics: getMetrics,
		messagesQueued: prometheus.NewDesc(
			"chat_service_messages_queued",
			"Total messages queued in chat service",
			nil, nil,
		),
		messagesSent: prometheus.NewDesc(
			"chat_service_messages_sent",
			"Total messages sent by chat service",
			nil, nil,
		),
		messagesFailed: prometheus.NewDesc(
			"chat_service_messages_failed",
			"Total messages failed in chat service",
			nil, nil,
		),
		messagesDropped: prometheus.NewDesc(
			"chat_service_messages_dropped",
			"Total messages dropped by chat service",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector
func (c *ChatServiceStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.messagesQueued
	ch <- c.messagesSent
	ch <- c.messagesFailed
	ch <- c.messagesDropped
}

// Collect implements prometheus.Collector
func (c *ChatServiceStatsCollector) Collect(ch chan<- prometheus.Metric) {
	metrics := c.getMetrics()

	ch <- prometheus.MustNewConstMetric(
		c.messagesQueued,
		prometheus.CounterValue,
		float64(metrics["queued"]),
	)
	ch <- prometheus.MustNewConstMetric(
		c.messagesSent,
		prometheus.CounterValue,
		float64(metrics["sent"]),
	)
	ch <- prometheus.MustNewConstMetric(
		c.messagesFailed,
		prometheus.CounterValue,
		float64(metrics["failed"]),
	)
	ch <- prometheus.MustNewConstMetric(
		c.messagesDropped,
		prometheus.CounterValue,
		float64(metrics["dropped"]),
	)
}

// RegisterCollectors registers all custom collectors
func RegisterCollectors(db *sql.DB, redisClient *redis.Client, chatMetrics func() map[string]int64) {
	if db != nil {
		prometheus.MustRegister(NewDatabaseStatsCollector(db))
	}

	if redisClient != nil {
		prometheus.MustRegister(NewRedisStatsCollector(redisClient))
	}

	if chatMetrics != nil {
		prometheus.MustRegister(NewChatServiceStatsCollector(chatMetrics))
	}
}

// UpdateSessionCount periodically updates the active session count
func UpdateSessionCount(ctx context.Context, redisClient *redis.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Count active sessions in Redis
			keys, err := redisClient.Keys(ctx, "session:*").Result()
			if err == nil {
				SetSessionsActive(len(keys))
			}
		}
	}
}
