package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Agent represents an AI agent definition (built-in or custom).
type Agent struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description"`
	IsOfficial  bool            `json:"is_official"`
	AuthorID    *uuid.UUID      `json:"author_id,omitempty"`
	Config      json.RawMessage `json:"config"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type CreateAgentRequest struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`
}

type UpdateAgentRequest struct {
	Name        *string          `json:"name,omitempty"`
	Description *string          `json:"description,omitempty"`
	Config      *json.RawMessage `json:"config,omitempty"`
}

// RepoAgent represents an agent installed on a repository.
type RepoAgent struct {
	ID            uuid.UUID       `json:"id"`
	RepoID        uuid.UUID       `json:"repo_id"`
	AgentID       uuid.UUID       `json:"agent_id"`
	AgentName     string          `json:"agent_name"`
	AgentSlug     string          `json:"agent_slug"`
	Enabled       bool            `json:"enabled"`
	Config        json.RawMessage `json:"config"`
	Instructions  string          `json:"instructions"`
	TriggerEvents []string        `json:"trigger_events"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type InstallAgentRequest struct {
	AgentSlug     string          `json:"agent_slug"`
	Config        json.RawMessage `json:"config"`
	Instructions  string          `json:"instructions"`
	TriggerEvents []string        `json:"trigger_events"`
}

type UpdateRepoAgentRequest struct {
	Enabled       *bool            `json:"enabled,omitempty"`
	Config        *json.RawMessage `json:"config,omitempty"`
	Instructions  *string          `json:"instructions,omitempty"`
	TriggerEvents *[]string        `json:"trigger_events,omitempty"`
}

// AgentDocument represents an agent-generated document (living knowledge base).
type AgentDocument struct {
	ID        uuid.UUID       `json:"id"`
	RepoID    uuid.UUID       `json:"repo_id"`
	AgentID   uuid.UUID       `json:"agent_id"`
	Title     string          `json:"title"`
	Content   string          `json:"content"`
	DocType   string          `json:"doc_type"`
	Metadata  json.RawMessage `json:"metadata"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type UpsertAgentDocumentRequest struct {
	AgentID  uuid.UUID       `json:"agent_id"`
	Title    string          `json:"title"`
	Content  string          `json:"content"`
	DocType  string          `json:"doc_type"`
	Metadata json.RawMessage `json:"metadata"`
}

// AgentTask represents a logged agent execution.
type AgentTask struct {
	ID           uuid.UUID       `json:"id"`
	RepoID       uuid.UUID       `json:"repo_id"`
	AgentID      uuid.UUID       `json:"agent_id"`
	TriggerEvent string          `json:"trigger_event"`
	TriggerRef   string          `json:"trigger_ref"`
	Status       string          `json:"status"`
	Provider     string          `json:"provider"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
	DurationMs   int             `json:"duration_ms"`
	Result       json.RawMessage `json:"result"`
	Error        string          `json:"error"`
	CreatedAt    time.Time       `json:"created_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

// TriggerAgentRequest is used to manually trigger an agent on a repo.
type TriggerAgentRequest struct {
	TriggerRef string `json:"trigger_ref"`
}
