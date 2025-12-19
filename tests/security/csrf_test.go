package security_test

import (
	"crypto/rand"
	"encoding/base64"
	"exc6/server"
	"exc6/server/middleware/csrf"
	"exc6/tests/setup"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"
)

type CSRFTestSuite struct {
	setup.TestSuite
	app *fiber.App
}

func TestCSRFSuite(t *testing.T) {
	suite.Run(t, new(CSRFTestSuite))
}

func (s *CSRFTestSuite) SetupSuite() {
	s.TestSuite.SetupSuite()

	srv, err := server.NewServer(s.Config, s.Queries, s.Redis, s.ChatSvc, s.SessionMgr, s.FriendSvc, s.GroupSvc)
	s.Require().NoError(err)
	s.app = srv.App
}

func (s *CSRFTestSuite) TestCSRFTokenGeneration() {
	user := s.CreateTestUser("csrfuser", "pass123")
	sessionID := s.CreateTestSession(user.Username, user.ID.String())

	req := httptest.NewRequest("GET", "/profile", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	resp, err := s.app.Test(req)
	s.NoError(err)

	// Should have CSRF token cookie
	cookies := resp.Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "csrf_token" {
			found = true
			s.NotEmpty(cookie.Value)
		}
	}
	s.True(found, "CSRF token cookie should be set")
}

func (s *CSRFTestSuite) TestPOSTWithoutCSRFToken() {
	user := s.CreateTestUser("nocsrf", "pass123")
	sessionID := s.CreateTestSession(user.Username, user.ID.String())

	req := httptest.NewRequest("POST", "/profile", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	resp, err := s.app.Test(req)
	s.NoError(err)
	s.Equal(http.StatusForbidden, resp.StatusCode) // Should be blocked
}

func (s *CSRFTestSuite) TestPOSTWithValidCSRFToken() {
	user := s.CreateTestUser("validcsrf", "pass123")
	sessionID := s.CreateTestSession(user.Username, user.ID.String())

	// Generate CSRF token
	storage := csrf.NewRedisStorage(s.Redis, 1*time.Hour)
	token, err := generateCSRFToken(storage, sessionID, 15*time.Minute)
	s.NoError(err)

	req := httptest.NewRequest("POST", "/profile", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})
	req.Header.Set("X-CSRF-Token", token)

	resp, err := s.app.Test(req)
	s.NoError(err)
	s.NotEqual(http.StatusForbidden, resp.StatusCode) // Should not be blocked
}

func generateCSRFToken(storage csrf.Storage, sessionID string, expiration time.Duration) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	token := base64.URLEncoding.EncodeToString(bytes)

	if err := storage.Set(sessionID, token, expiration); err != nil {
		return "", err
	}

	return token, nil
}
