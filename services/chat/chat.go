package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"exc6/apperrors"
	"exc6/db"
	"exc6/pkg/breaker"
	"exc6/pkg/logger"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
)

const (
	RecentMessagesCacheSize = 100
	MessageCacheTTL         = 24 * time.Hour
	MessageBufferSize       = 1000
	BatchFlushSize          = 100
	BatchFlushInterval      = 100 * time.Millisecond

	// Persistent queue configuration
	PersistentQueueKey = "chat:pending_messages"
	ProcessingQueueKey = "chat:processing_messages"
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
	ctx           context.Context
	cancel        context.CancelFunc

	// Circuit breakers with proper configuration
	cbRedis *gobreaker.CircuitBreaker
	cbKafka *gobreaker.CircuitBreaker

	// Metrics for monitoring
	metrics struct {
		messagesQueued  atomic.Int64
		messagesSent    atomic.Int64
		messagesFailed  atomic.Int64
		messagesDropped atomic.Int64
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

		// Configure Redis circuit breaker - aggressive settings for cache
		cbRedis: breaker.New(breaker.Config{
			Name:        "redis-chat",
			MaxRequests: 5,
			Interval:    30 * time.Second,
			Timeout:     15 * time.Second,
			Threshold:   0.4, // Trip at 40% failure rate
			MinRequests: 5,
		}),

		// Configure Kafka circuit breaker - more lenient for message queue
		cbKafka: breaker.New(breaker.Config{
			Name:        "kafka-chat",
			MaxRequests: 10,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
			Threshold:   0.6, // Trip at 60% failure rate
			MinRequests: 10,
		}),
	}

	// Recover any messages left in processing state from previous crash
	go cs.recoverProcessingMessages()

	// Start background workers
	cs.wg.Add(2)
	go cs.messageWriter()
	go cs.persistentQueueWorker()

	logger.Info("Chat service initialized with circuit breakers")

	return cs, nil
}

// SendMessage with comprehensive circuit breaker protection
func (cs *ChatService) SendMessage(ctx context.Context, from, to, content string) (*ChatMessage, error) {
	msg := &ChatMessage{
		MessageID: uuid.NewString(),
		FromID:    from,
		ToID:      to,
		Content:   content,
		Timestamp: time.Now().Unix(),
	}

	// 0. Persist to PostgreSQL (Primary Source of Truth)
	if err := cs.persistMessageToDB(ctx, msg); err != nil {
		logger.WithFields(map[string]any{
			"from":  from,
			"to":    to,
			"error": err.Error(),
		}).Error("Failed to persist message to database")
		// We continue - system is designed for high availability,
		// but this is a critical failure for persistence.
	}

	// 1. Cache message in Redis with circuit breaker
	if _, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return nil, cs.cacheMessage(ctx, msg)
	}); err != nil {
		// Create rich error with full context
		cacheErr := apperrors.NewCacheError(
			"message_cache_write",
			cs.GetConversationKey(from, to),
			err,
		).WithDetails("message_id", msg.MessageID).
			WithDetails("from", from).
			WithDetails("to", to).
			WithContext("circuit_breaker_state", cs.cbRedis.State().String())

		// Log with structured fields
		logger.WithFields(cacheErr.LogFields()).Error("Failed to cache message")

		// Continue - caching failure is not fatal
	}

	// 2. Increment unread count
	if _, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return nil, cs.IncrementUnreadCount(ctx, to, from)
	}); err != nil {
		unreadErr := apperrors.NewCacheError(
			"unread_counter_increment",
			fmt.Sprintf("chat:unread:%s", to),
			err,
		).WithDetails("recipient", to).
			WithDetails("sender", from)

		logger.WithFields(unreadErr.LogFields()).Warn("Failed to increment unread count")
	}

	// 3. Buffer message for Kafka
	select {
	case cs.messageBuffer <- msg:
		cs.incrementMetric("queued")
	default:
		// Buffer full - persist to Redis queue
		logger.WithFields(map[string]any{
			"message_id":  msg.MessageID,
			"buffer_size": len(cs.messageBuffer),
			"from":        from,
			"to":          to,
		}).Warn("Message buffer full, persisting to Redis queue")

		if _, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
			return nil, cs.persistMessageToQueue(ctx, msg)
		}); err != nil {
			deliveryErr := apperrors.NewMessageDeliveryError(
				from,
				to,
				"buffer_full_and_redis_unavailable",
				err,
			).WithDetails("message_id", msg.MessageID).
				WithDetails("buffer_capacity", cap(cs.messageBuffer)).
				WithDetails("buffer_length", len(cs.messageBuffer)).
				WithContext("circuit_breaker_state", cs.cbRedis.State().String())

			logger.WithFields(deliveryErr.LogFields()).Error("Message delivery failed")
			cs.incrementMetric("failed")

			return nil, deliveryErr
		}
		cs.incrementMetric("queued")
	}

	// 4. Publish to Redis Pub/Sub (best effort)
	msgJSON, _ := json.Marshal(msg)
	if _, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return nil, cs.rdb.Publish(ctx, "chat:messages", msgJSON).Err()
	}); err != nil {
		pubsubErr := apperrors.NewCacheError(
			"pubsub_publish",
			"chat:messages",
			err,
		).WithDetails("message_id", msg.MessageID).
			WithDetails("from", from).
			WithDetails("to", to)

		logger.WithFields(pubsubErr.LogFields()).Warn("Failed to publish to Redis Pub/Sub")
	}

	return msg, nil
}

