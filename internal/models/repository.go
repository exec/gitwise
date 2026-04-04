package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Repository struct {
	ID            uuid.UUID       `json:"id"`
	OwnerID       uuid.UUID       `json:"owner_id"`
	OwnerName     string          `json:"owner_name"` // populated via join
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	DefaultBranch string          `json:"default_branch"`
	Visibility    string          `json:"visibility"`
	LanguageStats json.RawMessage `json:"language_stats"`
	Topics        []string        `json:"topics"`
	StarsCount    int             `json:"stars_count"`
	ForksCount    int             `json:"forks_count"`
	CloneURL      string          `json:"clone_url,omitempty"` // computed
	SSHURL        string          `json:"ssh_url,omitempty"`   // computed
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type CreateRepoRequest struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Visibility    string   `json:"visibility"`
	DefaultBranch string   `json:"default_branch"`
	Topics        []string `json:"topics"`
	AutoInit      bool     `json:"auto_init"`
}

type UpdateRepoRequest struct {
	Description   *string  `json:"description,omitempty"`
	Visibility    *string  `json:"visibility,omitempty"`
	DefaultBranch *string  `json:"default_branch,omitempty"`
	Topics        []string `json:"topics,omitempty"`
}

type ListReposParams struct {
	OwnerID    uuid.UUID
	Visibility string
	Sort       string // name, created, updated, stars
	Direction  string // asc, desc
	Cursor     string
	Limit      int
}
