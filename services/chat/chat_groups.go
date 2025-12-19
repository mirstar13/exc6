package chat

import (
	"context"
	"encoding/json"
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

	// Cache message in Redis
	if err := cs.cacheGroupMessage(ctx, msg); err != nil {
		// Log but don't fail
	}

	// Try to buffer message
	select {
	case cs.messageBuffer <- msg:
		cs.incrementMetric("queued")
	default:
		// Buffer full - persist to Redis queue
		if err := cs.persistMessageToQueue(ctx, msg); err != nil {
			cs.incrementMetric("failed")
			return nil, fmt.Errorf("failed to persist message: %w", err)
		}
		cs.incrementMetric("queued")
	}

	// Publish to Redis Pub/Sub for real-time delivery
	msgJSON, _ := json.Marshal(msg)
	// Use group-specific channel
	channelName := fmt.Sprintf("chat:group:%s", groupID)
	if err := cs.rdb.Publish(ctx, channelName, msgJSON).Err(); err != nil {
		// Log but don't fail - message is still queued
	}

	return msg, nil
}

// GetGroupHistory retrieves message history for a group
func (cs *ChatService) GetGroupHistory(ctx context.Context, groupID string) ([]*ChatMessage, error) {
	cacheKey := fmt.Sprintf("chat:group:%s:messages", groupID)

	results, err := cs.rdb.ZRange(ctx, cacheKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	messages := make([]*ChatMessage, 0, len(results))
	for _, result := range results {
		var msg ChatMessage
		if err := json.Unmarshal([]byte(result), &msg); err != nil {
			continue
		}
		messages = append(messages, &msg)
	}

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
	pipe.ZRemRangeByRank(ctx, cacheKey, 0, -RecentMessagesCacheSize-1)
	pipe.Expire(ctx, cacheKey, MessageCacheTTL)

	_, err = pipe.Exec(ctx)
	return err
}

// SubscribeToGroup subscribes to group messages
func (cs *ChatService) SubscribeToGroup(ctx context.Context, groupID string) *redis.PubSub {
	channelName := fmt.Sprintf("chat:group:%s", groupID)
	return cs.rdb.Subscribe(ctx, channelName)
}
