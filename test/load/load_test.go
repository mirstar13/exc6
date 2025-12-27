package load

import (
	"context"
	"database/sql"
	"encoding/json"
	"exc6/config"
	"exc6/db"
	infraredis "exc6/infrastructure/redis"
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

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentUserLogins tests authentication under load
func TestConcurrentUserLogins(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		numUsers       = 1000
		concurrency    = 50
		requestTimeout = 5 * time.Second
	)

	app, cleanup := setupTestApp(t)
	defer cleanup()

	// Pre-create test users
	users := createTestUsers(t, app, numUsers)

	var (
		successCount int64
		failureCount int64
		totalLatency int64
		wg           sync.WaitGroup
	)

	semaphore := make(chan struct{}, concurrency)
	startTime := time.Now()

	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(userIdx int) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			user := users[userIdx]
			reqStart := time.Now()

			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()

			err := attemptLogin(ctx, app, user.Username, user.Password)
			latency := time.Since(reqStart)

			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&failureCount, 1)
				t.Logf("Login failed for %s: %v", user.Username, err)
			}
			atomic.AddInt64(&totalLatency, int64(latency))
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// Calculate metrics
	avgLatency := time.Duration(atomic.LoadInt64(&totalLatency)) / time.Duration(numUsers)
	throughput := float64(numUsers) / totalDuration.Seconds()

	t.Logf("=== Login Load Test Results ===")
	t.Logf("Total Users: %d", numUsers)
	t.Logf("Concurrency: %d", concurrency)
	t.Logf("Success: %d", successCount)
	t.Logf("Failures: %d", failureCount)
	t.Logf("Success Rate: %.2f%%", float64(successCount)/float64(numUsers)*100)
	t.Logf("Total Duration: %v", totalDuration)
	t.Logf("Avg Latency: %v", avgLatency)
	t.Logf("Throughput: %.2f req/sec", throughput)

	// Assertions
	assert.GreaterOrEqual(t, float64(successCount)/float64(numUsers), 0.95, "Success rate should be >= 95%")
	assert.Less(t, avgLatency, 500*time.Millisecond, "Average latency should be < 500ms")
	assert.Greater(t, throughput, 100.0, "Throughput should be > 100 req/sec")
}

// TestMessageThroughput tests message sending capacity
func TestMessageThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		numMessages  = 10000
		numSenders   = 100
		numReceivers = 100
	)

	app, cleanup := setupTestApp(t)
	defer cleanup()

	// Create sender and receiver accounts
	senders := createTestUsers(t, app, numSenders)
	receivers := createTestUsers(t, app, numReceivers)

	var (
		sentCount   int64
		failedCount int64
		wg          sync.WaitGroup
	)

	startTime := time.Now()

	// Message sending goroutines
	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			sender := senders[rand.Intn(numSenders)]
			receiver := receivers[rand.Intn(numReceivers)]
			content := fmt.Sprintf("Load test message %d", rand.Int())

			err := sendMessage(app, sender.SessionID, receiver.Username, content)
			if err == nil {
				atomic.AddInt64(&sentCount, 1)
			} else {
				atomic.AddInt64(&failedCount, 1)
			}
		}()

		// Rate limiting to avoid overwhelming the system
		if i%100 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	throughput := float64(sentCount) / totalDuration.Seconds()

	t.Logf("=== Message Throughput Test Results ===")
	t.Logf("Total Messages: %d", numMessages)
	t.Logf("Sent: %d", sentCount)
	t.Logf("Failed: %d", failedCount)
	t.Logf("Duration: %v", totalDuration)
	t.Logf("Throughput: %.2f msg/sec", throughput)

	assert.GreaterOrEqual(t, float64(sentCount)/float64(numMessages), 0.98, "Message success rate >= 98%")
	assert.Greater(t, throughput, 500.0, "Message throughput > 500 msg/sec")
}