// persistMessageToQueue with circuit breaker
func (cs *ChatService) persistMessageToQueue(ctx context.Context, msg *ChatMessage) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Already wrapped in circuit breaker by caller
	return cs.rdb.RPush(ctx, PersistentQueueKey, msgJSON).Err()
}

// recoverProcessingMessages re-queues messages that were stuck in processing
func (cs *ChatService) recoverProcessingMessages() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for {
		// Move from Processing back to Pending (Right to Right)
		// LMOVE processing pending RIGHT RIGHT
		_, err := cs.rdb.LMove(ctx, ProcessingQueueKey, PersistentQueueKey, "RIGHT", "RIGHT").Result()
		if err == redis.Nil {
			break
		}
		if err != nil {
			logger.WithError(err).Error("Failed to recover processing messages")
			break
		}
		logger.Info("Recovered orphaned message from processing queue")
	}
}

// processQueuedMessages with Reliable Queue Pattern (LMOVE)
func (cs *ChatService) processQueuedMessages() {
	ctx, cancel := context.WithTimeout(cs.ctx, 10*time.Second)
	defer cancel()

	// 1. Reliable Move from Pending to Processing
	msgResult, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return cs.rdb.LMove(ctx, PersistentQueueKey, ProcessingQueueKey, "LEFT", "RIGHT").Result()
	})

	if err != nil {
		if err != redis.Nil {
			logger.WithError(err).Warn("Circuit breaker: Failed to pop message (LMOVE)")
		}
		return
	}

	msgJSON, ok := msgResult.(string)
	if !ok || len(msgJSON) == 0 {
		// Handle empty message to prevent unmarshal error
		return
	}

	var msg ChatMessage
	if err := json.Unmarshal([]byte(msgJSON), &msg); err != nil {
		logger.WithField("error", err).Error("Failed to unmarshal queued message")
		// Remove corrupted message
		cs.rdb.LRem(ctx, ProcessingQueueKey, 1, msgJSON)
		return
	}

	// 2. Process (Send to Kafka)
	if err := cs.sendToKafkaWithRetry(&msg, MaxRetries); err != nil {
		logger.WithFields(map[string]any{
			"message_id": msg.MessageID,
			"error":      err.Error(),
		}).Error("Failed to send queued message. It remains in Processing Queue for recovery.")
		cs.incrementMetric("failed")
	} else {
		// 3. Success: Remove from Processing Queue
		if _, err := cs.rdb.LRem(ctx, ProcessingQueueKey, 1, msgJSON).Result(); err != nil {
			logger.WithError(err).Error("Failed to remove message from processing queue after success")
		}
		cs.incrementMetric("sent")
	}
}

