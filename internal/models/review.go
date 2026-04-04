package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Review struct {
	ID          uuid.UUID       `json:"id"`
	PRID        uuid.UUID       `json:"pr_id"`
	AuthorID    uuid.UUID       `json:"author_id"`
	AuthorName  string          `json:"author_name"` // populated via join
	Type        string          `json:"type"`         // approval, changes_requested, comment, dismissal
	Body        string          `json:"body"`
	Comments    json.RawMessage `json:"comments"` // inline comments JSON
	SubmittedAt time.Time       `json:"submitted_at"`
}

type CreateReviewRequest struct {
	Type     string              `json:"type"` // approval, changes_requested, comment
	Body     string              `json:"body"`
	Comments []InlineCommentInput `json:"comments"`
}

type InlineCommentInput struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side"`      // left, right
	Body     string `json:"body"`
	ThreadID string `json:"thread_id"` // groups comments into threads
	Resolved bool   `json:"resolved"`  // whether this comment resolves the thread
}

type ResolveThreadRequest struct {
	Resolved bool `json:"resolved"`
}
