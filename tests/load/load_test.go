package load

import (
	"context"
	"database/sql"
	"encoding/json"
	"exc6/config"
	"exc6/db"
	infraredis "exc6/infrastructure/redis"
	"exc6/pkg/logger"
	"exc6/server"
	_websocket "exc6/server/websocket"
	"exc6/services/calls"
	"exc6/services/chat"
	"exc6/services/friends"
	"exc6/services/groups"
	"exc6/services/sessions"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	fastws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testLogger *logger.Logger
)

func init() {
	// Initialize test logger
	testLogger = logger.New("./log/load_test.log")
	testLogger.SetLevel(logger.DEBUG)
}

// TestConcurrentUserLogins tests authentication under load
func TestConcurrentUserLogins(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	testLogger.Info("========================================")
	testLogger.Info("Starting Concurrent User Logins Test")
	testLogger.Info("========================================")

	const (
		numUsers       = 1000
		concurrency    = 50
		requestTimeout = 5 * time.Second
	)

	testLogger.WithFields(map[string]any{
		"num_users":       numUsers,
		"concurrency":     concurrency,
		"request_timeout": requestTimeout,
	}).Info("Test configuration")

	app, cleanup := setupTestApp(t)
	defer cleanup()

	testLogger.Info("Creating test users...")
	startUserCreation := time.Now()
	users := createTestUsers(t, app, numUsers)
	userCreationDuration := time.Since(startUserCreation)

	testLogger.WithFields(map[string]any{
		"count":    len(users),
		"duration": userCreationDuration,
		"rate":     float64(len(users)) / userCreationDuration.Seconds(),
	}).Info("Test users created")

	var (
		successCount   int64
		failureCount   int64
		totalLatency   int64
		wg             sync.WaitGroup
		progressTicker = time.NewTicker(2 * time.Second)
	)
	defer progressTicker.Stop()

	// Progress reporting goroutine
	go func() {
		for range progressTicker.C {
			current := atomic.LoadInt64(&successCount) + atomic.LoadInt64(&failureCount)
			testLogger.WithFields(map[string]any{
				"completed": current,
				"total":     numUsers,
				"progress":  fmt.Sprintf("%.1f%%", float64(current)/float64(numUsers)*100),
				"success":   atomic.LoadInt64(&successCount),
				"failures":  atomic.LoadInt64(&failureCount),
			}).Info("Login test progress")
		}
	}()

	semaphore := make(chan struct{}, concurrency)
	startTime := time.Now()

	testLogger.Info("Starting concurrent login attempts...")

	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(userIdx int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			user := users[userIdx]
			reqStart := time.Now()

			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()

			err := attemptLogin(ctx, app, user.Username, user.Password)
			latency := time.Since(reqStart)

			if err == nil {
				atomic.AddInt64(&successCount, 1)
				if userIdx%100 == 0 {
					testLogger.WithFields(map[string]any{
						"username": user.Username,
						"latency":  latency,
					}).Debug("Login successful")
				}
			} else {
				atomic.AddInt64(&failureCount, 1)
				testLogger.WithFields(map[string]any{
					"username": user.Username,
					"error":    err.Error(),
					"latency":  latency,
				}).Error("Login failed")
			}
			atomic.AddInt64(&totalLatency, int64(latency))
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// Calculate metrics
	avgLatency := time.Duration(atomic.LoadInt64(&totalLatency)) / time.Duration(numUsers)
	throughput := float64(numUsers) / totalDuration.Seconds()
	successRate := float64(successCount) / float64(numUsers) * 100

	testLogger.WithFields(map[string]any{
		"total_users":    numUsers,
		"concurrency":    concurrency,
		"success":        successCount,
		"failures":       failureCount,
		"success_rate":   fmt.Sprintf("%.2f%%", successRate),
		"total_duration": totalDuration,
		"avg_latency":    avgLatency,
		"throughput":     fmt.Sprintf("%.2f req/sec", throughput),
	}).Info("=== Login Load Test Results ===")

	t.Logf("=== Login Load Test Results ===")
	t.Logf("Total Users: %d", numUsers)
	t.Logf("Concurrency: %d", concurrency)
	t.Logf("Success: %d", successCount)
	t.Logf("Failures: %d", failureCount)
	t.Logf("Success Rate: %.2f%%", successRate)
	t.Logf("Total Duration: %v", totalDuration)
	t.Logf("Avg Latency: %v", avgLatency)
	t.Logf("Throughput: %.2f req/sec", throughput)

	assert.GreaterOrEqual(t, successRate, 95.0, "Success rate should be >= 95%")
	assert.Less(t, avgLatency, 500*time.Millisecond, "Average latency should be < 500ms")
	assert.Greater(t, throughput, 100.0, "Throughput should be > 100 req/sec")

	testLogger.Info("Login load test completed successfully")
}

// TestMessageThroughput tests message sending capacity
func TestMessageThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	testLogger.Info("========================================")
	testLogger.Info("Starting Message Throughput Test")
	testLogger.Info("========================================")

	const (
		numMessages  = 10000
		numSenders   = 100
		numReceivers = 100
	)

	testLogger.WithFields(map[string]any{
		"num_messages":  numMessages,
		"num_senders":   numSenders,
		"num_receivers": numReceivers,
	}).Info("Test configuration")

	app, cleanup := setupTestApp(t)
	defer cleanup()

	testLogger.Info("Creating sender accounts...")
	senders := createTestUsers(t, app, numSenders)
	testLogger.WithField("count", len(senders)).Info("Senders created")

	testLogger.Info("Creating receiver accounts...")
	receivers := createTestUsers(t, app, numReceivers)
	testLogger.WithField("count", len(receivers)).Info("Receivers created")

	var (
		sentCount      int64
		failedCount    int64
		wg             sync.WaitGroup
		progressTicker = time.NewTicker(2 * time.Second)
	)
	defer progressTicker.Stop()

	// Progress reporting
	go func() {
		for range progressTicker.C {
			current := atomic.LoadInt64(&sentCount) + atomic.LoadInt64(&failedCount)
			testLogger.WithFields(map[string]any{
				"sent":     atomic.LoadInt64(&sentCount),
				"failed":   atomic.LoadInt64(&failedCount),
				"progress": fmt.Sprintf("%.1f%%", float64(current)/float64(numMessages)*100),
			}).Info("Message sending progress")
		}
	}()

	startTime := time.Now()
	testLogger.Info("Starting message sending...")

	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(msgIdx int) {
			defer wg.Done()

			sender := senders[rand.Intn(numSenders)]
			receiver := receivers[rand.Intn(numReceivers)]
			content := fmt.Sprintf("Load test message %d", rand.Int())

			err := sendMessage(app, sender.SessionID, sender.CSRFToken, receiver.Username, content)
			if err == nil {
				atomic.AddInt64(&sentCount, 1)
				if msgIdx%1000 == 0 {
					testLogger.WithFields(map[string]any{
						"from": sender.Username,
						"to":   receiver.Username,
					}).Debug("Message sent successfully")
				}
			} else {
				atomic.AddInt64(&failedCount, 1)
				testLogger.WithFields(map[string]any{
					"from":  sender.Username,
					"to":    receiver.Username,
					"error": err.Error(),
				}).Error("Message send failed")
			}
		}(i)

		if i%100 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	throughput := float64(sentCount) / totalDuration.Seconds()
	successRate := float64(sentCount) / float64(numMessages) * 100

	testLogger.WithFields(map[string]any{
		"total_messages": numMessages,
		"sent":           sentCount,
		"failed":         failedCount,
		"success_rate":   fmt.Sprintf("%.2f%%", successRate),
		"duration":       totalDuration,
		"throughput":     fmt.Sprintf("%.2f msg/sec", throughput),
	}).Info("=== Message Throughput Test Results ===")

	t.Logf("=== Message Throughput Test Results ===")
	t.Logf("Total Messages: %d", numMessages)
	t.Logf("Sent: %d", sentCount)
	t.Logf("Failed: %d", failedCount)
	t.Logf("Duration: %v", totalDuration)
	t.Logf("Throughput: %.2f msg/sec", throughput)

	assert.GreaterOrEqual(t, successRate, 98.0, "Message success rate >= 98%")
	assert.Greater(t, throughput, 500.0, "Message throughput > 500 msg/sec")

	testLogger.Info("Message throughput test completed successfully")
}

