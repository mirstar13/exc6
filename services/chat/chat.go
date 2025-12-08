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
	MessageBufferSize       = 1000 // Bounded buffer to prevent memory leaks
	BatchFlushSize          = 100  // Flush after this many messages
	BatchFlushInterval      = 100 * time.Millisecond
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
	// REMOVED: ctx field - contexts should be passed per-operation
}

func NewChatService(ctx context.Context, rdb *redis.Client, qdb *db.Queries, kafkaAddr string) (*ChatService, error) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": kafkaAddr,
		"client.id":         "go-fiber-dashboard",
		"acks":              "all",
	})
	if err != nil {
		return nil, err
	}

	cs := &ChatService{
		rdb:           rdb,
		qdb:           qdb,
		producer:      p,
		kafkaTopic:    "chat-history",
		messageBuffer: make(chan *ChatMessage, MessageBufferSize),
		shutdownChan:  make(chan struct{}),
	}

	// Start background message writer
	cs.wg.Add(1)
	go cs.messageWriter()

	return cs, nil
}

func (cs *ChatService) messageWriter() {
	defer cs.wg.Done()

	ticker := time.NewTicker(BatchFlushInterval)
	defer ticker.Stop()

	batch := make([]*kafka.Message, 0, BatchFlushSize)

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

			msgJSON, err := json.Marshal(msg)
			if err != nil {
				logger.WithFields(map[string]interface{}{
					"message_id": msg.MessageID,
					"error":      err.Error(),
				}).Error("Failed to marshal message")
				continue
			}

			chatKey := getChatKey(msg.FromID, msg.ToID)
			topic := cs.kafkaTopic

			batch = append(batch, &kafka.Message{
				TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
				Key:            []byte(chatKey),
				Value:          msgJSON,
			})

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

func (cs *ChatService) flushBatch(batch []*kafka.Message) {
	for _, msg := range batch {
		if err := cs.producer.Produce(msg, nil); err != nil {
			logger.WithFields(map[string]interface{}{
				"topic": *msg.TopicPartition.Topic,
				"error": err.Error(),
			}).Error("Failed to produce message to Kafka")
		}
	}
	// Wait for messages to be delivered
	cs.producer.Flush(5000)
}

func (cs *ChatService) SubscribeToMessages(ctx context.Context) *redis.PubSub {
	return cs.rdb.Subscribe(ctx, "chat:messages")
}

func getChatKey(user1, user2 string) string {
	users := []string{user1, user2}
	sort.Strings(users)
	return fmt.Sprintf("chat:%s:%s", users[0], users[1])
}

func (cs *ChatService) getConversationKey(user1, user2 string) string {
	users := []string{user1, user2}
	sort.Strings(users)
	return fmt.Sprintf("chat:conv:%s:%s", users[0], users[1])
}

func (cs *ChatService) GetContacts(currentUsername string) ([]string, error) {
	var contacts []string

	// Use background context with timeout for DB query
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

func (cs *ChatService) SendMessage(ctx context.Context, from, to, content string) (*ChatMessage, error) {
	msg := &ChatMessage{
		MessageID: uuid.NewString(),
		FromID:    from,
		ToID:      to,
		Content:   content,
		Timestamp: time.Now().Unix(),
	}

	// Cache message in Redis
	if err := cs.cacheMessage(ctx, msg); err != nil {
		logger.WithFields(map[string]interface{}{
			"message_id": msg.MessageID,
			"from":       msg.FromID,
			"to":         msg.ToID,
			"error":      err.Error(),
		}).Error("Failed to cache message")
	}

	// Send to buffer (non-blocking with timeout)
	select {
	case cs.messageBuffer <- msg:
		// Successfully buffered
	case <-time.After(1 * time.Second):
		// Buffer is full, log error but don't block
		logger.WithFields(map[string]interface{}{
			"message_id": msg.MessageID,
			"from":       msg.FromID,
			"to":         msg.ToID,
		}).Error("Message buffer full, dropping message")
		return nil, fmt.Errorf("message buffer full")
	}

	// Publish to Redis for real-time delivery
	msgJSON, _ := json.Marshal(msg)
	if err := cs.rdb.Publish(ctx, "chat:messages", msgJSON).Err(); err != nil {
		logger.WithFields(map[string]interface{}{
			"message_id": msg.MessageID,
			"channel":    "chat:messages",
			"error":      err.Error(),
		}).Error("Failed to publish message to Redis")
	}

	return msg, nil
}

func (cs *ChatService) cacheMessage(ctx context.Context, msg *ChatMessage) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	conversationKey := cs.getConversationKey(msg.FromID, msg.ToID)

	pipe := cs.rdb.Pipeline()

	// Add message to sorted set
	pipe.ZAdd(ctx, conversationKey, redis.Z{
		Score:  float64(msg.Timestamp),
		Member: msgJSON,
	})

	// Keep only recent messages
	pipe.ZRemRangeByRank(ctx, conversationKey, 0, -RecentMessagesCacheSize-1)

	// Set expiration
	pipe.Expire(ctx, conversationKey, MessageCacheTTL)

	_, err = pipe.Exec(ctx)
	return err
}

func (cs *ChatService) GetHistory(ctx context.Context, user1, user2 string) ([]*ChatMessage, error) {
	conversationKey := cs.getConversationKey(user1, user2)

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

// Close performs graceful shutdown of the chat service
func (cs *ChatService) Close() error {
	cs.shutdownOnce.Do(func() {
		// Signal shutdown
		close(cs.shutdownChan)

		// Close message buffer
		close(cs.messageBuffer)

		// Wait for writer to finish
		cs.wg.Wait()

		// Close Kafka producer
		cs.producer.Close()
	})

	return nil
}
