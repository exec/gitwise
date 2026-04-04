package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/milestone"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

type MilestoneHandler struct {
	repos      *repo.Service
	milestones *milestone.Service
}

func NewMilestoneHandler(repos *repo.Service, milestones *milestone.Service) *MilestoneHandler {
	return &MilestoneHandler{repos: repos, milestones: milestones}
}

func (h *MilestoneHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	if repository.OwnerID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you do not own this repository")
		return
	}

	var req models.CreateMilestoneRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	m, err := h.milestones.Create(r.Context(), repository.ID, req)
	if errors.Is(err, milestone.ErrInvalidTitle) {
		writeError(w, http.StatusBadRequest, "validation_error", "milestone title is required (max 255 chars)")
		return
	}
	if errors.Is(err, milestone.ErrDuplicateTitle) {
		writeError(w, http.StatusConflict, "duplicate", "milestone with this title already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create milestone")
		return
	}

	writeJSON(w, http.StatusCreated, m)
}

func (h *MilestoneHandler) List(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	viewerID := middleware.GetUserID(r.Context())

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, viewerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	milestones, err := h.milestones.List(r.Context(), repository.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list milestones")
		return
	}

	writeJSON(w, http.StatusOK, milestones)
}

func (h *MilestoneHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	if repository.OwnerID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you do not own this repository")
		return
	}

	milestoneID, err := uuid.Parse(chi.URLParam(r, "milestoneID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid milestone ID")
		return
	}

	var req models.UpdateMilestoneRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	m, err := h.milestones.Update(r.Context(), repository.ID, milestoneID, req)
	if errors.Is(err, milestone.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "milestone not found")
		return
	}
	if errors.Is(err, milestone.ErrInvalidTitle) {
		writeError(w, http.StatusBadRequest, "validation_error", "milestone title is required (max 255 chars)")
		return
	}
	if errors.Is(err, milestone.ErrDuplicateTitle) {
		writeError(w, http.StatusConflict, "duplicate", "milestone with this title already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update milestone")
		return
	}

	writeJSON(w, http.StatusOK, m)
}

func (h *MilestoneHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	if repository.OwnerID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you do not own this repository")
		return
	}

	milestoneID, err := uuid.Parse(chi.URLParam(r, "milestoneID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid milestone ID")
		return
	}

	if err := h.milestones.Delete(r.Context(), repository.ID, milestoneID); err != nil {
		if errors.Is(err, milestone.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "milestone not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete milestone")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