// TestWebSocketConnectionStorm tests concurrent WebSocket connections
func TestWebSocketConnectionStorm(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	testLogger.Info("========================================")
	testLogger.Info("Starting WebSocket Connection Storm Test")
	testLogger.Info("========================================")

	const (
		numConnections = 1000
		holdDuration   = 30 * time.Second
	)

	testLogger.WithFields(map[string]any{
		"num_connections": numConnections,
		"hold_duration":   holdDuration,
	}).Info("Test configuration")

	app, cleanup := setupTestApp(t)
	defer cleanup()

	// Start a single test server for all connections
	serverAddr, stopServer := startTestServer(app)
	defer stopServer()

	testLogger.Info("Creating test user accounts...")
	users := createTestUsers(t, app, numConnections)
	testLogger.WithField("count", len(users)).Info("Users created")

	var (
		connectedCount    int64
		disconnectedCount int64
		messagesReceived  int64
		wg                sync.WaitGroup
		progressTicker    = time.NewTicker(5 * time.Second)
	)
	defer progressTicker.Stop()

	// Progress reporting
	go func() {
		for range progressTicker.C {
			testLogger.WithFields(map[string]any{
				"connected":    atomic.LoadInt64(&connectedCount),
				"disconnected": atomic.LoadInt64(&disconnectedCount),
				"messages_rx":  atomic.LoadInt64(&messagesReceived),
			}).Info("WebSocket connection status")
		}
	}()

	startTime := time.Now()
	testLogger.Info("Starting WebSocket connections...")

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(userIdx int) {
			defer wg.Done()

			user := users[userIdx]
			// Pass the server address
			ws, err := connectWebSocket(serverAddr, user.SessionID)
			if err != nil {
				atomic.AddInt64(&disconnectedCount, 1)
				testLogger.WithFields(map[string]any{
					"username": user.Username,
					"error":    err.Error(),
				}).Error("WebSocket connection failed")
				return
			}
			defer ws.Close()

			atomic.AddInt64(&connectedCount, 1)
			if userIdx%100 == 0 {
				testLogger.WithField("username", user.Username).Debug("WebSocket connected")
			}

			ctx, cancel := context.WithTimeout(context.Background(), holdDuration)
			defer cancel()

			// Use a goroutine to close the connection when context triggers.
			// This unblocks the read loop safely.
			go func() {
				<-ctx.Done()
				// Send a close message instead of forcing the socket shut immediately.
				// This gives the server a chance to process the close frame.
				ws.WriteMessage(fastws.CloseMessage, fastws.FormatCloseMessage(fastws.CloseNormalClosure, ""))
				time.Sleep(100 * time.Millisecond) // Give a tiny window for network flush
				ws.Close()
			}()

			for {
				// Read with a long deadline; we rely on ws.Close() (from context) to break the loop.
				// Do NOT use a short deadline and retry, as that causes panics in the websocket library.
				msg, err := receiveMessage(ws)
				if err != nil {
					// We expect an error when the connection is closed.
					return
				}

				if msg != nil {
					atomic.AddInt64(&messagesReceived, 1)
				}
			}
		}(i)

		if i%50 == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	connectionRate := float64(connectedCount) / totalDuration.Seconds()
	successRate := float64(connectedCount) / float64(numConnections) * 100

	testLogger.WithFields(map[string]any{
		"target_connections": numConnections,
		"connected":          connectedCount,
		"failed":             disconnectedCount,
		"success_rate":       fmt.Sprintf("%.2f%%", successRate),
		"messages_received":  messagesReceived,
		"duration":           totalDuration,
		"connection_rate":    fmt.Sprintf("%.2f/sec", connectionRate),
	}).Info("=== WebSocket Connection Storm Results ===")

	t.Logf("=== WebSocket Connection Storm Results ===")
	t.Logf("Target Connections: %d", numConnections)
	t.Logf("Connected: %d", connectedCount)
	t.Logf("Failed: %d", disconnectedCount)
	t.Logf("Messages Received: %d", messagesReceived)
	t.Logf("Duration: %v", totalDuration)
	t.Logf("Connection Rate: %.2f/sec", connectionRate)

	assert.GreaterOrEqual(t, successRate, 95.0, "Connection success rate >= 95%")

	testLogger.Info("WebSocket connection storm test completed successfully")
}

