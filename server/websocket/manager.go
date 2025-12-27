package websocket

import (
	"context"
	"encoding/json"
	"exc6/apperrors"
	"exc6/pkg/logger"
	"exc6/services/groups"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// MessageType represents different WebSocket message types
type MessageType string

const (
	MessageTypeChat         MessageType = "chat"
	MessageTypeGroupChat    MessageType = "group_chat"
	MessageTypeNotification MessageType = "notification"
	MessageTypeCallSignal   MessageType = "call_signal"
	MessageTypeCallOffer    MessageType = "call_offer"
	MessageTypeCallAnswer   MessageType = "call_answer"
	MessageTypeCallICE      MessageType = "call_ice"
	MessageTypeCallEnd      MessageType = "call_end"
	MessageTypeCallRinging  MessageType = "call_ringing"
	MessageTypePing         MessageType = "ping"
	MessageTypePong         MessageType = "pong"

	// Redis Channels
	PubSubChannelGlobal = "ws:broadcast:global"
	PubSubPrefixUser    = "ws:user:"
)

// Message represents a WebSocket message
type Message struct {
	Type      MessageType            `json:"type"`
	ID        string                 `json:"id,omitempty"`
	From      string                 `json:"from"`
	To        string                 `json:"to,omitempty"`
	GroupID   string                 `json:"group_id,omitempty"`
	Content   string                 `json:"content,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp int64                  `json:"timestamp"`
}

// Client represents a WebSocket client connection
type Client struct {
	ID       string
	Username string
	Conn     *websocket.Conn
	Send     chan *Message
	Manager  *Manager
	mu       sync.Mutex
}

// Manager manages WebSocket connections
type Manager struct {
	clients      map[string]*Client // username -> client
	Register     chan *Client
	unRegister   chan *Client
	broadcast    chan *Message
	mu           *sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	groupService *groups.GroupService
	rdb          *redis.Client
}

// NewManager creates a new WebSocket manager
func NewManager(ctx context.Context, rdb *redis.Client) *Manager {
	bgCtx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		clients:    make(map[string]*Client),
		Register:   make(chan *Client, 10),
		unRegister: make(chan *Client, 10),
		broadcast:  make(chan *Message, 1000),
		mu:         &sync.RWMutex{},
		ctx:        bgCtx,
		cancel:     cancel,
		rdb:        rdb,
	}

	go m.run()
	go m.subscribeToGlobalBroadcast()
	return m
}

func (m *Manager) SetGroupService(gs *groups.GroupService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groupService = gs
}

func (m *Manager) run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-m.Register:
			m.RegisterClient(client)

		case client := <-m.unRegister:
			m.unRegisterClient(client)

		case message := <-m.broadcast:
			m.broadcastMessage(message)

		case <-ticker.C:
			m.sendPingToAll()

		case <-m.ctx.Done():
			m.closeAllClients()
			return
		}
	}
}

// subscribeToGlobalBroadcast listens for messages published by other server instances
func (m *Manager) subscribeToGlobalBroadcast() {
	pubsub := m.rdb.Subscribe(m.ctx, PubSubChannelGlobal)
	defer pubsub.Close()

	ch := pubsub.Channel()

	for {
		select {
		case msg := <-ch:
			if msg == nil {
				logger.Warn("Redis PubSub channel closed, stopping subscription")
				return
			}

			var message Message
			if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
				logger.WithError(err).Error("Failed to unmarshal redis message")
				continue
			}
			// Route the message locally
			m.handleRemoteMessage(&message)
		case <-m.ctx.Done():
			return
		}
	}
}

// handleRemoteMessage attempts to deliver a message received from Redis
func (m *Manager) handleRemoteMessage(message *Message) {
	// If it's a direct message, check if user is local
	if message.To != "" {
		m.mu.RLock()
		client, exists := m.clients[message.To]
		m.mu.RUnlock()

		if exists {
			select {
			case client.Send <- message:
			default:
				logger.WithField("to", message.To).Warn("Local client buffer full for remote message")
			}
		}
	}
	// Group logic is handled by the instance that originated the broadcast
	// because getting group members on every instance is expensive.
	// Alternative: The originator sends individual messages to users via Redis.
}

func (m *Manager) RegisterClient(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existingClient, exists := m.clients[client.Username]; exists {
		existingClient.Close()
	}

	m.clients[client.Username] = client

	// Optional: Subscribe to user-specific Redis channel for highly scalable architecture
	// For now, Global Broadcast + Local Check is sufficient for <10k users

	logger.WithFields(map[string]interface{}{
		"username":      client.Username,
		"total_clients": len(m.clients),
	}).Info("Client Registered")
}

func (m *Manager) unRegisterClient(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existingClient, exists := m.clients[client.Username]; exists {
		if existingClient.ID == client.ID {
			delete(m.clients, client.Username)
			close(client.Send)
		}
	}
}

// broadcastMessage sends a message to specific recipients
func (m *Manager) broadcastMessage(message *Message) {
	// 1. Handle Direct Messages
	if message.To != "" {
		m.sendDirectMessage(message)
		return
	}

	// 2. Handle Group Messages
	if message.GroupID != "" {
		m.sendGroupMessage(message)
	}
}

func (m *Manager) sendDirectMessage(message *Message) {
	m.mu.RLock()
	client, isLocal := m.clients[message.To]
	m.mu.RUnlock()

	if isLocal {
		select {
		case client.Send <- message:
		default:
			logger.WithField("to", message.To).Warn("Client buffer full")
		}
	} else {
		// [FIX] Publish to Redis if not local
		m.publishToRedis(message)
	}
}

// [FIX] Optimized Group Broadcast (O(M) instead of O(N))
func (m *Manager) sendGroupMessage(message *Message) {
	if m.groupService == nil {
		return
	}

	// Fetch members only once
	members, err := m.groupService.GetGroupMembers(context.Background(), message.GroupID, message.From)
	if err != nil {
		logger.WithError(err).Warn("Failed to fetch group members")
		return
	}

	// Iterate over MEMBERS, not CLIENTS
	for _, member := range members {
		if member.Username == message.From {
			continue
		}

		m.mu.RLock()
		client, isLocal := m.clients[member.Username]
		m.mu.RUnlock()

		if isLocal {
			select {
			case client.Send <- message:
			default:
				// drop
			}
		} else {
			// If not local, we need to send it to them via Redis.
			// Optimization: To prevent flooding Redis with N messages for N group members,
			// sophisticated systems use "Fan-out on Read" or "Group Channels".
			// For simplicity and correctness here: Send direct message via Redis.
			msgCopy := *message
			msgCopy.To = member.Username
			m.publishToRedis(&msgCopy)
		}
	}
}

func (m *Manager) publishToRedis(message *Message) {
	payload, _ := json.Marshal(message)
	m.rdb.Publish(m.ctx, PubSubChannelGlobal, payload)
}

func (m *Manager) SendToUser(username string, message *Message) error {
	m.mu.RLock()
	client, exists := m.clients[username]
	m.mu.RUnlock()

	if exists {
		select {
		case client.Send <- message:
			return nil
		default:
			return apperrors.New(apperrors.ErrCodeInternal, "Buffer full", 500)
		}
	}

	// User not local, try Redis
	message.To = username
	m.publishToRedis(message)
	return nil
}

// BroadcastToGroup sends a message to all group members
func (m *Manager) BroadcastToGroup(groupID string, message *Message) {
	message.GroupID = groupID
	select {
	case m.broadcast <- message:
	default:
		logger.Warn("Broadcast buffer full")
	}
}

// sendPingToAll sends ping to all connected clients
func (m *Manager) sendPingToAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ping := &Message{
		Type:      MessageTypePing,
		Timestamp: time.Now().Unix(),
	}

	for username, client := range m.clients {
		select {
		case client.Send <- ping:
		default:
			logger.WithField("username", username).Warn("Could not send ping, buffer full")
		}
	}
}

func (m *Manager) IsUserOnline(username string) bool {
	// Note: This only checks LOCAL online status.
	// For distributed checking, you'd need to query Redis keys (e.g., SET "users:online" "username")
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.clients[username]
	return exists
}

// GetOnlineUsers returns list of online usernames
func (m *Manager) GetOnlineUsers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	users := make([]string, 0, len(m.clients))
	for username := range m.clients {
		users = append(users, username)
	}
	return users
}

// closeAllClients closes all client connections
func (m *Manager) closeAllClients() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, client := range m.clients {
		client.Close()
	}
	m.clients = make(map[string]*Client)
}

// Close shuts down the manager
func (m *Manager) Close() {
	m.cancel()
	close(m.Register)
	close(m.unRegister)
	close(m.broadcast)
}

// NewClient creates a new WebSocket client
func NewClient(username string, conn *websocket.Conn, manager *Manager) *Client {
	return &Client{
		ID:       uuid.NewString(),
		Username: username,
		Conn:     conn,
		Send:     make(chan *Message, 256),
		Manager:  manager,
	}
}

// ReadPump reads messages from the WebSocket connection
func (c *Client) ReadPump() {
	defer func() {
		c.Manager.unRegister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var msg Message
		err := c.Conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.WithError(err).Error("WebSocket read error")
			}
			break
		}

		msg.From = c.Username
		msg.Timestamp = time.Now().Unix()

		// Handle different message types
		c.handleMessage(&msg)
	}
}

// WritePump writes messages to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()

		// Recover from panics to prevent server crash during connection storms
		if r := recover(); r != nil {
			logger.WithFields(map[string]interface{}{
				"username": c.Username,
				"error":    r,
			}).Warn("Recovered from panic in WritePump")
		}

		if c.Conn != nil {
			c.Conn.Close()
		}
	}()

	for {
		select {
		case message, ok := <-c.Send:
			// Safety check before setting deadline
			if c.Conn == nil {
				return
			}

			// This SetWriteDeadline is often where the panic happens if connection is dead
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

			if !ok {
				// The channel was closed by the manager
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			err := c.Conn.WriteJSON(message)
			if err != nil {
				// Log at debug level to avoid spamming logs during load tests
				logger.WithField("user", c.Username).Debug("WebSocket write error (client likely disconnected)")
				return
			}

		case <-ticker.C:
			// Safety check before setting deadline
			if c.Conn == nil {
				return
			}

			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming messages
func (c *Client) handleMessage(msg *Message) {
	switch msg.Type {
	case MessageTypePong:
		// Pong received, connection is alive

	case MessageTypeChat, MessageTypeGroupChat:
		// Forward to broadcast channel
		select {
		case c.Manager.broadcast <- msg:
		default:
			logger.Warn("Broadcast channel full")
		}

	case MessageTypeCallOffer, MessageTypeCallAnswer, MessageTypeCallICE, MessageTypeCallRinging, MessageTypeCallEnd:
		// Forward call signaling messages
		select {
		case c.Manager.broadcast <- msg:
		default:
			logger.Warn("Broadcast channel full for call signal")
		}
	}
}

// SendMessage sends a message to this client
func (c *Client) SendMessage(msg *Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case c.Send <- msg:
		return nil
	default:
		logger.Error("Client send buffer full")
		return apperrors.New(apperrors.ErrCodeInternal, "Client send buffer full", 500)
	}
}

// Close closes the client connection
func (c *Client) Close() {
	c.Conn.Close()
}
