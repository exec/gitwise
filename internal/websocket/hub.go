package websocket

import (
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
)

// sendBufSize is the depth of the per-connection outbound channel.
// If the channel is full (slow/stalled client), the connection is dropped
// rather than blocking the hub — preventing one slow client from stalling all broadcasts.
const sendBufSize = 256

// conn wraps a WebSocket connection with a non-blocking outbound channel.
// A dedicated writer goroutine drains the channel so SendToUser never blocks.
type conn struct {
	ws   *ws.Conn
	send chan []byte
}

// Hub manages WebSocket connections per user. Multiple connections per user
// are supported (e.g. multiple browser tabs).
type Hub struct {
	mu    sync.RWMutex
	conns map[uuid.UUID][]*conn
}

func NewHub() *Hub {
	return &Hub{conns: make(map[uuid.UUID][]*conn)}
}

// Register adds a WebSocket connection for the given user and starts the
// per-connection writer goroutine.
func (h *Hub) Register(userID uuid.UUID, rawConn *ws.Conn) {
	c := &conn{
		ws:   rawConn,
		send: make(chan []byte, sendBufSize),
	}

	h.mu.Lock()
	h.conns[userID] = append(h.conns[userID], c)
	h.mu.Unlock()

	slog.Debug("websocket: registered connection", "user_id", userID)

	// Start the writer goroutine for this connection.
	go c.writePump(userID, h)
}

// Unregister removes a specific WebSocket connection for the given user.
// It is safe to call from any goroutine. The writer goroutine will exit when
// it observes the channel close.
func (h *Hub) Unregister(userID uuid.UUID, rawConn *ws.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.conns[userID]
	for i, c := range conns {
		if c.ws == rawConn {
			// Close the send channel to signal writePump to exit.
			close(c.send)
			h.conns[userID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	if len(h.conns[userID]) == 0 {
		delete(h.conns, userID)
	}
	slog.Debug("websocket: unregistered connection", "user_id", userID)
}

// SendToUser enqueues a message for all active WebSocket connections for the
// given user. The enqueue is non-blocking: if a connection's buffer is full the
// connection is closed and removed (slow/stalled client).
func (h *Hub) SendToUser(userID uuid.UUID, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.conns[userID]
	var alive []*conn
	for _, c := range conns {
		select {
		case c.send <- msg:
			alive = append(alive, c)
		default:
			// Channel full — drop and close the connection.
			slog.Warn("websocket: send buffer full, closing connection", "user_id", userID)
			close(c.send)
			c.ws.Close()
		}
	}

	if len(alive) == 0 {
		delete(h.conns, userID)
	} else {
		h.conns[userID] = alive
	}
}

// writePump drains the send channel and writes messages to the WebSocket
// connection. It also sends periodic ping frames so the read pump can detect
// dead clients via the pong deadline.
func (c *conn) writePump(userID uuid.UUID, h *Hub) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// Channel closed by Unregister — send close frame and exit.
				c.ws.SetWriteDeadline(time.Now().Add(writeTimeout)) //nolint:errcheck
				c.ws.WriteMessage(ws.CloseMessage, ws.FormatCloseMessage(ws.CloseNormalClosure, ""))
				return
			}
			c.ws.SetWriteDeadline(time.Now().Add(writeTimeout)) //nolint:errcheck
			if err := c.ws.WriteMessage(ws.TextMessage, msg); err != nil {
				slog.Warn("websocket: write error, closing connection",
					"user_id", userID, "error", err)
				h.unregisterConn(userID, c)
				return
			}

		case <-ticker.C:
			c.ws.SetWriteDeadline(time.Now().Add(writeTimeout)) //nolint:errcheck
			if err := c.ws.WriteMessage(ws.PingMessage, nil); err != nil {
				slog.Warn("websocket: ping error, closing connection",
					"user_id", userID, "error", err)
				h.unregisterConn(userID, c)
				return
			}
		}
	}
}

// unregisterConn removes a specific conn pointer (used internally by writePump
// on write failure, where we have the *conn rather than a *ws.Conn).
func (h *Hub) unregisterConn(userID uuid.UUID, target *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.conns[userID]
	for i, c := range conns {
		if c == target {
			h.conns[userID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	if len(h.conns[userID]) == 0 {
		delete(h.conns, userID)
	}
}
