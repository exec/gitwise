package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/mention"
	"github.com/gitwise-io/gitwise/internal/services/notification"
	"github.com/gitwise-io/gitwise/internal/services/user"
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

// parseLimit parses an integer "limit" query parameter, clamping it to [defaultV, max].
// A missing, zero, or negative value returns defaultV; a value above max returns max.
func parseLimit(r *http.Request, defaultV, max int) int {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return defaultV
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return defaultV
	}
	if v > max {
		return max
	}
	return v
}

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

// decodeJSONObject decodes a JSON object body, rejecting non-object top-level values
// (arrays, strings, numbers, booleans). Uses the same size limit as decodeJSON.
func decodeJSONObject(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodySize)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	// Peek at first token to ensure it's an object.
	tok, err := dec.Token()
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return ErrBodyTooLarge
		}
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("request body must be a JSON object")
	}

	// Decode the rest of the object into v using a fresh decoder that sees the whole body.
	// We can't rewind, so use the existing decoder which has consumed the '{' — rebuild v
	// by wrapping in a map and decoding manually is complex; instead unmarshal via buffered approach.
	// Simpler: use a raw message to decode the complete object via standard Unmarshal.
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return ErrBodyTooLarge
		}
		return err
	}
	// Reconstruct the full object for final decode.
	fullObj := append([]byte{'{'}, raw...)
	return json.Unmarshal(fullObj, v)
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

// processMentions parses @mentions from text and creates a notification for
// each mentioned user, skipping the author. entityType is "issue" or "pull request".
func processMentions(ctx context.Context, text string, authorID uuid.UUID, owner, repoName, entityType, entityPath string, number int, authorName string, users *user.Service, notifications *notification.Service) {
	usernames := mention.Parse(text)
	if len(usernames) == 0 {
		return
	}

	link := fmt.Sprintf("/%s/%s/%s/%d", owner, repoName, entityPath, number)
	title := fmt.Sprintf("%s mentioned you in %s #%d", authorName, entityType, number)

	// Truncate body for notification preview
	body := text
	if len(body) > 500 {
		body = body[:500] + "..."
	}

	for _, username := range usernames {
		mentioned, err := users.GetByUsername(ctx, username)
		if err != nil {
			continue
		}
		if mentioned.ID == authorID {
			continue
		}

		if _, err := notifications.Create(ctx, mentioned.ID, "mention", title, body, link); err != nil {
			slog.Error("failed to create mention notification",
				"mentioned_user", username,
				"author", authorName,
				"link", link,
				"error", err,
			)
		}
	}
}
