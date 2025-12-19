package chat

import (
	"context"
	"encoding/json"
	"exc6/db"
	"exc6/pkg/logger"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	RecentMessagesCacheSize = 100
	MessageCacheTTL         = 24 * time.Hour
	MessageBufferSize       = 1000
	BatchFlushSize          = 100
	BatchFlushInterval      = 100 * time.Millisecond

	// Persistent queue configuration
	PersistentQueueKey = "chat:pending_messages"
	MaxRetries         = 3
	RetryBackoff       = 5 * time.Second
)

type ChatService struct {
	rdb           *redis.Client
	qdb           *db.Queries
	producer      *kafka.Producer
	kafkaTopic    string
	messageBuffer chan *ChatMessage
	shutdownOnce  sync.Once
	shutdownChan  chan struct{}
	wg            sync.WaitGroup
	ctx           context.Context    // Background context for workers
	cancel        context.CancelFunc // Cancel function for graceful shutdown

	// Metrics for monitoring
	metrics struct {
		mu              sync.RWMutex
		messagesQueued  int64
		messagesSent    int64
		messagesFailed  int64
		messagesDropped int64
	}
}

func NewChatService(ctx context.Context, rdb *redis.Client, qdb *db.Queries, kafkaAddr string) (*ChatService, error) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": kafkaAddr,
		"client.id":         "go-fiber-dashboard",
		"acks":              "all",
		"retries":           3,
		"retry.backoff.ms":  100,
	})
	if err != nil {
		return nil, err
	}

	// Create a background context that's independent of the input context
	// This ensures workers keep running even if the input context is cancelled
	bgCtx, cancel := context.WithCancel(context.Background())

	cs := &ChatService{
		rdb:           rdb,
		qdb:           qdb,
		producer:      p,
		kafkaTopic:    "chat-history",
		messageBuffer: make(chan *ChatMessage, MessageBufferSize),
		shutdownChan:  make(chan struct{}),
		ctx:           bgCtx,
		cancel:        cancel,
	}

	// Start background workers
	cs.wg.Add(2)
	go cs.messageWriter()
	go cs.persistentQueueWorker()

	return cs, nil
}

// SendMessage now persists to Redis first for durability
func (cs *ChatService) SendMessage(ctx context.Context, from, to, content string) (*ChatMessage, error) {
	msg := &ChatMessage{
		MessageID: uuid.NewString(),
		FromID:    from,
		ToID:      to,
		Content:   content,
		Timestamp: time.Now().Unix(),
	}

	// 1. Cache message in Redis immediately for read consistency
	if err := cs.cacheMessage(ctx, msg); err != nil {
		logger.WithFields(map[string]any{
			"message_id": msg.MessageID,
			"from":       msg.FromID,
			"to":         msg.ToID,
			"error":      err.Error(),
		}).Error("Failed to cache message")
	}

	// 2. Try to buffer message (non-blocking)
	select {
	case cs.messageBuffer <- msg:
		cs.incrementMetric("queued")
	default:
		// Buffer full - persist to Redis queue instead
		logger.WithFields(map[string]any{
			"message_id":  msg.MessageID,
			"buffer_size": len(cs.messageBuffer),
		}).Warn("Message buffer full, persisting to Redis queue")

		if err := cs.persistMessageToQueue(ctx, msg); err != nil {
			cs.incrementMetric("failed")
			return nil, fmt.Errorf("failed to persist message: %w", err)
		}
		cs.incrementMetric("queued")
	}

	// 3. Publish to Redis Pub/Sub for real-time delivery (best effort)
	msgJSON, _ := json.Marshal(msg)
	if err := cs.rdb.Publish(ctx, "chat:messages", msgJSON).Err(); err != nil {
		logger.WithFields(map[string]any{
			"message_id": msg.MessageID,
			"channel":    "chat:messages",
			"error":      err.Error(),
		}).Warn("Failed to publish message to Redis Pub/Sub")
		// Don't fail - message is still queued
	}

	return msg, nil
}

// persistMessageToQueue stores messages in Redis list when buffer is full
func (cs *ChatService) persistMessageToQueue(ctx context.Context, msg *ChatMessage) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Use RPUSH to add to end of queue
	return cs.rdb.RPush(ctx, PersistentQueueKey, msgJSON).Err()
}

