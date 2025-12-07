package chat

type ChatMessage struct {
	MessageID string `json:"id"`
	FromID    string `json:"from"`
	ToID      string `json:"to"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}
