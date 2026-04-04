package search

import (
	"context"
	"fmt"

	"github.com/gitwise-io/gitwise/internal/services/embedding"
)

// SearchSemantic performs vector similarity search when embeddings are available.
// Falls back to empty results when embeddings are disabled.
func (s *Service) SearchSemantic(ctx context.Context, query string, table string, limit int) ([]SearchResult, error) {
	if s.embeddingSvc == nil || !s.embeddingSvc.IsEnabled() {
		return nil, nil
	}

	// Map table to the appropriate id and embedding columns
	idCol, embeddingCol, err := embeddingColumns(table)
	if err != nil {
		return nil, err
	}

	results, err := s.embeddingSvc.SemanticSearch(ctx, table, idCol, embeddingCol, query, limit)
	if err != nil {
		return nil, fmt.Errorf("semantic search %s: %w", table, err)
	}

	// Convert to SearchResult
	var out []SearchResult
	for _, r := range results {
		out = append(out, SearchResult{
			Type:  tableToType(table),
			ID:    r.ID,
			Score: r.Similarity,
		})
	}

	return out, nil
}

// SetEmbeddingService wires the embedding service into the search service.
func (s *Service) SetEmbeddingService(svc *embedding.Service) {
	s.embeddingSvc = svc
}

func embeddingColumns(table string) (idCol, embeddingCol string, err error) {
	switch table {
	case "issues":
		return "id", "title_embedding", nil
	case "pull_requests":
		return "id", "title_embedding", nil
	case "commit_metadata":
		return "sha", "message_embedding", nil
	case "repositories":
		return "id", "description_embedding", nil
	default:
		return "", "", fmt.Errorf("no embedding columns for table: %s", table)
	}
}

func tableToType(table string) string {
	switch table {
	case "issues":
		return "issue"
	case "pull_requests":
		return "pr"
	case "commit_metadata":
		return "commit"
	case "repositories":
		return "repo"
	default:
		return table
	}
}