// sendToKafkaWithRetry with circuit breaker protection
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
		// Wrap Kafka produce in circuit breaker
		_, err := breaker.Execute(cs.cbKafka, func() (any, error) {
			deliveryChan := make(chan kafka.Event, 1)

			if err := cs.producer.Produce(kafkaMsg, deliveryChan); err != nil {
				return nil, err
			}

			// Wait for delivery confirmation with timeout
			select {
			case e := <-deliveryChan:
				m := e.(*kafka.Message)
				if m.TopicPartition.Error != nil {
					return nil, m.TopicPartition.Error
				}
				return nil, nil
			case <-time.After(5 * time.Second):
				return nil, fmt.Errorf("delivery timeout")
			}
		})

		if err == nil {
			return nil // Success!
		}

		lastErr = err

		// Check if it's a circuit breaker error
		if err == gobreaker.ErrOpenState {
			logger.WithField("attempt", attempt).Warn("Circuit breaker open for Kafka, backing off")
			time.Sleep(RetryBackoff * 2) // Longer backoff for circuit breaker
		} else {
			time.Sleep(RetryBackoff * time.Duration(attempt+1))
		}
	}

	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// messageWriter with circuit breaker awareness
func (cs *ChatService) messageWriter() {
	defer cs.wg.Done()

	ticker := time.NewTicker(BatchFlushInterval)
	defer ticker.Stop()

	batch := make([]*ChatMessage, 0, BatchFlushSize)

	for {
		select {
		case msg, ok := <-cs.messageBuffer:
			if !ok {
				if len(batch) > 0 {
					cs.flushBatch(batch)
				}
				return
			}

			batch = append(batch, msg)

			if len(batch) >= BatchFlushSize {
				cs.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				cs.flushBatch(batch)
				batch = batch[:0]
			}

		case <-cs.shutdownChan:
			if len(batch) > 0 {
				cs.flushBatch(batch)
			}
			return
		}
	}
}

// flushBatch with circuit breaker protection
func (cs *ChatService) flushBatch(batch []*ChatMessage) {
	successCount := 0

	for _, msg := range batch {
		if err := cs.sendToKafkaWithRetry(msg, MaxRetries); err != nil {
			logger.WithFields(map[string]any{
				"message_id": msg.MessageID,
				"error":      err.Error(),
			}).Error("Failed to send message in batch")

			// Persist failed message to Redis queue with circuit breaker
			ctx, cancel := context.WithTimeout(cs.ctx, 2*time.Second)
			msgJSON, _ := json.Marshal(msg)

			if _, requeueErr := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
				return nil, cs.rdb.RPush(ctx, PersistentQueueKey, msgJSON).Err()
			}); requeueErr != nil {
				logger.WithError(requeueErr).Error("Circuit breaker: Failed to requeue failed message")
			}
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

// GetHistory with circuit breaker and DB fallback
func (cs *ChatService) GetHistory(ctx context.Context, user1, user2 string) ([]*ChatMessage, error) {
	conversationKey := cs.GetConversationKey(user1, user2)

	// Try Redis first
	result, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return cs.rdb.ZRange(ctx, conversationKey, 0, -1).Result()
	})

	var messages []*ChatMessage

	if err == nil {
		results := result.([]string)
		for _, res := range results {
			var msg ChatMessage
			if err := json.Unmarshal([]byte(res), &msg); err != nil {
				continue
			}
			messages = append(messages, &msg)
		}
	}

	// If Redis returned nothing or failed, try DB
	if len(messages) == 0 {
		logger.WithFields(map[string]interface{}{
			"user1": user1,
			"user2": user2,
		}).Info("Cache empty/miss, fetching history from DB")

		dbMessages, err := cs.qdb.GetMessagesBetweenUsers(ctx, db.GetMessagesBetweenUsersParams{
			Username:   user1,
			Username_2: user2,
			Limit:      100,
			Offset:     0,
		})

		if err == nil {
			// Convert DB models to ChatMessage struct
			// Note: DB returns newest first, we need to reverse for chat window (oldest first)
			for i := len(dbMessages) - 1; i >= 0; i-- {
				dbMsg := dbMessages[i]
				msg := &ChatMessage{
					MessageID: dbMsg.MessageID,
					FromID:    dbMsg.FromUsername,
					ToID:      dbMsg.ToUsername,
					Content:   dbMsg.Content,
					Timestamp: dbMsg.CreatedAt.Unix(),
				}
				messages = append(messages, msg)

				// Optional: Populate cache (async)
				go func(m *ChatMessage) {
					// Use background context to not cancel on HTTP timeout
					cs.cacheMessage(context.Background(), m)
				}(msg)
			}
		} else {
			logger.WithError(err).Error("Failed to fetch messages from DB")
		}
	}

	return messages, nil
}