// TestDatabaseQueryPerformance tests database under load
func TestDatabaseQueryPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	testLogger.Info("========================================")
	testLogger.Info("Starting Database Query Performance Test")
	testLogger.Info("========================================")

	const (
		numQueries  = 5000
		concurrency = 50
	)

	testLogger.WithFields(map[string]any{
		"num_queries": numQueries,
		"concurrency": concurrency,
	}).Info("Test configuration")

	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	queryTypes := []struct {
		name string
		fn   func() error
	}{
		{"GetUserByUsername", func() error { return queryGetUserByUsername(testDB) }},
		{"GetFriendsList", func() error { return queryGetFriendsList(testDB) }},
		{"GetChatHistory", func() error { return queryGetChatHistory(testDB) }},
		{"SearchUsers", func() error { return querySearchUsers(testDB) }},
	}

	results := make(map[string]*QueryStats)
	var wg sync.WaitGroup

	for _, qt := range queryTypes {
		testLogger.WithField("query_type", qt.name).Info("Starting query performance test")

		results[qt.name] = &QueryStats{}
		stats := results[qt.name]

		semaphore := make(chan struct{}, concurrency)
		startTime := time.Now()

		for i := 0; i < numQueries; i++ {
			wg.Add(1)
			go func(queryFn func() error, queryIdx int) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				reqStart := time.Now()
				err := queryFn()
				latency := time.Since(reqStart)

				stats.mu.Lock()
				if err == nil {
					stats.SuccessCount++
				} else {
					stats.FailureCount++
					if queryIdx%100 == 0 {
						testLogger.WithFields(map[string]any{
							"query": qt.name,
							"error": err.Error(),
						}).Debug("Query error")
					}
				}
				stats.TotalLatency += latency
				if latency > stats.MaxLatency {
					stats.MaxLatency = latency
				}
				stats.mu.Unlock()
			}(qt.fn, i)

			// Add small delay every 100 queries to prevent overwhelming the connection pool
			if i > 0 && i%100 == 0 {
				time.Sleep(5 * time.Millisecond)
			}
		}

		wg.Wait()
		stats.TotalDuration = time.Since(startTime)

		testLogger.WithFields(map[string]any{
			"query_type": qt.name,
			"completed":  stats.SuccessCount + stats.FailureCount,
			"success":    stats.SuccessCount,
			"failed":     stats.FailureCount,
		}).Info("Query type completed")
	}

	testLogger.Info("=== Database Query Performance Results ===")
	t.Logf("=== Database Query Performance Results ===")

	for name, stats := range results {
		totalQueries := stats.SuccessCount + stats.FailureCount
		avgLatency := stats.TotalLatency / time.Duration(totalQueries)
		qps := float64(stats.SuccessCount) / stats.TotalDuration.Seconds()
		successRate := float64(stats.SuccessCount) / float64(totalQueries) * 100

		testLogger.WithFields(map[string]any{
			"query":        name,
			"success":      stats.SuccessCount,
			"failures":     stats.FailureCount,
			"success_rate": fmt.Sprintf("%.2f%%", successRate),
			"avg_latency":  avgLatency,
			"max_latency":  stats.MaxLatency,
			"qps":          fmt.Sprintf("%.2f", qps),
		}).Info("Query performance metrics")

		t.Logf("Query: %s", name)
		t.Logf("  Success: %d", stats.SuccessCount)
		t.Logf("  Failures: %d", stats.FailureCount)
		t.Logf("  Avg Latency: %v", avgLatency)
		t.Logf("  Max Latency: %v", stats.MaxLatency)
		t.Logf("  QPS: %.2f", qps)

		assert.GreaterOrEqual(t, successRate, 95.0, "Query success rate >= 95%")
		assert.Less(t, avgLatency, 500*time.Millisecond, "Avg query latency < 500ms")
	}

	testLogger.Info("Database query performance test completed successfully")
}

