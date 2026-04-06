package middleware

import (
	"net/http"

	"github.com/gitwise-io/gitwise/internal/services/user"
)

// RequireAdmin returns middleware that checks the authenticated user has is_admin = true.
// Returns 404 (not 403) to avoid revealing the admin path exists to non-admins.
func RequireAdmin(users *user.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == nil {
				http.NotFound(w, r)
				return
			}

			u, err := users.GetByID(r.Context(), *userID)
			if err != nil || !u.IsAdmin {
				http.NotFound(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
