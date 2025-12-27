package sessions

import (
	"context"
	"exc6/pkg/breaker"
	"exc6/pkg/logger"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
)

type Session struct {
	SessionID    string
	UserID       string
	Username     string
	LastActivity int64
	LoginTime    int64
}

func NewSession(sessionID, userID, username string, lastActivity, loginTime int64) *Session {
	return &Session{
		SessionID:    sessionID,
		UserID:       userID,
		Username:     username,
		LastActivity: lastActivity,
		LoginTime:    loginTime,
	}
}

func (s *Session) Marshal() map[string]any {
	return map[string]any{
		"session_id":    s.SessionID,
		"user_id":       s.UserID,
		"username":      s.Username,
		"last_activity": s.LastActivity,
		"login_time":    s.LoginTime,
	}
}

func (s *Session) Unmarshal(data map[string]string) error {
	s.SessionID = data["session_id"]
	s.UserID = data["user_id"]
	s.Username = data["username"]

	var err error
	s.LastActivity, err = strconv.ParseInt(data["last_activity"], 10, 64)
	if err != nil {
		return err
	}

	s.LoginTime, err = strconv.ParseInt(data["login_time"], 10, 64)
	return err
}

type SessionManager struct {
	rdb *redis.Client
	cb  *gobreaker.CircuitBreaker
}

func NewSessionManager(rdb *redis.Client) *SessionManager {
	return &SessionManager{
		rdb: rdb,
		cb: breaker.New(breaker.Config{
			Name:        "redis-sessions",
			MaxRequests: 5,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
			Threshold:   0.5,
			MinRequests: 5,
		}),
	}
}

func (smngr *SessionManager) SaveSession(ctx context.Context, session *Session) error {
	sessionKey := "session:" + session.SessionID

	_, err := breaker.ExecuteCtx(ctx, smngr.cb, func() (interface{}, error) {
		if err := smngr.rdb.HSet(ctx, sessionKey, session.Marshal()).Err(); err != nil {
			return nil, err
		}
		return nil, smngr.rdb.Expire(ctx, sessionKey, 24*time.Hour).Err()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"session_id": session.SessionID,
			"error":      err.Error(),
		}).Error("Circuit breaker: Failed to save session")
		return err
	}

	return nil
}

func (smngr *SessionManager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	sessionKey := "session:" + sessionID

	result, err := breaker.ExecuteCtx(ctx, smngr.cb, func() (interface{}, error) {
		return smngr.rdb.HGetAll(ctx, sessionKey).Result()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"session_id": sessionID,
			"error":      err.Error(),
		}).Error("Circuit breaker: Failed to get session")
		return nil, err
	}

	sessionData := result.(map[string]string)
	if len(sessionData) == 0 {
		return nil, nil
	}

	session := &Session{}
	if err := session.Unmarshal(sessionData); err != nil {
		return nil, err
	}

	return session, nil
}

func (smngr *SessionManager) ListActiveSessions(ctx context.Context) ([]*Session, error) {
	var sessions []*Session

	result, err := breaker.ExecuteCtx(ctx, smngr.cb, func() (interface{}, error) {
		iter := smngr.rdb.Scan(ctx, 0, "session:*", 0).Iterator()
		var keys []string
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}
		if err := iter.Err(); err != nil {
			return nil, err
		}
		return keys, nil
	})

	if err != nil {
		logger.WithError(err).Error("Circuit breaker: Failed to list sessions")
		return nil, err
	}

	keys := result.([]string)
	for _, key := range keys {
		sessionData, err := smngr.rdb.HGetAll(ctx, key).Result()
		if err != nil {
			continue
		}

		session := &Session{}
		if err := session.Unmarshal(sessionData); err != nil {
			continue
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (smngr *SessionManager) UpdateSessionField(ctx context.Context, sessionID, field, value string) error {
	sessionKey := "session:" + sessionID

	_, err := breaker.ExecuteCtx(ctx, smngr.cb, func() (interface{}, error) {
		exists, err := smngr.rdb.Exists(ctx, sessionKey).Result()
		if err != nil {
			return nil, err
		}
		if exists == 0 {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, smngr.rdb.HSet(ctx, sessionKey, field, value).Err()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"session_id": sessionID,
			"field":      field,
			"error":      err.Error(),
		}).Error("Circuit breaker: Failed to update session field")
		return err
	}

	return nil
}

func (smngr *SessionManager) RenewSession(ctx context.Context, sessionID string) error {
	sessionKey := "session:" + sessionID

	_, err := breaker.ExecuteCtx(ctx, smngr.cb, func() (interface{}, error) {
		exists, err := smngr.rdb.Exists(ctx, sessionKey).Result()
		if err != nil {
			return nil, err
		}
		if exists == 0 {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}

		// Update last activity timestamp
		if err := smngr.rdb.HSet(ctx, sessionKey, "last_activity", time.Now().Unix()).Err(); err != nil {
			return nil, err
		}

		// Renew TTL
		return nil, smngr.rdb.Expire(ctx, sessionKey, 24*time.Hour).Err()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"session_id": sessionID,
			"error":      err.Error(),
		}).Error("Circuit breaker: Failed to renew session")
		return err
	}

	return nil
}

func (smngr *SessionManager) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := breaker.ExecuteCtx(ctx, smngr.cb, func() (interface{}, error) {
		return nil, smngr.rdb.Del(ctx, "session:"+sessionID).Err()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"session_id": sessionID,
			"error":      err.Error(),
		}).Error("Circuit breaker: Failed to delete session")
		return err
	}

	return nil
}

// GetMetrics returns circuit breaker metrics
func (smngr *SessionManager) GetMetrics() map[string]interface{} {
	state := smngr.cb.State()
	counts := smngr.cb.Counts()

	return map[string]interface{}{
		"state":                 state.String(),
		"total_requests":        counts.Requests,
		"total_successes":       counts.TotalSuccesses,
		"total_failures":        counts.TotalFailures,
		"consecutive_successes": counts.ConsecutiveSuccesses,
		"consecutive_failures":  counts.ConsecutiveFailures,
	}
}
