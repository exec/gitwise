package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ChatConversation represents a chat session between a user and the AI assistant.
type ChatConversation struct {
	ID        uuid.UUID `json:"id"`
	RepoID    uuid.UUID `json:"repo_id"`
	UserID    uuid.UUID `json:"user_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateConversationRequest struct {
	Title string `json:"title"`
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	ID             uuid.UUID       `json:"id"`
	ConversationID uuid.UUID       `json:"conversation_id"`
	Role           string          `json:"role"` // user, assistant
	Content        string          `json:"content"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
}

type SendMessageRequest struct {
	Content string `json:"content"`
}

// SendMessageResponse returns both the user and assistant messages.
type SendMessageResponse struct {
	UserMessage      *ChatMessage `json:"user_message"`
	AssistantMessage *ChatMessage `json:"assistant_message"`
}

// LLMMessage is a message in the format expected by LLM providers.
type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
