package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gitwise-io/gitwise/internal/services/mirror"
)

// AdminMirrorHandler handles admin-only mirror endpoints.
type AdminMirrorHandler struct {
	mirrors *mirror.Service
}

// NewAdminMirrorHandler constructs an AdminMirrorHandler.
func NewAdminMirrorHandler(mirrors *mirror.Service) *AdminMirrorHandler {
	return &AdminMirrorHandler{mirrors: mirrors}
}

// List returns every configured mirror across the instance. Admin-only.
// Auth and admin checks are enforced by the RequireAuth + RequireAdmin
// middleware applied to the /admin-* route group in server.go.
func (h *AdminMirrorHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.mirrors.ListAll(r.Context())
	if err != nil {
		slog.Error("admin mirror list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "mirror_error", "failed to load mirrors")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mirrors": rows})
}
