// fileName: mirstar13/exc6/exc6-main/services/chat/chat_groups.go

package chat

import (
	"context"
	"encoding/json"
	"exc6/pkg/logger"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// SendGroupMessage sends a message to a group
func (cs *ChatService) SendGroupMessage(ctx context.Context, from, groupID, content string) (*ChatMessage, error) {
	msg := &ChatMessage{
		MessageID: uuid.NewString(),
		FromID:    from,
		GroupID:   groupID,
		Content:   content,
		Timestamp: time.Now().Unix(),
		IsGroup:   true,
	}

	logger.WithFields(map[string]interface{}{
		"message_id": msg.MessageID,
		"from":       from,
		"group_id":   groupID,
	}).Debug("Creating group message")

	// 1. Cache message in Redis immediately for read consistency
	if err := cs.cacheGroupMessage(ctx, msg); err != nil {
		logger.WithFields(map[string]interface{}{
			"message_id": msg.MessageID,
			"from":       from,
			"group_id":   groupID,
			"error":      err.Error(),
		}).Error("Failed to cache group message")
		// Continue - caching failure shouldn't prevent message delivery
	}

	// 2. Try to buffer message (non-blocking)
	select {
	case cs.messageBuffer <- msg:
		cs.incrementMetric("queued")
		logger.WithFields(map[string]interface{}{
			"message_id": msg.MessageID,
			"group_id":   groupID,
		}).Debug("Group message queued in buffer")
	default:
		// Buffer full - persist to Redis queue instead
		logger.WithFields(map[string]interface{}{
			"message_id":  msg.MessageID,
			"buffer_size": len(cs.messageBuffer),
			"group_id":    groupID,
		}).Warn("Message buffer full, persisting group message to Redis queue")

		if err := cs.persistMessageToQueue(ctx, msg); err != nil {
			cs.incrementMetric("failed")
			logger.WithError(err).Error("Failed to persist group message to queue")
			return nil, fmt.Errorf("failed to persist message: %w", err)
		}
		cs.incrementMetric("queued")
	}

	// 3. Publish to Redis Pub/Sub for real-time delivery (CRITICAL FOR GROUP CHAT)
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		logger.WithError(err).Error("Failed to marshal group message for pub/sub")
		// Don't fail - message is still queued
		return msg, nil
	}

	// Publish to global channel so WebSocket manager picks it up
	publishCtx, publishCancel := context.WithTimeout(ctx, 2*time.Second)
	defer publishCancel()

	if err := cs.rdb.Publish(publishCtx, "chat:messages", msgJSON).Err(); err != nil {
		logger.WithFields(map[string]interface{}{
			"message_id": msg.MessageID,
			"group_id":   groupID,
			"error":      err.Error(),
		}).Error("Failed to publish group message to global Redis Pub/Sub")
	} else {
		logger.WithFields(map[string]interface{}{
			"message_id": msg.MessageID,
			"group_id":   groupID,
		}).Debug("Group message published to global Redis Pub/Sub")
	}

	return msg, nil
}

// GetGroupHistory retrieves message history for a group
func (cs *ChatService) GetGroupHistory(ctx context.Context, groupID string) ([]*ChatMessage, error) {
	cacheKey := fmt.Sprintf("chat:group:%s:messages", groupID)

	logger.WithFields(map[string]interface{}{
		"group_id":  groupID,
		"cache_key": cacheKey,
	}).Debug("Fetching group message history")

	results, err := cs.rdb.ZRange(ctx, cacheKey, 0, -1).Result()
	if err != nil {
		logger.WithError(err).Error("Failed to fetch group history from Redis")
		return nil, err
	}

	messages := make([]*ChatMessage, 0, len(results))
	for _, result := range results {
		var msg ChatMessage
		if err := json.Unmarshal([]byte(result), &msg); err != nil {
			logger.WithError(err).Warn("Failed to unmarshal group message from cache")
			continue
		}
		messages = append(messages, &msg)
	}

	logger.WithFields(map[string]interface{}{
		"group_id":      groupID,
		"message_count": len(messages),
	}).Debug("Group history fetched")

	return messages, nil
}

// cacheGroupMessage stores a group message in Redis
func (cs *ChatService) cacheGroupMessage(ctx context.Context, msg *ChatMessage) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	cacheKey := fmt.Sprintf("chat:group:%s:messages", msg.GroupID)

	pipe := cs.rdb.Pipeline()
	pipe.ZAdd(ctx, cacheKey, redis.Z{
		Score:  float64(msg.Timestamp),
		Member: msgJSON,
	})
	// Keep only the most recent messages
	pipe.ZRemRangeByRank(ctx, cacheKey, 0, -RecentMessagesCacheSize-1)
	// Set expiration
	pipe.Expire(ctx, cacheKey, MessageCacheTTL)

	_, err = pipe.Exec(ctx)

	if err != nil {
		logger.WithError(err).Error("Failed to cache group message in Redis")
	} else {
		logger.WithFields(map[string]interface{}{
			"cache_key":  cacheKey,
			"message_id": msg.MessageID,
		}).Debug("Group message cached successfully")
	}

	return err
}

// SubscribeToGroup subscribes to group messages
func (cs *ChatService) SubscribeToGroup(ctx context.Context, groupID string) *redis.PubSub {
	channelName := fmt.Sprintf("chat:group:%s", groupID)

	logger.WithFields(map[string]interface{}{
		"group_id": groupID,
		"channel":  channelName,
	}).Debug("Subscribing to group channel")

	return cs.rdb.Subscribe(ctx, channelName)
}
