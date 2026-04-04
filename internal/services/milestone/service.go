package milestone

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrNotFound       = errors.New("milestone not found")
	ErrInvalidTitle   = errors.New("milestone title is required")
	ErrDuplicateTitle = errors.New("milestone with this title already exists")
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, repoID uuid.UUID, req models.CreateMilestoneRequest) (*models.Milestone, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" || len(title) > 255 {
		return nil, ErrInvalidTitle
	}

	m := &models.Milestone{
		ID:          uuid.New(),
		RepoID:      repoID,
		Title:       title,
		Description: req.Description,
		DueDate:     req.DueDate,
		Status:      "open",
		CreatedAt:   time.Now(),
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO milestones (id, repo_id, title, description, due_date, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		m.ID, m.RepoID, m.Title, m.Description, m.DueDate, m.Status, m.CreatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrDuplicateTitle
		}
		return nil, fmt.Errorf("insert milestone: %w", err)
	}

	return m, nil
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID) ([]models.Milestone, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, title, description, due_date, status, created_at
		FROM milestones
		WHERE repo_id = $1
		ORDER BY created_at DESC`, repoID,
	)
	if err != nil {
		return nil, fmt.Errorf("query milestones: %w", err)
	}
	defer rows.Close()

	var milestones []models.Milestone
	for rows.Next() {
		var m models.Milestone
		if err := rows.Scan(&m.ID, &m.RepoID, &m.Title, &m.Description, &m.DueDate, &m.Status, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan milestone: %w", err)
		}
		milestones = append(milestones, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate milestones: %w", err)
	}
	return milestones, nil
}

func (s *Service) Update(ctx context.Context, repoID uuid.UUID, milestoneID uuid.UUID, req models.UpdateMilestoneRequest) (*models.Milestone, error) {
	setClauses := []string{}
	args := []any{milestoneID, repoID}
	argIdx := 3

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" || len(title) > 255 {
			return nil, ErrInvalidTitle
		}
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, title)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.DueDate != nil {
		setClauses = append(setClauses, fmt.Sprintf("due_date = $%d", argIdx))
		args = append(args, *req.DueDate)
		argIdx++
	}
	if req.Status != nil {
		if *req.Status != "open" && *req.Status != "closed" {
			return nil, fmt.Errorf("invalid status: must be open or closed")
		}
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *req.Status)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.getByID(ctx, milestoneID)
	}

	query := fmt.Sprintf(`UPDATE milestones SET %s WHERE id = $1 AND repo_id = $2 RETURNING id, repo_id, title, description, due_date, status, created_at`,
		strings.Join(setClauses, ", "))

	m := &models.Milestone{}
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&m.ID, &m.RepoID, &m.Title, &m.Description, &m.DueDate, &m.Status, &m.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrDuplicateTitle
		}
		return nil, fmt.Errorf("update milestone: %w", err)
	}
	return m, nil
}

func (s *Service) Delete(ctx context.Context, repoID uuid.UUID, milestoneID uuid.UUID) error {
	// Clear milestone_id on any issues referencing this milestone within the same repo
	_, err := s.db.Exec(ctx, `UPDATE issues SET milestone_id = NULL WHERE milestone_id = $1 AND repo_id = $2`, milestoneID, repoID)
	if err != nil {
		return fmt.Errorf("clear milestone from issues: %w", err)
	}

	tag, err := s.db.Exec(ctx, `DELETE FROM milestones WHERE id = $1 AND repo_id = $2`, milestoneID, repoID)
	if err != nil {
		return fmt.Errorf("delete milestone: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) getByID(ctx context.Context, milestoneID uuid.UUID) (*models.Milestone, error) {
	m := &models.Milestone{}
	err := s.db.QueryRow(ctx, `
		SELECT id, repo_id, title, description, due_date, status, created_at
		FROM milestones WHERE id = $1`, milestoneID,
	).Scan(&m.ID, &m.RepoID, &m.Title, &m.Description, &m.DueDate, &m.Status, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query milestone: %w", err)
	}
	return m, nil
}