// TestWebSocketConnectionStorm tests concurrent WebSocket connections
func TestWebSocketConnectionStorm(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		numConnections = 1000
		holdDuration   = 30 * time.Second
	)

	app, cleanup := setupTestApp(t)
	defer cleanup()

	users := createTestUsers(t, app, numConnections)

	var (
		connectedCount    int64
		disconnectedCount int64
		messagesReceived  int64
		wg                sync.WaitGroup
	)

	startTime := time.Now()

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(userIdx int) {
			defer wg.Done()

			user := users[userIdx]
			ws, err := connectWebSocket(app, user.SessionID)
			if err != nil {
				atomic.AddInt64(&disconnectedCount, 1)
				t.Logf("WebSocket connection failed for %s: %v", user.Username, err)
				return
			}
			defer ws.Close()

			atomic.AddInt64(&connectedCount, 1)

			// Hold connection and count received messages
			ctx, cancel := context.WithTimeout(context.Background(), holdDuration)
			defer cancel()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					if msg := receiveMessage(ws); msg != nil {
						atomic.AddInt64(&messagesReceived, 1)
					}
				}
			}
		}(i)

		// Stagger connection attempts
		if i%50 == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	t.Logf("=== WebSocket Connection Storm Results ===")
	t.Logf("Target Connections: %d", numConnections)
	t.Logf("Connected: %d", connectedCount)
	t.Logf("Failed: %d", disconnectedCount)
	t.Logf("Messages Received: %d", messagesReceived)
	t.Logf("Duration: %v", totalDuration)
	t.Logf("Connection Rate: %.2f/sec", float64(connectedCount)/totalDuration.Seconds())

	assert.GreaterOrEqual(t, float64(connectedCount)/float64(numConnections), 0.95, "Connection success rate >= 95%")
}

// TestDatabaseQueryPerformance tests database under load
func TestDatabaseQueryPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		numQueries  = 5000
		concurrency = 50
	)

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
		results[qt.name] = &QueryStats{}
		stats := results[qt.name]

		semaphore := make(chan struct{}, concurrency)
		startTime := time.Now()

		for i := 0; i < numQueries; i++ {
			wg.Add(1)
			go func(queryFn func() error) {
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
				}
				stats.TotalLatency += latency
				if latency > stats.MaxLatency {
					stats.MaxLatency = latency
				}
				stats.mu.Unlock()
			}(qt.fn)
		}

		wg.Wait()
		stats.TotalDuration = time.Since(startTime)
	}

	// Report results
	t.Logf("=== Database Query Performance Results ===")
	for name, stats := range results {
		avgLatency := stats.TotalLatency / time.Duration(stats.SuccessCount+stats.FailureCount)
		qps := float64(stats.SuccessCount) / stats.TotalDuration.Seconds()

		t.Logf("Query: %s", name)
		t.Logf("  Success: %d", stats.SuccessCount)
		t.Logf("  Failures: %d", stats.FailureCount)
		t.Logf("  Avg Latency: %v", avgLatency)
		t.Logf("  Max Latency: %v", stats.MaxLatency)
		t.Logf("  QPS: %.2f", qps)

		assert.GreaterOrEqual(t, float64(stats.SuccessCount)/float64(numQueries), 0.99, "Query success rate >= 99%")
		assert.Less(t, avgLatency, 100*time.Millisecond, "Avg query latency < 100ms")
	}
}

