package embedding

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service manages embedding generation and vector search.
type Service struct {
	db       *pgxpool.Pool
	provider Provider
	enabled  bool
}

// NewService creates an embedding service. Pass nil provider to disable.
func NewService(db *pgxpool.Pool, provider Provider) *Service {
	if provider == nil {
		provider = &NoopProvider{}
	}
	_, isNoop := provider.(*NoopProvider)
	return &Service{
		db:       db,
		provider: provider,
		enabled:  !isNoop,
	}
}

// IsEnabled returns true when a real embedding provider is configured.
func (s *Service) IsEnabled() bool {
	return s.enabled
}

// SemanticResult holds a single vector similarity search result.
type SemanticResult struct {
	ID         string  `json:"id"`
	Similarity float64 `json:"similarity"`
}

// EmbedAndStore generates an embedding for text and stores it on the given row.
func (s *Service) EmbedAndStore(ctx context.Context, table, idColumn, textColumn, embeddingColumn string, id string, text string) error {
	if !s.enabled {
		return nil
	}

	embeddings, err := s.provider.Embed(ctx, []string{text})
	if err != nil {
		return fmt.Errorf("embed text: %w", err)
	}
	if len(embeddings) == 0 || embeddings[0] == nil {
		return nil
	}

	vecStr := float32SliceToVectorLiteral(embeddings[0])
	query := fmt.Sprintf(
		`UPDATE %s SET %s = $1::vector WHERE %s = $2`,
		sanitizeIdentifier(table),
		sanitizeIdentifier(embeddingColumn),
		sanitizeIdentifier(idColumn),
	)

	_, err = s.db.Exec(ctx, query, vecStr, id)
	if err != nil {
		return fmt.Errorf("store embedding: %w", err)
	}

	return nil
}

// BackfillTable finds rows where the embedding column IS NULL, generates
// embeddings in batches, and stores them. Returns the count of rows processed.
func (s *Service) BackfillTable(ctx context.Context, table, idColumn, textColumn, embeddingColumn string, batchSize int) (int, error) {
	if !s.enabled {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 50
	}

	query := fmt.Sprintf(
		`SELECT %s, %s FROM %s WHERE %s IS NULL AND %s != '' LIMIT $1`,
		sanitizeIdentifier(idColumn),
		sanitizeIdentifier(textColumn),
		sanitizeIdentifier(table),
		sanitizeIdentifier(embeddingColumn),
		sanitizeIdentifier(textColumn),
	)

	rows, err := s.db.Query(ctx, query, batchSize)
	if err != nil {
		return 0, fmt.Errorf("query null embeddings: %w", err)
	}
	defer rows.Close()

	type row struct {
		id   string
		text string
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.text); err != nil {
			return 0, fmt.Errorf("scan row: %w", err)
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate rows: %w", err)
	}

	if len(pending) == 0 {
		return 0, nil
	}

	// Extract texts for batch embedding
	texts := make([]string, len(pending))
	for i, r := range pending {
		texts[i] = r.text
	}

	embeddings, err := s.provider.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("batch embed: %w", err)
	}

	// Store each embedding
	stored := 0
	updateQuery := fmt.Sprintf(
		`UPDATE %s SET %s = $1::vector WHERE %s = $2`,
		sanitizeIdentifier(table),
		sanitizeIdentifier(embeddingColumn),
		sanitizeIdentifier(idColumn),
	)

	for i, r := range pending {
		if i >= len(embeddings) || embeddings[i] == nil {
			continue
		}
		vecStr := float32SliceToVectorLiteral(embeddings[i])
		if _, err := s.db.Exec(ctx, updateQuery, vecStr, r.id); err != nil {
			slog.Warn("store embedding failed", "table", table, "id", r.id, "error", err)
			continue
		}
		stored++
	}

	return stored, nil
}

// SemanticSearch embeds the query string and performs a vector similarity search.
// idColumn is the column to return as the result ID (e.g. "id" or "sha").
func (s *Service) SemanticSearch(ctx context.Context, table, idColumn, embeddingColumn string, query string, limit int) ([]SemanticResult, error) {
	if !s.enabled {
		return nil, nil
	}

	embeddings, err := s.provider.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(embeddings) == 0 || embeddings[0] == nil {
		return nil, nil
	}

	vecStr := float32SliceToVectorLiteral(embeddings[0])
	sql := fmt.Sprintf(
		`SELECT %s, 1 - (%s <=> $1::vector) AS similarity FROM %s WHERE %s IS NOT NULL ORDER BY %s <=> $1::vector LIMIT $2`,
		sanitizeIdentifier(idColumn),
		sanitizeIdentifier(embeddingColumn),
		sanitizeIdentifier(table),
		sanitizeIdentifier(embeddingColumn),
		sanitizeIdentifier(embeddingColumn),
	)

	rows, err := s.db.Query(ctx, sql, vecStr, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var results []SemanticResult
	for rows.Next() {
		var r SemanticResult
		if err := rows.Scan(&r.ID, &r.Similarity); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		results = append(results, r)
	}

	return results, nil
}

// float32SliceToVectorLiteral converts a float32 slice to a pgvector literal string like "[0.1,0.2,0.3]".
func float32SliceToVectorLiteral(v []float32) string {
	buf := make([]byte, 0, len(v)*8+2)
	buf = append(buf, '[')
	for i, f := range v {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, fmt.Sprintf("%g", f)...)
	}
	buf = append(buf, ']')
	return string(buf)
}

// sanitizeIdentifier validates that a SQL identifier contains only safe characters.
// This prevents SQL injection when building dynamic queries for table/column names.
func sanitizeIdentifier(id string) string {
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return "invalid_identifier"
		}
	}
	return id
}
