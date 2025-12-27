package websocket

import (
	"context"
	"exc6/apperrors"
	"exc6/pkg/logger"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"
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
	clients    map[string]*Client // username -> client
	Register   chan *Client
	unRegister chan *Client
	broadcast  chan *Message
	mu         *sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewManager creates a new WebSocket manager
func NewManager(ctx context.Context) *Manager {
	bgCtx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		clients:    make(map[string]*Client),
		Register:   make(chan *Client, 10),
		unRegister: make(chan *Client, 10),
		broadcast:  make(chan *Message, 1000),
		mu:         &sync.RWMutex{},
		ctx:        bgCtx,
		cancel:     cancel,
	}

	go m.run()
	return m
}

// run handles client registration, unregistration, and message broadcasting
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

// RegisterClient Registers a new client
func (m *Manager) RegisterClient(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close existing connection if user is already connected
	if existingClient, exists := m.clients[client.Username]; exists {
		logger.WithFields(map[string]interface{}{
			"username": client.Username,
			"old_id":   existingClient.ID,
			"new_id":   client.ID,
		}).Info("Replacing existing WebSocket connection")

		existingClient.Close()
	}

	m.clients[client.Username] = client

	logger.WithFields(map[string]interface{}{
		"username":      client.Username,
		"client_id":     client.ID,
		"total_clients": len(m.clients),
	}).Info("WebSocket client Registered")
}

// unRegisterClient removes a client
func (m *Manager) unRegisterClient(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existingClient, exists := m.clients[client.Username]; exists {
		// Only remove if it's the same client instance
		if existingClient.ID == client.ID {
			delete(m.clients, client.Username)
			close(client.Send)

			logger.WithFields(map[string]interface{}{
				"username":      client.Username,
				"client_id":     client.ID,
				"total_clients": len(m.clients),
			}).Info("WebSocket client unRegistered")
		}
	}
}

// broadcastMessage sends a message to specific recipients
func (m *Manager) broadcastMessage(message *Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Send to specific user
	if message.To != "" {
		if client, exists := m.clients[message.To]; exists {
			select {
			case client.Send <- message:
				logger.WithFields(map[string]interface{}{
					"type": message.Type,
					"from": message.From,
					"to":   message.To,
				}).Debug("Message sent to recipient")
			default:
				logger.WithFields(map[string]interface{}{
					"to": message.To,
				}).Warn("Client send buffer full, dropping message")
			}
		}
	}

	// Broadcast to group
	if message.GroupID != "" {
		// Get all group members and send to each
		for username, client := range m.clients {
			if username != message.From { // Don't send back to sender
				select {
				case client.Send <- message:
				default:
					logger.WithField("username", username).Warn("Client buffer full")
				}
			}
		}
	}
}

// SendToUser sends a message to a specific user
func (m *Manager) SendToUser(username string, message *Message) error {
	m.mu.RLock()
	client, exists := m.clients[username]
	m.mu.RUnlock()

	if !exists {
		logger.WithField("username", username).Error("Client not connected")
		return nil
	}

	select {
	case client.Send <- message:
		return nil
	default:
		logger.WithField("username", username).Error("Client send buffer full")
		return apperrors.New(apperrors.ErrCodeInternal, "Client send buffer full", 500)
	}
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

// IsUserOnline checks if a user is connected
func (m *Manager) IsUserOnline(username string) bool {
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

// Client methods

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
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			err := c.Conn.WriteJSON(message)
			if err != nil {
				logger.WithError(err).Error("WebSocket write error")
				return
			}

		case <-ticker.C:
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
