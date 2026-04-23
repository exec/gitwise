package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/services/importer"
)

// ImportHandler handles repository import endpoints.
type ImportHandler struct {
	importSvc *importer.Service
}

// NewImportHandler creates a new import handler.
func NewImportHandler(importSvc *importer.Service) *ImportHandler {
	return &ImportHandler{importSvc: importSvc}
}

// ImportGitHub starts a GitHub repository import.
func (h *ImportHandler) ImportGitHub(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req importer.GitHubImportRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if strings.TrimSpace(req.RepoURL) == "" {
		writeFieldError(w, http.StatusBadRequest, "required", "Repository URL is required", "repo_url")
		return
	}

	if req.Visibility != "" && req.Visibility != "public" && req.Visibility != "private" {
		writeFieldError(w, http.StatusBadRequest, "invalid_value", "visibility must be 'public' or 'private'", "visibility")
		return
	}

	jobID, err := h.importSvc.StartGitHubImport(r.Context(), *userID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "import_error", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"id":     jobID,
		"status": "running",
	})
}

// ImportGitLab starts a GitLab repository import.
func (h *ImportHandler) ImportGitLab(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req importer.GitLabImportRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if strings.TrimSpace(req.ProjectURL) == "" {
		writeFieldError(w, http.StatusBadRequest, "required", "Project URL is required", "project_url")
		return
	}

	if req.Visibility != "" && req.Visibility != "public" && req.Visibility != "private" {
		writeFieldError(w, http.StatusBadRequest, "invalid_value", "visibility must be 'public' or 'private'", "visibility")
		return
	}

	jobID, err := h.importSvc.StartGitLabImport(r.Context(), *userID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "import_error", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"id":     jobID,
		"status": "running",
	})
}

// GetImportStatus returns the current status of an import job.
// The job must belong to the authenticated user; otherwise 404 is returned
// so callers cannot enumerate job IDs.
func (h *ImportHandler) GetImportStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "required", "job ID is required")
		return
	}

	status, err := h.importSvc.GetStatus(r.Context(), jobID, *userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "import job not found")
		return
	}

	writeJSON(w, http.StatusOK, status)
}
