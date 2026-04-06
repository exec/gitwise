package middleware

import (
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// APIRateLimit returns middleware that applies different rate limits based on
// the HTTP method: write operations (POST, PUT, PATCH, DELETE) get a tighter
// budget than read operations (GET, HEAD, OPTIONS). This avoids needing to
// wrap every individual route with .With().
func APIRateLimit(rdb *redis.Client, readLimit, writeLimit int, window time.Duration) func(http.Handler) http.Handler {
	readMW := rateLimit(rdb, readLimit, window, "api_read")
	writeMW := rateLimit(rdb, writeLimit, window, "api_write")

	return func(next http.Handler) http.Handler {
		readHandler := readMW(next)
		writeHandler := writeMW(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				writeHandler.ServeHTTP(w, r)
			default:
				readHandler.ServeHTTP(w, r)
			}
		})
	}
}
