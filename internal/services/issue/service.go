package issue

import (
	"context"
	"encoding/json"
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
	ErrNotFound       = errors.New("issue not found")
	ErrInvalidTitle   = errors.New("title is required")
	ErrInvalidStatus  = errors.New("invalid status")
	ErrForbidden      = errors.New("access denied")
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, repoID, authorID uuid.UUID, req models.CreateIssueRequest) (*models.Issue, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" || len(title) > 500 {
		return nil, ErrInvalidTitle
	}

	priority := req.Priority
	if priority == "" {
		priority = "none"
	}
	if !isValidPriority(priority) {
		return nil, fmt.Errorf("%w: must be critical, high, medium, low, or none", ErrInvalidStatus)
	}

	labels := req.Labels
	if labels == nil {
		labels = []string{}
	}

	// Get the next issue/PR number for this repo
	var number int
	err := s.db.QueryRow(ctx, `SELECT next_repo_number($1)`, repoID).Scan(&number)
	if err != nil {
		return nil, fmt.Errorf("get next number: %w", err)
	}

	issue := &models.Issue{
		ID:        uuid.New(),
		RepoID:    repoID,
		Number:    number,
		AuthorID:  authorID,
		Title:     title,
		Body:      req.Body,
		Status:    "open",
		Labels:    labels,
		Assignees: []uuid.UUID{},
		LinkedPRs: []uuid.UUID{},
		Priority:  priority,
		Metadata:  json.RawMessage(`{}`),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO issues (id, repo_id, number, author_id, title, body, status, labels, assignees, linked_prs, priority, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		issue.ID, issue.RepoID, issue.Number, issue.AuthorID, issue.Title, issue.Body,
		issue.Status, issue.Labels, issue.Assignees, issue.LinkedPRs, issue.Priority,
		issue.Metadata, issue.CreatedAt, issue.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert issue: %w", err)
	}

	// Populate author name
	if err := s.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, authorID).Scan(&issue.AuthorName); err != nil {
		slog.Warn("issue create: author name lookup failed", "author_id", authorID, "error", err)
	}

	return issue, nil
}

func (s *Service) GetByNumber(ctx context.Context, repoID uuid.UUID, number int) (*models.Issue, error) {
	issue := &models.Issue{}
	err := s.db.QueryRow(ctx, `
		SELECT i.id, i.repo_id, i.number, i.author_id, u.username, i.title, i.body,
		       i.status, i.labels, i.assignees, i.milestone_id, i.linked_prs, i.priority,
		       i.metadata, i.closed_at, i.created_at, i.updated_at
		FROM issues i
		JOIN users u ON u.id = i.author_id
		WHERE i.repo_id = $1 AND i.number = $2`, repoID, number,
	).Scan(
		&issue.ID, &issue.RepoID, &issue.Number, &issue.AuthorID, &issue.AuthorName,
		&issue.Title, &issue.Body, &issue.Status, &issue.Labels, &issue.Assignees,
		&issue.MilestoneID, &issue.LinkedPRs, &issue.Priority, &issue.Metadata,
		&issue.ClosedAt, &issue.CreatedAt, &issue.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query issue: %w", err)
	}
	return issue, nil
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID, status string, limit int) ([]models.Issue, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	query := `
		SELECT i.id, i.repo_id, i.number, i.author_id, u.username, i.title, i.body,
		       i.status, i.labels, i.assignees, i.milestone_id, i.linked_prs, i.priority,
		       i.metadata, i.closed_at, i.created_at, i.updated_at
		FROM issues i
		JOIN users u ON u.id = i.author_id
		WHERE i.repo_id = $1`

	args := []any{repoID}
	argIdx := 2

	if status != "" && isValidIssueStatus(status) {
		query += fmt.Sprintf(` AND i.status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}

	query += ` ORDER BY i.created_at DESC`
	query += fmt.Sprintf(` LIMIT $%d`, argIdx)
	args = append(args, limit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query issues: %w", err)
	}
	defer rows.Close()

	var issues []models.Issue
	for rows.Next() {
		var i models.Issue
		if err := rows.Scan(
			&i.ID, &i.RepoID, &i.Number, &i.AuthorID, &i.AuthorName,
			&i.Title, &i.Body, &i.Status, &i.Labels, &i.Assignees,
			&i.MilestoneID, &i.LinkedPRs, &i.Priority, &i.Metadata,
			&i.ClosedAt, &i.CreatedAt, &i.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		issues = append(issues, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issues: %w", err)
	}
	return issues, nil
}

func (s *Service) Update(ctx context.Context, repoID uuid.UUID, number int, req models.UpdateIssueRequest) (*models.Issue, error) {
	setClauses := []string{"updated_at = now()"}
	args := []any{repoID, number}
	argIdx := 3

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" || len(title) > 500 {
			return nil, ErrInvalidTitle
		}
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, title)
		argIdx++
	}
	if req.Body != nil {
		setClauses = append(setClauses, fmt.Sprintf("body = $%d", argIdx))
		args = append(args, *req.Body)
		argIdx++
	}
	if req.Status != nil {
		if !isValidIssueStatus(*req.Status) {
			return nil, ErrInvalidStatus
		}
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *req.Status)
		argIdx++

		if *req.Status == "closed" || *req.Status == "duplicate" {
			setClauses = append(setClauses, fmt.Sprintf("closed_at = $%d", argIdx))
			args = append(args, time.Now())
			argIdx++
		} else if *req.Status == "open" {
			setClauses = append(setClauses, "closed_at = NULL")
		}
	}
	if req.Labels != nil {
		labels := *req.Labels
		setClauses = append(setClauses, fmt.Sprintf("labels = $%d", argIdx))
		args = append(args, labels)
		argIdx++
	}
	if req.Priority != nil {
		if !isValidPriority(*req.Priority) {
			return nil, fmt.Errorf("%w: must be critical, high, medium, low, or none", ErrInvalidStatus)
		}
		setClauses = append(setClauses, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, *req.Priority)
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE issues SET %s
		WHERE repo_id = $1 AND number = $2
		RETURNING id, repo_id, number, author_id,
		          (SELECT username FROM users WHERE id = author_id),
		          title, body, status, labels, assignees, milestone_id, linked_prs,
		          priority, metadata, closed_at, created_at, updated_at`,
		strings.Join(setClauses, ", "))

	issue := &models.Issue{}
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&issue.ID, &issue.RepoID, &issue.Number, &issue.AuthorID, &issue.AuthorName,
		&issue.Title, &issue.Body, &issue.Status, &issue.Labels, &issue.Assignees,
		&issue.MilestoneID, &issue.LinkedPRs, &issue.Priority, &issue.Metadata,
		&issue.ClosedAt, &issue.CreatedAt, &issue.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update issue: %w", err)
	}
	return issue, nil
}

func isValidIssueStatus(s string) bool {
	switch s {
	case "open", "closed", "duplicate":
		return true
	}
	return false
}

func isValidPriority(p string) bool {
	switch p {
	case "critical", "high", "medium", "low", "none":
		return true
	}
	return false
}
