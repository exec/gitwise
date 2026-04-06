package middleware

import "net/http"

// SecurityHeaders returns middleware that sets recommended security headers
// on every response. These headers mitigate clickjacking, MIME-sniffing,
// and other common browser-side attacks.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		// X-XSS-Protection: 0 disables the legacy XSS auditor which can
		// introduce vulnerabilities. CSP is the modern replacement.
		h.Set("X-XSS-Protection", "0")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' ws: wss:")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		next.ServeHTTP(w, r)
	})
}
