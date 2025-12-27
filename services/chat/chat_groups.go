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

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	pipe := cs.rdb.Pipeline()

	// 1. Cache message
	cacheKey := fmt.Sprintf("chat:group:%s:messages", msg.GroupID)
	pipe.ZAdd(ctx, cacheKey, redis.Z{
		Score:  float64(msg.Timestamp),
		Member: msgJSON,
	})
	// Keep only the most recent messages
	pipe.ZRemRangeByRank(ctx, cacheKey, 0, -RecentMessagesCacheSize-1)
	pipe.Expire(ctx, cacheKey, MessageCacheTTL)

	// 2. Publish
	groupChannel := fmt.Sprintf("chat:group:%s", msg.GroupID)
	pipe.Publish(ctx, groupChannel, msgJSON)

	if _, err := pipe.Exec(ctx); err != nil {
		logger.WithError(err).Error("Failed to pipeline group message")
		// Log error but generally we can proceed if it was queued in memory (though here we rely on Redis)
		// For robustness, you might want to return the error
		return nil, fmt.Errorf("failed to send group message: %w", err)
	}

	// 3. Buffer logic (optional, for metrics or persistence queue)
	select {
	case cs.messageBuffer <- msg:
		cs.incrementMetric("queued")
	default:
		// buffer full logic
		cs.incrementMetric("queued_full")
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

	return messages, nil
}

// SubscribeToGroup subscribes to group messages
func (cs *ChatService) SubscribeToGroup(ctx context.Context, groupID string) *redis.PubSub {
	channelName := fmt.Sprintf("chat:group:%s", groupID)
	return cs.rdb.Subscribe(ctx, channelName)
}
