package websocket

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
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
