package models

import "github.com/google/uuid"

type Label struct {
	ID          uuid.UUID `json:"id"`
	RepoID      uuid.UUID `json:"repo_id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"` // hex color e.g. #ff0000
	Description string    `json:"description"`
}

type CreateLabelRequest struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type UpdateLabelRequest struct {
	Name        *string `json:"name,omitempty"`
	Color       *string `json:"color,omitempty"`
	Description *string `json:"description,omitempty"`
}
