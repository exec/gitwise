package protection

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrNotFound       = errors.New("branch protection rule not found")
	ErrDuplicate      = errors.New("branch protection rule already exists for this pattern")
	ErrInvalidPattern = errors.New("branch pattern is required")
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, repoID uuid.UUID, req models.CreateBranchProtectionRequest) (*models.BranchProtection, error) {
	pattern := strings.TrimSpace(req.BranchPattern)
	if pattern == "" || len(pattern) > 255 {
		return nil, ErrInvalidPattern
	}

	rule := &models.BranchProtection{
		ID:              uuid.New(),
		RepoID:          repoID,
		BranchPattern:   pattern,
		RequiredReviews: req.RequiredReviews,
		RequireLinear:   req.RequireLinear,
	}

	err := s.db.QueryRow(ctx, `
		INSERT INTO branch_protection_rules (id, repo_id, branch_pattern, required_reviews, require_linear)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`,
		rule.ID, rule.RepoID, rule.BranchPattern, rule.RequiredReviews, rule.RequireLinear,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("insert branch protection rule: %w", err)
	}

	return rule, nil
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID) ([]models.BranchProtection, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, branch_pattern, required_reviews, require_linear, created_at, updated_at
		FROM branch_protection_rules
		WHERE repo_id = $1
		ORDER BY branch_pattern ASC`, repoID,
	)
	if err != nil {
		return nil, fmt.Errorf("query branch protection rules: %w", err)
	}
	defer rows.Close()

	var rules []models.BranchProtection
	for rows.Next() {
		var r models.BranchProtection
		if err := rows.Scan(&r.ID, &r.RepoID, &r.BranchPattern, &r.RequiredReviews, &r.RequireLinear, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan branch protection rule: %w", err)
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate branch protection rules: %w", err)
	}
	return rules, nil
}

func (s *Service) Update(ctx context.Context, ruleID uuid.UUID, req models.UpdateBranchProtectionRequest) (*models.BranchProtection, error) {
	setClauses := []string{"updated_at = now()"}
	args := []any{ruleID}
	argIdx := 2

	if req.RequiredReviews != nil {
		setClauses = append(setClauses, fmt.Sprintf("required_reviews = $%d", argIdx))
		args = append(args, *req.RequiredReviews)
		argIdx++
	}
	if req.RequireLinear != nil {
		setClauses = append(setClauses, fmt.Sprintf("require_linear = $%d", argIdx))
		args = append(args, *req.RequireLinear)
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE branch_protection_rules SET %s
		WHERE id = $1
		RETURNING id, repo_id, branch_pattern, required_reviews, require_linear, created_at, updated_at`,
		strings.Join(setClauses, ", "))

	rule := &models.BranchProtection{}
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&rule.ID, &rule.RepoID, &rule.BranchPattern, &rule.RequiredReviews, &rule.RequireLinear, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update branch protection rule: %w", err)
	}
	return rule, nil
}

func (s *Service) Delete(ctx context.Context, ruleID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM branch_protection_rules WHERE id = $1`, ruleID)
	if err != nil {
		return fmt.Errorf("delete branch protection rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Check returns the branch protection rule for the given repo and branch name.
// Returns nil, nil if no rule matches.
func (s *Service) Check(ctx context.Context, repoID uuid.UUID, branchName string) (*models.BranchProtection, error) {
	rule := &models.BranchProtection{}
	err := s.db.QueryRow(ctx, `
		SELECT id, repo_id, branch_pattern, required_reviews, require_linear, created_at, updated_at
		FROM branch_protection_rules
		WHERE repo_id = $1 AND branch_pattern = $2`,
		repoID, branchName,
	).Scan(&rule.ID, &rule.RepoID, &rule.BranchPattern, &rule.RequiredReviews, &rule.RequireLinear, &rule.CreatedAt, &rule.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check branch protection: %w", err)
	}
	return rule, nil
}
