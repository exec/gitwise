package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/mirror"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

type MirrorHandler struct {
	repos   *repo.Service
	mirrors *mirror.Service
}

func NewMirrorHandler(repos *repo.Service, mirrors *mirror.Service) *MirrorHandler {
	return &MirrorHandler{repos: repos, mirrors: mirrors}
}

func (h *MirrorHandler) Get(w http.ResponseWriter, r *http.Request) {
	repository := lookupOwnedRepo(h.repos, w, r)
	if repository == nil {
		return
	}
	m, err := h.mirrors.Get(r.Context(), repository.ID)
	if errors.Is(err, mirror.ErrMirrorNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"mirror": nil})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mirror_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mirror": m})
}

func (h *MirrorHandler) Configure(w http.ResponseWriter, r *http.Request) {
	repository := lookupOwnedRepo(h.repos, w, r)
	if repository == nil {
		return
	}
	var req models.ConfigureMirrorRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}
	m, err := h.mirrors.Configure(r.Context(), repository.ID, req)
	switch {
	case errors.Is(err, mirror.ErrInvalidDirection),
		errors.Is(err, mirror.ErrInvalidTarget),
		errors.Is(err, mirror.ErrInvalidInterval),
		errors.Is(err, mirror.ErrPATRequired):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "mirror_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mirror": m})
}

func (h *MirrorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	repository := lookupOwnedRepo(h.repos, w, r)
	if repository == nil {
		return
	}
	if err := h.mirrors.Remove(r.Context(), repository.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "mirror_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *MirrorHandler) SyncNow(w http.ResponseWriter, r *http.Request) {
	repository := lookupOwnedRepo(h.repos, w, r)
	if repository == nil {
		return
	}
	// Run async — caller gets 202 and reads status via GET.
	repoID := repository.ID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if _, err := h.mirrors.SyncNow(ctx, repoID, models.MirrorTriggerManual); err != nil {
			// logged / persisted inside the service
			_ = err
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

func (h *MirrorHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	repository := lookupOwnedRepo(h.repos, w, r)
	if repository == nil {
		return
	}
	runs, err := h.mirrors.ListRuns(r.Context(), repository.ID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mirror_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}
