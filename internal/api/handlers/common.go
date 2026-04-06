package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

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
