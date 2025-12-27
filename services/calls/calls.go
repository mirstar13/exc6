package calls

import (
	"context"
	"encoding/json"
	"exc6/pkg/breaker"
	"exc6/pkg/logger"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
)

// CallState represents the state of a call
type CallState string

const (
	CallStateInitiating CallState = "initiating"
	CallStateRinging    CallState = "ringing"
	CallStateActive     CallState = "active"
	CallStateEnding     CallState = "ending"
	CallStateEnded      CallState = "ended"
)

// Call represents an active or past call
type Call struct {
	ID         string    `json:"id"`
	Caller     string    `json:"caller"`
	Callee     string    `json:"callee"`
	State      CallState `json:"state"`
	StartedAt  int64     `json:"started_at"`
	AnsweredAt int64     `json:"answered_at,omitempty"`
	EndedAt    int64     `json:"ended_at,omitempty"`
	Duration   int64     `json:"duration,omitempty"`
	EndedBy    string    `json:"ended_by,omitempty"`
}

// CallService manages voice calls and WebRTC signaling
type CallService struct {
	rdb         *redis.Client
	cb          *gobreaker.CircuitBreaker
	activeCalls map[string]*Call
	userCalls   map[string]string
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewCallService creates a new call service
func NewCallService(ctx context.Context, rdb *redis.Client) *CallService {
	bgCtx, cancel := context.WithCancel(context.Background())

	cs := &CallService{
		rdb:         rdb,
		activeCalls: make(map[string]*Call),
		userCalls:   make(map[string]string),
		ctx:         bgCtx,
		cancel:      cancel,
		cb: breaker.New(breaker.Config{
			Name:        "redis-calls",
			MaxRequests: 5,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
			Threshold:   0.5,
			MinRequests: 5,
		}),
	}

	go cs.cleanupStaleCall()

	return cs
}

// InitiateCall initiates a new call
func (cs *CallService) InitiateCall(caller, callee string) (*Call, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Check if either user is already in a call
	if existingCallID, inCall := cs.userCalls[caller]; inCall {
		return nil, fmt.Errorf("caller already in call: %s", existingCallID)
	}
	if existingCallID, inCall := cs.userCalls[callee]; inCall {
		return nil, fmt.Errorf("callee already in call: %s", existingCallID)
	}

	call := &Call{
		ID:        uuid.NewString(),
		Caller:    caller,
		Callee:    callee,
		State:     CallStateInitiating,
		StartedAt: time.Now().Unix(),
	}

	cs.activeCalls[call.ID] = call
	cs.userCalls[caller] = call.ID
	cs.userCalls[callee] = call.ID

	// Persist to Redis with circuit breaker
	if err := cs.saveCallToRedis(call); err != nil {
		logger.WithError(err).Error("Circuit breaker: Failed to save call to Redis")
		// Don't fail the call initiation if Redis is down
	}

	logger.WithFields(map[string]any{
		"call_id": call.ID,
		"caller":  caller,
		"callee":  callee,
	}).Info("Call initiated")

	return call, nil
}

// saveCallToRedis saves call state to Redis with circuit breaker
func (cs *CallService) saveCallToRedis(call *Call) error {
	ctx, cancel := context.WithTimeout(cs.ctx, 3*time.Second)
	defer cancel()

	key := fmt.Sprintf("call:%s", call.ID)

	_, err := breaker.ExecuteCtx(ctx, cs.cb, func() (interface{}, error) {
		data, err := json.Marshal(call)
		if err != nil {
			return nil, err
		}
		return nil, cs.rdb.Set(ctx, key, data, 24*time.Hour).Err()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"call_id": call.ID,
			"error":   err.Error(),
		}).Error("Circuit breaker: Failed to save call")
		return err
	}

	return nil
}

