package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/repo"
	"github.com/gitwise-io/gitwise/internal/services/webhook"
)

type WebhookHandler struct {
	repos    *repo.Service
	webhooks *webhook.Service
}

func NewWebhookHandler(repos *repo.Service, webhooks *webhook.Service) *WebhookHandler {
	return &WebhookHandler{repos: repos, webhooks: webhooks}
}

// lookupOwnedRepo resolves the repo from URL params and verifies the caller owns it.
// Returns the repo or writes an error and returns nil.
func (h *WebhookHandler) lookupOwnedRepo(w http.ResponseWriter, r *http.Request) *models.Repository {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return nil
	}

	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return nil
	}

	if repository.OwnerID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you do not own this repository")
		return nil
	}

	return repository
}

func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	repository := h.lookupOwnedRepo(w, r)
	if repository == nil {
		return
	}

	var req models.CreateWebhookRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	wh, err := h.webhooks.Create(r.Context(), repository.ID, req)
	if errors.Is(err, webhook.ErrInvalidURL) {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid webhook URL: must be http or https")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create webhook")
		return
	}

	writeJSON(w, http.StatusCreated, wh)
}

func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	repository := h.lookupOwnedRepo(w, r)
	if repository == nil {
		return
	}

	webhooks, err := h.webhooks.List(r.Context(), repository.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list webhooks")
		return
	}

	writeJSON(w, http.StatusOK, webhooks)
}

func (h *WebhookHandler) Get(w http.ResponseWriter, r *http.Request) {
	repository := h.lookupOwnedRepo(w, r)
	if repository == nil {
		return
	}

	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid webhook ID")
		return
	}

	wh, err := h.webhooks.Get(r.Context(), repository.ID, webhookID)
	if errors.Is(err, webhook.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "webhook not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get webhook")
		return
	}

	writeJSON(w, http.StatusOK, wh)
}

func (h *WebhookHandler) Update(w http.ResponseWriter, r *http.Request) {
	repository := h.lookupOwnedRepo(w, r)
	if repository == nil {
		return
	}

	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid webhook ID")
		return
	}

	var req models.UpdateWebhookRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	wh, err := h.webhooks.Update(r.Context(), repository.ID, webhookID, req)
	if errors.Is(err, webhook.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "webhook not found")
		return
	}
	if errors.Is(err, webhook.ErrInvalidURL) {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid webhook URL: must be http or https")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update webhook")
		return
	}

	writeJSON(w, http.StatusOK, wh)
}

func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	repository := h.lookupOwnedRepo(w, r)
	if repository == nil {
		return
	}

	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid webhook ID")
		return
	}

	if err := h.webhooks.Delete(r.Context(), repository.ID, webhookID); err != nil {
		if errors.Is(err, webhook.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete webhook")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *WebhookHandler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	repository := h.lookupOwnedRepo(w, r)
	if repository == nil {
		return
	}

	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid webhook ID")
		return
	}

	// Verify webhook belongs to this repo
	if _, err := h.webhooks.Get(r.Context(), repository.ID, webhookID); err != nil {
		if errors.Is(err, webhook.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to verify webhook")
		return
	}

	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	deliveries, err := h.webhooks.ListDeliveries(r.Context(), webhookID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list deliveries")
		return
	}

	writeJSON(w, http.StatusOK, deliveries)
}

func (h *WebhookHandler) Test(w http.ResponseWriter, r *http.Request) {
	repository := h.lookupOwnedRepo(w, r)
	if repository == nil {
		return
	}

	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid webhook ID")
		return
	}

	wh, err := h.webhooks.Get(r.Context(), repository.ID, webhookID)
	if errors.Is(err, webhook.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "webhook not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get webhook")
		return
	}

	payload := map[string]any{
		"action": "ping",
		"hook_id": wh.ID,
		"repository": map[string]any{
			"id":   repository.ID,
			"name": repository.Name,
		},
	}

	h.webhooks.Dispatch(r.Context(), repository.ID, "ping", payload)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ping sent"})
}
