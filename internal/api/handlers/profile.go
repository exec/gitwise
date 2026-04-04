package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/user"
)

type ProfileHandler struct {
	userSvc *user.Service
}

func NewProfileHandler(userSvc *user.Service) *ProfileHandler {
	return &ProfileHandler{userSvc: userSvc}
}

// UpdateProfile handles PATCH /api/v1/user/profile (authenticated).
func (h *ProfileHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	var req models.UpdateUserRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	u, err := h.userSvc.Update(r.Context(), *userID, req)
	if errors.Is(err, user.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update profile")
		return
	}

	writeJSON(w, http.StatusOK, u)
}

// GetContributions handles GET /api/v1/users/{username}/contributions.
func (h *ProfileHandler) GetContributions(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	u, err := h.userSvc.GetByUsername(r.Context(), username)
	if errors.Is(err, user.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get user")
		return
	}

	from, to, err := parseContributionRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_params", err.Error())
		return
	}

	days, err := h.userSvc.GetContributions(r.Context(), u.ID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get contributions")
		return
	}

	writeJSON(w, http.StatusOK, days)
}

// ListPinnedRepos handles GET /api/v1/users/{username}/pinned-repos.
func (h *ProfileHandler) ListPinnedRepos(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	u, err := h.userSvc.GetByUsername(r.Context(), username)
	if errors.Is(err, user.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get user")
		return
	}

	pinned, err := h.userSvc.ListPinnedRepos(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list pinned repos")
		return
	}

	writeJSON(w, http.StatusOK, pinned)
}

// SetPinnedRepos handles PUT /api/v1/user/pinned-repos (authenticated).
func (h *ProfileHandler) SetPinnedRepos(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	var req models.SetPinnedReposRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if err := h.userSvc.SetPinnedRepos(r.Context(), *userID, req.RepoIDs); err != nil {
		if errors.Is(err, user.ErrTooManyPins) {
			writeError(w, http.StatusBadRequest, "too_many_pins", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to set pinned repos")
		return
	}

	pinned, err := h.userSvc.ListPinnedRepos(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list pinned repos")
		return
	}

	writeJSON(w, http.StatusOK, pinned)
}

// GetActivity handles GET /api/v1/users/{username}/activity (stub).
func (h *ProfileHandler) GetActivity(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func parseContributionRange(r *http.Request) (time.Time, time.Time, error) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	now := time.Now()
	layout := "2006-01-02"

	var from, to time.Time
	var err error

	if fromStr == "" {
		from = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	} else {
		from, err = time.Parse(layout, fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("invalid 'from' date, expected YYYY-MM-DD")
		}
	}

	if toStr == "" {
		to = now.Truncate(24 * time.Hour)
	} else {
		to, err = time.Parse(layout, toStr)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("invalid 'to' date, expected YYYY-MM-DD")
		}
	}

	return from, to, nil
}
