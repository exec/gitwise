package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/services/user"
)

// Auth middleware checks for session cookie or API token.
// It sets the user ID in the request context if authenticated.
// Does not reject unauthenticated requests — use RequireAuth for that.
type Auth struct {
	sessions *SessionManager
	users    *user.Service
}

func NewAuth(sessions *SessionManager, users *user.Service) *Auth {
	return &Auth{sessions: sessions, users: users}
}

func (a *Auth) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try API token first (Authorization: Bearer gw_...)
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			if token, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
				u, err := a.users.ValidateToken(r.Context(), token)
				if err == nil {
					ctx := context.WithValue(r.Context(), UserIDKey, u.ID)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// Try session cookie
		session, _, err := a.sessions.Get(r.Context(), r)
		if err == nil && session != nil {
			ctx := context.WithValue(r.Context(), UserIDKey, session.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Unauthenticated — continue without user context
		next.ServeHTTP(w, r)
	})
}

// RequireAuth rejects unauthenticated requests.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetUserID(r.Context()) == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"errors":[{"code":"unauthorized","message":"authentication required"}]}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUserID extracts the authenticated user ID from context.
func GetUserID(ctx context.Context) *uuid.UUID {
	id, ok := ctx.Value(UserIDKey).(uuid.UUID)
	if !ok {
		return nil
	}
	return &id
}
