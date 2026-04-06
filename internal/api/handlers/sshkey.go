package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/sshkey"
)

type SSHKeyHandler struct {
	sshkeys *sshkey.Service
}

func NewSSHKeyHandler(sshkeys *sshkey.Service) *SSHKeyHandler {
	return &SSHKeyHandler{sshkeys: sshkeys}
}

func (h *SSHKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req models.CreateSSHKeyRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	key, err := h.sshkeys.Add(r.Context(), *userID, req)
	if errors.Is(err, sshkey.ErrInvalidKey) {
		writeError(w, http.StatusBadRequest, "invalid_key", err.Error())
		return
	}
	if errors.Is(err, sshkey.ErrDuplicateKey) {
		writeError(w, http.StatusConflict, "duplicate_key", "this SSH key has already been added")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to add SSH key")
		return
	}

	writeJSON(w, http.StatusCreated, key)
}

func (h *SSHKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	keys, err := h.sshkeys.List(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list SSH keys")
		return
	}

	writeJSON(w, http.StatusOK, keys)
}

func (h *SSHKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	keyIDStr := chi.URLParam(r, "keyID")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid key ID")
		return
	}

	if err := h.sshkeys.Delete(r.Context(), *userID, keyID); err != nil {
		if errors.Is(err, sshkey.ErrKeyNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "SSH key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete SSH key")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
