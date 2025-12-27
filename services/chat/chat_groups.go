package chat

import (
	"context"
	"encoding/json"
	"exc6/pkg/breaker"
	"exc6/pkg/logger"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
)

// SendGroupMessage sends a message to a group with circuit breaker protection
func (cs *ChatService) SendGroupMessage(ctx context.Context, from, groupID, content string) (*ChatMessage, error) {
	msg := &ChatMessage{
		MessageID: uuid.NewString(),
		FromID:    from,
		GroupID:   groupID,
		Content:   content,
		Timestamp: time.Now().Unix(),
		IsGroup:   true,
	}

	logger.WithFields(map[string]any{
		"message_id": msg.MessageID,
		"from":       from,
		"group_id":   groupID,
	}).Debug("Creating group message")

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	// Use circuit breaker for Redis operations
	_, err = breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		pipe := cs.rdb.Pipeline()

		// 1. Cache message
		cacheKey := fmt.Sprintf("chat:group:%s:messages", msg.GroupID)
		pipe.ZAdd(ctx, cacheKey, redis.Z{
			Score:  float64(msg.Timestamp),
			Member: msgJSON,
		})
		pipe.ZRemRangeByRank(ctx, cacheKey, 0, -RecentMessagesCacheSize-1)
		pipe.Expire(ctx, cacheKey, MessageCacheTTL)

		// 2. Publish to global chat:messages channel for WebSocket relay
		pipe.Publish(ctx, "chat:messages", msgJSON)

		_, err := pipe.Exec(ctx)
		return nil, err
	})

	if err != nil {
		logger.WithFields(map[string]any{
			"message_id": msg.MessageID,
			"group_id":   groupID,
			"error":      err.Error(),
		}).Error("Circuit breaker: Failed to send group message to Redis")
	}

	// 3. Buffer for Kafka persistence
	select {
	case cs.messageBuffer <- msg:
		cs.incrementMetric("queued")
	default:
		logger.WithFields(map[string]any{
			"message_id":  msg.MessageID,
			"buffer_size": len(cs.messageBuffer),
		}).Warn("Message buffer full for group message")

		if _, persistErr := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
			return nil, cs.persistMessageToQueue(ctx, msg)
		}); persistErr != nil {
			logger.WithFields(map[string]any{
				"message_id": msg.MessageID,
				"error":      persistErr.Error(),
			}).Error("Circuit breaker: Failed to persist group message to queue")
			cs.incrementMetric("failed")
			return nil, fmt.Errorf("failed to persist group message: %w", persistErr)
		}
		cs.incrementMetric("queued")
	}

	return msg, nil
}

// GetGroupHistory retrieves message history for a group with circuit breaker
func (cs *ChatService) GetGroupHistory(ctx context.Context, groupID string) ([]*ChatMessage, error) {
	cacheKey := fmt.Sprintf("chat:group:%s:messages", groupID)

	logger.WithFields(map[string]any{
		"group_id":  groupID,
		"cache_key": cacheKey,
	}).Debug("Fetching group message history")

	result, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return cs.rdb.ZRange(ctx, cacheKey, 0, -1).Result()
	})

	if err != nil {
		logger.WithFields(map[string]any{
			"group_id": groupID,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to fetch group history from Redis")

		return nil, fmt.Errorf("failed to fetch group history: %w", err)
	}

	results := result.([]string)
	messages := make([]*ChatMessage, 0, len(results))
	for _, res := range results {
		var msg ChatMessage
		if err := json.Unmarshal([]byte(res), &msg); err != nil {
			logger.WithError(err).Warn("Failed to unmarshal group message from cache")
			continue
		}
		messages = append(messages, &msg)
	}

	logger.WithFields(map[string]any{
		"group_id":      groupID,
		"message_count": len(messages),
	}).Debug("Retrieved group history")

	return messages, nil
}

// SubscribeToGroup subscribes to group messages with circuit breaker
func (cs *ChatService) SubscribeToGroup(ctx context.Context, groupID string) *redis.PubSub {
	channelName := fmt.Sprintf("chat:group:%s", groupID)

	result, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return cs.rdb.Subscribe(ctx, channelName), nil
	})

	if err != nil {
		logger.WithFields(map[string]any{
			"group_id": groupID,
			"channel":  channelName,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to subscribe to group channel")
		return nil
	}

	logger.WithFields(map[string]any{
		"group_id": groupID,
		"channel":  channelName,
	}).Debug("Subscribed to group channel")

	return result.(*redis.PubSub)
}

// IncrementGroupUnreadCount increments unread count for a group
func (cs *ChatService) IncrementGroupUnreadCount(ctx context.Context, groupID, senderUsername string, memberUsernames []string) error {
	// Don't increment for the sender
	for _, member := range memberUsernames {
		if member == senderUsername {
			continue
		}

		key := fmt.Sprintf("chat:unread:%s", member)
		groupKey := fmt.Sprintf("group:%s", groupID)

		_, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
			return nil, cs.rdb.HIncrBy(ctx, key, groupKey, 1).Err()
		})

		if err != nil {
			logger.WithFields(map[string]interface{}{
				"member":   member,
				"group_id": groupID,
				"error":    err.Error(),
			}).Warn("Circuit breaker: Failed to increment group unread count")
		}
	}

	return nil
}

// MarkGroupRead marks a group as read for a user
func (cs *ChatService) MarkGroupRead(ctx context.Context, username, groupID string) error {
	key := fmt.Sprintf("chat:unread:%s", username)
	groupKey := fmt.Sprintf("group:%s", groupID)

	_, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return nil, cs.rdb.HDel(ctx, key, groupKey).Err()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username": username,
			"group_id": groupID,
			"error":    err.Error(),
		}).Warn("Circuit breaker: Failed to mark group read")
	}

	return err
}

// Additional helper: Check circuit breaker health for group operations
func (cs *ChatService) IsGroupMessagingHealthy() bool {
	redisState := cs.cbRedis.State()
	kafkaState := cs.cbKafka.State()

	// Both circuit breakers should be closed or half-open for healthy operation
	return redisState != gobreaker.StateOpen && kafkaState != gobreaker.StateOpen
}

// GetGroupCircuitBreakerStatus returns the status of circuit breakers
func (cs *ChatService) GetGroupCircuitBreakerStatus() map[string]string {
	return map[string]string{
		"redis": cs.cbRedis.State().String(),
		"kafka": cs.cbKafka.State().String(),
	}
}
