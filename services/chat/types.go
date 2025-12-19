package chat

type ChatMessage struct {
	MessageID string `json:"id"`
	FromID    string `json:"from"`
	ToID      string `json:"to"`
	GroupID   string `json:"group_id,omitempty"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
	IsGroup   bool   `json:"is_group"`
}
