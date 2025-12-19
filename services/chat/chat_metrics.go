package chat

import (
	"context"
	"encoding/json"
	"exc6/pkg/metrics"
	"time"
)

// Add to SendMessage method - measure delivery latency
func (cs *ChatService) SendMessageWithMetrics(ctx context.Context, from, to, content string) (*ChatMessage, error) {
	start := time.Now()

	msg, err := cs.SendMessage(ctx, from, to, content)

	if err != nil {
		metrics.IncrementMessagesFailed()
		return nil, err
	}

	// Record latency
	metrics.RecordMessageDeliveryLatency(time.Since(start).Seconds())
	metrics.IncrementMessagesQueued()

	return msg, nil
}

// Update flushBatch to record metrics
func (cs *ChatService) flushBatchWithMetrics(batch []*ChatMessage) {
	metrics.RecordKafkaBatchSize(len(batch))

	successCount := 0

	for _, msg := range batch {
		if err := cs.sendToKafkaWithRetry(msg, MaxRetries); err != nil {
			// Failed message
			metrics.IncrementMessagesFailed()

			// Persist to Redis queue
			ctx, cancel := context.WithTimeout(cs.ctx, 2*time.Second)
			msgJSON, _ := json.Marshal(msg)
			cs.rdb.RPush(ctx, PersistentQueueKey, msgJSON)
			cancel()
		} else {
			successCount++
			metrics.IncrementMessagesSent()
		}
	}

	// Update buffer size gauge
	metrics.SetMessageBufferSize(len(cs.messageBuffer))
}

// Add to messageWriter goroutine
func (cs *ChatService) messageWriterWithMetrics() {
	defer cs.wg.Done()

	ticker := time.NewTicker(BatchFlushInterval)
	defer ticker.Stop()

	batch := make([]*ChatMessage, 0, BatchFlushSize)

	for {
		select {
		case msg, ok := <-cs.messageBuffer:
			if !ok {
				if len(batch) > 0 {
					cs.flushBatchWithMetrics(batch)
				}
				return
			}

			batch = append(batch, msg)
			metrics.SetMessageBufferSize(len(cs.messageBuffer))

			if len(batch) >= BatchFlushSize {
				cs.flushBatchWithMetrics(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				cs.flushBatchWithMetrics(batch)
				batch = batch[:0]
			}

			// Update buffer size gauge periodically
			metrics.SetMessageBufferSize(len(cs.messageBuffer))

		case <-cs.shutdownChan:
			if len(batch) > 0 {
				cs.flushBatchWithMetrics(batch)
			}
			return
		}
	}
}
