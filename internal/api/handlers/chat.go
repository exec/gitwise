package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/git"
	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/chat"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

type ChatHandler struct {
	repos   *repo.Service
	chat    *chat.Service
	context *chat.ContextBuilder
	gitSvc  *git.Service
}

func NewChatHandler(repos *repo.Service, chatSvc *chat.Service, ctxBuilder *chat.ContextBuilder, gitSvc *git.Service) *ChatHandler {
	return &ChatHandler{repos: repos, chat: chatSvc, context: ctxBuilder, gitSvc: gitSvc}
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

// sseWrite writes a single SSE event to the response writer and flushes.
func sseWrite(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		slog.Error("failed to marshal SSE data", "error", err)
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonBytes)
	flusher.Flush()
}

// SendMessage sends a message in a conversation and streams the AI response via SSE.
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

	// 2. Set SSE headers — from this point on we cannot use writeError
	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.Error("response writer does not support flushing")
		writeError(w, http.StatusInternalServerError, "server_error", "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// 3. Send user message event
	sseWrite(w, flusher, "user_message", userMsg)

	// 4. Build context
	contextStr, err := h.context.BuildChatContext(r.Context(), repository.ID, req.Content, 4000)
	if err != nil {
		slog.Warn("failed to build chat context, proceeding without context", "error", err)
		contextStr = ""
	}

	// 5. Get conversation history for LLM
	messages, err := h.chat.ListMessages(r.Context(), convID)
	if err != nil {
		slog.Error("failed to list messages", "error", err)
		sseWrite(w, flusher, "error", map[string]string{"message": "failed to get conversation history"})
		sseWrite(w, flusher, "done", map[string]string{})
		return
	}

	// 6. Generate LLM response with streaming
	var assistantContent string
	if h.chat.HasLLM() {
		systemPrompt := fmt.Sprintf(`You are the Gitwise AI assistant for the repository %s/%s. You help users understand their codebase, answer questions about code, and assist with development tasks.

You have access to the repository's file tree and can read file contents. To read a file, include this tag in your response:
<read_file>path/to/file</read_file>

You can request multiple files at once. After you request files, you'll receive their contents and can then provide your answer.

When referencing code, always mention the file path. Be concise and helpful.`, repository.OwnerName, repository.Name)
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

		// Stream the first iteration
		assistantContent, err = h.streamLLMResponse(w, flusher, r.Context(), systemPrompt, llmMsgs, 4096)
		if err != nil {
			slog.Error("LLM streaming failed", "error", err)
			assistantContent = "I'm sorry, I was unable to generate a response. The AI provider may be unavailable. Please try again later."
			sseWrite(w, flusher, "chunk", map[string]string{"content": assistantContent})
		}

		// Agentic loop: resolve <read_file> tool calls
		const maxToolIterations = 5
		for i := 0; i < maxToolIterations; i++ {
			files := parseReadFileRequests(assistantContent)
			if len(files) == 0 {
				break
			}

			slog.Debug("chat tool call: read_file", "files", files, "iteration", i+1)

			// Notify frontend about tool call with filenames, and signal to replace streamed content
			sseWrite(w, flusher, "tool_call", map[string]any{"files": files})
			// Tell frontend to clear the partial response that contained <read_file> tags
			sseWrite(w, flusher, "clear_stream", map[string]string{})

			var fileContents strings.Builder
			for _, path := range files {
				content, readErr := readRepoFile(h.gitSvc, repository.OwnerName, repository.Name, repository.DefaultBranch, path)
				if readErr != nil {
					fmt.Fprintf(&fileContents, "File %s: error reading - %s\n\n", path, readErr)
				} else {
					fmt.Fprintf(&fileContents, "File %s:\n```\n%s\n```\n\n", path, content)
				}
			}

			llmMsgs = append(llmMsgs,
				models.LLMMessage{Role: "assistant", Content: assistantContent},
				models.LLMMessage{Role: "user", Content: "Here are the requested file contents:\n\n" + fileContents.String()},
			)

			// Stream the next iteration
			assistantContent, err = h.streamLLMResponse(w, flusher, r.Context(), systemPrompt, llmMsgs, 4096)
			if err != nil {
				slog.Error("LLM streaming failed during tool loop", "error", err, "iteration", i+1)
				break
			}
		}

		// Strip any remaining <read_file> tags from the final response
		assistantContent = stripReadFileTags(assistantContent)
	} else {
		assistantContent = "The AI assistant is not configured. Please configure an LLM provider in your Gitwise settings to enable chat."
		sseWrite(w, flusher, "chunk", map[string]string{"content": assistantContent})
	}

	// 7. Save assistant message
	assistantMsg, err := h.chat.AddMessage(r.Context(), convID, "assistant", assistantContent)
	if err != nil {
		slog.Error("failed to save assistant message", "error", err)
		sseWrite(w, flusher, "error", map[string]string{"message": "failed to save response"})
		sseWrite(w, flusher, "done", map[string]string{})
		return
	}

	// 8. Send the final complete assistant message and done event
	sseWrite(w, flusher, "assistant_message", assistantMsg)
	sseWrite(w, flusher, "done", map[string]string{})
}

// streamLLMResponse streams one LLM generation to the client via SSE chunks,
// accumulating and returning the full response text.
func (h *ChatHandler) streamLLMResponse(w http.ResponseWriter, flusher http.Flusher, ctx context.Context, systemPrompt string, llmMsgs []models.LLMMessage, maxTokens int) (string, error) {
	ch, err := h.chat.GenerateStreamResponse(ctx, systemPrompt, llmMsgs, maxTokens)
	if err != nil {
		return "", err
	}

	var accumulated strings.Builder
	for chunk := range ch {
		if chunk.Content != "" {
			accumulated.WriteString(chunk.Content)
			sseWrite(w, flusher, "chunk", map[string]string{"content": chunk.Content})
		}
	}
	return accumulated.String(), nil
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

// readFileRe matches <read_file>path</read_file> tags in LLM responses.
var readFileRe = regexp.MustCompile(`<read_file>(.*?)</read_file>`)

// parseReadFileRequests extracts file paths from <read_file> tags in the response.
func parseReadFileRequests(content string) []string {
	matches := readFileRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	var paths []string
	for _, m := range matches {
		p := strings.TrimSpace(m[1])
		if p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// stripReadFileTags removes any <read_file>...</read_file> tags from the content.
func stripReadFileTags(content string) string {
	return readFileRe.ReplaceAllString(content, "")
}

// readRepoFile reads a file from the repository via the git service with a size cap.
func readRepoFile(gitSvc *git.Service, owner, repoName, ref, path string) (string, error) {
	if gitSvc == nil {
		return "", fmt.Errorf("git service not available")
	}
	blob, err := gitSvc.GetBlob(owner, repoName, ref, path)
	if err != nil {
		return "", err
	}
	if blob.IsBinary {
		return "", fmt.Errorf("binary file")
	}
	content := blob.Content
	const maxFileSize = 50000
	if len(content) > maxFileSize {
		content = content[:maxFileSize] + "\n... (truncated, file too large)"
	}
	return content, nil
}
