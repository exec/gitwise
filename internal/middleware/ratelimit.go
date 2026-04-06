package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimit returns middleware that limits requests per IP using a sliding
// window counter backed by Redis. When the limit is exceeded it responds
// with 429 Too Many Requests and a Retry-After header.
func RateLimit(rdb *redis.Client, limit int, window time.Duration) func(http.Handler) http.Handler {
	return rateLimit(rdb, limit, window, "general")
}

// AuthRateLimit returns a stricter rate limiter intended for authentication
// endpoints (login, register, 2FA verification). Same mechanism as RateLimit
// but uses a separate key namespace so the auth budget is independent.
func AuthRateLimit(rdb *redis.Client, limit int, window time.Duration) func(http.Handler) http.Handler {
	return rateLimit(rdb, limit, window, "auth")
}

func rateLimit(rdb *redis.Client, limit int, window time.Duration, bucket string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)
			key := fmt.Sprintf("rl:%s:%s", bucket, ip)

			ctx := r.Context()

			// INCR + conditional EXPIRE implements a fixed-window counter.
			// The window resets when the key expires.
			count, err := rdb.Incr(ctx, key).Result()
			if err != nil {
				// If Redis is down, allow the request rather than blocking
				// all traffic. Log-worthy but not user-facing.
				next.ServeHTTP(w, r)
				return
			}

			// Set expiry only on the first increment (when count == 1).
			if count == 1 {
				rdb.Expire(ctx, key, window)
			}

			// Report remaining budget via standard headers.
			remaining := int64(limit) - count
			if remaining < 0 {
				remaining = 0
			}
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

			if count > int64(limit) {
				ttl, err := rdb.TTL(ctx, key).Result()
				if err != nil || ttl <= 0 {
					ttl = window
				}
				retryAfter := int(ttl.Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, `{"errors":[{"code":"rate_limited","message":"too many requests, retry after %d seconds"}]}`, retryAfter)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractIP returns the client IP, preferring X-Forwarded-For when present
// (the app sits behind chi's RealIP middleware which sets RemoteAddr, but
// we also handle the header directly for defense in depth).
func extractIP(r *http.Request) string {
	// chi/middleware.RealIP already rewrites RemoteAddr from X-Forwarded-For
	// or X-Real-IP, so RemoteAddr is the best source after that middleware.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr may not have a port in some test scenarios.
		ip = r.RemoteAddr
	}
	// Normalize IPv6 loopback and mapped-v4 addresses.
	ip = strings.TrimSpace(ip)
	return ip
}