// TestRedisFailover tests system behavior when Redis fails
func TestRedisFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	testLogger.Info("========================================")
	testLogger.Info("Starting Redis Failover Test")
	testLogger.Info("========================================")

	app, cleanup := setupTestApp(t)
	defer cleanup()

	users := createTestUsers(t, app, 1)
	user := users[0]

	// Phase 1: Normal operation
	testLogger.Info("Phase 1: Testing normal operation")
	err := sendMessage(app, user.SessionID, user.CSRFToken, "testuser", "baseline message")
	require.NoError(t, err)
	testLogger.Info("Baseline message sent successfully")

	// Phase 2: Redis failure
	testLogger.Warn("Phase 2: Simulating Redis failure")
	stopRedis(t)
	defer startRedis(t)

	time.Sleep(2 * time.Second)
	testLogger.Info("Redis should now be down")

	// Messages should queue but not fail
	testLogger.Info("Attempting to send message during Redis outage")
	err = sendMessage(app, user.SessionID, user.CSRFToken, "testuser", "during outage")
	assert.NoError(t, err, "Messages should queue during Redis outage")
	testLogger.Info("Message queued successfully during outage")

	// Phase 3: Redis recovery
	testLogger.Info("Phase 3: Restoring Redis")
	startRedis(t)
	time.Sleep(5 * time.Second)
	testLogger.Info("Redis restored, waiting for circuit breaker recovery")

	// Verify recovery
	testLogger.Info("Verifying system recovery")
	err = sendMessage(app, user.SessionID, user.CSRFToken, "testuser", "after recovery")
	assert.NoError(t, err, "Messages should work after Redis recovery")
	testLogger.Info("Post-recovery message sent successfully")

	// Check circuit breaker metrics
	metrics := getCircuitBreakerMetrics(t, app)
	testLogger.WithFields(map[string]any{
		"metrics": metrics,
	}).Info("Circuit Breaker State After Failover")

	t.Logf("Circuit Breaker State: %+v", metrics)

	testLogger.Info("Redis failover test completed successfully")
}

