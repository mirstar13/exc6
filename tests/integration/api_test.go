package integration_test

import (
	"encoding/json"
	"exc6/server"
	"exc6/tests/setup"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"
)

type APITestSuite struct {
	setup.TestSuite
	app *fiber.App
}

func TestAPISuite(t *testing.T) {
	suite.Run(t, new(APITestSuite))
}

func (s *APITestSuite) SetupSuite() {
	s.TestSuite.SetupSuite()

	// Create test server
	srv, err := server.NewServer(s.Config, s.Queries, s.Redis, s.ChatSvc, s.SessionMgr, s.FriendSvc)
	s.Require().NoError(err)
	s.app = srv.App
}

func (s *APITestSuite) TestHealthCheck() {
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	resp, err := s.app.Test(req)
	s.NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	s.Equal("operational", result["status"])
}

func (s *APITestSuite) TestRegisterEndpoint() {
	formData := url.Values{}
	formData.Set("username", "apitest")
	formData.Set("password", "SecurePass123!")
	formData.Set("confirm_password", "SecurePass123!")

	req := httptest.NewRequest("POST", "/register", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.True(resp.StatusCode < 400, "Registration should succeed with status < 400, got: %d", resp.StatusCode)

	resp.Body.Close()
}

func (s *APITestSuite) TestLoginEndpoint() {
	// Create user first
	user := s.CreateTestUser("loginapi", "TestPass123!")

	// Create form data
	formData := url.Values{}
	formData.Set("username", user.Username)
	formData.Set("password", "TestPass123!")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.app.Test(req)
	s.NoError(err)

	// Should set session cookie
	cookies := resp.Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "session_id" {
			found = true
			s.NotEmpty(cookie.Value)
		}
	}
	s.True(found, "Session cookie should be set")
}

func (s *APITestSuite) TestProtectedEndpointWithoutAuth() {
	req := httptest.NewRequest("GET", "/dashboard", nil)
	resp, err := s.app.Test(req)
	s.NoError(err)
	s.Equal(http.StatusFound, resp.StatusCode) // Should redirect
}

func (s *APITestSuite) TestProtectedEndpointWithAuth() {
	user := s.CreateTestUser("authtest", "pass123")
	sessionID := s.CreateTestSession(user.Username, user.ID.String())

	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	resp, err := s.app.Test(req)
	s.NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
}
