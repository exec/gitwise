package comment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/pagination"
)

const maxCommentBody = 100_000 // 100KB max comment body

var (
	ErrNotFound    = errors.New("comment not found")
	ErrEmptyBody   = errors.New("comment body is required")
	ErrBodyTooLong = errors.New("comment body exceeds maximum length")
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
	if len(body) > maxCommentBody {
		return nil, ErrBodyTooLong
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
		cursorTime, cursorID, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		if cursorID == uuid.Nil {
			query += fmt.Sprintf(` AND c.created_at > $%d`, argIdx)
			args = append(args, cursorTime)
			argIdx++
		} else {
			query += fmt.Sprintf(` AND (c.created_at > $%d OR (c.created_at = $%d AND c.id > $%d))`, argIdx, argIdx, argIdx+1)
			args = append(args, cursorTime, cursorID)
			argIdx += 2
		}
	}

	query += ` ORDER BY c.created_at ASC, c.id ASC`
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
		nextCursor = pagination.EncodeCursor(comments[limit-1].CreatedAt, comments[limit-1].ID)
	}

	return comments, nextCursor, nil
}

func (s *Service) Update(ctx context.Context, commentID, authorID uuid.UUID, req models.UpdateCommentRequest) (*models.Comment, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, ErrEmptyBody
	}
	if len(body) > maxCommentBody {
		return nil, ErrBodyTooLong
	}

	comment := &models.Comment{}
	err := s.db.QueryRow(ctx, `
		UPDATE comments SET body_history = body_history || jsonb_build_array(jsonb_build_object('body', body, 'edited_at', now())), body = $2, updated_at = now()
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