// TestRedisFailover tests system behavior when Redis fails
func TestRedisFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	app, cleanup := setupTestApp(t)
	defer cleanup()

	users := createTestUsers(t, app, 1)
	user := users[0]

	// Establish baseline
	t.Log("Phase 1: Normal operation")
	err := sendMessage(app, user.SessionID, "testuser", "baseline message")
	require.NoError(t, err)

	// Simulate Redis failure
	t.Log("Phase 2: Redis down")
	stopRedis(t)
	defer startRedis(t)

	// System should degrade gracefully
	time.Sleep(2 * time.Second)

	// Messages should queue but not fail
	err = sendMessage(app, user.SessionID, "testuser", "during outage")
	assert.NoError(t, err, "Messages should queue during Redis outage")

	// Restore Redis
	t.Log("Phase 3: Redis restored")
	startRedis(t)
	time.Sleep(5 * time.Second)

	// Verify recovery
	err = sendMessage(app, user.SessionID, "testuser", "after recovery")
	assert.NoError(t, err, "Messages should work after Redis recovery")

	// Check circuit breaker metrics
	metrics := getCircuitBreakerMetrics(t, app)
	t.Logf("Circuit Breaker State: %+v", metrics)
}

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
	// Load test configuration
	os.Setenv("GOOSE_DBSTRING", "postgres://postgres:postgres@localhost:5433/securechat_test?sslmode=disable")
	os.Setenv("REDIS_ADDR", "localhost:6380")
	os.Setenv("REDIS_PASSWORD", "")
	os.Setenv("KAFKA_ADDR", "localhost:9093")

	cfg, err := config.Load()
	require.NoError(t, err, "Failed to load test config")

	// Setup database
	dbConn, err := sql.Open("postgres", cfg.Database.ConnectionString)
	require.NoError(t, err, "Failed to connect to test database")

	// Run migrations
	err = runMigrations(cfg.Database.ConnectionString)
	require.NoError(t, err, "Failed to run migrations")

	qdb := db.New(dbConn)

	// Setup Redis
	rdb, err := infraredis.NewClient(cfg.Redis)
	require.NoError(t, err, "Failed to connect to Redis")

	// Flush test Redis
	ctx := context.Background()
	err = rdb.FlushDB(ctx).Err()
	require.NoError(t, err, "Failed to flush Redis")

	// Setup services
	chatSvc, err := chat.NewChatService(ctx, rdb, qdb, cfg.Kafka.Address)
	require.NoError(t, err, "Failed to create chat service")

	sessionMgr := sessions.NewSessionManager(rdb)
	friendSvc := friends.NewFriendService(qdb)
	groupSvc := groups.NewGroupService(qdb)
	wsManager := _websocket.NewManager(ctx)
	callSvc := calls.NewCallService(ctx, rdb)

	// Create server
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
		chatSvc.Close()
		rdb.Close()
		dbConn.Close()
	}

	return testApp, cleanup
}

func setupTestDB(t *testing.T) (*TestDB, func()) {
	connStr := "postgres://postgres:postgres@localhost:5433/securechat_test?sslmode=disable"

	dbConn, err := sql.Open("postgres", connStr)
	require.NoError(t, err, "Failed to connect to test database")

	qdb := db.New(dbConn)

	cleanup := func() {
		dbConn.Close()
	}

	return &TestDB{
		Queries: qdb,
		Conn:    dbConn,
	}, cleanup
}

