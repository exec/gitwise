package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	gitpkg "github.com/gitwise-io/gitwise/internal/git"
	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

type BrowseHandler struct {
	repos *repo.Service
	git   *gitpkg.Service
}

func NewBrowseHandler(repos *repo.Service, git *gitpkg.Service) *BrowseHandler {
	return &BrowseHandler{repos: repos, git: git}
}

func (h *BrowseHandler) GetTree(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")
	treePath := chi.URLParam(r, "*")

	// Verify repo exists
	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, middleware.GetUserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	if ref == "" {
		ref = repository.DefaultBranch
	}

	entries, err := h.git.ListTree(owner, repoName, ref, treePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "path or ref not found")
		return
	}

	writeJSON(w, http.StatusOK, entries)
}

func (h *BrowseHandler) GetBlob(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")
	filePath := chi.URLParam(r, "*")

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, middleware.GetUserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	if ref == "" {
		ref = repository.DefaultBranch
	}

	blob, err := h.git.GetBlob(owner, repoName, ref, filePath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "resolve ref") {
			writeError(w, http.StatusNotFound, "not_found", "file not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get file")
		return
	}

	writeJSON(w, http.StatusOK, blob)
}

func (h *BrowseHandler) GetRawBlob(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")
	filePath := chi.URLParam(r, "*")

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, middleware.GetUserID(r.Context()))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if ref == "" {
		ref = repository.DefaultBranch
	}

	blob, err := h.git.GetBlob(owner, repoName, ref, filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(blob.Content))
}

func (h *BrowseHandler) ListCommits(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, middleware.GetUserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = repository.DefaultBranch
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	commits, hasMore, err := h.git.ListCommits(owner, repoName, ref, page, perPage)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "ref not found")
		return
	}

	meta := &models.ResponseMeta{}
	if hasMore {
		if page <= 0 {
			page = 1
		}
		meta.NextCursor = strconv.Itoa(page + 1)
	}

	writeJSONMeta(w, http.StatusOK, commits, meta)
}

func (h *BrowseHandler) GetCommit(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	sha := chi.URLParam(r, "sha")

	_, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, middleware.GetUserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	detail, err := h.git.GetCommit(owner, repoName, sha)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "commit not found")
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

func (h *BrowseHandler) ListBranches(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	_, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, middleware.GetUserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	branches, err := h.git.ListBranches(owner, repoName)
	if err != nil {
		// Empty repos have no branches — return empty array instead of 500
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	writeJSON(w, http.StatusOK, branches)
}
