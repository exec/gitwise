package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/protection"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

type ProtectionHandler struct {
	repos      *repo.Service
	protection *protection.Service
}

func NewProtectionHandler(repos *repo.Service, protection *protection.Service) *ProtectionHandler {
	return &ProtectionHandler{repos: repos, protection: protection}
}

func (h *ProtectionHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateBranchProtectionRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	rule, err := h.protection.Create(r.Context(), repository.ID, req)
	if errors.Is(err, protection.ErrInvalidPattern) {
		writeError(w, http.StatusBadRequest, "validation_error", "branch pattern is required (max 255 chars)")
		return
	}
	if errors.Is(err, protection.ErrDuplicate) {
		writeError(w, http.StatusConflict, "duplicate", "branch protection rule already exists for this pattern")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create branch protection rule")
		return
	}

	writeJSON(w, http.StatusCreated, rule)
}

func (h *ProtectionHandler) List(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	viewerID := middleware.GetUserID(r.Context())

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, viewerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	rules, err := h.protection.List(r.Context(), repository.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list branch protection rules")
		return
	}

	writeJSON(w, http.StatusOK, rules)
}

func (h *ProtectionHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	ruleID, err := uuid.Parse(chi.URLParam(r, "ruleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid rule ID")
		return
	}

	var req models.UpdateBranchProtectionRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	rule, err := h.protection.Update(r.Context(), repository.ID, ruleID, req)
	if errors.Is(err, protection.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "branch protection rule not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update branch protection rule")
		return
	}

	writeJSON(w, http.StatusOK, rule)
}

func (h *ProtectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	ruleID, err := uuid.Parse(chi.URLParam(r, "ruleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid rule ID")
		return
	}

	if err := h.protection.Delete(r.Context(), repository.ID, ruleID); err != nil {
		if errors.Is(err, protection.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "branch protection rule not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete branch protection rule")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
