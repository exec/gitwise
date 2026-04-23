package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/services/user"
)

// adminCacheTTL is how long an is_admin result is cached before re-querying.
// A 30 s lag is acceptable; operators can wait or bounce the process for immediate effect.
const adminCacheTTL = 30 * time.Second

type adminCacheEntry struct {
	isAdmin   bool
	expiresAt time.Time
}

// adminCache is an in-process LRU-lite: a plain map protected by a mutex.
// For typical Gitwise deployments (≤ a few thousand users) this is sufficient.
// The cache is invalidated when the TTL expires.
type adminCache struct {
	mu      sync.Mutex
	entries map[uuid.UUID]adminCacheEntry
}

var globalAdminCache = &adminCache{entries: make(map[uuid.UUID]adminCacheEntry)}

func (c *adminCache) get(id uuid.UUID) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	if !ok || time.Now().After(e.expiresAt) {
		return false, false
	}
	return e.isAdmin, true
}

func (c *adminCache) set(id uuid.UUID, isAdmin bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = adminCacheEntry{isAdmin: isAdmin, expiresAt: time.Now().Add(adminCacheTTL)}
}

// InvalidateAdminCache removes a specific user's cached admin flag.
// Call this whenever is_admin is modified for a user.
func InvalidateAdminCache(id uuid.UUID) {
	globalAdminCache.mu.Lock()
	defer globalAdminCache.mu.Unlock()
	delete(globalAdminCache.entries, id)
}

// RequireAdmin returns middleware that checks the authenticated user has is_admin = true.
// The result is cached in-process for adminCacheTTL to avoid N queries per request.
// Returns 404 (not 403) to avoid revealing the admin path exists to non-admins.
func RequireAdmin(users *user.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == nil {
				http.NotFound(w, r)
				return
			}

			// Fast path: cached result.
			if isAdmin, ok := globalAdminCache.get(*userID); ok {
				if !isAdmin {
					http.NotFound(w, r)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Slow path: query DB and populate cache.
			u, err := users.GetByID(r.Context(), *userID)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			globalAdminCache.set(*userID, u.IsAdmin)
			if !u.IsAdmin {
				http.NotFound(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
