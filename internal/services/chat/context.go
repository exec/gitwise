package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/services/agent"
)

// ContextBuilder assembles relevant context for LLM prompts.
type ContextBuilder struct {
	db       *pgxpool.Pool
	agentSvc *agent.Service
}

// NewContextBuilder creates a new context builder.
func NewContextBuilder(db *pgxpool.Pool, agentSvc *agent.Service) *ContextBuilder {
	return &ContextBuilder{
		db:       db,
		agentSvc: agentSvc,
	}
}

// BuildChatContext assembles context for a chat message.
// Searches agent docs, code files, issues/PRs by relevance to the query.
// Returns a formatted context string that fits within maxTokens (approximated as chars/4).
func (cb *ContextBuilder) BuildChatContext(ctx context.Context, repoID uuid.UUID, query string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 4000
	}
	// Rough approximation: 1 token ~= 4 chars
	maxChars := maxTokens * 4

	var sections []string

	// 1. Repo metadata
	repoCtx, err := cb.buildRepoSection(ctx, repoID)
	if err == nil && repoCtx != "" {
		sections = append(sections, repoCtx)
	}

	// 2. Agent-generated docs (most valuable for chat context)
	docsCtx, err := cb.buildDocsSection(ctx, repoID, query)
	if err == nil && docsCtx != "" {
		sections = append(sections, docsCtx)
	}

	result := strings.Join(sections, "\n\n---\n\n")

	// Truncate if needed
	if len(result) > maxChars {
		result = result[:maxChars] + "\n\n[Context truncated]"
	}

	return result, nil
}

// BuildReviewContext assembles context for a code review.
// Includes the diff, relevant agent docs, and custom instructions.
func (cb *ContextBuilder) BuildReviewContext(ctx context.Context, repoID uuid.UUID, diff string, instructions string) (string, error) {
	var sections []string

	// 1. Custom instructions
	if instructions != "" {
		sections = append(sections, fmt.Sprintf("## Custom Instructions\n\n%s", instructions))
	}

	// 2. Repo metadata
	repoCtx, err := cb.buildRepoSection(ctx, repoID)
	if err == nil && repoCtx != "" {
		sections = append(sections, repoCtx)
	}

	// 3. Agent-generated convention docs
	if cb.agentSvc != nil {
		docs, err := cb.agentSvc.GetDocumentsByType(ctx, repoID, "conventions")
		if err == nil && len(docs) > 0 {
			var convParts []string
			for _, doc := range docs {
				convParts = append(convParts, fmt.Sprintf("### %s\n\n%s", doc.Title, doc.Content))
			}
			sections = append(sections, fmt.Sprintf("## Coding Conventions\n\n%s", strings.Join(convParts, "\n\n")))
		}
	}

	// 4. Diff
	if diff != "" {
		sections = append(sections, fmt.Sprintf("## Code Diff\n\n```diff\n%s\n```", diff))
	}

	return strings.Join(sections, "\n\n---\n\n"), nil
}

// buildRepoSection builds the repository metadata context section.
func (cb *ContextBuilder) buildRepoSection(ctx context.Context, repoID uuid.UUID) (string, error) {
	var name, description, languageStats string
	err := cb.db.QueryRow(ctx, `
		SELECT r.name, COALESCE(r.description, ''),
			COALESCE(r.language_stats::text, '{}')
		FROM repositories r
		WHERE r.id = $1`, repoID).Scan(&name, &description, &languageStats)
	if err != nil {
		return "", fmt.Errorf("query repo: %w", err)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("## Repository: %s", name))
	if description != "" {
		parts = append(parts, fmt.Sprintf("Description: %s", description))
	}
	if languageStats != "" && languageStats != "{}" {
		parts = append(parts, fmt.Sprintf("Languages: %s", languageStats))
	}

	return strings.Join(parts, "\n"), nil
}

// buildDocsSection builds the agent documents context section.
func (cb *ContextBuilder) buildDocsSection(ctx context.Context, repoID uuid.UUID, query string) (string, error) {
	if cb.agentSvc == nil {
		return "", nil
	}

	var docs []struct {
		title   string
		content string
		docType string
	}

	// If we have a query, search for relevant docs; otherwise get all
	if query != "" {
		results, err := cb.agentSvc.SearchDocuments(ctx, repoID, query, 5)
		if err != nil {
			return "", err
		}
		for _, d := range results {
			docs = append(docs, struct {
				title   string
				content string
				docType string
			}{d.Title, d.Content, d.DocType})
		}
	} else {
		results, err := cb.agentSvc.ListDocuments(ctx, repoID)
		if err != nil {
			return "", err
		}
		for _, d := range results {
			docs = append(docs, struct {
				title   string
				content string
				docType string
			}{d.Title, d.Content, d.DocType})
		}
	}

	if len(docs) == 0 {
		return "", nil
	}

	var parts []string
	parts = append(parts, "## Agent-Generated Documentation")
	for _, doc := range docs {
		parts = append(parts, fmt.Sprintf("### [%s] %s\n\n%s", doc.docType, doc.title, doc.content))
	}

	return strings.Join(parts, "\n\n"), nil
}
