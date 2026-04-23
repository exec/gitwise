package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrNotFound      = errors.New("review not found")
	ErrInvalidType   = errors.New("invalid review type")
	ErrSelfReview    = errors.New("cannot review your own pull request")
	ErrThreadNotFound = errors.New("thread not found")
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, prID, authorID uuid.UUID, req models.CreateReviewRequest) (*models.Review, error) {
	if !isValidReviewType(req.Type) {
		return nil, ErrInvalidType
	}

	// Check that reviewer is not the PR author
	var prAuthorID uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT author_id FROM pull_requests WHERE id = $1`, prID).Scan(&prAuthorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query PR: %w", err)
	}
	if prAuthorID == authorID {
		return nil, ErrSelfReview
	}

	commentsJSON, err := json.Marshal(req.Comments)
	if err != nil {
		commentsJSON = []byte("[]")
	}

	review := &models.Review{
		ID:          uuid.New(),
		PRID:        prID,
		AuthorID:    authorID,
		Type:        req.Type,
		Body:        req.Body,
		Comments:    commentsJSON,
		SubmittedAt: time.Now(),
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO reviews (id, pr_id, author_id, type, body, comments, submitted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		review.ID, review.PRID, review.AuthorID, review.Type,
		review.Body, review.Comments, review.SubmittedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert review: %w", err)
	}

	if err := s.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, authorID).Scan(&review.AuthorName); err != nil {
		slog.Warn("review create: author name lookup failed", "author_id", authorID, "error", err)
	}

	// Update review summary on the PR
	if err := s.updateReviewSummary(ctx, prID); err != nil {
		return nil, fmt.Errorf("update review summary: %w", err)
	}

	return review, nil
}

func (s *Service) ListByPR(ctx context.Context, prID uuid.UUID) ([]models.Review, error) {
	rows, err := s.db.Query(ctx, `
		SELECT r.id, r.pr_id, r.author_id, u.username, r.type, r.body, r.comments, r.submitted_at
		FROM reviews r
		JOIN users u ON u.id = r.author_id
		WHERE r.pr_id = $1
		ORDER BY r.submitted_at ASC`, prID,
	)
	if err != nil {
		return nil, fmt.Errorf("query reviews: %w", err)
	}
	defer rows.Close()

	var reviews []models.Review
	for rows.Next() {
		var r models.Review
		if err := rows.Scan(
			&r.ID, &r.PRID, &r.AuthorID, &r.AuthorName,
			&r.Type, &r.Body, &r.Comments, &r.SubmittedAt,
		); err != nil {
			return nil, fmt.Errorf("scan review: %w", err)
		}
		reviews = append(reviews, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reviews: %w", err)
	}
	return reviews, nil
}

func (s *Service) ResolveThread(ctx context.Context, prID uuid.UUID, threadID string, resolved bool) error {
	result, err := s.db.Exec(ctx, `
		UPDATE reviews SET comments = (
			SELECT COALESCE(jsonb_agg(
				CASE WHEN elem->>'thread_id' = $2
				     THEN jsonb_set(elem, '{resolved}', to_jsonb($3::boolean))
				     ELSE elem
				END
			), '[]'::jsonb)
			FROM jsonb_array_elements(comments) elem
		)
		WHERE pr_id = $1
		AND EXISTS (
			SELECT 1 FROM jsonb_array_elements(comments) e WHERE e->>'thread_id' = $2
		)`, prID, threadID, resolved,
	)
	if err != nil {
		return fmt.Errorf("resolve thread: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrThreadNotFound
	}

	if err := s.updateReviewSummary(ctx, prID); err != nil {
		return fmt.Errorf("update review summary: %w", err)
	}
	return nil
}

func (s *Service) updateReviewSummary(ctx context.Context, prID uuid.UUID) error {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (r.author_id) u.username, r.type
		FROM reviews r
		JOIN users u ON u.id = r.author_id
		WHERE r.pr_id = $1
		ORDER BY r.author_id, r.submitted_at DESC`, prID,
	)
	if err != nil {
		return fmt.Errorf("update review summary: query reviews: %w", err)
	}
	defer rows.Close()

	var approvedBy, changesRequestedBy []string

	for rows.Next() {
		var username, reviewType string
		if err := rows.Scan(&username, &reviewType); err != nil {
			return fmt.Errorf("update review summary: scan review: %w", err)
		}
		switch reviewType {
		case "approval":
			approvedBy = append(approvedBy, username)
		case "changes_requested":
			changesRequestedBy = append(changesRequestedBy, username)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("update review summary: iterate reviews: %w", err)
	}

	var reviewsCount int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM reviews WHERE pr_id = $1`, prID).Scan(&reviewsCount); err != nil {
		return fmt.Errorf("update review summary: count reviews: %w", err)
	}

	var commentsCount int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM comments WHERE pr_id = $1`, prID).Scan(&commentsCount); err != nil {
		return fmt.Errorf("update review summary: count comments: %w", err)
	}

	// Count thread resolution status across all reviews for this PR
	threadsResolved, threadsUnresolved := s.countThreads(ctx, prID)

	summary, _ := json.Marshal(map[string]any{
		"approved_by":          approvedBy,
		"changes_requested_by": changesRequestedBy,
		"reviews_count":        reviewsCount,
		"comments_count":       commentsCount,
		"threads_resolved":     threadsResolved,
		"threads_unresolved":   threadsUnresolved,
	})

	if _, err := s.db.Exec(ctx, `UPDATE pull_requests SET review_summary = $2, updated_at = now() WHERE id = $1`, prID, summary); err != nil {
		return fmt.Errorf("update review summary: update pull_request: %w", err)
	}
	return nil
}

func (s *Service) countThreads(ctx context.Context, prID uuid.UUID) (resolved int, unresolved int) {
	reviews, err := s.ListByPR(ctx, prID)
	if err != nil {
		slog.Error("count threads: list reviews failed", "pr_id", prID, "error", err)
		return 0, 0
	}

	// Collect all comments across all reviews and track thread resolution
	// A thread is resolved if any comment in it has resolved=true
	threadResolved := make(map[string]bool)
	for _, r := range reviews {
		var comments []models.InlineCommentInput
		if err := json.Unmarshal(r.Comments, &comments); err != nil {
			continue
		}
		for _, c := range comments {
			if c.ThreadID == "" {
				continue
			}
			if _, exists := threadResolved[c.ThreadID]; !exists {
				threadResolved[c.ThreadID] = false
			}
			if c.Resolved {
				threadResolved[c.ThreadID] = true
			}
		}
	}

	for _, isResolved := range threadResolved {
		if isResolved {
			resolved++
		} else {
			unresolved++
		}
	}
	return resolved, unresolved
}

func isValidReviewType(t string) bool {
	switch t {
	case "approval", "changes_requested", "comment":
		return true
	}
	return false
}
