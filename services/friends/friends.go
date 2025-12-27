package friends

import (
	"context"
	"exc6/apperrors"
	"exc6/db"
	"time"

	"github.com/google/uuid"
)

// FriendService handles friend-related operations
type FriendService struct {
	qdb *db.Queries
}

func NewFriendService(qdb *db.Queries) *FriendService {
	return &FriendService{qdb: qdb}
}

// FriendInfo represents a friend with their user details
type FriendInfo struct {
	FriendID   string
	Username   string
	Icon       string
	CustomIcon string
	Accepted   bool
	CreatedAt  time.Time
}

// GetUserFriends returns all accepted friends for a user
func (fs *FriendService) GetUserFriends(ctx context.Context, username string) ([]FriendInfo, error) {
	// Get user
	user, err := fs.qdb.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, apperrors.NewUserNotFound()
	}

	// [FIX] Use optimized query to avoid N+1 loop
	rows, err := fs.qdb.GetFriendsWithDetails(ctx, uuid.NullUUID{UUID: user.ID, Valid: true})
	if err != nil {
		return nil, apperrors.NewDatabaseError("get friends", err)
	}

	friends := make([]FriendInfo, 0, len(rows))
	for _, row := range rows {
		friends = append(friends, FriendInfo{
			FriendID:   row.ID.String(),
			Username:   row.Username,
			Icon:       row.Icon.String,
			CustomIcon: row.CustomIcon.String,
			Accepted:   row.Accepted,
			CreatedAt:  row.CreatedAt,
		})
	}

	return friends, nil
}

// GetFriendRequests returns pending friend requests for a user
func (fs *FriendService) GetFriendRequests(ctx context.Context, username string) ([]FriendInfo, error) {
	user, err := fs.qdb.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, apperrors.NewUserNotFound()
	}

	requests, err := fs.qdb.GetFriendRequests(ctx, uuid.NullUUID{UUID: user.ID, Valid: true})
	if err != nil {
		return nil, apperrors.NewDatabaseError("get friend requests", err)
	}

	result := make([]FriendInfo, 0, len(requests))
	for _, req := range requests {
		if !req.UserID.Valid {
			continue
		}

		requester, err := fs.qdb.GetUserByID(ctx, req.UserID.UUID)
		if err != nil {
			continue
		}

		result = append(result, FriendInfo{
			FriendID:   requester.ID.String(),
			Username:   requester.Username,
			Icon:       requester.Icon.String,
			CustomIcon: requester.CustomIcon.String,
			Accepted:   false,
			CreatedAt:  req.CreatedAt,
		})
	}

	return result, nil
}

// SendFriendRequest sends a friend request to another user
func (fs *FriendService) SendFriendRequest(ctx context.Context, fromUsername, toUsername string) error {
	if fromUsername == toUsername {
		return apperrors.NewBadRequest("Cannot send friend request to yourself")
	}

	fromUser, err := fs.qdb.GetUserByUsername(ctx, fromUsername)
	if err != nil {
		return apperrors.NewUserNotFound()
	}

	toUser, err := fs.qdb.GetUserByUsername(ctx, toUsername)
	if err != nil {
		return apperrors.NewBadRequest("User not found")
	}

	// Check if friendship already exists
	existing, _ := fs.qdb.GetFriends(ctx, uuid.NullUUID{UUID: fromUser.ID, Valid: true})
	for _, f := range existing {
		if (f.UserID.Valid && f.UserID.UUID == toUser.ID) ||
			(f.FriendID.Valid && f.FriendID.UUID == toUser.ID) {
			return apperrors.NewBadRequest("Friend request already exists")
		}
	}

	_, err = fs.qdb.AddFriend(ctx, db.AddFriendParams{
		UserID:   uuid.NullUUID{UUID: fromUser.ID, Valid: true},
		FriendID: uuid.NullUUID{UUID: toUser.ID, Valid: true},
	})

	if err != nil {
		return apperrors.NewDatabaseError("send friend request", err)
	}

	return nil
}