// Helper types and functions

type QueryStats struct {
	mu            sync.Mutex
	SuccessCount  int64
	FailureCount  int64
	TotalLatency  time.Duration
	MaxLatency    time.Duration
	TotalDuration time.Duration
}

type TestUser struct {
	Username  string
	Password  string
	SessionID string
	CSRFToken string
}

type TestApp struct {
	App        *fiber.App
	DB         *db.Queries
	RDB        *redis.Client
	ChatSvc    *chat.ChatService
	SessionMgr *sessions.SessionManager
}

type TestDB struct {
	Queries *db.Queries
	Conn    *sql.DB
}

func setupTestApp(t *testing.T) (*TestApp, func()) {
	testLogger.Info("Setting up test application")

	os.Setenv("GOOSE_DBSTRING", "postgres://postgres:postgres@127.0.0.1:5433/securechat_test?sslmode=disable")
	os.Setenv("REDIS_ADDR", "127.0.0.1:6380")
	os.Setenv("REDIS_PASSWORD", "")
	os.Setenv("KAFKA_ADDR", "127.0.0.1:9093")

	// Increase rate limits for load testing to prevent errors during user creation
	os.Setenv("RATE_LIMIT_CAPACITY", "10000")
	os.Setenv("RATE_LIMIT_REFILL", "1000")

	os.Setenv("LOG_FILE", "./tests/load/log/server.log")

	cfg, err := config.Load()
	require.NoError(t, err, "Failed to load test config")

	dbString := os.Getenv("GOOSE_DBSTRING")
	require.NotEmpty(t, dbString, "GOOSE_DBSTRING should not be empty")

	testLogger.WithFields(map[string]any{
		"db_addr":    "127.0.0.1:5433",
		"redis_addr": cfg.Redis.Address,
		"kafka_addr": cfg.Kafka.Address,
	}).Info("Test infrastructure configured")

	testLogger.Info("Opening database connection")
	dbConn, err := sql.Open("postgres", dbString)
	require.NoError(t, err, "Failed to open database connection")

	// Configure connection pool for load testing
	dbConn.SetMaxOpenConns(100)
	dbConn.SetMaxIdleConns(10)
	dbConn.SetConnMaxLifetime(time.Hour)

	testLogger.WithFields(map[string]any{
		"max_open_conns": 100,
		"max_idle_conns": 10,
	}).Info("Database connection pool configured")

	// Verify database connection
	testLogger.Info("Verifying database connection")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := dbConn.PingContext(ctx); err != nil {
		testLogger.WithError(err).Error("Database ping failed")
		require.NoError(t, err, "Failed to ping database")
	}
	testLogger.Info("Database connection verified")

	qdb := db.New(dbConn)

	testLogger.Info("Connecting to Redis")
	rdb, err := infraredis.NewClient(cfg.Redis)
	require.NoError(t, err, "Failed to connect to Redis")

	ctx = context.Background()
	err = rdb.FlushDB(ctx).Err()
	require.NoError(t, err, "Failed to flush Redis")
	testLogger.Info("Redis flushed")

	testLogger.Info("Initializing services")
	chatSvc, err := chat.NewChatService(ctx, rdb, qdb, cfg.Kafka.Address)
	require.NoError(t, err, "Failed to create chat service")

	sessionMgr := sessions.NewSessionManager(rdb)
	friendSvc := friends.NewFriendService(qdb)
	groupSvc := groups.NewGroupService(qdb)
	wsManager := _websocket.NewManager(ctx)
	callSvc := calls.NewCallService(ctx, rdb)

	srv, err := server.NewServer(cfg, qdb, rdb, chatSvc, sessionMgr, friendSvc, groupSvc, wsManager, callSvc)
	require.NoError(t, err, "Failed to create server")

	testApp := &TestApp{
		App:        srv.App,
		DB:         qdb,
		RDB:        rdb,
		ChatSvc:    chatSvc,
		SessionMgr: sessionMgr,
	}

	cleanup := func() {
		testLogger.Info("Cleaning up test application")
		chatSvc.Close()
		rdb.Close()
		dbConn.Close()
		testLogger.Info("Test application cleanup completed")
	}

	testLogger.Info("Test application setup completed")
	return testApp, cleanup
}

