package friends

import (
	"context"
	"exc6/apperrors"
	"exc6/db"
	"exc6/pkg/breaker"
	"exc6/pkg/logger"
	"time"

	"github.com/google/uuid"
	"github.com/sony/gobreaker"
)

// FriendService handles friend-related operations
type FriendService struct {
	qdb *db.Queries
	cb  *gobreaker.CircuitBreaker
}

func NewFriendService(qdb *db.Queries) *FriendService {
	return &FriendService{
		qdb: qdb,
		cb: breaker.New(breaker.Config{
			Name:        "postgres-friends",
			MaxRequests: 10,
			Interval:    60 * time.Second,
			Timeout:     45 * time.Second,
			Threshold:   0.6, // Higher threshold for DB
			MinRequests: 10,
		}),
	}
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
	result, err := breaker.ExecuteCtx(ctx, fs.cb, func() (interface{}, error) {
		// Get user
		user, err := fs.qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
		}

		// Use optimized query
		rows, err := fs.qdb.GetFriendsWithDetails(ctx, uuid.NullUUID{UUID: user.ID, Valid: true})
		if err != nil {
			return nil, err
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
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username": username,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to get user friends")
		return nil, apperrors.NewDatabaseError("get friends", err)
	}

	return result.([]FriendInfo), nil
}

// GetFriendRequests returns pending friend requests for a user
func (fs *FriendService) GetFriendRequests(ctx context.Context, username string) ([]FriendInfo, error) {
	result, err := breaker.ExecuteCtx(ctx, fs.cb, func() (interface{}, error) {
		user, err := fs.qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
		}

		requests, err := fs.qdb.GetFriendRequests(ctx, uuid.NullUUID{UUID: user.ID, Valid: true})
		if err != nil {
			return nil, err
		}

		friends := make([]FriendInfo, 0, len(requests))
		for _, req := range requests {
			if !req.UserID.Valid {
				continue
			}

			requester, err := fs.qdb.GetUserByID(ctx, req.UserID.UUID)
			if err != nil {
				continue
			}

			friends = append(friends, FriendInfo{
				FriendID:   requester.ID.String(),
				Username:   requester.Username,
				Icon:       requester.Icon.String,
				CustomIcon: requester.CustomIcon.String,
				Accepted:   false,
				CreatedAt:  req.CreatedAt,
			})
		}

		return friends, nil
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username": username,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to get friend requests")
		return nil, apperrors.NewDatabaseError("get friend requests", err)
	}

	return result.([]FriendInfo), nil
}

// SendFriendRequest sends a friend request to another user
func (fs *FriendService) SendFriendRequest(ctx context.Context, fromUsername, toUsername string) error {
	if fromUsername == toUsername {
		return apperrors.NewBadRequest("Cannot send friend request to yourself")
	}

	_, err := breaker.ExecuteCtx(ctx, fs.cb, func() (interface{}, error) {
		fromUser, err := fs.qdb.GetUserByUsername(ctx, fromUsername)
		if err != nil {
			return nil, err
		}

		toUser, err := fs.qdb.GetUserByUsername(ctx, toUsername)
		if err != nil {
			return nil, apperrors.NewBadRequest("User not found")
		}

		// Check if friendship already exists
		existing, _ := fs.qdb.GetFriends(ctx, uuid.NullUUID{UUID: fromUser.ID, Valid: true})
		for _, f := range existing {
			if (f.UserID.Valid && f.UserID.UUID == toUser.ID) ||
				(f.FriendID.Valid && f.FriendID.UUID == toUser.ID) {
				return nil, apperrors.NewBadRequest("Friend request already exists")
			}
		}

		_, err = fs.qdb.AddFriend(ctx, db.AddFriendParams{
			UserID:   uuid.NullUUID{UUID: fromUser.ID, Valid: true},
			FriendID: uuid.NullUUID{UUID: toUser.ID, Valid: true},
		})

		return nil, err
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"from":  fromUsername,
			"to":    toUsername,
			"error": err.Error(),
		}).Error("Circuit breaker: Failed to send friend request")
		return apperrors.NewDatabaseError("send friend request", err)
	}

	return nil
}

