package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/notification"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

type NotificationHandler struct {
	notifications *notification.Service
	repos         *repo.Service
}

func NewNotificationHandler(notifications *notification.Service, repos *repo.Service) *NotificationHandler {
	return &NotificationHandler{notifications: notifications, repos: repos}
}

func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	unreadOnly := r.URL.Query().Get("unread_only") == "true"
	limit := parseLimit(r, 25, 100)

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

// GetPreferences returns the notification preferences for the authenticated user.
func (h *NotificationHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	prefs, err := h.notifications.GetPreferences(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get notification preferences")
		return
	}

	writeJSON(w, http.StatusOK, prefs)
}

// UpdatePreferences updates the notification preferences for the authenticated user.
func (h *NotificationHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req models.UpdateNotificationPreferencesRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	prefs, err := h.notifications.UpdatePreferences(r.Context(), *userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update notification preferences")
		return
	}

	writeJSON(w, http.StatusOK, prefs)
}

// WatchRepo subscribes the authenticated user to a repository's notifications.
func (h *NotificationHandler) WatchRepo(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	if err := h.notifications.WatchRepo(r.Context(), *userID, repository.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to watch repository")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"watching": true})
}

// UnwatchRepo unsubscribes the authenticated user from a repository's notifications.
func (h *NotificationHandler) UnwatchRepo(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	if err := h.notifications.UnwatchRepo(r.Context(), *userID, repository.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to unwatch repository")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"watching": false})
}

// GetWatchStatus returns whether the authenticated user is watching a repository,
// along with the total watcher count.
func (h *NotificationHandler) GetWatchStatus(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	userID := middleware.GetUserID(r.Context())

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	count, err := h.notifications.WatcherCount(r.Context(), repository.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get watcher count")
		return
	}

	watching := false
	if userID != nil {
		watching, err = h.notifications.IsWatching(r.Context(), *userID, repository.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "server_error", "failed to check watch status")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"watching": watching,
		"count":    count,
	})
}