func setupTestDB(t *testing.T) (*TestDB, func()) {
	testLogger.Info("Setting up test database")

	connStr := os.Getenv("GOOSE_DBSTRING")

	dbConn, err := sql.Open("postgres", connStr)
	require.NoError(t, err, "Failed to connect to test database")

	// Configure connection pool for load testing
	dbConn.SetMaxOpenConns(100)
	dbConn.SetMaxIdleConns(10)
	dbConn.SetConnMaxLifetime(time.Hour)

	testLogger.WithFields(map[string]any{
		"max_open_conns": 100,
		"max_idle_conns": 10,
	}).Info("Database connection pool configured")

	// Verify database connection
	testLogger.Info("Verifying database connection")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := dbConn.PingContext(ctx); err != nil {
		testLogger.WithError(err).Error("Database ping failed")
		require.NoError(t, err, "Failed to ping database")
	}
	testLogger.Info("Database connection verified")

	qdb := db.New(dbConn)

	cleanup := func() {
		testLogger.Info("Closing test database connection")
		dbConn.Close()
	}

	testLogger.Info("Test database setup completed")
	return &TestDB{
		Queries: qdb,
		Conn:    dbConn,
	}, cleanup
}

func createTestUsers(t *testing.T, app *TestApp, count int) []TestUser {
	testLogger.WithField("count", count).Info("Creating test users")

	users := make([]TestUser, count)

	for i := 0; i < count; i++ {
		username := fmt.Sprintf("loadtest_user_%d_%d", time.Now().Unix(), i)
		password := "TestPass123!"

		form := url.Values{}
		form.Add("username", username)
		form.Add("password", password)
		form.Add("confirm_password", password)

		req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := app.App.Test(req, -1)
		require.NoError(t, err, "Failed to register user %s", username)

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			testLogger.WithFields(map[string]any{
				"username":    username,
				"status_code": resp.StatusCode,
				"body":        string(body),
			}).Error("User registration failed")
			require.Equal(t, http.StatusOK, resp.StatusCode, "Registration failed for %s: %s", username, string(body))
		}

		sessionID, csrfToken, err := loginUser(app, username, password)
		require.NoError(t, err, "Failed to login user %s", username)

		users[i] = TestUser{
			Username:  username,
			Password:  password,
			SessionID: sessionID,
			CSRFToken: csrfToken,
		}

		if (i+1)%100 == 0 {
			testLogger.WithFields(map[string]any{
				"created": i + 1,
				"total":   count,
			}).Debug("User creation progress")
		}
	}

	testLogger.WithField("count", len(users)).Info("Test users created successfully")
	return users
}

