package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Issue struct {
	ID          uuid.UUID       `json:"id"`
	RepoID      uuid.UUID       `json:"repo_id"`
	Number      int             `json:"number"`
	AuthorID    uuid.UUID       `json:"author_id"`
	AuthorName  string          `json:"author_name"` // populated via join
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	Status      string          `json:"status"` // open, closed, duplicate
	Labels      []string        `json:"labels"`
	Assignees   []uuid.UUID     `json:"assignees"`
	MilestoneID *uuid.UUID      `json:"milestone_id,omitempty"`
	LinkedPRs   []uuid.UUID     `json:"linked_prs"`
	Priority    string          `json:"priority"` // critical, high, medium, low, none
	Metadata    json.RawMessage `json:"metadata"`
	ClosedAt    *time.Time      `json:"closed_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type CreateIssueRequest struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Labels   []string `json:"labels"`
	Priority string   `json:"priority"`
}

type UpdateIssueRequest struct {
	Title    *string  `json:"title,omitempty"`
	Body     *string  `json:"body,omitempty"`
	Status   *string  `json:"status,omitempty"`
	Labels   *[]string `json:"labels"`
	Priority *string  `json:"priority,omitempty"`
}
