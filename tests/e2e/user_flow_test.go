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

func (s *E2ETestSuite) TestCompleteUserJourney() {
	// 1. Register new user
	username := "e2euser"
	password := "SecurePass123!"

	registerReq := httptest.NewRequest("POST", "/register", strings.NewReader(
		fmt.Sprintf("username=%s&password=%s&confirm_password=%s", username, password, password),
	))
	registerReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	registerResp, err := s.app.Test(registerReq)
	s.NoError(err)
	s.Equal(http.StatusOK, registerResp.StatusCode)

	// 2. Login
	loginReq := httptest.NewRequest("POST", "/login", strings.NewReader(
		fmt.Sprintf("username=%s&password=%s", username, password),
	))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	loginResp, err := s.app.Test(loginReq)
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

	dashboardResp, err := s.app.Test(dashboardReq)
	s.NoError(err)
	s.Equal(http.StatusOK, dashboardResp.StatusCode)

	// 4. View profile
	profileReq := httptest.NewRequest("GET", "/profile", nil)
	profileReq.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	profileResp, err := s.app.Test(profileReq)
	s.NoError(err)
	s.Equal(http.StatusOK, profileResp.StatusCode)

	// 5. Logout
	logoutReq := httptest.NewRequest("POST", "/logout", nil)
	logoutReq.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	logoutResp, err := s.app.Test(logoutReq)
	s.NoError(err)

	_ = logoutResp

	// 6. Try to access dashboard after logout (should fail)
	dashboardReq2 := httptest.NewRequest("GET", "/dashboard", nil)
	dashboardReq2.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})

	dashboardResp2, err := s.app.Test(dashboardReq2)
	s.NoError(err)
	s.Equal(http.StatusFound, dashboardResp2.StatusCode) // Should redirect
}

func (s *E2ETestSuite) TestFriendRequestWorkflow() {
	// Create two users
	user1 := s.CreateTestUser("friend1", "pass123")
	user2 := s.CreateTestUser("friend2", "pass123")

	sessionID1 := s.CreateTestSession(user1.Username, user1.ID.String())
	sessionID2 := s.CreateTestSession(user2.Username, user2.ID.String())

	// 1. User1 sends friend request
	sendReq := httptest.NewRequest("POST", "/friends/request/"+user2.Username, nil)
	sendReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID1})

	sendResp, err := s.app.Test(sendReq)
	s.NoError(err)
	s.Equal(http.StatusOK, sendResp.StatusCode)

	// 2. User2 sees pending request
	friendsReq := httptest.NewRequest("GET", "/friends", nil)
	friendsReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID2})

	friendsResp, err := s.app.Test(friendsReq)
	s.NoError(err)
	body, _ := io.ReadAll(friendsResp.Body)
	s.Contains(string(body), user1.Username) // Should see request from user1

	// 3. User2 accepts request
	acceptReq := httptest.NewRequest("POST", "/friends/accept/"+user1.Username, nil)
	acceptReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID2})

	acceptResp, err := s.app.Test(acceptReq)
	s.NoError(err)
	s.Equal(http.StatusOK, acceptResp.StatusCode)

	// 4. Both users should see each other as friends
	// Verify in database
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

	chatResp, err := s.app.Test(chatReq)
	s.NoError(err)
	s.Equal(http.StatusOK, chatResp.StatusCode)

	// 2. Send message
	sendReq := httptest.NewRequest("POST", "/chat/"+user2.Username,
		strings.NewReader("content=Hello friend!"))
	sendReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID1})
	sendReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	sendResp, err := s.app.Test(sendReq)
	s.NoError(err)
	s.Equal(http.StatusOK, sendResp.StatusCode)

	// 3. Verify message in history
	time.Sleep(500 * time.Millisecond) // Wait for caching

	history, err := s.ChatSvc.GetHistory(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)
	s.Len(history, 1)
	s.Equal("Hello friend!", history[0].Content)
}
