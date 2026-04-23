package embedding

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

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

// embedWithRetry calls provider.Embed with exponential backoff + jitter.
// It attempts up to maxAttempts times (delays: ~1s, ~2s, ~4s).
// Returns the embeddings on success, or an error after all retries are exhausted.
func (s *Service) embedWithRetry(ctx context.Context, texts []string) ([][]float32, error) {
	const maxAttempts = 3
	baseDelay := time.Second

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		embeddings, err := s.provider.Embed(ctx, texts)
		if err == nil {
			return embeddings, nil
		}
		lastErr = err

		if attempt == maxAttempts-1 {
			break
		}

		// Exponential backoff with full jitter: sleep up to baseDelay * 2^attempt
		maxJitter := baseDelay << uint(attempt) // 1s, 2s, 4s
		jitter := time.Duration(rand.Int63n(int64(maxJitter)))
		slog.Warn("embedding provider failed, retrying",
			"attempt", attempt+1,
			"max_attempts", maxAttempts,
			"backoff_ms", jitter.Milliseconds(),
			"error", err,
		)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(jitter):
		}
	}
	return nil, lastErr
}

// recordEmbeddingFailure persists a failure record so a worker can retry later.
func (s *Service) recordEmbeddingFailure(ctx context.Context, table, id, reason string) {
	_, err := s.db.Exec(ctx,
		`INSERT INTO embedding_failures (id, table_name, reason, failed_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (id, table_name) DO UPDATE SET reason = EXCLUDED.reason, failed_at = EXCLUDED.failed_at`,
		id, table, reason,
	)
	if err != nil {
		slog.Warn("failed to record embedding failure", "table", table, "id", id, "error", err)
	}
}

// BackfillTable finds rows where the embedding column IS NULL, generates
// embeddings in batches, and stores them. Returns the count of rows processed.
// On provider failure (after retries) each failing row ID is recorded in
// embedding_failures for later re-processing.
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

	// Extract texts for batch embedding (with retry/backoff)
	texts := make([]string, len(pending))
	for i, r := range pending {
		texts[i] = r.text
	}

	embeddings, err := s.embedWithRetry(ctx, texts)
	if err != nil {
		// All retries exhausted — record every pending row as failed
		slog.Error("batch embed failed after all retries", "table", table, "batch_size", len(pending), "error", err)
		for _, r := range pending {
			s.recordEmbeddingFailure(ctx, table, r.id, err.Error())
		}
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
			s.recordEmbeddingFailure(ctx, table, r.id, err.Error())
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
// Note: quoted identifiers with special characters are not supported by design —
// all Gitwise table/column names use lowercase letters, digits, and underscores only.
func sanitizeIdentifier(id string) string {
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return "invalid_identifier"
		}
	}
	return id
}
