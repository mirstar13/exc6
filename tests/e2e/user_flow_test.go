package e2e_test

import (
	"bytes"
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

// Helper: Extract CSRF token from response
func (s *E2ETestSuite) extractCSRFToken(resp *http.Response) string {
	// Try cookie first (most reliable)
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "csrf_token" {
			return cookie.Value
		}
	}

	// Try to extract from HTML meta tag
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	// Important: Restore the body so it can be read again if needed
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	bodyStr := string(bodyBytes)

	// Look for: <meta name="csrf-token" content="TOKEN">
	if idx := strings.Index(bodyStr, `name="csrf-token"`); idx != -1 {
		// Find the content attribute after this
		contentStart := strings.Index(bodyStr[idx:], `content="`)
		if contentStart != -1 {
			contentStart += idx + 9 // Move past 'content="'
			contentEnd := strings.Index(bodyStr[contentStart:], `"`)
			if contentEnd != -1 {
				token := bodyStr[contentStart : contentStart+contentEnd]
				return token
			}
		}
	}

	return ""
}

// Helper: Make authenticated request with automatic CSRF handling
func (s *E2ETestSuite) makeAuthRequest(method, path, body, sessionID string) (*http.Response, error) {
	// For state-changing methods, we need CSRF token
	needsCSRF := method != "GET" && method != "HEAD" && method != "OPTIONS"

	var csrfToken string

	if needsCSRF {
		// Make a separate GET request to obtain CSRF token
		getReq := httptest.NewRequest("GET", "/dashboard", nil)
		getReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})

		getResp, err := s.app.Test(getReq, -1)
		if err != nil {
			return nil, fmt.Errorf("failed to get CSRF token page: %w", err)
		}

		csrfToken = s.extractCSRFToken(getResp)
		getResp.Body.Close()

		if csrfToken == "" {
			return nil, fmt.Errorf("could not extract CSRF token from dashboard")
		}
	}

	// Create the actual request
	req := httptest.NewRequest(method, path, strings.NewReader(body))

	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	// Add session cookie
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})

	// Add CSRF token header if we have one
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}

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
	s.True(registerResp.StatusCode < 400, "Registration should succeed, got status: %d", registerResp.StatusCode)
	registerResp.Body.Close()

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
	s.NotEmpty(sessionID, "Session cookie should be set")
	loginResp.Body.Close()

	// 3. Access dashboard
	dashboardReq := httptest.NewRequest("GET", "/dashboard", nil)
	dashboardReq.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	dashboardResp, err := s.app.Test(dashboardReq, -1)
	s.NoError(err)
	s.Equal(http.StatusOK, dashboardResp.StatusCode, "Dashboard should be accessible")
	dashboardResp.Body.Close()

	// 4. View profile
	profileResp, err := s.makeAuthRequest("GET", "/profile", "", sessionID)
	s.NoError(err)
	s.Equal(http.StatusOK, profileResp.StatusCode, "Profile should be accessible")
	profileResp.Body.Close()

	// 5. Logout
	logoutResp, err := s.makeAuthRequest("POST", "/logout", "", sessionID)
	s.NoError(err)
	logoutResp.Body.Close()

	// 6. Try to access dashboard after logout (should redirect)
	dashboardReq2 := httptest.NewRequest("GET", "/dashboard", nil)
	dashboardReq2.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	dashboardResp2, err := s.app.Test(dashboardReq2, -1)
	s.NoError(err)
	s.True(dashboardResp2.StatusCode == http.StatusFound || dashboardResp2.StatusCode == http.StatusUnauthorized,
		"After logout, should redirect or return unauthorized")
	dashboardResp2.Body.Close()
}

func (s *E2ETestSuite) TestFriendRequestWorkflow() {
	// Create two users
	user1 := s.CreateTestUser("friend1", "pass123")
	user2 := s.CreateTestUser("friend2", "pass123")

	sessionID1 := s.CreateTestSession(user1.Username, user1.ID.String())
	sessionID2 := s.CreateTestSession(user2.Username, user2.ID.String())

	// 1. User1 sends friend request
	sendResp, err := s.makeAuthRequest("POST", "/friends/request/"+user2.Username, "", sessionID1)
	s.Require().NoError(err, "Should be able to send friend request")
	s.Equal(http.StatusOK, sendResp.StatusCode, "Friend request should succeed")
	sendResp.Body.Close()

	// 2. User2 sees pending request
	friendsReq := httptest.NewRequest("GET", "/friends", nil)
	friendsReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID2})

	friendsResp, err := s.app.Test(friendsReq, -1)
	s.NoError(err)
	body, _ := io.ReadAll(friendsResp.Body)
	s.Contains(string(body), user1.Username, "User2 should see friend request from User1")
	friendsResp.Body.Close()

	// 3. User2 accepts request
	acceptResp, err := s.makeAuthRequest("POST", "/friends/accept/"+user1.Username, "", sessionID2)
	s.Require().NoError(err, "Should be able to accept friend request")
	s.Equal(http.StatusOK, acceptResp.StatusCode, "Friend accept should succeed")
	acceptResp.Body.Close()

	// 4. Both users should see each other as friends
	friends1, err := s.FriendSvc.GetUserFriends(s.Ctx, user1.Username)
	s.NoError(err)
	s.Len(friends1, 1, "User1 should have 1 friend")

	friends2, err := s.FriendSvc.GetUserFriends(s.Ctx, user2.Username)
	s.NoError(err)
	s.Len(friends2, 1, "User2 should have 1 friend")
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
	chatResp, err := s.makeAuthRequest("GET", "/chat/"+user2.Username, "", sessionID1)
	s.NoError(err)
	s.Equal(http.StatusOK, chatResp.StatusCode, "Chat window should load")
	chatResp.Body.Close()

	// 2. Send message with CSRF
	sendResp, err := s.makeAuthRequest("POST", "/chat/"+user2.Username, "content=Hello friend!", sessionID1)
	s.Require().NoError(err, "Should be able to send message")
	s.Equal(http.StatusOK, sendResp.StatusCode, "Message send should succeed")
	sendResp.Body.Close()

	// 3. Verify message in history
	time.Sleep(500 * time.Millisecond)

	history, err := s.ChatSvc.GetHistory(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)
	s.Require().Len(history, 1, "Should have exactly 1 message")
	s.Equal("Hello friend!", history[0].Content, "Message content should match")
}
