package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/org"
	"github.com/gitwise-io/gitwise/internal/services/user"
)

type OrgHandler struct {
	orgSvc  *org.Service
	userSvc *user.Service
}

func NewOrgHandler(orgSvc *org.Service, userSvc *user.Service) *OrgHandler {
	return &OrgHandler{orgSvc: orgSvc, userSvc: userSvc}
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

// Create handles POST /api/v1/orgs.
func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req models.CreateOrgRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	o, err := h.orgSvc.Create(r.Context(), *userID, req)
	if errors.Is(err, org.ErrInvalidName) {
		writeError(w, http.StatusBadRequest, "invalid_name", err.Error())
		return
	}
	if errors.Is(err, org.ErrNameConflict) {
		writeError(w, http.StatusConflict, "name_conflict", "name conflicts with an existing username")
		return
	}
	if errors.Is(err, org.ErrDuplicate) {
		writeError(w, http.StatusConflict, "duplicate", "organization name already taken")
		return
	}
	if err != nil {
		slog.Error("failed to create organization", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create organization")
		return
	}

	writeJSON(w, http.StatusCreated, o)
}

// Update handles PUT /api/v1/orgs/{name}.
func (h *OrgHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	name := chi.URLParam(r, "name")

	isOwner, err := h.orgSvc.IsOwner(r.Context(), name, *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to check permissions")
		return
	}
	if !isOwner {
		writeError(w, http.StatusForbidden, "forbidden", "only org owners can update the organization")
		return
	}

	var req models.UpdateOrgRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	o, err := h.orgSvc.Update(r.Context(), name, req)
	if errors.Is(err, org.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "organization not found")
		return
	}
	if err != nil {
		slog.Error("failed to update organization", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update organization")
		return
	}

	writeJSON(w, http.StatusOK, o)
}

// Delete handles DELETE /api/v1/orgs/{name}.
func (h *OrgHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	name := chi.URLParam(r, "name")

	isOwner, err := h.orgSvc.IsOwner(r.Context(), name, *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to check permissions")
		return
	}
	if !isOwner {
		writeError(w, http.StatusForbidden, "forbidden", "only org owners can delete the organization")
		return
	}

	if err := h.orgSvc.Delete(r.Context(), name); err != nil {
		if errors.Is(err, org.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "organization not found")
			return
		}
		slog.Error("failed to delete organization", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete organization")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// AddOrUpdateMember handles PUT /api/v1/orgs/{name}/members/{username}.
func (h *OrgHandler) AddOrUpdateMember(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	name := chi.URLParam(r, "name")
	username := chi.URLParam(r, "username")

	isOwner, err := h.orgSvc.IsOwner(r.Context(), name, *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to check permissions")
		return
	}
	if !isOwner {
		writeError(w, http.StatusForbidden, "forbidden", "only org owners can manage members")
		return
	}

	var req models.OrgMemberRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	role := req.Role
	if role == "" {
		role = "member"
	}

	if err := h.orgSvc.AddMember(r.Context(), name, username, role); err != nil {
		if errors.Is(err, org.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "organization not found")
			return
		}
		if errors.Is(err, org.ErrInvalidRole) {
			writeError(w, http.StatusBadRequest, "invalid_role", err.Error())
			return
		}
		slog.Error("failed to add/update member", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to add or update member")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RemoveMember handles DELETE /api/v1/orgs/{name}/members/{username}.
func (h *OrgHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	name := chi.URLParam(r, "name")
	username := chi.URLParam(r, "username")

	isOwner, err := h.orgSvc.IsOwner(r.Context(), name, *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to check permissions")
		return
	}
	if !isOwner {
		writeError(w, http.StatusForbidden, "forbidden", "only org owners can manage members")
		return
	}

	if err := h.orgSvc.RemoveMember(r.Context(), name, username); err != nil {
		if errors.Is(err, org.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "organization not found")
			return
		}
		if errors.Is(err, org.ErrMemberNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "member not found")
			return
		}
		if errors.Is(err, org.ErrLastOwner) {
			writeError(w, http.StatusBadRequest, "last_owner", "cannot remove the last owner of an organization")
			return
		}
		slog.Error("failed to remove member", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to remove member")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListUserOrgs handles GET /api/v1/user/orgs.
func (h *OrgHandler) ListUserOrgs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	orgs, err := h.orgSvc.ListUserOrgs(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list organizations")
		return
	}

	writeJSON(w, http.StatusOK, orgs)
}

// Resolve handles GET /api/v1/resolve/{name}. Returns {type, data} for user or org.
func (h *OrgHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	// Check user first
	u, err := h.userSvc.GetByUsername(r.Context(), name)
	if err == nil {
		writeJSON(w, http.StatusOK, models.NamespaceResult{Type: "user", Data: u})
		return
	}

	// Check org
	o, err := h.orgSvc.GetByName(r.Context(), name)
	if err == nil {
		writeJSON(w, http.StatusOK, models.NamespaceResult{Type: "org", Data: o})
		return
	}

	writeError(w, http.StatusNotFound, "not_found", "user or organization not found")
}