// saveCallHistory saves completed call to history with circuit breaker
func (cs *CallService) saveCallHistory(call *Call) error {
	if call.State != CallStateEnded {
		return nil
	}

	ctx, cancel := context.WithTimeout(cs.ctx, 3*time.Second)
	defer cancel()

	_, err := breaker.ExecuteCtx(ctx, cs.cb, func() (interface{}, error) {
		data, err := json.Marshal(call)
		if err != nil {
			return nil, err
		}

		pipe := cs.rdb.Pipeline()

		callerKey := fmt.Sprintf("call_history:%s", call.Caller)
		calleeKey := fmt.Sprintf("call_history:%s", call.Callee)

		score := float64(call.EndedAt)

		pipe.ZAdd(ctx, callerKey, redis.Z{Score: score, Member: data})
		pipe.ZAdd(ctx, calleeKey, redis.Z{Score: score, Member: data})

		// Keep only last 100 calls
		pipe.ZRemRangeByRank(ctx, callerKey, 0, -101)
		pipe.ZRemRangeByRank(ctx, calleeKey, 0, -101)

		// Expire after 30 days
		pipe.Expire(ctx, callerKey, 30*24*time.Hour)
		pipe.Expire(ctx, calleeKey, 30*24*time.Hour)

		_, err = pipe.Exec(ctx)
		return nil, err
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"call_id": call.ID,
			"error":   err.Error(),
		}).Error("Circuit breaker: Failed to save call history")
		return err
	}

	return nil
}

// GetCallHistory retrieves call history for a user with circuit breaker
func (cs *CallService) GetCallHistory(username string, limit int) ([]*Call, error) {
	ctx, cancel := context.WithTimeout(cs.ctx, 5*time.Second)
	defer cancel()

	key := fmt.Sprintf("call_history:%s", username)

	result, err := breaker.ExecuteCtx(ctx, cs.cb, func() (interface{}, error) {
		return cs.rdb.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
			Min:    "-inf",
			Max:    "+inf",
			Offset: 0,
			Count:  int64(limit),
		}).Result()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username": username,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to get call history")
		return nil, err
	}

	results := result.([]string)
	calls := make([]*Call, 0, len(results))
	for _, res := range results {
		var call Call
		if err := json.Unmarshal([]byte(res), &call); err != nil {
			logger.WithError(err).Warn("Failed to unmarshal call history")
			continue
		}
		calls = append(calls, &call)
	}

	return calls, nil
}

// GetMissedCalls returns missed calls with circuit breaker
func (cs *CallService) GetMissedCalls(ctx context.Context, username string) ([]*Call, error) {
	history, err := cs.GetCallHistory(username, 50)
	if err != nil {
		return nil, err
	}

	// Get last seen timestamp
	lastSeenKey := fmt.Sprintf("calls:seen:%s", username)

	result, err := breaker.ExecuteCtx(ctx, cs.cb, func() (interface{}, error) {
		return cs.rdb.Get(ctx, lastSeenKey).Int64()
	})

	lastSeenVal := int64(0)
	if err == nil {
		lastSeenVal = result.(int64)
	}

	missed := make([]*Call, 0)
	for _, call := range history {
		if call.Callee == username && call.AnsweredAt == 0 && call.State == CallStateEnded {
			if call.EndedAt > lastSeenVal {
				missed = append(missed, call)
			}
		}
	}
	return missed, nil
}

// MarkCallsSeen updates the timestamp with circuit breaker
func (cs *CallService) MarkCallsSeen(ctx context.Context, username string) error {
	key := fmt.Sprintf("calls:seen:%s", username)

	_, err := breaker.ExecuteCtx(ctx, cs.cb, func() (interface{}, error) {
		return nil, cs.rdb.Set(ctx, key, time.Now().Unix(), 0).Err()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username": username,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to mark calls as seen")
		return err
	}

	return nil
}

// UpdateCallState updates the state of a call
func (cs *CallService) UpdateCallState(callID string, newState CallState) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	call, exists := cs.activeCalls[callID]
	if !exists {
		return fmt.Errorf("call not found: %s", callID)
	}

	oldState := call.State
	call.State = newState

	switch newState {
	case CallStateActive:
		call.AnsweredAt = time.Now().Unix()
	case CallStateEnded:
		call.EndedAt = time.Now().Unix()
		if call.AnsweredAt > 0 {
			call.Duration = call.EndedAt - call.AnsweredAt
		}
	}

	// Persist with circuit breaker (non-blocking)
	if err := cs.saveCallToRedis(call); err != nil {
		logger.WithError(err).Warn("Failed to update call in Redis (continuing anyway)")
	}

	logger.WithFields(map[string]any{
		"call_id":   callID,
		"old_state": oldState,
		"new_state": newState,
	}).Info("Call state updated")

	return nil
}

