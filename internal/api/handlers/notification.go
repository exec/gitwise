package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/services/notification"
)

type NotificationHandler struct {
	notifications *notification.Service
}

func NewNotificationHandler(notifications *notification.Service) *NotificationHandler {
	return &NotificationHandler{notifications: notifications}
}

func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	unreadOnly := r.URL.Query().Get("unread_only") == "true"
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	notifications, err := h.notifications.ListForUser(r.Context(), *userID, unreadOnly, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list notifications")
		return
	}

	writeJSON(w, http.StatusOK, notifications)
}

func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	notifIDStr := chi.URLParam(r, "notifID")
	notifID, err := uuid.Parse(notifIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid notification ID")
		return
	}

	err = h.notifications.MarkRead(r.Context(), notifID, *userID)
	if errors.Is(err, notification.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "notification not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to mark notification as read")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	err := h.notifications.MarkAllRead(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to mark all notifications as read")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