func (cs *ChatService) GetHistoryBefore(ctx context.Context, user1, user2 string, beforeTimestamp int64) ([]*ChatMessage, error) {
	// For pagination, we skip Redis and go straight to DB because Redis only holds recent messages

	// Convert timestamp to time.Time
	beforeTime := time.Unix(beforeTimestamp, 0)

	dbMessages, err := cs.qdb.GetMessagesBetweenUsersPaginated(ctx, db.GetMessagesBetweenUsersPaginatedParams{
		Username:   user1,
		Username_2: user2,
		CreatedAt:  beforeTime,
		Limit:      50,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to fetch paginated history: %w", err)
	}

	var messages []*ChatMessage
	// Reverse order for chat window (oldest first)
	for i := len(dbMessages) - 1; i >= 0; i-- {
		dbMsg := dbMessages[i]
		msg := &ChatMessage{
			MessageID: dbMsg.MessageID,
			FromID:    dbMsg.FromUsername,
			ToID:      dbMsg.ToUsername,
			Content:   dbMsg.Content,
			Timestamp: dbMsg.CreatedAt.Unix(),
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// GetUnreadMessages with circuit breaker
func (cs *ChatService) GetUnreadMessages(ctx context.Context, username string) (map[string]int, error) {
	key := fmt.Sprintf("chat:unread:%s", username)

	result, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return cs.rdb.HGetAll(ctx, key).Result()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username": username,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to get unread messages")
		return make(map[string]int), nil
	}

	resultMap := result.(map[string]string)
	unread := make(map[string]int)
	for sender, countStr := range resultMap {
		var count int
		fmt.Sscanf(countStr, "%d", &count)
		if count > 0 {
			unread[sender] = count
		}
	}
	return unread, nil
}

// IncrementUnreadCount with circuit breaker (already wrapped by caller)
func (cs *ChatService) IncrementUnreadCount(ctx context.Context, recipient, sender string) error {
	key := fmt.Sprintf("chat:unread:%s", recipient)
	return cs.rdb.HIncrBy(ctx, key, sender, 1).Err()
}

// MarkConversationRead with circuit breaker
func (cs *ChatService) MarkConversationRead(ctx context.Context, recipient, sender string) error {
	key := fmt.Sprintf("chat:unread:%s", recipient)

	_, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return nil, cs.rdb.HDel(ctx, key, sender).Err()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"recipient": recipient,
			"sender":    sender,
			"error":     err.Error(),
		}).Warn("Circuit breaker: Failed to mark conversation read")
	}

	return err
}

// MarkAllRead with circuit breaker
func (cs *ChatService) MarkAllRead(ctx context.Context, username string) error {
	key := fmt.Sprintf("chat:unread:%s", username)

	_, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return nil, cs.rdb.Del(ctx, key).Err()
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username": username,
			"error":    err.Error(),
		}).Warn("Circuit breaker: Failed to mark all read")
	}

	return err
}

// SubscribeToMessages with circuit breaker
func (cs *ChatService) SubscribeToMessages(ctx context.Context) *redis.PubSub {
	result, err := breaker.ExecuteCtx(ctx, cs.cbRedis, func() (any, error) {
		return cs.rdb.Subscribe(ctx, "chat:messages"), nil
	})

	if err != nil {
		logger.WithField("error", err).Error("Circuit breaker: Redis unavailable for subscription")
		return nil
	}

	return result.(*redis.PubSub)
}

// Helper functions
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

func (cs *ChatService) GetConversationKey(user1, user2 string) string {
	users := []string{user1, user2}
	sort.Strings(users)
	return fmt.Sprintf("chat:conv:%s:%s", users[0], users[1])
}

func getChatKey(user1, user2 string) string {
	users := []string{user1, user2}
	sort.Strings(users)
	return fmt.Sprintf("chat:%s:%s", users[0], users[1])
}