// AnswerCall marks a call as answered
func (cs *CallService) AnswerCall(callID, username string) error {
	cs.mu.RLock()
	call, exists := cs.activeCalls[callID]
	cs.mu.RUnlock()

	if !exists {
		return fmt.Errorf("call not found: %s", callID)
	}

	if call.Callee != username {
		return fmt.Errorf("user %s is not the callee", username)
	}

	return cs.UpdateCallState(callID, CallStateActive)
}

// EndCall ends a call
func (cs *CallService) EndCall(callID, username string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	call, exists := cs.activeCalls[callID]
	if !exists {
		return fmt.Errorf("call not found: %s", callID)
	}

	if call.Caller != username && call.Callee != username {
		return fmt.Errorf("user %s is not part of this call", username)
	}

	call.State = CallStateEnded
	call.EndedAt = time.Now().Unix()
	call.EndedBy = username

	if call.AnsweredAt > 0 {
		call.Duration = call.EndedAt - call.AnsweredAt
	}

	// Remove from active tracking
	delete(cs.userCalls, call.Caller)
	delete(cs.userCalls, call.Callee)
	delete(cs.activeCalls, callID)

	// Persist to Redis for history
	if err := cs.saveCallToRedis(call); err != nil {
		logger.WithError(err).Error("Failed to save ended call to Redis")
	}

	// Store in call history
	if err := cs.saveCallHistory(call); err != nil {
		logger.WithError(err).Error("Failed to save call history")
	}

	logger.WithFields(map[string]any{
		"call_id":  callID,
		"ended_by": username,
		"duration": call.Duration,
	}).Info("Call ended")

	return nil
}

// GetCall retrieves a call by ID
func (cs *CallService) GetCall(callID string) (*Call, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	call, exists := cs.activeCalls[callID]
	if !exists {
		return nil, fmt.Errorf("call not found: %s", callID)
	}

	return call, nil
}

// GetUserActiveCall gets the active call for a user
func (cs *CallService) GetUserActiveCall(username string) (*Call, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	callID, inCall := cs.userCalls[username]
	if !inCall {
		return nil, fmt.Errorf("user not in active call")
	}

	call, exists := cs.activeCalls[callID]
	if !exists {
		return nil, fmt.Errorf("call data not found")
	}

	return call, nil
}

// IsUserInCall checks if a user is currently in a call
func (cs *CallService) IsUserInCall(username string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	_, inCall := cs.userCalls[username]
	return inCall
}

// cleanupStaleCalls removes stale calls
func (cs *CallService) cleanupStaleCall() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cs.mu.Lock()
			now := time.Now().Unix()

			for callID, call := range cs.activeCalls {
				if call.State == CallStateRinging || call.State == CallStateInitiating {
					if now-call.StartedAt > 60 {
						logger.WithFields(map[string]any{
							"call_id": callID,
							"state":   call.State,
							"age":     now - call.StartedAt,
						}).Info("Cleaning up stale call")

						call.State = CallStateEnded
						call.EndedAt = now
						call.EndedBy = "system"

						delete(cs.userCalls, call.Caller)
						delete(cs.userCalls, call.Callee)
						delete(cs.activeCalls, callID)

						cs.saveCallHistory(call)
					}
				}
			}

			cs.mu.Unlock()

		case <-cs.ctx.Done():
			return
		}
	}
}

// GetMetrics returns call service and circuit breaker metrics
func (cs *CallService) GetStats() map[string]any {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Get circuit breaker metrics
	cbState := cs.cb.State()
	cbCounts := cs.cb.Counts()

	return map[string]any{
		"active_calls":  len(cs.activeCalls),
		"users_in_call": len(cs.userCalls),
		"circuit_breaker": map[string]interface{}{
			"state":                 cbState.String(),
			"total_requests":        cbCounts.Requests,
			"total_successes":       cbCounts.TotalSuccesses,
			"total_failures":        cbCounts.TotalFailures,
			"consecutive_successes": cbCounts.ConsecutiveSuccesses,
			"consecutive_failures":  cbCounts.ConsecutiveFailures,
		},
	}
}

// Close closes the call service
func (cs *CallService) Close() {
	cs.cancel()
}
