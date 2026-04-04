package websocket

import (
	"log/slog"
	"sync"

	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
)

// Hub manages WebSocket connections per user. Multiple connections per user
// are supported (e.g. multiple browser tabs).
type Hub struct {
	mu    sync.RWMutex
	conns map[uuid.UUID][]*ws.Conn
}

func NewHub() *Hub {
	return &Hub{conns: make(map[uuid.UUID][]*ws.Conn)}
}

// Register adds a WebSocket connection for the given user.
func (h *Hub) Register(userID uuid.UUID, conn *ws.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[userID] = append(h.conns[userID], conn)
	slog.Debug("websocket: registered connection", "user_id", userID)
}

// Unregister removes a specific WebSocket connection for the given user.
func (h *Hub) Unregister(userID uuid.UUID, conn *ws.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.conns[userID]
	for i, c := range conns {
		if c == conn {
			h.conns[userID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	if len(h.conns[userID]) == 0 {
		delete(h.conns, userID)
	}
	slog.Debug("websocket: unregistered connection", "user_id", userID)
}

// SendToUser sends a message to all active WebSocket connections for the
// given user. Connections that fail to write are closed and removed.
func (h *Hub) SendToUser(userID uuid.UUID, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.conns[userID]
	var alive []*ws.Conn
	for _, c := range conns {
		if err := c.WriteMessage(ws.TextMessage, msg); err != nil {
			slog.Warn("websocket: write failed, closing connection",
				"user_id", userID, "error", err)
			c.Close()
			continue
		}
		alive = append(alive, c)
	}

	if len(alive) == 0 {
		delete(h.conns, userID)
	} else {
		h.conns[userID] = alive
	}
}
