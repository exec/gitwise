package models

import (
	"time"

	"github.com/google/uuid"
)

type Comment struct {
	ID         uuid.UUID  `json:"id"`
	RepoID     uuid.UUID  `json:"repo_id"`
	IssueID    *uuid.UUID `json:"issue_id,omitempty"`
	PRID       *uuid.UUID `json:"pr_id,omitempty"`
	AuthorID   uuid.UUID  `json:"author_id"`
	AuthorName string     `json:"author_name"` // populated via join
	Body       string     `json:"body"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CreateCommentRequest struct {
	Body string `json:"body"`
}

type UpdateCommentRequest struct {
	Body string `json:"body"`
}
