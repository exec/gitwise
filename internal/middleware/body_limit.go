package middleware

import "net/http"

// DefaultBodyLimit is the default maximum request body size (50 MiB).
// Git push routes use a larger limit configured in server wiring.
const DefaultBodyLimit = 50 << 20 // 50 MiB

// MaxBodySize returns middleware that limits the request body to maxBytes.
// If the body exceeds the limit, subsequent reads return an error and the
// server responds with 413 Request Entity Too Large.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
