package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/chat"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

type ChatHandler struct {
	repos   *repo.Service
	chat    *chat.Service
	context *chat.ContextBuilder
}

func NewChatHandler(repos *repo.Service, chatSvc *chat.Service, ctxBuilder *chat.ContextBuilder) *ChatHandler {
	return &ChatHandler{repos: repos, chat: chatSvc, context: ctxBuilder}
}

// resolveRepoForChat resolves the repo from URL params for chat endpoints.
func (h *ChatHandler) resolveRepoForChat(w http.ResponseWriter, r *http.Request) *models.Repository {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	viewerID := middleware.GetUserID(r.Context())

	repository, err := h.repos.GetByOwnerAndName(r.Context(), owner, repoName, viewerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return nil
	}
	return repository
}

// CreateConversation starts a new chat conversation.
// POST /api/v1/repos/{owner}/{repo}/chat
func (h *ChatHandler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	repository := h.resolveRepoForChat(w, r)
	if repository == nil {
		return
	}

	var req models.CreateConversationRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	conv, err := h.chat.CreateConversation(r.Context(), repository.ID, *userID, req.Title)
	if err != nil {
		slog.Error("failed to create conversation", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create conversation")
		return
	}

	writeJSON(w, http.StatusCreated, conv)
}

// ListConversations returns all conversations for the current user in a repo.
// GET /api/v1/repos/{owner}/{repo}/chat
func (h *ChatHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	repository := h.resolveRepoForChat(w, r)
	if repository == nil {
		return
	}

	convs, err := h.chat.ListConversations(r.Context(), repository.ID, *userID)
	if err != nil {
		slog.Error("failed to list conversations", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list conversations")
		return
	}

	writeJSON(w, http.StatusOK, convs)
}

// GetConversation returns a conversation with its messages.
// GET /api/v1/repos/{owner}/{repo}/chat/{id}
func (h *ChatHandler) GetConversation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	_ = h.resolveRepoForChat(w, r)

	convIDStr := chi.URLParam(r, "id")
	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid conversation ID")
		return
	}

	conv, err := h.chat.GetConversation(r.Context(), convID)
	if errors.Is(err, chat.ErrConversationNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	if err != nil {
		slog.Error("failed to get conversation", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get conversation")
		return
	}

	// Only the conversation owner can view it
	if conv.UserID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you can only view your own conversations")
		return
	}

	// Also fetch messages
	messages, err := h.chat.ListMessages(r.Context(), convID)
	if err != nil {
		slog.Error("failed to list messages", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get messages")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"conversation": conv,
		"messages":     messages,
	})
}

// SendMessage sends a message in a conversation and gets an AI response.
// POST /api/v1/repos/{owner}/{repo}/chat/{id}/messages
func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	repository := h.resolveRepoForChat(w, r)
	if repository == nil {
		return
	}

	convIDStr := chi.URLParam(r, "id")
	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid conversation ID")
		return
	}

	// Verify conversation exists and belongs to user
	conv, err := h.chat.GetConversation(r.Context(), convID)
	if errors.Is(err, chat.ErrConversationNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	if err != nil {
		slog.Error("failed to get conversation", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get conversation")
		return
	}
	if conv.UserID != *userID {
		writeError(w, http.StatusForbidden, "forbidden", "you can only send messages in your own conversations")
		return
	}

	var req models.SendMessageRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "message content is required")
		return
	}

	// 1. Save user message
	userMsg, err := h.chat.AddMessage(r.Context(), convID, "user", req.Content)
	if err != nil {
		slog.Error("failed to save user message", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to save message")
		return
	}

	// 2. Build context
	contextStr, err := h.context.BuildChatContext(r.Context(), repository.ID, req.Content, 4000)
	if err != nil {
		slog.Warn("failed to build chat context, proceeding without context", "error", err)
		contextStr = ""
	}

	// 3. Get conversation history for LLM
	messages, err := h.chat.ListMessages(r.Context(), convID)
	if err != nil {
		slog.Error("failed to list messages", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get conversation history")
		return
	}

	// 4. Try to generate LLM response
	var assistantContent string
	if h.chat.HasLLM() {
		systemPrompt := "You are the Gitwise AI assistant for a code repository. Help the user understand their codebase, answer questions about code, and assist with development tasks."
		if contextStr != "" {
			systemPrompt += "\n\nHere is relevant context about the repository:\n\n" + contextStr
		}

		// Convert conversation history to LLM messages
		var llmMsgs []models.LLMMessage
		for _, m := range messages {
			llmMsgs = append(llmMsgs, models.LLMMessage{
				Role:    m.Role,
				Content: m.Content,
			})
		}

		assistantContent, err = h.chat.GenerateResponse(r.Context(), systemPrompt, llmMsgs, 4096)
		if err != nil {
			slog.Error("LLM generation failed", "error", err)
			assistantContent = "I'm sorry, I was unable to generate a response. The AI provider may be unavailable. Please try again later."
		}
	} else {
		assistantContent = "The AI assistant is not configured. Please configure an LLM provider in your Gitwise settings to enable chat."
	}

	// 5. Save assistant message
	assistantMsg, err := h.chat.AddMessage(r.Context(), convID, "assistant", assistantContent)
	if err != nil {
		slog.Error("failed to save assistant message", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to save response")
		return
	}

	// 6. Return both messages
	writeJSON(w, http.StatusOK, &models.SendMessageResponse{
		UserMessage:      userMsg,
		AssistantMessage: assistantMsg,
	})
}

// DeleteConversation deletes a conversation.
// DELETE /api/v1/repos/{owner}/{repo}/chat/{id}
func (h *ChatHandler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	convIDStr := chi.URLParam(r, "id")
	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid conversation ID")
		return
	}

	err = h.chat.DeleteConversation(r.Context(), convID, *userID)
	if errors.Is(err, chat.ErrConversationNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	if errors.Is(err, chat.ErrForbidden) {
		writeError(w, http.StatusForbidden, "forbidden", "you can only delete your own conversations")
		return
	}
	if err != nil {
		slog.Error("failed to delete conversation", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete conversation")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
