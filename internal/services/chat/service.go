package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrConversationNotFound = errors.New("conversation not found")
	ErrForbidden            = errors.New("access denied")
	ErrEmptyMessage         = errors.New("message content is required")
	ErrInvalidRole          = errors.New("invalid message role")
)

// ChatStreamChunk is a single piece of a streaming LLM response.
type ChatStreamChunk struct {
	Content string
	Done    bool
}

// LLMGenerator is the interface for generating LLM responses.
// The implementation will be provided by the LLM Gateway (built by another agent).
type LLMGenerator interface {
	Generate(ctx context.Context, systemPrompt string, messages []models.LLMMessage, maxTokens int) (string, error)
	GenerateStream(ctx context.Context, systemPrompt string, messages []models.LLMMessage, maxTokens int) (<-chan ChatStreamChunk, error)
}

type Service struct {
	db  *pgxpool.Pool
	llm LLMGenerator
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

// SetLLMGenerator wires the LLM generator into the chat service.
func (s *Service) SetLLMGenerator(llm LLMGenerator) {
	s.llm = llm
}

// CreateConversation starts a new chat conversation for a user in a repo.
func (s *Service) CreateConversation(ctx context.Context, repoID, userID uuid.UUID, title string) (*models.ChatConversation, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New conversation"
	}
	if len(title) > 500 {
		title = title[:500]
	}

	conv := &models.ChatConversation{
		ID:        uuid.New(),
		RepoID:    repoID,
		UserID:    userID,
		Title:     title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO chat_conversations (id, repo_id, user_id, title, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		conv.ID, conv.RepoID, conv.UserID, conv.Title, conv.CreatedAt, conv.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert conversation: %w", err)
	}

	return conv, nil
}

// ListConversations returns all conversations for a user in a repo.
func (s *Service) ListConversations(ctx context.Context, repoID, userID uuid.UUID) ([]models.ChatConversation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, user_id, title, created_at, updated_at
		FROM chat_conversations
		WHERE repo_id = $1 AND user_id = $2
		ORDER BY updated_at DESC`, repoID, userID)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()

	var convs []models.ChatConversation
	for rows.Next() {
		var c models.ChatConversation
		if err := rows.Scan(&c.ID, &c.RepoID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		convs = append(convs, c)
	}
	if convs == nil {
		convs = []models.ChatConversation{}
	}
	return convs, rows.Err()
}

// GetConversation returns a conversation by ID.
func (s *Service) GetConversation(ctx context.Context, convID uuid.UUID) (*models.ChatConversation, error) {
	var c models.ChatConversation
	err := s.db.QueryRow(ctx, `
		SELECT id, repo_id, user_id, title, created_at, updated_at
		FROM chat_conversations
		WHERE id = $1`, convID).Scan(&c.ID, &c.RepoID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConversationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query conversation: %w", err)
	}
	return &c, nil
}

// DeleteConversation deletes a conversation (only the owner can delete).
func (s *Service) DeleteConversation(ctx context.Context, convID, userID uuid.UUID) error {
	conv, err := s.GetConversation(ctx, convID)
	if err != nil {
		return err
	}
	if conv.UserID != userID {
		return ErrForbidden
	}

	_, err = s.db.Exec(ctx, `DELETE FROM chat_conversations WHERE id = $1`, convID)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}

// AddMessage adds a message to a conversation.
func (s *Service) AddMessage(ctx context.Context, convID uuid.UUID, role, content string) (*models.ChatMessage, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrEmptyMessage
	}
	if role != "user" && role != "assistant" {
		return nil, ErrInvalidRole
	}

	msg := &models.ChatMessage{
		ID:             uuid.New(),
		ConversationID: convID,
		Role:           role,
		Content:        content,
		Metadata:       json.RawMessage(`{}`),
		CreatedAt:      time.Now(),
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO chat_messages (id, conversation_id, role, content, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		msg.ID, msg.ConversationID, msg.Role, msg.Content, msg.Metadata, msg.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}

	// Update conversation's updated_at
	_, err = s.db.Exec(ctx, `
		UPDATE chat_conversations SET updated_at = $1 WHERE id = $2`,
		msg.CreatedAt, convID)
	if err != nil {
		// Non-fatal: log but don't fail
		fmt.Printf("failed to update conversation timestamp: %v\n", err)
	}

	return msg, nil
}

// ListMessages returns all messages in a conversation, ordered by creation time.
func (s *Service) ListMessages(ctx context.Context, convID uuid.UUID) ([]models.ChatMessage, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, conversation_id, role, content, metadata, created_at
		FROM chat_messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC`, convID)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var msgs []models.ChatMessage
	for rows.Next() {
		var m models.ChatMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Metadata, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []models.ChatMessage{}
	}
	return msgs, rows.Err()
}

// HasLLM returns true if an LLM generator is configured.
func (s *Service) HasLLM() bool {
	return s.llm != nil
}

// GenerateResponse uses the LLM generator to produce an assistant response.
// Returns the generated text, or an error if no LLM is configured.
func (s *Service) GenerateResponse(ctx context.Context, systemPrompt string, messages []models.LLMMessage, maxTokens int) (string, error) {
	if s.llm == nil {
		return "", fmt.Errorf("no LLM provider configured")
	}
	return s.llm.Generate(ctx, systemPrompt, messages, maxTokens)
}

// GenerateStreamResponse uses the LLM generator to produce a streaming assistant response.
// Returns a channel of ChatStreamChunk, or an error if no LLM is configured.
func (s *Service) GenerateStreamResponse(ctx context.Context, systemPrompt string, messages []models.LLMMessage, maxTokens int) (<-chan ChatStreamChunk, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("no LLM provider configured")
	}
	return s.llm.GenerateStream(ctx, systemPrompt, messages, maxTokens)
}
