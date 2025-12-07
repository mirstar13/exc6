package chat

import (
	"context"
	"encoding/json"
	"exc6/db"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	RecentMessagesCacheSize = 100
	MessageCacheTTL         = 24 * time.Hour
)

type ChatService struct {
	rdb           *redis.Client
	udb           *db.UsersDB
	producer      *kafka.Producer
	kafkaTopic    string
	messageBuffer chan *ChatMessage
}

func NewChatService(rdb *redis.Client, udb *db.UsersDB, kafkaAddr string) (*ChatService, error) {
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
		udb:           udb,
		producer:      p,
		kafkaTopic:    "chat-history",
		messageBuffer: make(chan *ChatMessage, 1000),
	}

	go cs.messageWriter()

	return cs, nil
}

func (cs *ChatService) messageWriter() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	batch := make([]*kafka.Message, 0, 100)

	for {
		select {
		case msg := <-cs.messageBuffer:
			msgJSON, _ := json.Marshal(msg)
			chatKey := getChatKey(msg.FromID, msg.ToID)
			topic := cs.kafkaTopic

			batch = append(batch, &kafka.Message{
				TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
				Key:            []byte(chatKey),
				Value:          msgJSON,
			})

			if len(batch) >= 100 {
				cs.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				cs.flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (cs *ChatService) flushBatch(batch []*kafka.Message) {
	for _, msg := range batch {
		if err := cs.producer.Produce(msg, nil); err != nil {
			log.Printf("Failed to produce message to Kafka: %v", err)
		}
	}
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

func (cs *ChatService) GetContacts(currentUsername string) []string {
	var contacts []string
	usernames := cs.udb.GetAllUsernames()

	for _, username := range usernames {
		if username != currentUsername {
			contacts = append(contacts, username)
		}
	}

	return contacts
}

func (cs *ChatService) SendMessage(ctx context.Context, from, to, content string) (*ChatMessage, error) {
	msg := &ChatMessage{
		MessageID: uuid.NewString(),
		FromID:    from,
		ToID:      to,
		Content:   content,
		Timestamp: time.Now().Unix(),
	}

	if err := cs.cacheMessage(ctx, msg); err != nil {
		log.Printf("Failed to cache message: %v", err)
	}

	cs.messageBuffer <- msg

	msgJSON, _ := json.Marshal(msg)
	if err := cs.rdb.Publish(ctx, "chat:messages", msgJSON).Err(); err != nil {
		log.Printf("Failed to publish message: %v", err)
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

	if len(messages) == 0 {
		log.Printf("No cached messages for %s<->%s", user1, user2)
	}

	return messages, nil
}

// Close cleans up resources
func (cs *ChatService) Close() error {
	cs.producer.Close()
	close(cs.messageBuffer)
	return nil
}
