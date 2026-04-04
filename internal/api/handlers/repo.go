package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

type RepoHandler struct {
	repos *repo.Service
}

func NewRepoHandler(repos *repo.Service) *RepoHandler {
	return &RepoHandler{repos: repos}
}

func (h *RepoHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req models.CreateRepoRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	repository, err := h.repos.Create(r.Context(), *userID, req)
	if errors.Is(err, repo.ErrInvalidName) {
		writeError(w, http.StatusBadRequest, "invalid_name", "repository name must be 2-100 alphanumeric characters")
		return
	}
	if errors.Is(err, repo.ErrDuplicate) {
		writeError(w, http.StatusConflict, "duplicate", "repository already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create repository")
		return
	}

	writeJSON(w, http.StatusCreated, repository)
}

func (h *RepoHandler) Get(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get repository")
		return
	}

	writeJSON(w, http.StatusOK, repository)
}

func (h *RepoHandler) ListByOwner(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	repos, err := h.repos.ListByOwner(r.Context(), owner, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list repositories")
		return
	}

	writeJSON(w, http.StatusOK, repos)
}

func (h *RepoHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	repos, err := h.repos.ListForUser(r.Context(), *userID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list repositories")
		return
	}

	writeJSON(w, http.StatusOK, repos)
}

func (h *RepoHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get repository")
		return
	}

	if repository.OwnerID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you do not own this repository")
		return
	}

	var req models.UpdateRepoRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	updated, err := h.repos.Update(r.Context(), repository.ID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update repository")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h *RepoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get repository")
		return
	}

	if repository.OwnerID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you do not own this repository")
		return
	}

	if err := h.repos.Delete(r.Context(), owner, repoName, repository.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete repository")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
