package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/comment"
	"github.com/gitwise-io/gitwise/internal/services/pull"
	"github.com/gitwise-io/gitwise/internal/services/repo"
	"github.com/gitwise-io/gitwise/internal/services/review"
)

type PullHandler struct {
	repos    *repo.Service
	pulls    *pull.Service
	reviews  *review.Service
	comments *comment.Service
}

func NewPullHandler(repos *repo.Service, pulls *pull.Service, reviews *review.Service, comments *comment.Service) *PullHandler {
	return &PullHandler{repos: repos, pulls: pulls, reviews: reviews, comments: comments}
}

func (h *PullHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreatePullRequestRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	pr, err := h.pulls.Create(r.Context(), repository.ID, *userID, owner, repoName, req)
	if errors.Is(err, pull.ErrInvalidTitle) {
		writeError(w, http.StatusBadRequest, "validation_error", "title is required (max 500 chars)")
		return
	}
	if errors.Is(err, pull.ErrInvalidBranch) || errors.Is(err, pull.ErrSameBranch) {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create pull request")
		return
	}

	writeJSON(w, http.StatusCreated, pr)
}

func (h *PullHandler) Get(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_number", "PR number must be an integer")
		return
	}

	pr, err := h.pulls.GetByNumber(r.Context(), repository.ID, number)
	if errors.Is(err, pull.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "pull request not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get pull request")
		return
	}

	writeJSON(w, http.StatusOK, pr)
}

func (h *PullHandler) List(w http.ResponseWriter, r *http.Request) {
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

	prs, nextCursor, err := h.pulls.List(r.Context(), repository.ID, status, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list pull requests")
		return
	}

	writeJSONMeta(w, http.StatusOK, prs, &models.ResponseMeta{NextCursor: nextCursor})
}

func (h *PullHandler) Update(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_number", "PR number must be an integer")
		return
	}

	// Only repo owner or PR author can update
	existingPR, err := h.pulls.GetByNumber(r.Context(), repository.ID, number)
	if errors.Is(err, pull.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "pull request not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get pull request")
		return
	}
	if existingPR.AuthorID != *userID && repository.OwnerID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to update this pull request")
		return
	}

	var req models.UpdatePullRequestRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	pr, err := h.pulls.Update(r.Context(), repository.ID, number, req)
	if errors.Is(err, pull.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "pull request not found")
		return
	}
	if errors.Is(err, pull.ErrInvalidStatus) || errors.Is(err, pull.ErrInvalidBranch) || errors.Is(err, pull.ErrInvalidTitle) {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update pull request")
		return
	}

	writeJSON(w, http.StatusOK, pr)
}

func (h *PullHandler) Merge(w http.ResponseWriter, r *http.Request) {
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

	// Only repo owner can merge (RBAC would expand this in the future)
	if repository.OwnerID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "only the repository owner can merge pull requests")
		return
	}

	number, err := strconv.Atoi(chi.URLParam(r, "number"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_number", "PR number must be an integer")
		return
	}

	var req models.MergePullRequestRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	pr, err := h.pulls.Merge(r.Context(), repository.ID, number, *userID, owner, repoName, req)
	if errors.Is(err, pull.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "pull request not found")
		return
	}
	if errors.Is(err, pull.ErrAlreadyMerged) {
		writeError(w, http.StatusConflict, "already_merged", "pull request is already merged")
		return
	}
	if errors.Is(err, pull.ErrNotOpen) {
		writeError(w, http.StatusConflict, "not_open", "pull request is not open")
		return
	}
	if errors.Is(err, pull.ErrMergeFailed) {
		writeError(w, http.StatusConflict, "merge_failed", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to merge pull request")
		return
	}

	writeJSON(w, http.StatusOK, pr)
}

func (h *PullHandler) GetDiff(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_number", "PR number must be an integer")
		return
	}

	diff, err := h.pulls.GetDiff(r.Context(), repository.ID, number, owner, repoName)
	if errors.Is(err, pull.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "pull request not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get diff")
		return
	}

	writeJSON(w, http.StatusOK, diff)
}

func (h *PullHandler) CreateReview(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_number", "PR number must be an integer")
		return
	}

	pr, err := h.pulls.GetByNumber(r.Context(), repository.ID, number)
	if errors.Is(err, pull.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "pull request not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get pull request")
		return
	}

	var req models.CreateReviewRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	rev, err := h.reviews.Create(r.Context(), pr.ID, *userID, req)
	if errors.Is(err, review.ErrInvalidType) {
		writeError(w, http.StatusBadRequest, "validation_error", "type must be approval, changes_requested, or comment")
		return
	}
	if errors.Is(err, review.ErrSelfReview) {
		writeError(w, http.StatusForbidden, "forbidden", "cannot review your own pull request")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create review")
		return
	}

	writeJSON(w, http.StatusCreated, rev)
}

func (h *PullHandler) ListReviews(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_number", "PR number must be an integer")
		return
	}

	pr, err := h.pulls.GetByNumber(r.Context(), repository.ID, number)
	if errors.Is(err, pull.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "pull request not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get pull request")
		return
	}

	reviews, err := h.reviews.ListByPR(r.Context(), pr.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list reviews")
		return
	}

	writeJSON(w, http.StatusOK, reviews)
}

func (h *PullHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_number", "PR number must be an integer")
		return
	}

	pr, err := h.pulls.GetByNumber(r.Context(), repository.ID, number)
	if errors.Is(err, pull.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "pull request not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get pull request")
		return
	}

	var req models.CreateCommentRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	c, err := h.comments.Create(r.Context(), repository.ID, nil, &pr.ID, *userID, req)
	if errors.Is(err, comment.ErrEmptyBody) {
		writeError(w, http.StatusBadRequest, "validation_error", "comment body is required")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create comment")
		return
	}

	writeJSON(w, http.StatusCreated, c)
}

func (h *PullHandler) ListComments(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_number", "PR number must be an integer")
		return
	}

	pr, err := h.pulls.GetByNumber(r.Context(), repository.ID, number)
	if errors.Is(err, pull.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "pull request not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get pull request")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	comments, nextCursor, err := h.comments.ListByPR(r.Context(), pr.ID, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list comments")
		return
	}

	writeJSONMeta(w, http.StatusOK, comments, &models.ResponseMeta{NextCursor: nextCursor})
}
