package e2e_test

import (
	"exc6/server"
	"exc6/tests/setup"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"
)

type E2ETestSuite struct {
	setup.TestSuite
	app *fiber.App
}

func TestE2ESuite(t *testing.T) {
	suite.Run(t, new(E2ETestSuite))
}

func (s *E2ETestSuite) SetupSuite() {
	s.TestSuite.SetupSuite()

	srv, err := server.NewServer(s.Config, s.Queries, s.Redis, s.ChatSvc, s.SessionMgr, s.FriendSvc)
	s.Require().NoError(err)
	s.app = srv.App
}

// Helper: Extract CSRF token from response cookies
func (s *E2ETestSuite) extractCSRFToken(resp *http.Response) string {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "csrf_token" {
			return cookie.Value
		}
	}
	return ""
}

// Helper: Make authenticated request with CSRF token
func (s *E2ETestSuite) makeAuthRequest(method, path, body, sessionID string) (*http.Response, error) {
	// For non-GET requests, get CSRF token first
	if method != "GET" && method != "HEAD" {
		getReq := httptest.NewRequest("GET", "/profile", nil)
		getReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})

		getResp, err := s.app.Test(getReq, -1)
		if err != nil {
			return nil, err
		}

		csrfToken := s.extractCSRFToken(getResp)

		// Now make the actual request with CSRF token
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})

		if csrfToken != "" {
			req.Header.Set("X-CSRF-Token", csrfToken)
		}

		return s.app.Test(req, -1)
	}

	// For GET requests, just add session
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	return s.app.Test(req, -1)
}

func (s *E2ETestSuite) TestCompleteUserJourney() {
	// 1. Register new user
	username := "e2euser"
	password := "SecurePass123!"

	registerReq := httptest.NewRequest("POST", "/register", strings.NewReader(
		fmt.Sprintf("username=%s&password=%s&confirm_password=%s", username, password, password),
	))
	registerReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	registerResp, err := s.app.Test(registerReq, -1)
	s.NoError(err)
	s.Equal(http.StatusOK, registerResp.StatusCode)

	// 2. Login
	loginReq := httptest.NewRequest("POST", "/login", strings.NewReader(
		fmt.Sprintf("username=%s&password=%s", username, password),
	))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	loginResp, err := s.app.Test(loginReq, -1)
	s.NoError(err)

	// Extract session cookie
	var sessionID string
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "session_id" {
			sessionID = cookie.Value
		}
	}
	s.NotEmpty(sessionID)

	// 3. Access dashboard
	dashboardReq := httptest.NewRequest("GET", "/dashboard", nil)
	dashboardReq.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	dashboardResp, err := s.app.Test(dashboardReq, -1)
	s.NoError(err)
	s.Equal(http.StatusOK, dashboardResp.StatusCode)

	// 4. View profile
	profileReq := httptest.NewRequest("GET", "/profile", nil)
	profileReq.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	profileResp, err := s.app.Test(profileReq, -1)
	s.NoError(err)
	s.Equal(http.StatusOK, profileResp.StatusCode)

	// 5. Logout
	logoutReq := httptest.NewRequest("POST", "/logout", nil)
	logoutReq.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	logoutResp, err := s.app.Test(logoutReq, -1)
	s.NoError(err)

	_ = logoutResp.Body.Close()

	// 6. Try to access dashboard after logout (should redirect)
	dashboardReq2 := httptest.NewRequest("GET", "/dashboard", nil)
	dashboardReq2.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	dashboardResp2, err := s.app.Test(dashboardReq2, -1)
	s.NoError(err)
	// Should redirect to login
	s.True(dashboardResp2.StatusCode == http.StatusFound || dashboardResp2.StatusCode == http.StatusUnauthorized)
}

func (s *E2ETestSuite) TestFriendRequestWorkflow() {
	// Create two users
	user1 := s.CreateTestUser("friend1", "pass123")
	user2 := s.CreateTestUser("friend2", "pass123")

	sessionID1 := s.CreateTestSession(user1.Username, user1.ID.String())
	sessionID2 := s.CreateTestSession(user2.Username, user2.ID.String())

	// 1. User1 sends friend request (with CSRF)
	sendResp, err := s.makeAuthRequest("POST", "/friends/request/"+user2.Username, "", sessionID1)
	s.NoError(err)
	s.Equal(http.StatusOK, sendResp.StatusCode)

	// 2. User2 sees pending request
	friendsReq := httptest.NewRequest("GET", "/friends", nil)
	friendsReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID2})

	friendsResp, err := s.app.Test(friendsReq, -1)
	s.NoError(err)
	body, _ := io.ReadAll(friendsResp.Body)
	s.Contains(string(body), user1.Username) // Should see request from user1

	// 3. User2 accepts request (with CSRF)
	acceptResp, err := s.makeAuthRequest("POST", "/friends/accept/"+user1.Username, "", sessionID2)
	s.NoError(err)
	s.Equal(http.StatusOK, acceptResp.StatusCode)

	// 4. Both users should see each other as friends
	friends1, err := s.FriendSvc.GetUserFriends(s.Ctx, user1.Username)
	s.NoError(err)
	s.Len(friends1, 1)

	friends2, err := s.FriendSvc.GetUserFriends(s.Ctx, user2.Username)
	s.NoError(err)
	s.Len(friends2, 1)
}

func (s *E2ETestSuite) TestChatWorkflow() {
	// Create two friends
	user1 := s.CreateTestUser("chat1", "pass123")
	user2 := s.CreateTestUser("chat2", "pass123")

	// Make them friends
	err := s.FriendSvc.SendFriendRequest(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)
	err = s.FriendSvc.AcceptFriendRequest(s.Ctx, user2.Username, user1.Username)
	s.NoError(err)

	sessionID1 := s.CreateTestSession(user1.Username, user1.ID.String())

	// 1. Load chat window
	chatReq := httptest.NewRequest("GET", "/chat/"+user2.Username, nil)
	chatReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID1})

	chatResp, err := s.app.Test(chatReq, -1)
	s.NoError(err)
	s.Equal(http.StatusOK, chatResp.StatusCode)

	// 2. Send message (with CSRF)
	sendResp, err := s.makeAuthRequest("POST", "/chat/"+user2.Username, "content=Hello friend!", sessionID1)
	s.NoError(err)
	s.Equal(http.StatusOK, sendResp.StatusCode)

	// 3. Verify message in history
	time.Sleep(500 * time.Millisecond) // Wait for caching

	history, err := s.ChatSvc.GetHistory(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)
	s.Require().Len(history, 1, "Should have exactly 1 message")
	s.Equal("Hello friend!", history[0].Content)
}
