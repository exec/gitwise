package websocket

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
)

const (
	// readLimit caps the size of a single inbound WebSocket frame (512 KiB).
	// Larger frames from a misbehaving client cause the connection to be closed.
	readLimit = 512 * 1024

	// writeTimeout is the per-write deadline applied in the writer goroutine.
	writeTimeout = 10 * time.Second

	// pongWait is how long we wait for a pong reply before declaring the
	// connection dead and closing it.
	pongWait = 60 * time.Second

	// pingInterval is how often the writer goroutine sends a ping frame.
	// Must be less than pongWait so a missed pong is detected before the next ping.
	pingInterval = (pongWait * 9) / 10
)

// HandleWS returns an HTTP handler that upgrades the connection to WebSocket.
// getUserID extracts the authenticated user ID from the request; it returns
// nil for unauthenticated requests.
func (h *Hub) HandleWS(getUserID func(r *http.Request) *uuid.UUID) http.HandlerFunc {
	upgrader := ws.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // non-browser clients don't send Origin
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			return u.Host == r.Host
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		if userID == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Warn("websocket: upgrade failed", "error", err)
			return
		}

		// Apply inbound size and timing limits per gorilla/websocket canonical patterns.
		// The pong handler resets the read deadline each time the client responds.
		// The write deadline is applied per-write in the conn.writePump goroutine.
		conn.SetReadLimit(readLimit)
		conn.SetReadDeadline(time.Now().Add(pongWait)) //nolint:errcheck
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongWait)) //nolint:errcheck
			return nil
		})

		// Register starts the per-connection writer goroutine.
		h.Register(*userID, conn)
		defer h.Unregister(*userID, conn)

		// Read pump — drains reads to detect client disconnect and to honour
		// the pong deadline set above. We don't act on inbound messages here;
		// the WebSocket is currently server-push only.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				if ws.IsUnexpectedCloseError(err, ws.CloseGoingAway, ws.CloseAbnormalClosure) {
					slog.Debug("websocket: read error", "user_id", userID, "error", err)
				}
				break
			}
		}
	}
}