// persistentQueueWorker processes messages from Redis queue
func (cs *ChatService) persistentQueueWorker() {
	defer cs.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cs.processQueuedMessages()
		case <-cs.shutdownChan:
			// Final drain on shutdown
			cs.processQueuedMessages()
			return
		}
	}
}

func (cs *ChatService) processQueuedMessages() {
	// Use background context with timeout for Redis operations
	ctx, cancel := context.WithTimeout(cs.ctx, 5*time.Second)
	defer cancel()

	// Check queue length
	queueLen, err := cs.rdb.LLen(ctx, PersistentQueueKey).Result()
	if err != nil || queueLen == 0 {
		return
	}

	logger.WithField("queue_length", queueLen).Debug("Processing queued messages")

	// Process up to 100 messages at a time
	count := int64(100)
	if queueLen < count {
		count = queueLen
	}

	// Pop messages from queue (LPOP is atomic)
	for i := int64(0); i < count; i++ {
		msgJSON, err := cs.rdb.LPop(ctx, PersistentQueueKey).Result()
		if err != nil {
			break
		}

		var msg ChatMessage
		if err := json.Unmarshal([]byte(msgJSON), &msg); err != nil {
			logger.WithField("error", err).Error("Failed to unmarshal queued message")
			continue
		}

		// Send to Kafka with retries
		if err := cs.sendToKafkaWithRetry(&msg, MaxRetries); err != nil {
			logger.WithFields(map[string]any{
				"message_id": msg.MessageID,
				"error":      err.Error(),
			}).Error("Failed to send queued message after retries")

			// Put back in queue for later retry
			msgJSON, _ := json.Marshal(msg)
			cs.rdb.RPush(ctx, PersistentQueueKey, msgJSON)
			cs.incrementMetric("failed")
			break // Stop processing to avoid cascading failures
		}

		cs.incrementMetric("sent")
	}
}

// sendToKafkaWithRetry attempts to send message to Kafka with exponential backoff
func (cs *ChatService) sendToKafkaWithRetry(msg *ChatMessage, maxRetries int) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	chatKey := getChatKey(msg.FromID, msg.ToID)
	topic := cs.kafkaTopic

	kafkaMsg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
		Key:            []byte(chatKey),
		Value:          msgJSON,
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		deliveryChan := make(chan kafka.Event, 1)

		if err := cs.producer.Produce(kafkaMsg, deliveryChan); err != nil {
			lastErr = err
			time.Sleep(RetryBackoff * time.Duration(attempt+1))
			continue
		}

		// Wait for delivery confirmation (with timeout)
		select {
		case e := <-deliveryChan:
			m := e.(*kafka.Message)
			if m.TopicPartition.Error != nil {
				lastErr = m.TopicPartition.Error
				time.Sleep(RetryBackoff * time.Duration(attempt+1))
				continue
			}
			return nil // Success!
		case <-time.After(5 * time.Second):
			lastErr = fmt.Errorf("delivery timeout")
			continue
		}
	}

	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// messageWriter processes messages from buffer
