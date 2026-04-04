package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gitwise-io/gitwise/internal/models"
)

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

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// WriteUserJSON writes a user response (used by server.go for inline handlers).
func WriteUserJSON(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, data)
}

// WriteReposJSON writes a repos list response.
func WriteReposJSON(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, data)
}