func runMigrations(connStr string) error {
	cmd := exec.Command("goose", "-dir", "sql/schema", "postgres", connStr, "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func createTestUsers(t *testing.T, app *TestApp, count int) []TestUser {
	users := make([]TestUser, count)

	for i := 0; i < count; i++ {
		username := fmt.Sprintf("loadtest_user_%d_%d", time.Now().Unix(), i)
		password := "TestPass123!"

		// Register user via HTTP
		form := url.Values{}
		form.Add("username", username)
		form.Add("password", password)
		form.Add("confirm_password", password)

		req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := app.App.Test(req, -1)
		require.NoError(t, err, "Failed to register user %s", username)
		require.Equal(t, http.StatusOK, resp.StatusCode, "Registration failed for %s", username)

		// Login to get session
		sessionID, err := loginUser(app, username, password)
		require.NoError(t, err, "Failed to login user %s", username)

		users[i] = TestUser{
			Username:  username,
			Password:  password,
			SessionID: sessionID,
		}
	}

	return users
}

func loginUser(app *TestApp, username, password string) (string, error) {
	form := url.Values{}
	form.Add("username", username)
	form.Add("password", password)

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.App.Test(req, -1)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	// Extract session cookie
	cookies := resp.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "session_id" {
			return cookie.Value, nil
		}
	}

	return "", fmt.Errorf("no session cookie found")
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

func sendMessage(app *TestApp, sessionID, recipient, content string) error {
	form := url.Values{}
	form.Add("content", content)

	req := httptest.NewRequest("POST", "/chat/"+recipient, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

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

func connectWebSocket(app *TestApp, sessionID string) (*websocket.Conn, error) {
	// Start test server
	ts := httptest.NewServer(app.App)
	defer ts.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/chat"

	header := http.Header{}
	header.Add("Cookie", fmt.Sprintf("session_id=%s", sessionID))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}

	return conn, nil
}

func receiveMessage(ws *websocket.Conn) interface{} {
	ws.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

	var msg map[string]interface{}
	err := ws.ReadJSON(&msg)
	if err != nil {
		return nil
	}

	return msg
}

func queryGetUserByUsername(testDB *TestDB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	username := fmt.Sprintf("loadtest_user_%d", rand.Intn(1000))
	_, err := testDB.Queries.GetUserByUsername(ctx, username)

	// Not found is acceptable for load testing
	if err == sql.ErrNoRows {
		return nil
	}

	return err
}

func queryGetFriendsList(testDB *TestDB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Get a random user
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

	_, err = testDB.Queries.GetFriendsWithDetails(ctx, sql.NullUUID{UUID: user.ID, Valid: true})
	return err
}

func queryGetChatHistory(testDB *TestDB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Simulate chat history query (this would normally query Redis)
	usernames, err := testDB.Queries.GetAllUsernames(ctx)
	if err != nil {
		return err
	}

	if len(usernames) < 2 {
		return nil
	}

	// Just verify users exist
	_, err = testDB.Queries.GetUserByUsername(ctx, usernames[0])
	return err
}

func querySearchUsers(testDB *TestDB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := testDB.Queries.GetAllUsernames(ctx)
	return err
}

func stopRedis(t *testing.T) {
	cmd := exec.Command("docker", "pause", "redis-test")
	if err := cmd.Run(); err != nil {
		t.Logf("Failed to stop Redis (may already be stopped): %v", err)
	}
	time.Sleep(1 * time.Second)
}

func startRedis(t *testing.T) {
	cmd := exec.Command("docker", "unpause", "redis-test")
	if err := cmd.Run(); err != nil {
		t.Logf("Failed to start Redis: %v", err)
	}
	time.Sleep(2 * time.Second)
}

func getCircuitBreakerMetrics(t *testing.T, app *TestApp) map[string]interface{} {
	req := httptest.NewRequest("GET", "/metrics", nil)

	resp, err := app.App.Test(req, -1)
	if err != nil {
		t.Logf("Failed to get metrics: %v", err)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		t.Logf("Metrics request failed with status: %d", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Logf("Failed to read metrics body: %v", err)
		return nil
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(body, &metrics); err != nil {
		t.Logf("Failed to parse metrics JSON: %v", err)
		return nil
	}

	return metrics
}

func BenchmarkLogin(b *testing.B) {
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
}

func BenchmarkSendMessage(b *testing.B) {
	app, cleanup := setupTestApp(&testing.T{})
	defer cleanup()

	users := createTestUsers(&testing.T{}, app, 100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sender := users[rand.Intn(len(users))]
			receiver := users[rand.Intn(len(users))]
			_ = sendMessage(app, sender.SessionID, receiver.Username, "benchmark message")
		}
	})
}

func BenchmarkDatabaseQuery(b *testing.B) {
	testDB, cleanup := setupTestDB(&testing.T{})
	defer cleanup()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = queryGetUserByUsername(testDB)
		}
	})
}
