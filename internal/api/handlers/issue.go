package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/comment"
	"github.com/gitwise-io/gitwise/internal/services/issue"
	"github.com/gitwise-io/gitwise/internal/services/repo"
	"github.com/gitwise-io/gitwise/internal/services/webhook"
)

type IssueHandler struct {
	repos    *repo.Service
	issues   *issue.Service
	comments *comment.Service
	webhooks *webhook.Service
}

func NewIssueHandler(repos *repo.Service, issues *issue.Service, comments *comment.Service, webhooks *webhook.Service) *IssueHandler {
	return &IssueHandler{repos: repos, issues: issues, comments: comments, webhooks: webhooks}
}

func (h *IssueHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateIssueRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	iss, err := h.issues.Create(r.Context(), repository.ID, *userID, req)
	if errors.Is(err, issue.ErrInvalidTitle) {
		writeError(w, http.StatusBadRequest, "validation_error", "title is required (max 500 chars)")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create issue")
		return
	}

	go h.webhooks.Dispatch(r.Context(), repository.ID, "issue.opened", map[string]any{
		"issue":      map[string]any{"number": iss.Number, "title": iss.Title},
		"repository": repository.Name,
		"owner":      owner,
		"sender":     iss.AuthorName,
	})

	writeJSON(w, http.StatusCreated, iss)
}

func (h *IssueHandler) Get(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	viewerID := middleware.GetUserID(r.Context())

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, viewerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	number, err := strconv.Atoi(chi.URLParam(r, "number"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_number", "issue number must be an integer")
		return
	}

	iss, err := h.issues.GetByNumber(r.Context(), repository.ID, number)
	if errors.Is(err, issue.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "issue not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get issue")
		return
	}

	writeJSON(w, http.StatusOK, iss)
}

func (h *IssueHandler) List(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	viewerID := middleware.GetUserID(r.Context())

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, viewerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	status := r.URL.Query().Get("status")
	cursor := r.URL.Query().Get("cursor")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	issues, nextCursor, err := h.issues.List(r.Context(), repository.ID, status, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list issues")
		return
	}

	writeJSONMeta(w, http.StatusOK, issues, &models.ResponseMeta{NextCursor: nextCursor})
}

func (h *IssueHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	number, err := strconv.Atoi(chi.URLParam(r, "number"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_number", "issue number must be an integer")
		return
	}

	// Only repo owner or issue author can update
	iss, err := h.issues.GetByNumber(r.Context(), repository.ID, number)
	if errors.Is(err, issue.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "issue not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get issue")
		return
	}
	if iss.AuthorID != *userID && repository.OwnerID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to update this issue")
		return
	}

	var req models.UpdateIssueRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	oldStatus := iss.Status

	iss, err = h.issues.Update(r.Context(), repository.ID, number, req)
	if errors.Is(err, issue.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "issue not found")
		return
	}
	if errors.Is(err, issue.ErrInvalidTitle) || errors.Is(err, issue.ErrInvalidStatus) {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update issue")
		return
	}

	if req.Status != nil && *req.Status != oldStatus {
		eventType := "issue.closed"
		if *req.Status == "open" {
			eventType = "issue.opened"
		}
		go h.webhooks.Dispatch(r.Context(), repository.ID, eventType, map[string]any{
			"issue":      map[string]any{"number": iss.Number, "title": iss.Title},
			"repository": repository.Name,
			"owner":      owner,
			"sender":     iss.AuthorName,
		})
	}

	writeJSON(w, http.StatusOK, iss)
}

func (h *IssueHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
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

	number, err := strconv.Atoi(chi.URLParam(r, "number"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_number", "issue number must be an integer")
		return
	}

	iss, err := h.issues.GetByNumber(r.Context(), repository.ID, number)
	if errors.Is(err, issue.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "issue not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get issue")
		return
	}

	var req models.CreateCommentRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	c, err := h.comments.Create(r.Context(), repository.ID, &iss.ID, nil, *userID, req)
	if errors.Is(err, comment.ErrEmptyBody) {
		writeError(w, http.StatusBadRequest, "validation_error", "comment body is required")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create comment")
		return
	}

	go h.webhooks.Dispatch(r.Context(), repository.ID, "comment.created", map[string]any{
		"comment":    map[string]any{"body": c.Body},
		"issue":      map[string]any{"number": iss.Number, "title": iss.Title},
		"repository": repository.Name,
		"owner":      owner,
		"sender":     c.AuthorName,
	})

	writeJSON(w, http.StatusCreated, c)
}

func (h *IssueHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	viewerID := middleware.GetUserID(r.Context())

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, viewerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	number, err := strconv.Atoi(chi.URLParam(r, "number"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_number", "issue number must be an integer")
		return
	}

	iss, err := h.issues.GetByNumber(r.Context(), repository.ID, number)
	if errors.Is(err, issue.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "issue not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get issue")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	comments, nextCursor, err := h.comments.ListByIssue(r.Context(), iss.ID, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list comments")
		return
	}

	writeJSONMeta(w, http.StatusOK, comments, &models.ResponseMeta{NextCursor: nextCursor})
}
