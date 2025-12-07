package sessions

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
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
}

func NewSessionManager(rdb *redis.Client) *SessionManager {
	return &SessionManager{rdb: rdb}
}

func (smngr *SessionManager) SaveSession(ctx context.Context, session *Session) error {
	sessionKey := "session:" + session.SessionID

	if err := smngr.rdb.HSet(ctx, sessionKey, session.Marshal()).Err(); err != nil {
		return err
	}

	return smngr.rdb.Expire(ctx, sessionKey, 24*time.Hour).Err()
}

func (smngr *SessionManager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	sessionKey := "session:" + sessionID

	sessionData, err := smngr.rdb.HGetAll(ctx, sessionKey).Result()
	if err != nil {
		return nil, err
	}

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

	iter := smngr.rdb.Scan(ctx, 0, "session:*", 0).Iterator()
	for iter.Next(ctx) {
		sessionData, err := smngr.rdb.HGetAll(ctx, iter.Val()).Result()
		if err != nil {
			continue
		}

		session := &Session{}
		if err := session.Unmarshal(sessionData); err != nil {
			continue
		}

		sessions = append(sessions, session)
	}

	if err := iter.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}

func (smngr *SessionManager) UpdateSessionField(ctx context.Context, userID, field, value string) error {
	sessionKey := "session:" + userID

	exists, err := smngr.rdb.Exists(ctx, sessionKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}

	return smngr.rdb.HSet(ctx, sessionKey, field, value).Err()
}

func (smngr *SessionManager) RenewSession(ctx context.Context, sessionID string) error {
	sessionKey := "session:" + sessionID

	exists, err := smngr.rdb.Exists(ctx, sessionKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("session not found")
	}

	smngr.rdb.HSet(ctx, sessionKey, "last_activity", time.Now().Unix())

	return smngr.rdb.Expire(ctx, sessionKey, 24*time.Hour).Err()
}

func (smngr *SessionManager) DeleteSession(ctx context.Context, sessionID string) error {
	return smngr.rdb.Del(ctx, "session:"+sessionID).Err()
}
