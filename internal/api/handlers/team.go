package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/org"
	"github.com/gitwise-io/gitwise/internal/services/team"
)

type TeamHandler struct {
	orgSvc  *org.Service
	teamSvc *team.Service
}

func NewTeamHandler(orgSvc *org.Service, teamSvc *team.Service) *TeamHandler {
	return &TeamHandler{orgSvc: orgSvc, teamSvc: teamSvc}
}

// requireOrgOwner resolves the org from the URL and verifies the caller is an owner.
// Returns the org on success, or writes an error and returns nil.
func (h *TeamHandler) requireOrgOwner(w http.ResponseWriter, r *http.Request) *models.Organization {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return nil
	}

	name := chi.URLParam(r, "name")
	o, err := h.orgSvc.GetByName(r.Context(), name)
	if errors.Is(err, org.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "organization not found")
		return nil
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get organization")
		return nil
	}

	isOwner, err := h.teamSvc.IsOrgOwner(r.Context(), o.ID, *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to check permissions")
		return nil
	}
	if !isOwner {
		writeError(w, http.StatusForbidden, "forbidden", "only organization owners can manage teams")
		return nil
	}

	return o
}

// resolveOrg resolves the org from the URL (no ownership check).
func (h *TeamHandler) resolveOrg(w http.ResponseWriter, r *http.Request) *models.Organization {
	name := chi.URLParam(r, "name")
	o, err := h.orgSvc.GetByName(r.Context(), name)
	if errors.Is(err, org.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "organization not found")
		return nil
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get organization")
		return nil
	}
	return o
}

// Create handles POST /api/v1/orgs/{name}/teams.
func (h *TeamHandler) Create(w http.ResponseWriter, r *http.Request) {
	o := h.requireOrgOwner(w, r)
	if o == nil {
		return
	}

	var req models.CreateTeamRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	t, err := h.teamSvc.Create(r.Context(), o.ID, req)
	if errors.Is(err, team.ErrInvalidName) {
		writeError(w, http.StatusBadRequest, "validation_error", "team name is required (max 100 chars)")
		return
	}
	if errors.Is(err, team.ErrInvalidPerm) {
		writeError(w, http.StatusBadRequest, "validation_error", "permission must be read, triage, write, or admin")
		return
	}
	if errors.Is(err, team.ErrDuplicate) {
		writeError(w, http.StatusConflict, "duplicate", "team already exists in this organization")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create team")
		return
	}

	writeJSON(w, http.StatusCreated, t)
}

// List handles GET /api/v1/orgs/{name}/teams.
func (h *TeamHandler) List(w http.ResponseWriter, r *http.Request) {
	o := h.resolveOrg(w, r)
	if o == nil {
		return
	}

	teams, err := h.teamSvc.List(r.Context(), o.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list teams")
		return
	}

	writeJSON(w, http.StatusOK, teams)
}

