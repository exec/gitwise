package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gitwise-io/gitwise/internal/models"
)

// ErrBodyTooLarge is returned when the request body exceeds maxBodySize.
var ErrBodyTooLarge = errors.New("request body too large")

func writeJSON(w http.ResponseWriter, status int, data any) {
	resp := models.APIResponse{Data: data}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func writeJSONMeta(w http.ResponseWriter, status int, data any, meta *models.ResponseMeta) {
	resp := models.APIResponse{Data: data, Meta: meta}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	resp := models.APIResponse{
		Errors: []models.APIError{{Code: code, Message: message}},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func writeFieldError(w http.ResponseWriter, status int, code, message, field string) {
	resp := models.APIResponse{
		Errors: []models.APIError{{Code: code, Message: message, Field: field}},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

const maxBodySize = 1 << 20 // 1 MB

func decodeJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodySize)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err != nil {
		// MaxBytesReader returns *http.MaxBytesError when limit is exceeded
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return ErrBodyTooLarge
		}
	}
	return err
}

// handleDecodeError writes the appropriate error for a decodeJSON failure.
// Returns true if an error was written (caller should return).
func handleDecodeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrBodyTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body too large")
	} else {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid request body")
	}
	return true
}

// WriteUserJSON writes a user response (used by server.go for inline handlers).
func WriteUserJSON(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, data)
}

// WriteReposJSON writes a repos list response.
func WriteReposJSON(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, data)
}
