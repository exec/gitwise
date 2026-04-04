package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/git"
	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrNotFound    = errors.New("repository not found")
	ErrDuplicate   = errors.New("repository already exists")
	ErrInvalidName = errors.New("invalid repository name")
	ErrForbidden   = errors.New("access denied")
)

var repoNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,98}[a-zA-Z0-9]$`)

type Service struct {
	db  *pgxpool.Pool
	git *git.Service
}

func NewService(db *pgxpool.Pool, gitSvc *git.Service) *Service {
	return &Service{db: db, git: gitSvc}
}

func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, req models.CreateRepoRequest) (*models.Repository, error) {
	if !repoNameRe.MatchString(req.Name) {
		return nil, ErrInvalidName
	}

	if req.Visibility == "" {
		req.Visibility = "public"
	}
	if req.DefaultBranch == "" {
		req.DefaultBranch = "main"
	}

	// Look up owner username
	var ownerName string
	err := s.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, ownerID).Scan(&ownerName)
	if err != nil {
		return nil, fmt.Errorf("lookup owner: %w", err)
	}

	repo := &models.Repository{
		ID:            uuid.New(),
		OwnerID:       ownerID,
		OwnerName:     ownerName,
		Name:          req.Name,
		Description:   req.Description,
		DefaultBranch: req.DefaultBranch,
		Visibility:    req.Visibility,
		LanguageStats: json.RawMessage(`{}`),
		Topics:        req.Topics,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if repo.Topics == nil {
		repo.Topics = []string{}
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO repositories (id, owner_id, owner_type, name, description, default_branch, visibility, language_stats, topics, created_at, updated_at)
		VALUES ($1, $2, 'user', $3, $4, $5, $6, $7, $8, $9, $10)`,
		repo.ID, repo.OwnerID, repo.Name, repo.Description, repo.DefaultBranch,
		repo.Visibility, repo.LanguageStats, repo.Topics, repo.CreatedAt, repo.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("insert repo: %w", err)
	}

	// Initialize counter
	s.db.Exec(ctx, `INSERT INTO repo_counters (repo_id) VALUES ($1) ON CONFLICT DO NOTHING`, repo.ID)

	// Init bare git repo on disk
	if err := s.git.InitBare(ownerName, req.Name); err != nil {
		return nil, fmt.Errorf("init bare repo: %w", err)
	}

	if req.AutoInit {
		if err := s.git.AutoInit(ownerName, req.Name, req.DefaultBranch); err != nil {
			slog.Error("auto-init failed", "repo", ownerName+"/"+req.Name, "error", err)
		}
	}

	return repo, nil
}

func (s *Service) GetByOwnerAndName(ctx context.Context, owner, name string) (*models.Repository, error) {
	repo := &models.Repository{}
	err := s.db.QueryRow(ctx, `
		SELECT r.id, r.owner_id, u.username, r.name, r.description, r.default_branch,
		       r.visibility, r.language_stats, r.topics, r.stars_count, r.forks_count,
		       r.created_at, r.updated_at
		FROM repositories r
		JOIN users u ON u.id = r.owner_id
		WHERE u.username = $1 AND r.name = $2`, strings.ToLower(owner), name,
	).Scan(
		&repo.ID, &repo.OwnerID, &repo.OwnerName, &repo.Name, &repo.Description,
		&repo.DefaultBranch, &repo.Visibility, &repo.LanguageStats, &repo.Topics,
		&repo.StarsCount, &repo.ForksCount, &repo.CreatedAt, &repo.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query repo: %w", err)
	}
	return repo, nil
}

func (s *Service) ListByOwner(ctx context.Context, owner string, limit int) ([]models.Repository, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	rows, err := s.db.Query(ctx, `
		SELECT r.id, r.owner_id, u.username, r.name, r.description, r.default_branch,
		       r.visibility, r.language_stats, r.topics, r.stars_count, r.forks_count,
		       r.created_at, r.updated_at
		FROM repositories r
		JOIN users u ON u.id = r.owner_id
		WHERE u.username = $1
		ORDER BY r.updated_at DESC
		LIMIT $2`, strings.ToLower(owner), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query repos: %w", err)
	}
	defer rows.Close()

	var repos []models.Repository
	for rows.Next() {
		var r models.Repository
		if err := rows.Scan(
			&r.ID, &r.OwnerID, &r.OwnerName, &r.Name, &r.Description,
			&r.DefaultBranch, &r.Visibility, &r.LanguageStats, &r.Topics,
			&r.StarsCount, &r.ForksCount, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repos: %w", err)
	}
	return repos, nil
}

func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID, limit int) ([]models.Repository, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	rows, err := s.db.Query(ctx, `
		SELECT r.id, r.owner_id, u.username, r.name, r.description, r.default_branch,
		       r.visibility, r.language_stats, r.topics, r.stars_count, r.forks_count,
		       r.created_at, r.updated_at
		FROM repositories r
		JOIN users u ON u.id = r.owner_id
		WHERE r.owner_id = $1
		ORDER BY r.updated_at DESC
		LIMIT $2`, userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query repos: %w", err)
	}
	defer rows.Close()

	var repos []models.Repository
	for rows.Next() {
		var r models.Repository
		if err := rows.Scan(
			&r.ID, &r.OwnerID, &r.OwnerName, &r.Name, &r.Description,
			&r.DefaultBranch, &r.Visibility, &r.LanguageStats, &r.Topics,
			&r.StarsCount, &r.ForksCount, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repos: %w", err)
	}
	return repos, nil
}

func (s *Service) Update(ctx context.Context, repoID uuid.UUID, req models.UpdateRepoRequest) (*models.Repository, error) {
	setClauses := []string{"updated_at = now()"}
	args := []any{repoID}
	argIdx := 2

	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.Visibility != nil {
		setClauses = append(setClauses, fmt.Sprintf("visibility = $%d", argIdx))
		args = append(args, *req.Visibility)
		argIdx++
	}
	if req.DefaultBranch != nil {
		setClauses = append(setClauses, fmt.Sprintf("default_branch = $%d", argIdx))
		args = append(args, *req.DefaultBranch)
		argIdx++
	}
	if req.Topics != nil {
		setClauses = append(setClauses, fmt.Sprintf("topics = $%d", argIdx))
		args = append(args, req.Topics)
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE repositories SET %s WHERE id = $1
		RETURNING id, owner_id, name, description, default_branch, visibility, language_stats, topics,
		          stars_count, forks_count, created_at, updated_at`,
		strings.Join(setClauses, ", "))

	repo := &models.Repository{}
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&repo.ID, &repo.OwnerID, &repo.Name, &repo.Description,
		&repo.DefaultBranch, &repo.Visibility, &repo.LanguageStats, &repo.Topics,
		&repo.StarsCount, &repo.ForksCount, &repo.CreatedAt, &repo.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update repo: %w", err)
	}
	return repo, nil
}

func (s *Service) Delete(ctx context.Context, ownerName string, repoName string, repoID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM repositories WHERE id = $1`, repoID)
	if err != nil {
		return fmt.Errorf("delete repo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	// Remove from disk
	s.git.Remove(ownerName, repoName)
	return nil
}