// Get handles GET /api/v1/orgs/{name}/teams/{team}.
func (h *TeamHandler) Get(w http.ResponseWriter, r *http.Request) {
	o := h.resolveOrg(w, r)
	if o == nil {
		return
	}

	teamName := chi.URLParam(r, "team")
	t, err := h.teamSvc.Get(r.Context(), o.ID, teamName)
	if errors.Is(err, team.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get team")
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// Update handles PUT /api/v1/orgs/{name}/teams/{team}.
func (h *TeamHandler) Update(w http.ResponseWriter, r *http.Request) {
	o := h.requireOrgOwner(w, r)
	if o == nil {
		return
	}

	teamName := chi.URLParam(r, "team")
	var req models.UpdateTeamRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	t, err := h.teamSvc.Update(r.Context(), o.ID, teamName, req)
	if errors.Is(err, team.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if errors.Is(err, team.ErrInvalidName) {
		writeError(w, http.StatusBadRequest, "validation_error", "team name is required (max 100 chars)")
		return
	}
	if errors.Is(err, team.ErrInvalidPerm) {
		writeError(w, http.StatusBadRequest, "validation_error", "permission must be read, triage, write, or admin")
		return
	}
	if errors.Is(err, team.ErrDuplicate) {
		writeError(w, http.StatusConflict, "duplicate", "team name already exists in this organization")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update team")
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// Delete handles DELETE /api/v1/orgs/{name}/teams/{team}.
func (h *TeamHandler) Delete(w http.ResponseWriter, r *http.Request) {
	o := h.requireOrgOwner(w, r)
	if o == nil {
		return
	}

	teamName := chi.URLParam(r, "team")
	if err := h.teamSvc.Delete(r.Context(), o.ID, teamName); err != nil {
		if errors.Is(err, team.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "team not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete team")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// AddMember handles PUT /api/v1/orgs/{name}/teams/{team}/members/{username}.
func (h *TeamHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	o := h.requireOrgOwner(w, r)
	if o == nil {
		return
	}

	teamName := chi.URLParam(r, "team")
	username := chi.URLParam(r, "username")

	err := h.teamSvc.AddMember(r.Context(), o.ID, teamName, username)
	if errors.Is(err, team.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if errors.Is(err, team.ErrMemberNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found or not an org member")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to add member")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// RemoveMember handles DELETE /api/v1/orgs/{name}/teams/{team}/members/{username}.
func (h *TeamHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	o := h.requireOrgOwner(w, r)
	if o == nil {
		return
	}

	teamName := chi.URLParam(r, "team")
	username := chi.URLParam(r, "username")

	err := h.teamSvc.RemoveMember(r.Context(), o.ID, teamName, username)
	if errors.Is(err, team.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if errors.Is(err, team.ErrMemberNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found or not a team member")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to remove member")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ListMembers handles GET /api/v1/orgs/{name}/teams/{team}/members.
func (h *TeamHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	o := h.resolveOrg(w, r)
	if o == nil {
		return
	}

	teamName := chi.URLParam(r, "team")
	members, err := h.teamSvc.ListMembers(r.Context(), o.ID, teamName)
	if errors.Is(err, team.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list members")
		return
	}

	writeJSON(w, http.StatusOK, members)
}

// AddRepo handles PUT /api/v1/orgs/{name}/teams/{team}/repos/{repo}.
func (h *TeamHandler) AddRepo(w http.ResponseWriter, r *http.Request) {
	o := h.requireOrgOwner(w, r)
	if o == nil {
		return
	}

	teamName := chi.URLParam(r, "team")
	repoName := chi.URLParam(r, "repo")

	err := h.teamSvc.AddRepo(r.Context(), o.ID, teamName, repoName)
	if errors.Is(err, team.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if errors.Is(err, team.ErrRepoNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "repository not found or not owned by this organization")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to add repository")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// RemoveRepo handles DELETE /api/v1/orgs/{name}/teams/{team}/repos/{repo}.
func (h *TeamHandler) RemoveRepo(w http.ResponseWriter, r *http.Request) {
	o := h.requireOrgOwner(w, r)
	if o == nil {
		return
	}

	teamName := chi.URLParam(r, "team")
	repoName := chi.URLParam(r, "repo")

	err := h.teamSvc.RemoveRepo(r.Context(), o.ID, teamName, repoName)
	if errors.Is(err, team.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if errors.Is(err, team.ErrRepoNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "repository not found or not assigned to this team")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to remove repository")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ListRepos handles GET /api/v1/orgs/{name}/teams/{team}/repos.
func (h *TeamHandler) ListRepos(w http.ResponseWriter, r *http.Request) {
	o := h.resolveOrg(w, r)
	if o == nil {
		return
	}

	teamName := chi.URLParam(r, "team")
	repos, err := h.teamSvc.ListRepos(r.Context(), o.ID, teamName)
	if errors.Is(err, team.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list repos")
		return
	}

	writeJSON(w, http.StatusOK, repos)
}
