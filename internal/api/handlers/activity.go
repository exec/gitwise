package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/activity"
	"github.com/gitwise-io/gitwise/internal/services/repo"
	"github.com/gitwise-io/gitwise/internal/services/user"
)

type ActivityHandler struct {
	repoSvc     *repo.Service
	activitySvc *activity.Service
	userSvc     *user.Service
}

func NewActivityHandler(repoSvc *repo.Service, activitySvc *activity.Service, userSvc *user.Service) *ActivityHandler {
	return &ActivityHandler{repoSvc: repoSvc, activitySvc: activitySvc, userSvc: userSvc}
}

func (h *ActivityHandler) ListByRepo(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	viewerID := middleware.GetUserID(r.Context())

	repository, err := h.repoSvc.GetByOwnerAndName(r.Context(), owner, repoName, viewerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	events, nextCursor, err := h.activitySvc.ListByRepo(r.Context(), repository.ID, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list activity")
		return
	}

	writeJSONMeta(w, http.StatusOK, events, &models.ResponseMeta{NextCursor: nextCursor})
}

func (h *ActivityHandler) ListByUser(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")

	u, err := h.userSvc.GetByUsername(r.Context(), username)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	events, nextCursor, err := h.activitySvc.ListByUser(r.Context(), u.ID, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list activity")
		return
	}

	writeJSONMeta(w, http.StatusOK, events, &models.ResponseMeta{NextCursor: nextCursor})
}
