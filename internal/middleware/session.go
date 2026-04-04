package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	SessionIDKey contextKey = "session_id"

	sessionCookie  = "gitwise_session"
	sessionPrefix  = "session:"
	sessionExpiry  = 7 * 24 * time.Hour
)

type SessionData struct {
	UserID uuid.UUID `json:"user_id"`
}

type SessionManager struct {
	redis *redis.Client
}

func NewSessionManager(rdb *redis.Client) *SessionManager {
	return &SessionManager{redis: rdb}
}

func (sm *SessionManager) Create(ctx context.Context, w http.ResponseWriter, userID uuid.UUID) error {
	sid := make([]byte, 32)
	if _, err := rand.Read(sid); err != nil {
		return fmt.Errorf("generate session id: %w", err)
	}
	sessionID := hex.EncodeToString(sid)

	data, _ := json.Marshal(SessionData{UserID: userID})
	if err := sm.redis.Set(ctx, sessionPrefix+sessionID, data, sessionExpiry).Err(); err != nil {
		return fmt.Errorf("store session: %w", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(sessionExpiry.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false, // set true in production behind TLS
	})

	return nil
}

func (sm *SessionManager) Get(ctx context.Context, r *http.Request) (*SessionData, string, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, "", nil
	}

	val, err := sm.redis.Get(ctx, sessionPrefix+cookie.Value).Result()
	if err == redis.Nil {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("get session: %w", err)
	}

	var data SessionData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, "", fmt.Errorf("unmarshal session: %w", err)
	}

	return &data, cookie.Value, nil
}

func (sm *SessionManager) Destroy(ctx context.Context, w http.ResponseWriter, sessionID string) error {
	sm.redis.Del(ctx, sessionPrefix+sessionID)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	return nil
}