func (cs *ChatService) GetContacts(currentUsername string) ([]string, error) {
	ctx, cancel := context.WithTimeout(cs.ctx, 5*time.Second)
	defer cancel()

	usernames, err := cs.qdb.GetAllUsernames(ctx)
	if err != nil {
		return nil, err
	}

	contacts := make([]string, 0, len(usernames))
	for _, username := range usernames {
		if username != currentUsername {
			contacts = append(contacts, username)
		}
	}

	return contacts, nil
}

func (cs *ChatService) persistMessageToDB(ctx context.Context, msg *ChatMessage) error {
	fromUser, err := cs.qdb.GetUserByUsername(ctx, msg.FromID)
	if err != nil {
		return fmt.Errorf("failed to get sender: %w", err)
	}

	var toUserID uuid.NullUUID
	var groupID uuid.NullUUID
	isGroup := sql.NullBool{Bool: msg.IsGroup, Valid: true}

	if msg.IsGroup {
		// Group Chat
		if msg.GroupID == "" {
			return fmt.Errorf("group id missing for group message")
		}

		// Assuming msg.GroupID is the group's UUID string (not name)
		// If it's the name, we'd need to fetch by name.
		// Checking codebase usage: ChatMessage structure suggests ID.
		gUUID, err := uuid.Parse(msg.GroupID)
		if err != nil {
			return fmt.Errorf("invalid group id: %w", err)
		}
		groupID = uuid.NullUUID{UUID: gUUID, Valid: true}

	} else {
		// 1:1 Chat
		if msg.ToID != "" {
			toUser, err := cs.qdb.GetUserByUsername(ctx, msg.ToID)
			if err != nil {
				return fmt.Errorf("failed to get recipient: %w", err)
			}
			toUserID = uuid.NullUUID{UUID: toUser.ID, Valid: true}
		}
	}

	_, err = cs.qdb.CreateMessage(ctx, db.CreateMessageParams{
		MessageID:  msg.MessageID,
		FromUserID: fromUser.ID,
		ToUserID:   toUserID,
		GroupID:    groupID,
		Content:    msg.Content,
		IsGroup:    isGroup,
	})

	return err
}

// Metrics helpers
func (cs *ChatService) incrementMetric(name string) {
	switch name {
	case "queued":
		cs.metrics.messagesQueued.Add(1)
	case "sent":
		cs.metrics.messagesSent.Add(1)
	case "failed":
		cs.metrics.messagesFailed.Add(1)
	case "dropped":
		cs.metrics.messagesDropped.Add(1)
	}
}

// GetMetrics returns comprehensive metrics including circuit breaker state
func (cs *ChatService) GetMetrics() map[string]any {
	// Get circuit breaker states
	redisState := cs.cbRedis.State()
	redisCounts := cs.cbRedis.Counts()

	kafkaState := cs.cbKafka.State()
	kafkaCounts := cs.cbKafka.Counts()

	return map[string]any{
		"messages": map[string]int64{
			"queued":  cs.metrics.messagesQueued.Load(),
			"sent":    cs.metrics.messagesSent.Load(),
			"failed":  cs.metrics.messagesFailed.Load(),
			"dropped": cs.metrics.messagesDropped.Load(),
		},
		"circuit_breakers": map[string]any{
			"redis": map[string]any{
				"state":                 redisState.String(),
				"total_requests":        redisCounts.Requests,
				"total_successes":       redisCounts.TotalSuccesses,
				"total_failures":        redisCounts.TotalFailures,
				"consecutive_successes": redisCounts.ConsecutiveSuccesses,
				"consecutive_failures":  redisCounts.ConsecutiveFailures,
			},
			"kafka": map[string]any{
				"state":                 kafkaState.String(),
				"total_requests":        kafkaCounts.Requests,
				"total_successes":       kafkaCounts.TotalSuccesses,
				"total_failures":        kafkaCounts.TotalFailures,
				"consecutive_successes": kafkaCounts.ConsecutiveSuccesses,
				"consecutive_failures":  kafkaCounts.ConsecutiveFailures,
			},
		},
	}
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
			cs.processQueuedMessages()
			return
		}
	}
}

// Close performs graceful shutdown
func (cs *ChatService) Close() error {
	cs.shutdownOnce.Do(func() {
		cs.cancel()
		close(cs.shutdownChan)
		cs.wg.Wait()
		close(cs.messageBuffer)
		cs.producer.Close()
		logger.Info("Chat service shutdown complete")
	})
	return nil
}
