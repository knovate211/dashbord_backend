// Package handler implements the WebSocket hub for the notification service.
// It maintains per-user connections and broadcasts events received from NATS.
package handler

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	notificationv1 "github.com/knovate211/proto/notification/v1"
)

var upgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  4096,
	// Allow all origins in dev; restrict in production via env config
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client represents a single WebSocket connection.
type Client struct {
	userID string
	conn   *websocket.Conn
	send   chan []byte
}

// Hub manages all active WebSocket connections.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]struct{} // userID → set of clients
	log     *zap.Logger
}

// NewHub creates and returns a Hub. Call hub.Run() in a goroutine.
func NewHub(log *zap.Logger) *Hub {
	return &Hub{
		clients: make(map[string]map[*Client]struct{}),
		log:     log,
	}
}

// ServeHTTP upgrades an HTTP request to a WebSocket and registers the client.
// Expects the user ID to be set via the "X-User-ID" header (set by gateway).
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		// Fall back to query param for testing
		userID = r.URL.Query().Get("user_id")
	}
	if userID == "" {
		http.Error(w, "user_id required", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("websocket upgrade failed", zap.Error(err))
		return
	}

	client := &Client{
		userID: userID,
		conn:   conn,
		send:   make(chan []byte, 64),
	}

	h.register(client)

	h.log.Info("client connected", zap.String("user_id", userID))

	// Writer goroutine
	go h.writePump(client)
	// Reader goroutine (handles pings/close frames)
	h.readPump(client)
}

// Broadcast sends an event to all connections for a specific user.
func (h *Hub) Broadcast(userID string, event *notificationv1.WebSocketEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		h.log.Error("marshal event failed", zap.Error(err))
		return
	}

	h.mu.RLock()
	clients, ok := h.clients[userID]
	h.mu.RUnlock()

	if !ok {
		return // user not connected
	}

	for c := range clients {
		select {
		case c.send <- data:
		default:
			// Send buffer full — client is slow; disconnect
			h.unregister(c)
		}
	}
}

// BroadcastAll sends an event to ALL connected clients (e.g., system announcements).
func (h *Hub) BroadcastAll(event *notificationv1.WebSocketEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		h.log.Error("marshal broadcast event", zap.Error(err))
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, clients := range h.clients {
		for c := range clients {
			select {
			case c.send <- data:
			default:
				go h.unregister(c)
			}
		}
	}
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.userID] == nil {
		h.clients[c.userID] = make(map[*Client]struct{})
	}
	h.clients[c.userID][c] = struct{}{}
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c.userID]; ok {
		delete(h.clients[c.userID], c)
		if len(h.clients[c.userID]) == 0 {
			delete(h.clients, c.userID)
		}
	}
	c.conn.Close()
	close(c.send)
	h.log.Info("client disconnected", zap.String("user_id", c.userID))
}

func (h *Hub) writePump(c *Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{}) //nolint:errcheck
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (h *Hub) readPump(c *Client) {
	defer h.unregister(c)

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)) //nolint:errcheck
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)) //nolint:errcheck
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.log.Warn("unexpected ws close", zap.String("user_id", c.userID), zap.Error(err))
			}
			break
		}
	}
}