// Returns sessionID, csrfToken, error
func loginUser(app *TestApp, username, password string) (string, string, error) {
	// 1. Perform Login
	form := url.Values{}
	form.Add("username", username)
	form.Add("password", password)

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.App.Test(req, -1)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	// Capture Session ID
	var sessionID string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session_id" {
			sessionID = cookie.Value
		}
	}
	if sessionID == "" {
		return "", "", fmt.Errorf("no session cookie found")
	}

	// 2. Perform GET / to acquire CSRF token
	// The middleware generates the token on GET requests to protected pages
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})

	resp, err = app.App.Test(req, -1)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch dashboard for csrf: %w", err)
	}

	var csrfToken string
	for _, cookie := range resp.Cookies() {
		// MATCH THE SERVER CONFIG: "csrf_token"
		if cookie.Name == "csrf_token" {
			csrfToken = cookie.Value
		}
	}

	// Optional: If token not in cookie, check headers (if your app sets it there)
	if csrfToken == "" {
		csrfToken = resp.Header.Get("X-CSRF-Token")
	}

	return sessionID, csrfToken, nil
}

func attemptLogin(ctx context.Context, app *TestApp, username, password string) error {
	form := url.Values{}
	form.Add("username", username)
	form.Add("password", password)

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(ctx)

	resp, err := app.App.Test(req, -1)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

func sendMessage(app *TestApp, sessionID, csrfToken, recipient, content string) error {
	form := url.Values{}
	form.Add("content", content)

	req := httptest.NewRequest("POST", "/chat/"+recipient, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// 1. Add Session Cookie
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	// 2. Add CSRF Token
	if csrfToken != "" {
		// Standard Header
		req.Header.Set("X-CSRF-Token", csrfToken)

		// Cookie
		req.AddCookie(&http.Cookie{
			Name:  "csrf_token",
			Value: csrfToken,
		})
	}

	resp, err := app.App.Test(req, -1)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send message failed: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

func startTestServer(app *TestApp) (string, func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		testLogger.WithError(err).Error("Failed to create listener")
		panic(err)
	}

	addr := listener.Addr().String()
	testLogger.WithField("address", addr).Info("Test server starting")

	go func() {
		if err := app.App.Listener(listener); err != nil {
			testLogger.WithError(err).Error("Server error")
		}
	}()

	time.Sleep(100 * time.Millisecond)

	cleanup := func() {
		testLogger.Info("Shutting down test server")
		app.App.Shutdown()
	}

	return addr, cleanup
}

func connectWebSocket(addr, sessionID string) (*fastws.Conn, error) {
	wsURL := fmt.Sprintf("ws://%s/ws/chat", addr)
	testLogger.WithField("url", wsURL).Debug("Connecting WebSocket")

	header := http.Header{}
	header.Add("Cookie", fmt.Sprintf("session_id=%s", sessionID))

	dialer := fastws.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		testLogger.WithError(err).Error("WebSocket dial failed")
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}

	return conn, nil
}

func receiveMessage(ws *fastws.Conn) (any, error) {
	// Use a long deadline to avoid timeout errors which break the connection state
	// in fasthttp/websocket. Cancellation is handled by closing the connection.
	ws.SetReadDeadline(time.Now().Add(5 * time.Minute))

	var msg map[string]any
	err := ws.ReadJSON(&msg)
	return msg, err
}

func queryGetUserByUsername(testDB *TestDB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	username := fmt.Sprintf("loadtest_user_%d", rand.Intn(1000))
	_, err := testDB.Queries.GetUserByUsername(ctx, username)

	if err == sql.ErrNoRows {
		return nil
	}

	return err
}

func queryGetFriendsList(testDB *TestDB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	usernames, err := testDB.Queries.GetAllUsernames(ctx)
	if err != nil {
		return err
	}

	if len(usernames) == 0 {
		return nil
	}

	username := usernames[rand.Intn(len(usernames))]
	user, err := testDB.Queries.GetUserByUsername(ctx, username)
	if err != nil {
		return err
	}

	_, err = testDB.Queries.GetFriendsWithDetails(ctx, uuid.NullUUID{UUID: user.ID, Valid: true})
	return err
}

func queryGetChatHistory(testDB *TestDB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	usernames, err := testDB.Queries.GetAllUsernames(ctx)
	if err != nil {
		return err
	}

	if len(usernames) < 2 {
		return nil
	}

	_, err = testDB.Queries.GetUserByUsername(ctx, usernames[0])
	return err
}

func querySearchUsers(testDB *TestDB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := testDB.Queries.GetAllUsernames(ctx)
	return err
}

func stopRedis(t *testing.T) {
	testLogger.Warn("Pausing Redis container")
	cmd := exec.Command("docker", "pause", "redis-test")
	if err := cmd.Run(); err != nil {
		testLogger.WithError(err).Warn("Failed to pause Redis (may already be paused)")
		t.Logf("Failed to stop Redis (may already be stopped): %v", err)
	}
	time.Sleep(1 * time.Second)
	testLogger.Info("Redis container paused")
}

func startRedis(t *testing.T) {
	testLogger.Info("Unpausing Redis container")
	cmd := exec.Command("docker", "unpause", "redis-test")
	if err := cmd.Run(); err != nil {
		testLogger.WithError(err).Error("Failed to unpause Redis")
		t.Logf("Failed to start Redis: %v", err)
	}
	time.Sleep(2 * time.Second)
	testLogger.Info("Redis container unpaused")
}

func getCircuitBreakerMetrics(t *testing.T, app *TestApp) map[string]any {
	req := httptest.NewRequest("GET", "/metrics", nil)

	resp, err := app.App.Test(req, -1)
	if err != nil {
		testLogger.WithError(err).Error("Failed to get metrics")
		t.Logf("Failed to get metrics: %v", err)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		testLogger.WithField("status", resp.StatusCode).Error("Metrics request failed")
		t.Logf("Metrics request failed with status: %d", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		testLogger.WithError(err).Error("Failed to read metrics body")
		t.Logf("Failed to read metrics body: %v", err)
		return nil
	}

	var metrics map[string]any
	if err := json.Unmarshal(body, &metrics); err != nil {
		testLogger.WithError(err).Error("Failed to parse metrics JSON")
		t.Logf("Failed to parse metrics JSON: %v", err)
		return nil
	}

	return metrics
}

func BenchmarkLogin(b *testing.B) {
	testLogger.Info("Starting login benchmark")

	app, cleanup := setupTestApp(&testing.T{})
	defer cleanup()

	users := createTestUsers(&testing.T{}, app, b.N)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			user := users[i%len(users)]
			_ = attemptLogin(context.Background(), app, user.Username, user.Password)
			i++
		}
	})

	testLogger.Info("Login benchmark completed")
}

func BenchmarkSendMessage(b *testing.B) {
	testLogger.Info("Starting message send benchmark")

	app, cleanup := setupTestApp(&testing.T{})
	defer cleanup()

	users := createTestUsers(&testing.T{}, app, 100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sender := users[rand.Intn(len(users))]
			receiver := users[rand.Intn(len(users))]
			_ = sendMessage(app, sender.SessionID, sender.CSRFToken, receiver.Username, "benchmark message")
		}
	})

	testLogger.Info("Message send benchmark completed")
}

func BenchmarkDatabaseQuery(b *testing.B) {
	testLogger.Info("Starting database query benchmark")

	testDB, cleanup := setupTestDB(&testing.T{})
	defer cleanup()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = queryGetUserByUsername(testDB)
		}
	})

	testLogger.Info("Database query benchmark completed")
}
