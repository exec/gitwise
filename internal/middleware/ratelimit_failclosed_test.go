package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newDeadRedis returns a *redis.Client pointed at a port that refuses connections,
// so any operation immediately returns a dial error — simulating Redis unavailability.
func newDeadRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1", // port 1 is reserved and always refused
		DialTimeout: 50 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond,
	})
}

func TestRateLimit_FailClosed_Redis503(t *testing.T) {
	rdb := newDeadRedis()
	defer rdb.Close()

	handler := RateLimit(rdb, 100, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:9999"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503 Service Unavailable on Redis error, got %d", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" {
		t.Error("want Retry-After header on Redis error")
	}
}

func TestAuthRateLimit_FailClosed_Redis503(t *testing.T) {
	rdb := newDeadRedis()
	defer rdb.Close()

	handler := AuthRateLimit(rdb, 10, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "1.2.3.4:9999"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503 Service Unavailable on Redis error (auth bucket), got %d", rec.Code)
	}
}
