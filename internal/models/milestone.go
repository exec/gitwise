package models

import (
	"time"

	"github.com/google/uuid"
)

type Milestone struct {
	ID          uuid.UUID  `json:"id"`
	RepoID      uuid.UUID  `json:"repo_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
}

type CreateMilestoneRequest struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueDate     *time.Time `json:"due_date,omitempty"`
}

type UpdateMilestoneRequest struct {
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Status      *string    `json:"status,omitempty"`
}
