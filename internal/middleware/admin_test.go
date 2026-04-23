package middleware

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAdminCache_SetAndGet(t *testing.T) {
	cache := &adminCache{entries: make(map[uuid.UUID]adminCacheEntry)}

	id := uuid.New()

	// Miss before set.
	_, ok := cache.get(id)
	if ok {
		t.Error("expected cache miss before any set")
	}

	// Set admin = true.
	cache.set(id, true)
	isAdmin, ok := cache.get(id)
	if !ok {
		t.Fatal("expected cache hit after set")
	}
	if !isAdmin {
		t.Error("expected isAdmin = true")
	}

	// Set admin = false for a different user.
	id2 := uuid.New()
	cache.set(id2, false)
	isAdmin2, ok := cache.get(id2)
	if !ok {
		t.Fatal("expected cache hit for second user")
	}
	if isAdmin2 {
		t.Error("expected isAdmin = false for second user")
	}
}

func TestAdminCache_TTLExpiry(t *testing.T) {
	cache := &adminCache{entries: make(map[uuid.UUID]adminCacheEntry)}
	id := uuid.New()

	// Manually insert a stale entry.
	cache.mu.Lock()
	cache.entries[id] = adminCacheEntry{
		isAdmin:   true,
		expiresAt: time.Now().Add(-time.Second), // already expired
	}
	cache.mu.Unlock()

	_, ok := cache.get(id)
	if ok {
		t.Error("expected cache miss for expired entry")
	}
}

func TestInvalidateAdminCache(t *testing.T) {
	// Use a fresh local cache to avoid global state pollution.
	cache := &adminCache{entries: make(map[uuid.UUID]adminCacheEntry)}
	id := uuid.New()
	cache.set(id, true)

	// Directly invalidate via the internal cache.
	cache.mu.Lock()
	delete(cache.entries, id)
	cache.mu.Unlock()

	_, ok := cache.get(id)
	if ok {
		t.Error("expected cache miss after invalidation")
	}
}
