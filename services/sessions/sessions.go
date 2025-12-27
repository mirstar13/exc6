package sessions

import (
	"context"
	"exc6/pkg/breaker"
	"exc6/pkg/logger"
	"fmt"
	"strconv"
	"sync"
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
	// Use Mutex+Map instead of sync.Map to avoid Go 1.24 HashTrieMap panic
	cache   map[string]*Session
	cacheMu sync.RWMutex
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
		cache: make(map[string]*Session),
	}
}

func (smngr *SessionManager) SaveSession(ctx context.Context, session *Session) error {
	// Save to local cache with Lock
	smngr.cacheMu.Lock()
	smngr.cache[session.SessionID] = session
	smngr.cacheMu.Unlock()

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
		}).Error("Circuit breaker: Failed to save session to Redis (persisted locally)")
		return err
	}

	return nil
}

func (smngr *SessionManager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	sessionKey := "session:" + sessionID

	// 1. Try to fetch from Redis
	result, err := breaker.ExecuteCtx(ctx, smngr.cb, func() (interface{}, error) {
		return smngr.rdb.HGetAll(ctx, sessionKey).Result()
	})

	// 2. Fallback to local cache if Redis fails
	if err != nil {
		logger.WithField("error", err).Warn("Circuit breaker open/error: Checking local session cache")

		smngr.cacheMu.RLock()
		session, ok := smngr.cache[sessionID]
		smngr.cacheMu.RUnlock()

		if ok {
			return session, nil
		}

		logger.WithFields(map[string]interface{}{
			"session_id": sessionID,
			"error":      err.Error(),
		}).Error("Circuit breaker: Failed to get session (not in cache)")
		return nil, err
	}

	sessionData := result.(map[string]string)
	if len(sessionData) == 0 {
		// Try cache if Redis returns nil
		smngr.cacheMu.RLock()
		session, ok := smngr.cache[sessionID]
		smngr.cacheMu.RUnlock()

		if ok {
			return session, nil
		}
		return nil, nil
	}

	session := &Session{}
	if err := session.Unmarshal(sessionData); err != nil {
		return nil, err
	}

	// Update local cache on successful read (Read-Through) with Lock
	smngr.cacheMu.Lock()
	smngr.cache[sessionID] = session
	smngr.cacheMu.Unlock()

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

	// Optimistic update for local cache with Lock
	smngr.cacheMu.Lock()
	if s, ok := smngr.cache[sessionID]; ok {
		// Note: We are modifying the struct in the map.
		if field == "last_activity" {
			if t, err := strconv.ParseInt(value, 10, 64); err == nil {
				s.LastActivity = t
			}
		}
	}
	smngr.cacheMu.Unlock()

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

	// Renew local cache with Lock
	smngr.cacheMu.Lock()
	if s, ok := smngr.cache[sessionID]; ok {
		s.LastActivity = time.Now().Unix()
	}
	smngr.cacheMu.Unlock()

	_, err := breaker.ExecuteCtx(ctx, smngr.cb, func() (interface{}, error) {
		exists, err := smngr.rdb.Exists(ctx, sessionKey).Result()
		if err != nil {
			return nil, err
		}
		if exists == 0 {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}

		if err := smngr.rdb.HSet(ctx, sessionKey, "last_activity", time.Now().Unix()).Err(); err != nil {
			return nil, err
		}

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
	// Delete from local cache with Lock
	smngr.cacheMu.Lock()
	delete(smngr.cache, sessionID)
	smngr.cacheMu.Unlock()

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
