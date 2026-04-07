package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/agent"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

type AgentHandler struct {
	repos  *repo.Service
	agents *agent.Service
}

func NewAgentHandler(repos *repo.Service, agents *agent.Service) *AgentHandler {
	return &AgentHandler{repos: repos, agents: agents}
}

// ListAvailable returns all available agents (official + user's custom).
// GET /api/v1/agents
func (h *AgentHandler) ListAvailable(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	agents, err := h.agents.ListAvailable(r.Context(), userID)
	if err != nil {
		slog.Error("failed to list agents", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list agents")
		return
	}

	writeJSON(w, http.StatusOK, agents)
}

// Create creates a new custom agent.
// POST /api/v1/agents
func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req models.CreateAgentRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	ag, err := h.agents.Create(r.Context(), *userID, req)
	if errors.Is(err, agent.ErrInvalidName) {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required (max 100 chars)")
		return
	}
	if errors.Is(err, agent.ErrInvalidSlug) {
		writeError(w, http.StatusBadRequest, "validation_error", "slug must be 2-100 lowercase alphanumeric characters with hyphens")
		return
	}
	if errors.Is(err, agent.ErrDuplicate) {
		writeError(w, http.StatusConflict, "duplicate", "an agent with this slug already exists")
		return
	}
	if err != nil {
		slog.Error("failed to create agent", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create agent")
		return
	}

	writeJSON(w, http.StatusCreated, ag)
}

// GetBySlug returns an agent by its slug.
// GET /api/v1/agents/{slug}
func (h *AgentHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	ag, err := h.agents.GetBySlug(r.Context(), slug)
	if errors.Is(err, agent.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if err != nil {
		slog.Error("failed to get agent", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get agent")
		return
	}

	writeJSON(w, http.StatusOK, ag)
}

// Update updates a custom agent.
// PUT /api/v1/agents/{slug}
func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	slug := chi.URLParam(r, "slug")

	var req models.UpdateAgentRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	ag, err := h.agents.Update(r.Context(), slug, *userID, req)
	if errors.Is(err, agent.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if errors.Is(err, agent.ErrForbidden) {
		writeError(w, http.StatusForbidden, "forbidden", "you can only update your own custom agents")
		return
	}
	if errors.Is(err, agent.ErrInvalidName) {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required (max 100 chars)")
		return
	}
	if err != nil {
		slog.Error("failed to update agent", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update agent")
		return
	}

	writeJSON(w, http.StatusOK, ag)
}

// Delete deletes a custom agent.
// DELETE /api/v1/agents/{slug}
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	slug := chi.URLParam(r, "slug")

	err := h.agents.Delete(r.Context(), slug, *userID)
	if errors.Is(err, agent.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if errors.Is(err, agent.ErrForbidden) {
		writeError(w, http.StatusForbidden, "forbidden", "you can only delete your own custom agents")
		return
	}
	if err != nil {
		slog.Error("failed to delete agent", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete agent")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// resolveRepo is a helper that resolves a repo from URL params and optionally
// checks ownership. Returns nil if it wrote an error response.
func (h *AgentHandler) resolveRepo(w http.ResponseWriter, r *http.Request) *models.Repository {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	viewerID := middleware.GetUserID(r.Context())

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, viewerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return nil
	}
	return repository
}

// resolveOwnedRepo resolves a repo and verifies the caller owns it.
func (h *AgentHandler) resolveOwnedRepo(w http.ResponseWriter, r *http.Request) *models.Repository {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return nil
	}

	repository := h.resolveRepo(w, r)
	if repository == nil {
		return nil
	}

	if repository.OwnerID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you do not own this repository")
		return nil
	}
	return repository
}

// ListForRepo returns agents installed on a repository.
// GET /api/v1/repos/{owner}/{repo}/agents
func (h *AgentHandler) ListForRepo(w http.ResponseWriter, r *http.Request) {
	repository := h.resolveRepo(w, r)
	if repository == nil {
		return
	}

	agents, err := h.agents.ListForRepo(r.Context(), repository.ID)
	if err != nil {
		slog.Error("failed to list repo agents", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list agents")
		return
	}

	writeJSON(w, http.StatusOK, agents)
}

// Install installs an agent on a repository.
// POST /api/v1/repos/{owner}/{repo}/agents
func (h *AgentHandler) Install(w http.ResponseWriter, r *http.Request) {
	repository := h.resolveOwnedRepo(w, r)
	if repository == nil {
		return
	}

	var req models.InstallAgentRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	ra, err := h.agents.Install(r.Context(), repository.ID, req.AgentSlug, req)
	if errors.Is(err, agent.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if errors.Is(err, agent.ErrAlreadyInstalled) {
		writeError(w, http.StatusConflict, "duplicate", "agent is already installed on this repository")
		return
	}
	if err != nil {
		slog.Error("failed to install agent", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to install agent")
		return
	}

	writeJSON(w, http.StatusCreated, ra)
}

// UpdateConfig updates an installed agent's configuration.
// PUT /api/v1/repos/{owner}/{repo}/agents/{slug}
func (h *AgentHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	repository := h.resolveOwnedRepo(w, r)
	if repository == nil {
		return
	}

	slug := chi.URLParam(r, "slug")

	var req models.UpdateRepoAgentRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	ra, err := h.agents.UpdateConfig(r.Context(), repository.ID, slug, req)
	if errors.Is(err, agent.ErrNotInstalled) {
		writeError(w, http.StatusNotFound, "not_found", "agent not installed on this repository")
		return
	}
	if err != nil {
		slog.Error("failed to update agent config", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update agent config")
		return
	}

	writeJSON(w, http.StatusOK, ra)
}

// Uninstall removes an agent from a repository.
// DELETE /api/v1/repos/{owner}/{repo}/agents/{slug}
func (h *AgentHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	repository := h.resolveOwnedRepo(w, r)
	if repository == nil {
		return
	}

	slug := chi.URLParam(r, "slug")

	err := h.agents.Uninstall(r.Context(), repository.ID, slug)
	if errors.Is(err, agent.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if errors.Is(err, agent.ErrNotInstalled) {
		writeError(w, http.StatusNotFound, "not_found", "agent not installed on this repository")
		return
	}
	if err != nil {
		slog.Error("failed to uninstall agent", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to uninstall agent")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TriggerAgent manually triggers an agent on a repository.
// POST /api/v1/repos/{owner}/{repo}/agents/{slug}/trigger
func (h *AgentHandler) TriggerAgent(w http.ResponseWriter, r *http.Request) {
	repository := h.resolveOwnedRepo(w, r)
	if repository == nil {
		return
	}

	slug := chi.URLParam(r, "slug")

	ra, err := h.agents.GetRepoAgent(r.Context(), repository.ID, slug)
	if errors.Is(err, agent.ErrNotInstalled) {
		writeError(w, http.StatusNotFound, "not_found", "agent not installed on this repository")
		return
	}
	if err != nil {
		slog.Error("failed to get repo agent", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get agent")
		return
	}

	var req models.TriggerAgentRequest
	// Body is optional for trigger
	_ = decodeJSON(r, &req)

	task := &models.AgentTask{
		RepoID:       repository.ID,
		AgentID:      ra.AgentID,
		TriggerEvent: "manual",
		TriggerRef:   req.TriggerRef,
		Status:       "queued",
		Provider:     "pending", // Will be set by the queue consumer
		Result:       json.RawMessage(`{}`),
	}

	created, err := h.agents.CreateTask(r.Context(), task)
	if err != nil {
		slog.Error("failed to create agent task", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to trigger agent")
		return
	}

	writeJSON(w, http.StatusAccepted, created)
}

// ListDocuments returns agent-generated documents for a repository.
// GET /api/v1/repos/{owner}/{repo}/docs
func (h *AgentHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	repository := h.resolveRepo(w, r)
	if repository == nil {
		return
	}

	docs, err := h.agents.ListDocuments(r.Context(), repository.ID)
	if err != nil {
		slog.Error("failed to list documents", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list documents")
		return
	}

	writeJSON(w, http.StatusOK, docs)
}

// GetDocument returns a single agent-generated document.
// GET /api/v1/repos/{owner}/{repo}/docs/{id}
func (h *AgentHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	_ = h.resolveRepo(w, r)

	docIDStr := chi.URLParam(r, "id")
	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid document ID")
		return
	}

	doc, err := h.agents.GetDocument(r.Context(), docID)
	if errors.Is(err, agent.ErrDocNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "document not found")
		return
	}
	if err != nil {
		slog.Error("failed to get document", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get document")
		return
	}

	writeJSON(w, http.StatusOK, doc)
}

// ListTasks returns agent task history for a repository.
// GET /api/v1/repos/{owner}/{repo}/tasks
func (h *AgentHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	repository := h.resolveRepo(w, r)
	if repository == nil {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	tasks, err := h.agents.ListTasks(r.Context(), repository.ID, limit, offset)
	if err != nil {
		slog.Error("failed to list tasks", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list tasks")
		return
	}

	writeJSON(w, http.StatusOK, tasks)
}

// GetTask returns a single agent task.
// GET /api/v1/repos/{owner}/{repo}/tasks/{id}
func (h *AgentHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	_ = h.resolveRepo(w, r)

	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid task ID")
		return
	}

	task, err := h.agents.GetTask(r.Context(), taskID)
	if errors.Is(err, agent.ErrTaskNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "task not found")
		return
	}
	if err != nil {
		slog.Error("failed to get task", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get task")
		return
	}

	writeJSON(w, http.StatusOK, task)
}
