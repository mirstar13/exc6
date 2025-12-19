package integration_test

import (
	"exc6/pkg/metrics"
	"exc6/server"
	"exc6/tests/setup"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"
)

type MetricsTestSuite struct {
	setup.TestSuite
	app *fiber.App
}

func TestMetricsSuite(t *testing.T) {
	suite.Run(t, new(MetricsTestSuite))
}

func (s *MetricsTestSuite) SetupSuite() {
	s.TestSuite.SetupSuite()

	// Reset collector state before registering
	metrics.ResetCollectorRegistry()

	metrics.RegisterCollectors(s.DB, s.Redis, s.ChatSvc.GetMetrics)

	srv, err := server.NewServerWithMetrics(
		s.Config,
		s.Queries,
		s.DB,
		s.Redis,
		s.ChatSvc,
		s.SessionMgr,
		s.FriendSvc,
		s.GroupSvc,
	)
	s.Require().NoError(err)
	s.app = srv.App
}

func (s *MetricsTestSuite) TestMetricsEndpointExists() {
	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := s.app.Test(req)

	s.NoError(err)
	s.Equal(200, resp.StatusCode, "Metrics endpoint should return 200")

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Verify Prometheus format
	s.Contains(bodyStr, "# HELP")
	s.Contains(bodyStr, "# TYPE")
}

func (s *MetricsTestSuite) TestHTTPMetricsRecorded() {
	// Make a request to generate metrics
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := s.app.Test(req)
	s.NoError(err)
	resp.Body.Close()

	// Check metrics endpoint
	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsResp, err := s.app.Test(metricsReq)
	s.NoError(err)

	body, _ := io.ReadAll(metricsResp.Body)
	bodyStr := string(body)

	// Verify HTTP metrics exist
	s.Contains(bodyStr, "http_requests_total", "Should have http_requests_total metric")
	s.Contains(bodyStr, "http_request_duration_seconds", "Should have http_request_duration_seconds metric")
}

func (s *MetricsTestSuite) TestChatMetricsRecorded() {
	user1 := s.CreateTestUser("metrics1", "pass123")
	user2 := s.CreateTestUser("metrics2", "pass123")

	// Send a message
	_, err := s.ChatSvc.SendMessage(s.Ctx, user1.Username, user2.Username, "test")
	s.NoError(err)

	// Check metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := s.app.Test(req)
	s.NoError(err)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Verify chat metrics exist
	s.Contains(bodyStr, "chat_messages_queued_total")
	s.Contains(bodyStr, "chat_message_buffer_size")
}

func (s *MetricsTestSuite) TestSessionMetricsRecorded() {
	user := s.CreateTestUser("sessionmetrics", "pass123")
	sessionID := s.CreateTestSession(user.Username, user.ID.String())

	_ = sessionID

	// Increment sessions metric
	metrics.IncrementSessionsCreated()

	// Check metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := s.app.Test(req)
	s.NoError(err)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Verify session metrics exist
	s.Contains(bodyStr, "sessions_created_total")
	s.Contains(bodyStr, "sessions_active")
}

func (s *MetricsTestSuite) TestLoginMetricsRecorded() {
	user := s.CreateTestUser("loginmetrics", "pass123")

	// Successful login
	metrics.RecordLoginAttempt(true)

	// Failed login
	metrics.RecordLoginAttempt(false)

	// Check metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := s.app.Test(req)
	s.NoError(err)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Verify login metrics exist
	s.Contains(bodyStr, "login_attempts_total")

	// Should have both success and failed labels
	s.Contains(bodyStr, `status="success"`)
	s.Contains(bodyStr, `status="failed"`)

	_ = user
}

func (s *MetricsTestSuite) TestErrorMetricsRecorded() {
	// Record an error
	metrics.RecordError("VALIDATION_FAILED", "400")

	// Check metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := s.app.Test(req)
	s.NoError(err)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Verify error metrics exist
	s.Contains(bodyStr, "errors_total")
	s.Contains(bodyStr, "VALIDATION_FAILED")
}

func (s *MetricsTestSuite) TestMetricsLabelsFormat() {
	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := s.app.Test(req)
	s.NoError(err)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	lines := strings.Split(bodyStr, "\n")

	for _, line := range lines {
		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Verify metric format: metric_name{labels} value timestamp
		if strings.Contains(line, "{") {
			// Has labels
			s.Contains(line, "}")
			s.Contains(line, "=")
		}
	}
}

func (s *MetricsTestSuite) TestMetricsCardinality() {
	// Make multiple requests to different endpoints
	endpoints := []string{"/", "/login-form", "/register-form"}

	for _, endpoint := range endpoints {
		req := httptest.NewRequest("GET", endpoint, nil)
		resp, _ := s.app.Test(req)
		resp.Body.Close()
	}

	// Check metrics
	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsResp, err := s.app.Test(metricsReq)
	s.NoError(err)

	body, _ := io.ReadAll(metricsResp.Body)
	bodyStr := string(body)

	// Count unique path labels for http_requests_total
	pathCount := 0
	lines := strings.Split(bodyStr, "\n")
	for _, line := range lines {
		if strings.Contains(line, "http_requests_total") &&
			strings.Contains(line, "path=") &&
			!strings.HasPrefix(line, "#") {
			pathCount++
		}
	}

	// Should have reasonable cardinality (not one metric per request)
	s.Less(pathCount, 20, "Path cardinality should be low")
}

func (s *MetricsTestSuite) TestDatabaseMetricsCollector() {
	// Trigger a database query
	_, err := s.Queries.GetAllUsernames(s.Ctx)
	s.NoError(err)

	// Check metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := s.app.Test(req)
	s.NoError(err)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Verify database pool metrics exist
	s.Contains(bodyStr, "database_connections_in_use", "Should have database_connections_in_use metric")
	s.Contains(bodyStr, "database_connections_idle", "Should have database_connections_idle metric")
	s.Contains(bodyStr, "database_max_open_connections", "Should have database_max_open_connections metric")
}

func (s *MetricsTestSuite) TestRedisMetricsCollector() {
	// Trigger a Redis operation
	err := s.Redis.Set(s.Ctx, "test:metrics", "value", 0).Err()
	s.NoError(err)

	// Check metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := s.app.Test(req)
	s.NoError(err)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Verify Redis pool metrics exist
	s.Contains(bodyStr, "redis_pool_hits_total", "Should have redis_pool_hits_total metric")
	s.Contains(bodyStr, "redis_pool_misses_total", "Should have redis_pool_misses_total metric")
	s.Contains(bodyStr, "redis_pool_total_connections", "Should have redis_pool_total_connections metric")
}

func (s *MetricsTestSuite) TestMetricsPerformance() {
	// Measure metrics endpoint performance
	iterations := 100

	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest("GET", "/metrics", nil)
		resp, err := s.app.Test(req, -1) // No timeout
		s.NoError(err)
		s.Equal(200, resp.StatusCode)
		resp.Body.Close()
	}
}
