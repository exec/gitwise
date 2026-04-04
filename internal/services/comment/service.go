package comment

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrNotFound    = errors.New("comment not found")
	ErrEmptyBody   = errors.New("comment body is required")
	ErrForbidden   = errors.New("access denied")
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, repoID uuid.UUID, issueID, prID *uuid.UUID, authorID uuid.UUID, req models.CreateCommentRequest) (*models.Comment, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, ErrEmptyBody
	}

	comment := &models.Comment{
		ID:        uuid.New(),
		RepoID:    repoID,
		IssueID:   issueID,
		PRID:      prID,
		AuthorID:  authorID,
		Body:      body,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO comments (id, repo_id, issue_id, pr_id, author_id, body, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		comment.ID, comment.RepoID, comment.IssueID, comment.PRID,
		comment.AuthorID, comment.Body, comment.CreatedAt, comment.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert comment: %w", err)
	}

	if err := s.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, authorID).Scan(&comment.AuthorName); err != nil {
		slog.Warn("comment create: author name lookup failed", "author_id", authorID, "error", err)
	}

	return comment, nil
}

func (s *Service) ListByIssue(ctx context.Context, issueID uuid.UUID, cursor string, limit int) ([]models.Comment, string, error) {
	return s.list(ctx, "issue_id", issueID, cursor, limit)
}

func (s *Service) ListByPR(ctx context.Context, prID uuid.UUID, cursor string, limit int) ([]models.Comment, string, error) {
	return s.list(ctx, "pr_id", prID, cursor, limit)
}

func (s *Service) list(ctx context.Context, col string, id uuid.UUID, cursor string, limit int) ([]models.Comment, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT c.id, c.repo_id, c.issue_id, c.pr_id, c.author_id, u.username,
		       c.body, c.created_at, c.updated_at
		FROM comments c
		JOIN users u ON u.id = c.author_id
		WHERE c.%s = $1`, col)

	args := []any{id}
	argIdx := 2

	if cursor != "" {
		cursorTime, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		query += fmt.Sprintf(` AND c.created_at > $%d`, argIdx)
		args = append(args, cursorTime)
		argIdx++
	}

	query += ` ORDER BY c.created_at ASC`
	query += fmt.Sprintf(` LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query comments: %w", err)
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var c models.Comment
		if err := rows.Scan(
			&c.ID, &c.RepoID, &c.IssueID, &c.PRID, &c.AuthorID, &c.AuthorName,
			&c.Body, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, "", fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate comments: %w", err)
	}

	var nextCursor string
	if len(comments) > limit {
		comments = comments[:limit]
		nextCursor = encodeCursor(comments[limit-1].CreatedAt)
	}

	return comments, nextCursor, nil
}

func (s *Service) Update(ctx context.Context, commentID, authorID uuid.UUID, req models.UpdateCommentRequest) (*models.Comment, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, ErrEmptyBody
	}

	comment := &models.Comment{}
	err := s.db.QueryRow(ctx, `
		UPDATE comments SET body = $2, updated_at = now()
		WHERE id = $1 AND author_id = $3
		RETURNING id, repo_id, issue_id, pr_id, author_id,
		          (SELECT username FROM users WHERE id = author_id),
		          body, created_at, updated_at`,
		commentID, body, authorID,
	).Scan(
		&comment.ID, &comment.RepoID, &comment.IssueID, &comment.PRID,
		&comment.AuthorID, &comment.AuthorName, &comment.Body,
		&comment.CreatedAt, &comment.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update comment: %w", err)
	}

	return comment, nil
}

func encodeCursor(t time.Time) string {
	return base64.StdEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano)))
}

func decodeCursor(cursor string) (time.Time, error) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, string(b))
}

func (s *Service) Delete(ctx context.Context, commentID, authorID uuid.UUID) error {
	// Check ownership first
	var owner uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT author_id FROM comments WHERE id = $1`, commentID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("query comment: %w", err)
	}
	if owner != authorID {
		return ErrForbidden
	}

	_, err = s.db.Exec(ctx, `DELETE FROM comments WHERE id = $1`, commentID)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	return nil
}
