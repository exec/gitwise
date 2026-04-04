package websocket

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
)

// HandleWS returns an HTTP handler that upgrades the connection to WebSocket.
// getUserID extracts the authenticated user ID from the request; it returns
// nil for unauthenticated requests.
func (h *Hub) HandleWS(getUserID func(r *http.Request) *uuid.UUID) http.HandlerFunc {
	upgrader := ws.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
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

		h.Register(*userID, conn)
		defer h.Unregister(*userID, conn)

		// Read pump — drain reads to detect client disconnect.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}
}
