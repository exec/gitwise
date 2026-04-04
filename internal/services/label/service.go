package label

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrNotFound    = errors.New("label not found")
	ErrDuplicate   = errors.New("label already exists")
	ErrInvalidName = errors.New("label name is required")
)

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, repoID uuid.UUID, req models.CreateLabelRequest) (*models.Label, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 255 {
		return nil, ErrInvalidName
	}

	color := req.Color
	if color == "" {
		color = "#888888"
	}
	if !colorRe.MatchString(color) {
		return nil, fmt.Errorf("invalid color: must be hex like #ff0000")
	}

	label := &models.Label{
		ID:          uuid.New(),
		RepoID:      repoID,
		Name:        name,
		Color:       color,
		Description: req.Description,
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO labels (id, repo_id, name, color, description)
		VALUES ($1, $2, $3, $4, $5)`,
		label.ID, label.RepoID, label.Name, label.Color, label.Description,
	)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("insert label: %w", err)
	}
	return label, nil
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID) ([]models.Label, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, name, color, description
		FROM labels
		WHERE repo_id = $1
		ORDER BY name ASC`, repoID,
	)
	if err != nil {
		return nil, fmt.Errorf("query labels: %w", err)
	}
	defer rows.Close()

	var labels []models.Label
	for rows.Next() {
		var l models.Label
		if err := rows.Scan(&l.ID, &l.RepoID, &l.Name, &l.Color, &l.Description); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		labels = append(labels, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate labels: %w", err)
	}
	return labels, nil
}

func (s *Service) Update(ctx context.Context, repoID, labelID uuid.UUID, req models.UpdateLabelRequest) (*models.Label, error) {
	setClauses := []string{}
	args := []any{labelID, repoID}
	argIdx := 3

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 255 {
			return nil, ErrInvalidName
		}
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, name)
		argIdx++
	}
	if req.Color != nil {
		if !colorRe.MatchString(*req.Color) {
			return nil, fmt.Errorf("invalid color: must be hex like #ff0000")
		}
		setClauses = append(setClauses, fmt.Sprintf("color = $%d", argIdx))
		args = append(args, *req.Color)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.getByID(ctx, labelID)
	}

	query := fmt.Sprintf(`UPDATE labels SET %s WHERE id = $1 AND repo_id = $2 RETURNING id, repo_id, name, color, description`,
		strings.Join(setClauses, ", "))

	label := &models.Label{}
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&label.ID, &label.RepoID, &label.Name, &label.Color, &label.Description,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("update label: %w", err)
	}
	return label, nil
}

func (s *Service) Delete(ctx context.Context, repoID, labelID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM labels WHERE id = $1 AND repo_id = $2`, labelID, repoID)
	if err != nil {
		return fmt.Errorf("delete label: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) getByID(ctx context.Context, labelID uuid.UUID) (*models.Label, error) {
	label := &models.Label{}
	err := s.db.QueryRow(ctx, `
		SELECT id, repo_id, name, color, description FROM labels WHERE id = $1`, labelID,
	).Scan(&label.ID, &label.RepoID, &label.Name, &label.Color, &label.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query label: %w", err)
	}
	return label, nil
}
