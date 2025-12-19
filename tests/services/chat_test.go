package services_test

import (
	"exc6/tests/setup"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ChatTestSuite struct {
	setup.TestSuite
}

func TestChatSuite(t *testing.T) {
	suite.Run(t, new(ChatTestSuite))
}

func (s *ChatTestSuite) TestSendMessage() {
	user1 := s.CreateTestUser("user1", "pass123")
	user2 := s.CreateTestUser("user2", "pass123")

	// Send message
	msg, err := s.ChatSvc.SendMessage(
		s.Ctx,
		user1.Username,
		user2.Username,
		"Hello, user2!",
	)
	s.NoError(err)
	s.NotNil(msg)
	s.Equal(user1.Username, msg.FromID)
	s.Equal(user2.Username, msg.ToID)
	s.Equal("Hello, user2!", msg.Content)
}

func (s *ChatTestSuite) TestGetChatHistory() {
	user1 := s.CreateTestUser("history1", "pass123")
	user2 := s.CreateTestUser("history2", "pass123")

	// Send multiple messages
	messages := []string{
		"First message",
		"Second message",
		"Third message",
	}

	for _, content := range messages {
		_, err := s.ChatSvc.SendMessage(s.Ctx, user1.Username, user2.Username, content)
		s.NoError(err)
		time.Sleep(100 * time.Millisecond) // Ensure different timestamps
	}

	// Wait for messages to be cached
	time.Sleep(500 * time.Millisecond)

	// Retrieve history
	history, err := s.ChatSvc.GetHistory(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)
	s.Len(history, 3)

	// Verify order (should be chronological)
	for i, msg := range history {
		s.Equal(messages[i], msg.Content)
	}
}

func (s *ChatTestSuite) TestBidirectionalChat() {
	user1 := s.CreateTestUser("bi1", "pass123")
	user2 := s.CreateTestUser("bi2", "pass123")

	// User1 sends to User2
	_, err := s.ChatSvc.SendMessage(s.Ctx, user1.Username, user2.Username, "Hi from user1")
	s.NoError(err)

	// User2 sends to User1
	_, err = s.ChatSvc.SendMessage(s.Ctx, user2.Username, user1.Username, "Hi from user2")
	s.NoError(err)

	time.Sleep(500 * time.Millisecond)

	// Both should see the same conversation
	history1, err := s.ChatSvc.GetHistory(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)

	history2, err := s.ChatSvc.GetHistory(s.Ctx, user2.Username, user1.Username)
	s.NoError(err)

	s.Len(history1, 2)
	s.Len(history2, 2)
}

func (s *ChatTestSuite) TestMessageCaching() {
	user1 := s.CreateTestUser("cache1", "pass123")
	user2 := s.CreateTestUser("cache2", "pass123")

	// Send message
	msg, err := s.ChatSvc.SendMessage(s.Ctx, user1.Username, user2.Username, "Cached message")
	s.NoError(err)

	_ = msg

	// Wait for caching
	time.Sleep(500 * time.Millisecond)

	// Message should be in cache (Redis)
	conversationKey := s.ChatSvc.GetConversationKey(user1.Username, user2.Username)
	count, err := s.Redis.ZCard(s.Ctx, conversationKey).Result()
	s.NoError(err)
	s.Equal(int64(1), count)
}
