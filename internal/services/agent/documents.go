package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrDocNotFound    = errors.New("agent document not found")
	ErrInvalidDocType = errors.New("invalid document type")
	ErrInvalidTitle   = errors.New("document title is required")
)

var validDocTypes = map[string]bool{
	"architecture": true,
	"component":    true,
	"api":          true,
	"dependency":   true,
	"conventions":  true,
	"onboarding":   true,
	"custom":       true,
}

// ListDocuments returns all agent-generated documents for a repository.
func (s *Service) ListDocuments(ctx context.Context, repoID uuid.UUID) ([]models.AgentDocument, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, agent_id, title, content, doc_type, metadata, version,
			created_at, updated_at
		FROM agent_documents
		WHERE repo_id = $1
		ORDER BY doc_type ASC, title ASC`, repoID)
	if err != nil {
		return nil, fmt.Errorf("query documents: %w", err)
	}
	defer rows.Close()

	return scanDocuments(rows)
}

// GetDocument returns a single agent document by ID.
func (s *Service) GetDocument(ctx context.Context, docID uuid.UUID) (*models.AgentDocument, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, repo_id, agent_id, title, content, doc_type, metadata, version,
			created_at, updated_at
		FROM agent_documents
		WHERE id = $1`, docID)

	doc, err := scanDocument(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDocNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query document: %w", err)
	}
	return doc, nil
}

// UpsertDocument creates or updates a document by repo_id + doc_type.
// If a document with the same repo_id, agent_id, and doc_type exists, it is updated
// with incremented version. Otherwise a new document is created.
func (s *Service) UpsertDocument(ctx context.Context, repoID uuid.UUID, req models.UpsertAgentDocumentRequest) (*models.AgentDocument, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" || len(title) > 500 {
		return nil, ErrInvalidTitle
	}

	docType := strings.TrimSpace(req.DocType)
	if !validDocTypes[docType] {
		return nil, ErrInvalidDocType
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}

	now := time.Now()

	// Try update first
	var existing models.AgentDocument
	err := s.db.QueryRow(ctx, `
		SELECT id, version FROM agent_documents
		WHERE repo_id = $1 AND agent_id = $2 AND doc_type = $3`,
		repoID, req.AgentID, docType).Scan(&existing.ID, &existing.Version)

	if err == nil {
		// Update existing document
		newVersion := existing.Version + 1
		_, err = s.db.Exec(ctx, `
			UPDATE agent_documents
			SET title = $1, content = $2, metadata = $3, version = $4, updated_at = $5
			WHERE id = $6`,
			title, req.Content, metadata, newVersion, now, existing.ID)
		if err != nil {
			return nil, fmt.Errorf("update document: %w", err)
		}

		return s.GetDocument(ctx, existing.ID)
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check existing doc: %w", err)
	}

	// Create new document
	doc := &models.AgentDocument{
		ID:        uuid.New(),
		RepoID:    repoID,
		AgentID:   req.AgentID,
		Title:     title,
		Content:   req.Content,
		DocType:   docType,
		Metadata:  metadata,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO agent_documents (id, repo_id, agent_id, title, content, doc_type, metadata, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		doc.ID, doc.RepoID, doc.AgentID, doc.Title, doc.Content, doc.DocType,
		doc.Metadata, doc.Version, doc.CreatedAt, doc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert document: %w", err)
	}

	return doc, nil
}

// DeleteDocument deletes an agent document by ID.
func (s *Service) DeleteDocument(ctx context.Context, docID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM agent_documents WHERE id = $1`, docID)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDocNotFound
	}
	return nil
}

// SearchDocuments performs semantic search over agent documents for a repository.
// Falls back to keyword search if embeddings are not available.
func (s *Service) SearchDocuments(ctx context.Context, repoID uuid.UUID, query string, limit int) ([]models.AgentDocument, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	// Keyword search fallback using ILIKE (semantic search via pgvector would
	// require the embedding pipeline, which is wired by the other agent).
	searchPattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"

	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, agent_id, title, content, doc_type, metadata, version,
			created_at, updated_at
		FROM agent_documents
		WHERE repo_id = $1
			AND (title ILIKE $2 OR content ILIKE $2)
		ORDER BY updated_at DESC
		LIMIT $3`, repoID, searchPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search documents: %w", err)
	}
	defer rows.Close()

	return scanDocuments(rows)
}

// GetDocumentsByType returns documents of a specific type for a repo.
func (s *Service) GetDocumentsByType(ctx context.Context, repoID uuid.UUID, docType string) ([]models.AgentDocument, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, agent_id, title, content, doc_type, metadata, version,
			created_at, updated_at
		FROM agent_documents
		WHERE repo_id = $1 AND doc_type = $2
		ORDER BY title ASC`, repoID, docType)
	if err != nil {
		return nil, fmt.Errorf("query documents by type: %w", err)
	}
	defer rows.Close()

	return scanDocuments(rows)
}

// scanDocument scans a single document from a row.
func scanDocument(row pgx.Row) (*models.AgentDocument, error) {
	var d models.AgentDocument
	err := row.Scan(&d.ID, &d.RepoID, &d.AgentID, &d.Title, &d.Content,
		&d.DocType, &d.Metadata, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return &d, err
}

// scanDocuments scans multiple documents from rows.
func scanDocuments(rows pgx.Rows) ([]models.AgentDocument, error) {
	var docs []models.AgentDocument
	for rows.Next() {
		var d models.AgentDocument
		if err := rows.Scan(&d.ID, &d.RepoID, &d.AgentID, &d.Title, &d.Content,
			&d.DocType, &d.Metadata, &d.Version, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		docs = append(docs, d)
	}
	if docs == nil {
		docs = []models.AgentDocument{}
	}
	return docs, rows.Err()
}