// AcceptFriendRequest accepts a pending friend request
func (fs *FriendService) AcceptFriendRequest(ctx context.Context, username, requesterUsername string) error {
	_, err := breaker.ExecuteCtx(ctx, fs.cb, func() (interface{}, error) {
		user, err := fs.qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
		}

		requester, err := fs.qdb.GetUserByUsername(ctx, requesterUsername)
		if err != nil {
			return nil, apperrors.NewBadRequest("Requester not found")
		}

		_, err = fs.qdb.AcceptFriend(ctx, db.AcceptFriendParams{
			UserID:   uuid.NullUUID{UUID: requester.ID, Valid: true},
			FriendID: uuid.NullUUID{UUID: user.ID, Valid: true},
		})

		return nil, err
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username":  username,
			"requester": requesterUsername,
			"error":     err.Error(),
		}).Error("Circuit breaker: Failed to accept friend request")
		return apperrors.NewDatabaseError("accept friend request", err)
	}

	return nil
}

// RemoveFriend removes a friendship
func (fs *FriendService) RemoveFriend(ctx context.Context, username, friendUsername string) error {
	_, err := breaker.ExecuteCtx(ctx, fs.cb, func() (interface{}, error) {
		user, err := fs.qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
		}

		friend, err := fs.qdb.GetUserByUsername(ctx, friendUsername)
		if err != nil {
			return nil, apperrors.NewBadRequest("Friend not found")
		}

		// Try removing in both directions
		_, err1 := fs.qdb.RemoveFreind(ctx, db.RemoveFreindParams{
			UserID:   uuid.NullUUID{UUID: user.ID, Valid: true},
			FriendID: uuid.NullUUID{UUID: friend.ID, Valid: true},
		})

		_, err2 := fs.qdb.RemoveFreind(ctx, db.RemoveFreindParams{
			UserID:   uuid.NullUUID{UUID: friend.ID, Valid: true},
			FriendID: uuid.NullUUID{UUID: user.ID, Valid: true},
		})

		if err1 != nil && err2 != nil {
			return nil, apperrors.NewBadRequest("Friendship not found")
		}

		return nil, nil
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username": username,
			"friend":   friendUsername,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to remove friend")
		return err
	}

	return nil
}

// SearchUsers searches for users by username
func (fs *FriendService) SearchUsers(ctx context.Context, currentUsername, query string) ([]FriendInfo, error) {
	if query == "" {
		return []FriendInfo{}, nil
	}

	result, err := breaker.ExecuteCtx(ctx, fs.cb, func() (interface{}, error) {
		currentUser, err := fs.qdb.GetUserByUsername(ctx, currentUsername)
		if err != nil {
			return nil, err
		}

		allUsernames, err := fs.qdb.GetAllUsernames(ctx)
		if err != nil {
			return nil, err
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
			if username == currentUsername {
				continue
			}

			// Simple prefix search
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

				if len(results) >= 10 {
					break
				}
			}
		}

		return results, nil
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username": currentUsername,
			"query":    query,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to search users")
		return nil, apperrors.NewDatabaseError("search users", err)
	}

	return result.([]FriendInfo), nil
}

// GetMetrics returns circuit breaker metrics
func (fs *FriendService) GetMetrics() map[string]interface{} {
	state := fs.cb.State()
	counts := fs.cb.Counts()

	return map[string]interface{}{
		"state":                 state.String(),
		"total_requests":        counts.Requests,
		"total_successes":       counts.TotalSuccesses,
		"total_failures":        counts.TotalFailures,
		"consecutive_successes": counts.ConsecutiveSuccesses,
		"consecutive_failures":  counts.ConsecutiveFailures,
	}
}
