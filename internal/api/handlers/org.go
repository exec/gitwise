package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/services/org"
)

type OrgHandler struct {
	orgSvc *org.Service
}

func NewOrgHandler(orgSvc *org.Service) *OrgHandler {
	return &OrgHandler{orgSvc: orgSvc}
}

// Get handles GET /api/v1/orgs/{name}.
func (h *OrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	o, err := h.orgSvc.GetByName(r.Context(), name)
	if errors.Is(err, org.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "organization not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get organization")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// ListMembers handles GET /api/v1/orgs/{name}/members.
func (h *OrgHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	o, err := h.orgSvc.GetByName(r.Context(), name)
	if errors.Is(err, org.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "organization not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get organization")
		return
	}

	members, err := h.orgSvc.ListMembers(r.Context(), o.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list members")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// ListRepos handles GET /api/v1/orgs/{name}/repos.
func (h *OrgHandler) ListRepos(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	o, err := h.orgSvc.GetByName(r.Context(), name)
	if errors.Is(err, org.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "organization not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get organization")
		return
	}

	limit := 30
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	viewerID := middleware.GetUserID(r.Context())
	repos, err := h.orgSvc.ListRepos(r.Context(), o.ID, viewerID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list repos")
		return
	}
	writeJSON(w, http.StatusOK, repos)
}
