package services_test

import (
	"exc6/tests/setup"
	"testing"

	"github.com/stretchr/testify/suite"
)

type FriendsTestSuite struct {
	setup.TestSuite
}

func TestFriendsSuite(t *testing.T) {
	suite.Run(t, new(FriendsTestSuite))
}

func (s *FriendsTestSuite) TestSendFriendRequest() {
	user1 := s.CreateTestUser("friend1", "pass123")
	user2 := s.CreateTestUser("friend2", "pass123")

	// Send friend request
	err := s.FriendSvc.SendFriendRequest(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)

	// Check pending requests
	requests, err := s.FriendSvc.GetFriendRequests(s.Ctx, user2.Username)
	s.NoError(err)
	s.Len(requests, 1)
	s.Equal(user1.Username, requests[0].Username)
	s.False(requests[0].Accepted)
}

func (s *FriendsTestSuite) TestAcceptFriendRequest() {
	user1 := s.CreateTestUser("accept1", "pass123")
	user2 := s.CreateTestUser("accept2", "pass123")

	// Send and accept request
	err := s.FriendSvc.SendFriendRequest(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)

	err = s.FriendSvc.AcceptFriendRequest(s.Ctx, user2.Username, user1.Username)
	s.NoError(err)

	// Check both users' friend lists
	friends1, err := s.FriendSvc.GetUserFriends(s.Ctx, user1.Username)
	s.NoError(err)
	s.Len(friends1, 1)
	s.Equal(user2.Username, friends1[0].Username)

	friends2, err := s.FriendSvc.GetUserFriends(s.Ctx, user2.Username)
	s.NoError(err)
	s.Len(friends2, 1)
	s.Equal(user1.Username, friends2[0].Username)

	// Pending requests should be empty
	requests, err := s.FriendSvc.GetFriendRequests(s.Ctx, user2.Username)
	s.NoError(err)
	s.Len(requests, 0)
}

func (s *FriendsTestSuite) TestRemoveFriend() {
	user1 := s.CreateTestUser("remove1", "pass123")
	user2 := s.CreateTestUser("remove2", "pass123")

	// Create friendship
	err := s.FriendSvc.SendFriendRequest(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)
	err = s.FriendSvc.AcceptFriendRequest(s.Ctx, user2.Username, user1.Username)
	s.NoError(err)

	// Remove friend
	err = s.FriendSvc.RemoveFriend(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)

	// Both should have empty friend lists
	friends1, err := s.FriendSvc.GetUserFriends(s.Ctx, user1.Username)
	s.NoError(err)
	s.Len(friends1, 0)

	friends2, err := s.FriendSvc.GetUserFriends(s.Ctx, user2.Username)
	s.NoError(err)
	s.Len(friends2, 0)
}

func (s *FriendsTestSuite) TestSearchUsers() {
	// Create multiple users
	users := []string{"alice", "alison", "bob", "charlie"}
	for _, username := range users {
		s.CreateTestUser(username, "pass123")
	}

	// Search for "ali"
	results, err := s.FriendSvc.SearchUsers(s.Ctx, "alice", "ali")
	s.NoError(err)
	s.Len(results, 1) // Only "alison" (alice herself is excluded)
	s.Equal("alison", results[0].Username)
}

func (s *FriendsTestSuite) TestCannotSendDuplicateRequest() {
	user1 := s.CreateTestUser("dup1", "pass123")
	user2 := s.CreateTestUser("dup2", "pass123")

	// Send first request
	err := s.FriendSvc.SendFriendRequest(s.Ctx, user1.Username, user2.Username)
	s.NoError(err)

	// Try to send again
	err = s.FriendSvc.SendFriendRequest(s.Ctx, user1.Username, user2.Username)
	s.Error(err) // Should fail
}
