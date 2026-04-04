package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/user"
)

type AuthHandler struct {
	users    *user.Service
	sessions *middleware.SessionManager
}

func NewAuthHandler(users *user.Service, sessions *middleware.SessionManager) *AuthHandler {
	return &AuthHandler{users: users, sessions: sessions}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid request body")
		return
	}

	u, err := h.users.Create(r.Context(), req)
	if errors.Is(err, user.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if errors.Is(err, user.ErrDuplicateUser) {
		writeError(w, http.StatusConflict, "duplicate", "username or email already taken")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create user")
		return
	}

	if err := h.sessions.Create(r.Context(), w, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create session")
		return
	}

	writeJSON(w, http.StatusCreated, u)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid request body")
		return
	}

	u, err := h.users.Authenticate(r.Context(), req.Login, req.Password)
	if errors.Is(err, user.ErrBadCredentials) {
		writeError(w, http.StatusUnauthorized, "bad_credentials", "invalid username/email or password")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "authentication failed")
		return
	}

	if err := h.sessions.Create(r.Context(), w, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create session")
		return
	}

	writeJSON(w, http.StatusOK, u)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session, sessionID, err := h.sessions.Get(r.Context(), r)
	if err != nil || session == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	h.sessions.Destroy(r.Context(), w, sessionID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	u, err := h.users.GetByID(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get user")
		return
	}

	writeJSON(w, http.StatusOK, u)
}

// Token management

func (h *AuthHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req models.CreateTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid request body")
		return
	}

	token, err := h.users.CreateToken(r.Context(), *userID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create token")
		return
	}

	writeJSON(w, http.StatusCreated, token)
}

func (h *AuthHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	tokens, err := h.users.ListTokens(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list tokens")
		return
	}

	writeJSON(w, http.StatusOK, tokens)
}

func (h *AuthHandler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	tokenIDStr := chi.URLParam(r, "tokenID")
	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid token ID")
		return
	}

	if err := h.users.DeleteToken(r.Context(), *userID, tokenID); err != nil {
		if errors.Is(err, user.ErrTokenNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
