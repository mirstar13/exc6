package sessions

import (
	"container/list"
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

	// LRU Cache
	cache     map[string]*list.Element
	evictList *list.List
	capacity  int
	cacheMu   sync.RWMutex
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
		cache:     make(map[string]*list.Element),
		evictList: list.New(),
		capacity:  10000, // Max 10k local sessions
	}
}

func (smngr *SessionManager) updateCache(session *Session) {
	smngr.cacheMu.Lock()
	defer smngr.cacheMu.Unlock()

	// Check if exists
	if elem, ok := smngr.cache[session.SessionID]; ok {
		smngr.evictList.MoveToFront(elem)
		elem.Value = session
		return
	}

	// Evict if full
	if smngr.evictList.Len() >= smngr.capacity {
		oldest := smngr.evictList.Back()
		if oldest != nil {
			smngr.evictList.Remove(oldest)
			s := oldest.Value.(*Session)
			delete(smngr.cache, s.SessionID)
		}
	}

	// Add new
	elem := smngr.evictList.PushFront(session)
	smngr.cache[session.SessionID] = elem
}

func (smngr *SessionManager) SaveSession(ctx context.Context, session *Session) error {
	// 1. Save to local cache synchronously (Critical for immediate consistency on this node)
	smngr.updateCache(session)

	// 2. Persist to Redis asynchronously (Write-Behind)
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sessionKey := "session:" + session.SessionID

		_, err := breaker.ExecuteCtx(bgCtx, smngr.cb, func() (interface{}, error) {
			pipe := smngr.rdb.Pipeline()
			pipe.HSet(bgCtx, sessionKey, session.Marshal())
			pipe.Expire(bgCtx, sessionKey, 24*time.Hour)
			_, err := pipe.Exec(bgCtx)
			return nil, err
		})

		if err != nil {
			logger.WithFields(map[string]interface{}{
				"session_id": session.SessionID,
				"error":      err.Error(),
			}).Error("Async session persistence to Redis failed (session remains in local cache)")
		}
	}()

	return nil
}

func (smngr *SessionManager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	sessionKey := "session:" + sessionID

	// 1. Try to fetch from Redis
	result, err := breaker.ExecuteCtx(ctx, smngr.cb, func() (interface{}, error) {
		return smngr.rdb.HGetAll(ctx, sessionKey).Result()
	})

	// 2. Fallback to local cache if Redis fails or returns error
	if err != nil {
		logger.WithField("error", err).Warn("Circuit breaker open/error: Checking local session cache")
		return smngr.getFromLocalCache(sessionID)
	}

	sessionData := result.(map[string]string)

	// 3. If Redis returns empty, check local cache
	if len(sessionData) == 0 {
		return smngr.getFromLocalCache(sessionID)
	}

	session := &Session{}
	if err := session.Unmarshal(sessionData); err != nil {
		return nil, err
	}

	// Update local cache on successful read (Read-Through)
	smngr.updateCache(session)

	return session, nil
}

// Helper to get from local cache with LRU promotion
func (smngr *SessionManager) getFromLocalCache(sessionID string) (*Session, error) {
	smngr.cacheMu.Lock() // Write lock needed for MoveToFront
	defer smngr.cacheMu.Unlock()

	if elem, ok := smngr.cache[sessionID]; ok {
		smngr.evictList.MoveToFront(elem)
		return elem.Value.(*Session), nil
	}
	return nil, nil // Not found in either
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

	// Optimistic update for local cache
	smngr.cacheMu.Lock()
	if elem, ok := smngr.cache[sessionID]; ok {
		smngr.evictList.MoveToFront(elem)
		s := elem.Value.(*Session)
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

	// Renew local cache
	smngr.cacheMu.Lock()
	if elem, ok := smngr.cache[sessionID]; ok {
		smngr.evictList.MoveToFront(elem)
		elem.Value.(*Session).LastActivity = time.Now().Unix()
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

		pipe := smngr.rdb.Pipeline()
		pipe.HSet(ctx, sessionKey, "last_activity", time.Now().Unix())
		pipe.Expire(ctx, sessionKey, 24*time.Hour)
		_, err = pipe.Exec(ctx)
		return nil, err
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
	// Delete from local cache
	smngr.cacheMu.Lock()
	if elem, ok := smngr.cache[sessionID]; ok {
		smngr.evictList.Remove(elem)
		delete(smngr.cache, sessionID)
	}
	smngr.cacheMu.Unlock()

	// Fire and forget delete from Redis
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		breaker.ExecuteCtx(bgCtx, smngr.cb, func() (interface{}, error) {
			return nil, smngr.rdb.Del(bgCtx, "session:"+sessionID).Err()
		})
	}()

	return nil
}

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
