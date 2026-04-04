package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/label"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

type LabelHandler struct {
	repos  *repo.Service
	labels *label.Service
}

func NewLabelHandler(repos *repo.Service, labels *label.Service) *LabelHandler {
	return &LabelHandler{repos: repos, labels: labels}
}

func (h *LabelHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateLabelRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	lbl, err := h.labels.Create(r.Context(), repository.ID, req)
	if errors.Is(err, label.ErrInvalidName) {
		writeError(w, http.StatusBadRequest, "validation_error", "label name is required (max 255 chars)")
		return
	}
	if errors.Is(err, label.ErrDuplicate) {
		writeError(w, http.StatusConflict, "duplicate", "label already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create label")
		return
	}

	writeJSON(w, http.StatusCreated, lbl)
}

func (h *LabelHandler) List(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	viewerID := middleware.GetUserID(r.Context())

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, viewerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	labels, err := h.labels.List(r.Context(), repository.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list labels")
		return
	}

	writeJSON(w, http.StatusOK, labels)
}

func (h *LabelHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	labelID, err := uuid.Parse(chi.URLParam(r, "labelID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid label ID")
		return
	}

	var req models.UpdateLabelRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	lbl, err := h.labels.Update(r.Context(), repository.ID, labelID, req)
	if errors.Is(err, label.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "label not found")
		return
	}
	if errors.Is(err, label.ErrDuplicate) {
		writeError(w, http.StatusConflict, "duplicate", "label name already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update label")
		return
	}

	writeJSON(w, http.StatusOK, lbl)
}

func (h *LabelHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	labelID, err := uuid.Parse(chi.URLParam(r, "labelID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid label ID")
		return
	}

	if err := h.labels.Delete(r.Context(), repository.ID, labelID); err != nil {
		if errors.Is(err, label.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "label not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete label")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