// AcceptFriendRequest accepts a pending friend request
func (fs *FriendService) AcceptFriendRequest(ctx context.Context, username, requesterUsername string) error {
	user, err := fs.qdb.GetUserByUsername(ctx, username)
	if err != nil {
		return apperrors.NewUserNotFound()
	}

	requester, err := fs.qdb.GetUserByUsername(ctx, requesterUsername)
	if err != nil {
		return apperrors.NewBadRequest("Requester not found")
	}

	_, err = fs.qdb.AcceptFriend(ctx, db.AcceptFriendParams{
		UserID:   uuid.NullUUID{UUID: requester.ID, Valid: true},
		FriendID: uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	if err != nil {
		return apperrors.NewDatabaseError("accept friend request", err)
	}

	return nil
}

// RemoveFriend removes a friendship (works for both accepted and pending)
func (fs *FriendService) RemoveFriend(ctx context.Context, username, friendUsername string) error {
	user, err := fs.qdb.GetUserByUsername(ctx, username)
	if err != nil {
		return apperrors.NewUserNotFound()
	}

	friend, err := fs.qdb.GetUserByUsername(ctx, friendUsername)
	if err != nil {
		return apperrors.NewBadRequest("Friend not found")
	}

	// Try removing in both directions (friendship can be stored either way)
	_, err1 := fs.qdb.RemoveFreind(ctx, db.RemoveFreindParams{
		UserID:   uuid.NullUUID{UUID: user.ID, Valid: true},
		FriendID: uuid.NullUUID{UUID: friend.ID, Valid: true},
	})

	_, err2 := fs.qdb.RemoveFreind(ctx, db.RemoveFreindParams{
		UserID:   uuid.NullUUID{UUID: friend.ID, Valid: true},
		FriendID: uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	// If both fail, return error
	if err1 != nil && err2 != nil {
		return apperrors.NewBadRequest("Friendship not found")
	}

	return nil
}

// SearchUsers searches for users by username (for adding friends)
func (fs *FriendService) SearchUsers(ctx context.Context, currentUsername, query string) ([]FriendInfo, error) {
	if query == "" {
		return []FriendInfo{}, nil
	}

	// Get current user
	currentUser, err := fs.qdb.GetUserByUsername(ctx, currentUsername)
	if err != nil {
		return nil, apperrors.NewUserNotFound()
	}

	// Get all usernames (in production, you'd want a proper search query)
	allUsernames, err := fs.qdb.GetAllUsernames(ctx)
	if err != nil {
		return nil, apperrors.NewDatabaseError("search users", err)
	}

	// Get current friends to exclude them
	friendships, _ := fs.qdb.GetFriends(ctx, uuid.NullUUID{UUID: currentUser.ID, Valid: true})
	friendMap := make(map[string]bool)
	for _, f := range friendships {
		if f.UserID.Valid && f.UserID.UUID != currentUser.ID {
			friendMap[f.UserID.UUID.String()] = true
		}
		if f.FriendID.Valid && f.FriendID.UUID != currentUser.ID {
			friendMap[f.FriendID.UUID.String()] = true
		}
	}

	// Filter matching usernames
	results := make([]FriendInfo, 0)
	for _, username := range allUsernames {
		// Skip current user
		if username == currentUsername {
			continue
		}

		// Simple contains search
		if len(username) >= len(query) && username[:len(query)] == query {
			user, err := fs.qdb.GetUserByUsername(ctx, username)
			if err != nil {
				continue
			}

			// Skip if already friends
			if friendMap[user.ID.String()] {
				continue
			}

			results = append(results, FriendInfo{
				FriendID:   user.ID.String(),
				Username:   user.Username,
				Icon:       user.Icon.String,
				CustomIcon: user.CustomIcon.String,
			})

			// Limit results
			if len(results) >= 10 {
				break
			}
		}
	}

	return results, nil
}
