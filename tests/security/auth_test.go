package security_test

import (
	"context"
	"exc6/server"
	"exc6/services/sessions"
	"exc6/tests/setup"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type AuthSecurityTestSuite struct {
	setup.TestSuite
	app *fiber.App
}

func TestAuthSecuritySuite(t *testing.T) {
	suite.Run(t, new(AuthSecurityTestSuite))
}

func (s *AuthSecurityTestSuite) SetupSuite() {
	// Call parent SetupSuite first
	s.TestSuite.SetupSuite()

	// Initialize the server
	srv, err := server.NewServer(s.Config, s.Queries, s.Redis, s.ChatSvc, s.SessionMgr, s.FriendSvc, s.GroupSvc)
	s.Require().NoError(err, "Failed to create server")
	s.Require().NotNil(srv, "Server should not be nil")
	s.Require().NotNil(srv.App, "Server app should not be nil")

	s.app = srv.App
}

func (s *AuthSecurityTestSuite) TearDownSuite() {
	s.TestSuite.TearDownSuite()
}

func (s *AuthSecurityTestSuite) TestWeakPasswordRejected() {
	s.Require().NotNil(s.app, "App should be initialized")

	// Create form data
	formData := url.Values{}
	formData.Set("username", "weakpass")
	formData.Set("password", "123")
	formData.Set("confirm_password", "123")

	req := httptest.NewRequest("POST", "/register", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.app.Test(req, -1) // -1 timeout means no timeout
	s.NoError(err)
	s.NotNil(resp)

	// Should fail validation (usually 400 Bad Request)
	s.True(resp.StatusCode >= 400, "Should return error status code")

	bodyBytes, _ := io.ReadAll(resp.Body)
	s.Contains(string(bodyBytes), "password", "Error message should mention password") // Case-insensitive check
}

func (s *AuthSecurityTestSuite) TestPasswordMismatchRejected() {
	s.Require().NotNil(s.app, "App should be initialized")

	// Create form data
	formData := url.Values{}
	formData.Set("username", "mismatch")
	formData.Set("password", "SecurePass123!")
	formData.Set("confirm_password", "DifferentPass123!")

	req := httptest.NewRequest("POST", "/register", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.NotNil(resp)

	// Should fail validation
	s.True(resp.StatusCode >= 400, "Should return error status code")

	bodyBytes, _ := io.ReadAll(resp.Body)
	s.Contains(string(bodyBytes), "match", "Error message should mention password mismatch")
}

func (s *AuthSecurityTestSuite) TestSessionExpiration() {
	user := s.CreateTestUser("expiretest", "SecurePass123!")

	// Create session with short TTL
	sessionID := uuid.NewString()
	session := sessions.NewSession(
		sessionID,
		user.ID.String(),
		user.Username,
		time.Now().Unix(),
		time.Now().Unix(),
	)

	// Save with 1 second TTL
	ctx, cancel := context.WithTimeout(s.Ctx, 5*time.Second)
	defer cancel()

	err := s.Redis.HSet(ctx, "session:"+sessionID, session.Marshal()).Err()
	s.NoError(err, "Failed to set session in Redis")

	err = s.Redis.Expire(ctx, "session:"+sessionID, 1*time.Second).Err()
	s.NoError(err, "Failed to set expiration on session")

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Session should be gone
	retrievedSession, err := s.SessionMgr.GetSession(s.Ctx, sessionID)
	s.NoError(err, "GetSession should not return error")
	s.Nil(retrievedSession, "Session should have expired")
}

func (s *AuthSecurityTestSuite) TestSuccessfulRegistration() {
	s.Require().NotNil(s.app, "App should be initialized")

	// Create form data with valid credentials
	formData := url.Values{}
	formData.Set("username", "validuser")
	formData.Set("password", "SecurePass123!")
	formData.Set("confirm_password", "SecurePass123!")

	req := httptest.NewRequest("POST", "/register", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.NotNil(resp)

	// Should succeed (200 or 302 redirect)
	s.True(resp.StatusCode < 400, "Should return success status code")
}
