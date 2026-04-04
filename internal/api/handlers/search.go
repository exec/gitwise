package handlers

import (
	"net/http"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/services/repo"
	"github.com/gitwise-io/gitwise/internal/services/search"
)

type SearchHandler struct {
	searchSvc *search.Service
	repoSvc   *repo.Service
}

func NewSearchHandler(searchSvc *search.Service, repoSvc *repo.Service) *SearchHandler {
	return &SearchHandler{searchSvc: searchSvc, repoSvc: repoSvc}
}

func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req search.SearchRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "query is required")
		return
	}

	resp, err := h.searchSvc.Search(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "search failed")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *SearchHandler) IndexRepo(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if req.Owner == "" || req.Repo == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "owner and repo are required")
		return
	}

	repository, err := h.repoSvc.GetByOwnerAndName(r.Context(), req.Owner, req.Repo, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return
	}

	err = h.searchSvc.IndexRepo(r.Context(), repository.ID, req.Owner, req.Repo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "indexing failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "indexed"})
}