func (cs *ChatService) messageWriter() {
	defer cs.wg.Done()

	ticker := time.NewTicker(BatchFlushInterval)
	defer ticker.Stop()

	batch := make([]*ChatMessage, 0, BatchFlushSize)

	for {
		select {
		case msg, ok := <-cs.messageBuffer:
			if !ok {
				// Channel closed, flush remaining and exit
				if len(batch) > 0 {
					cs.flushBatch(batch)
				}
				return
			}

			batch = append(batch, msg)

			// Flush when batch is full
			if len(batch) >= BatchFlushSize {
				cs.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			// Periodic flush of pending messages
			if len(batch) > 0 {
				cs.flushBatch(batch)
				batch = batch[:0]
			}

		case <-cs.shutdownChan:
			// Graceful shutdown: flush remaining messages
			if len(batch) > 0 {
				cs.flushBatch(batch)
			}
			return
		}
	}
}

func (cs *ChatService) flushBatch(batch []*ChatMessage) {
	successCount := 0

	for _, msg := range batch {
		if err := cs.sendToKafkaWithRetry(msg, MaxRetries); err != nil {
			logger.WithFields(map[string]any{
				"message_id": msg.MessageID,
				"error":      err.Error(),
			}).Error("Failed to send message in batch")

			// Persist failed message to Redis queue for retry
			ctx, cancel := context.WithTimeout(cs.ctx, 2*time.Second)
			msgJSON, _ := json.Marshal(msg)
			cs.rdb.RPush(ctx, PersistentQueueKey, msgJSON)
			cancel()

			cs.incrementMetric("failed")
		} else {
			successCount++
			cs.incrementMetric("sent")
		}
	}

	logger.WithFields(map[string]any{
		"batch_size": len(batch),
		"success":    successCount,
		"failed":     len(batch) - successCount,
	}).Debug("Batch processed")
}

// Metrics helpers
func (cs *ChatService) incrementMetric(name string) {
	cs.metrics.mu.Lock()
	defer cs.metrics.mu.Unlock()

	switch name {
	case "queued":
		cs.metrics.messagesQueued++
	case "sent":
		cs.metrics.messagesSent++
	case "failed":
		cs.metrics.messagesFailed++
	case "dropped":
		cs.metrics.messagesDropped++
	}
}

func (cs *ChatService) GetMetrics() map[string]int64 {
	cs.metrics.mu.RLock()
	defer cs.metrics.mu.RUnlock()

	return map[string]int64{
		"queued":  cs.metrics.messagesQueued,
		"sent":    cs.metrics.messagesSent,
		"failed":  cs.metrics.messagesFailed,
		"dropped": cs.metrics.messagesDropped,
	}
}

func (cs *ChatService) SubscribeToMessages(ctx context.Context) *redis.PubSub {
	return cs.rdb.Subscribe(ctx, "chat:messages")
}

func getChatKey(user1, user2 string) string {
	users := []string{user1, user2}
	sort.Strings(users)
	return fmt.Sprintf("chat:%s:%s", users[0], users[1])
}

func (cs *ChatService) GetConversationKey(user1, user2 string) string {
	users := []string{user1, user2}
	sort.Strings(users)
	return fmt.Sprintf("chat:conv:%s:%s", users[0], users[1])
}

func (cs *ChatService) cacheMessage(ctx context.Context, msg *ChatMessage) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	conversationKey := cs.GetConversationKey(msg.FromID, msg.ToID)

	pipe := cs.rdb.Pipeline()
	pipe.ZAdd(ctx, conversationKey, redis.Z{
		Score:  float64(msg.Timestamp),
		Member: msgJSON,
	})
	pipe.ZRemRangeByRank(ctx, conversationKey, 0, -RecentMessagesCacheSize-1)
	pipe.Expire(ctx, conversationKey, MessageCacheTTL)

	_, err = pipe.Exec(ctx)
	return err
}

func (cs *ChatService) GetHistory(ctx context.Context, user1, user2 string) ([]*ChatMessage, error) {
	conversationKey := cs.GetConversationKey(user1, user2)

	results, err := cs.rdb.ZRange(ctx, conversationKey, 0, -1).Result()
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

func (cs *ChatService) GetContacts(currentUsername string) ([]string, error) {
	var contacts []string

	ctx, cancel := context.WithTimeout(cs.ctx, 5*time.Second)
	defer cancel()

	usernames, err := cs.qdb.GetAllUsernames(ctx)
	if err != nil {
		return nil, err
	}

	for _, username := range usernames {
		if username != currentUsername {
			contacts = append(contacts, username)
		}
	}

	return contacts, nil
}

// Close performs graceful shutdown
func (cs *ChatService) Close() error {
	cs.shutdownOnce.Do(func() {
		// Cancel background context
		cs.cancel()

		// Signal shutdown to all workers
		close(cs.shutdownChan)

		// Wait for workers to finish processing
		cs.wg.Wait()

		// Now safe to close message buffer
		close(cs.messageBuffer)

		// Close Kafka producer
		cs.producer.Close()

		logger.Info("Chat service shutdown complete")
	})

	return nil
}
