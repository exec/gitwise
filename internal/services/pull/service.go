package pull

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

	"github.com/gitwise-io/gitwise/internal/git"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/protection"
)

var (
	ErrNotFound        = errors.New("pull request not found")
	ErrInvalidTitle    = errors.New("title is required")
	ErrInvalidStatus   = errors.New("invalid status")
	ErrInvalidBranch   = errors.New("invalid branch")
	ErrSameBranch      = errors.New("source and target branches must differ")
	ErrAlreadyMerged   = errors.New("pull request is already merged")
	ErrNotOpen         = errors.New("pull request is not open")
	ErrMergeFailed     = errors.New("merge failed")
	ErrForbidden            = errors.New("access denied")
	ErrInsufficientReviews = errors.New("insufficient approving reviews")
	ErrLinearRequired      = errors.New("linear history required: use rebase or squash strategy")
)

type Service struct {
	db   *pgxpool.Pool
	git  *git.Service
	prot *protection.Service
}

func NewService(db *pgxpool.Pool, gitSvc *git.Service, protSvc *protection.Service) *Service {
	return &Service{db: db, git: gitSvc, prot: protSvc}
}

func (s *Service) Create(ctx context.Context, repoID, authorID uuid.UUID, ownerName, repoName string, req models.CreatePullRequestRequest) (*models.PullRequest, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" || len(title) > 500 {
		return nil, ErrInvalidTitle
	}

	if err := git.ValidateBranchName(req.SourceBranch); err != nil {
		return nil, fmt.Errorf("%w: source: %v", ErrInvalidBranch, err)
	}
	if err := git.ValidateBranchName(req.TargetBranch); err != nil {
		return nil, fmt.Errorf("%w: target: %v", ErrInvalidBranch, err)
	}
	if req.SourceBranch == req.TargetBranch {
		return nil, ErrSameBranch
	}

	// Verify both branches exist
	if _, err := s.git.ResolveRef(ownerName, repoName, req.SourceBranch); err != nil {
		return nil, fmt.Errorf("%w: source branch %q not found", ErrInvalidBranch, req.SourceBranch)
	}
	if _, err := s.git.ResolveRef(ownerName, repoName, req.TargetBranch); err != nil {
		return nil, fmt.Errorf("%w: target branch %q not found", ErrInvalidBranch, req.TargetBranch)
	}

	// Compute diff stats
	diffStats, err := s.git.CompareBranches(ownerName, repoName, req.TargetBranch, req.SourceBranch)
	if err != nil {
		return nil, fmt.Errorf("compute diff: %w", err)
	}

	statsJSON, _ := json.Marshal(map[string]any{
		"files_changed": diffStats.Stats.TotalFiles,
		"insertions":    diffStats.Stats.TotalAdditions,
		"deletions":     diffStats.Stats.TotalDeletions,
	})

	status := "open"
	if req.Draft {
		status = "draft"
	}

	var number int
	err = s.db.QueryRow(ctx, `SELECT next_repo_number($1)`, repoID).Scan(&number)
	if err != nil {
		return nil, fmt.Errorf("get next number: %w", err)
	}

	pr := &models.PullRequest{
		ID:            uuid.New(),
		RepoID:        repoID,
		Number:        number,
		AuthorID:      authorID,
		Title:         title,
		Body:          req.Body,
		SourceBranch:  req.SourceBranch,
		TargetBranch:  req.TargetBranch,
		Status:        status,
		Intent:        json.RawMessage(`{}`),
		DiffStats:     statsJSON,
		ReviewSummary: json.RawMessage(`{}`),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO pull_requests (id, repo_id, number, author_id, title, body, source_branch, target_branch, status, intent, diff_stats, review_summary, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		pr.ID, pr.RepoID, pr.Number, pr.AuthorID, pr.Title, pr.Body,
		pr.SourceBranch, pr.TargetBranch, pr.Status, pr.Intent, pr.DiffStats,
		pr.ReviewSummary, pr.CreatedAt, pr.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert PR: %w", err)
	}

	if err := s.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, authorID).Scan(&pr.AuthorName); err != nil {
		slog.Warn("PR create: author name lookup failed", "author_id", authorID, "error", err)
	}

	return pr, nil
}

func (s *Service) GetByNumber(ctx context.Context, repoID uuid.UUID, number int) (*models.PullRequest, error) {
	pr := &models.PullRequest{}
	err := s.db.QueryRow(ctx, `
		SELECT p.id, p.repo_id, p.number, p.author_id, u.username,
		       p.title, p.body, p.source_branch, p.target_branch, p.status,
		       p.intent, p.diff_stats, p.review_summary, p.merge_strategy,
		       p.merged_by, p.merged_at, p.closed_at, p.created_at, p.updated_at
		FROM pull_requests p
		JOIN users u ON u.id = p.author_id
		WHERE p.repo_id = $1 AND p.number = $2`, repoID, number,
	).Scan(
		&pr.ID, &pr.RepoID, &pr.Number, &pr.AuthorID, &pr.AuthorName,
		&pr.Title, &pr.Body, &pr.SourceBranch, &pr.TargetBranch, &pr.Status,
		&pr.Intent, &pr.DiffStats, &pr.ReviewSummary, &pr.MergeStrategy,
		&pr.MergedByID, &pr.MergedAt, &pr.ClosedAt, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query PR: %w", err)
	}

	// Populate merged_by name
	if pr.MergedByID != nil {
		s.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, *pr.MergedByID).Scan(&pr.MergedByName)
	}

	return pr, nil
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID, status string, limit int) ([]models.PullRequest, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	query := `
		SELECT p.id, p.repo_id, p.number, p.author_id, u.username,
		       p.title, p.body, p.source_branch, p.target_branch, p.status,
		       p.intent, p.diff_stats, p.review_summary, p.merge_strategy,
		       p.merged_by, p.merged_at, p.closed_at, p.created_at, p.updated_at
		FROM pull_requests p
		JOIN users u ON u.id = p.author_id
		WHERE p.repo_id = $1`

	args := []any{repoID}
	argIdx := 2

	if status != "" && isValidPRStatus(status) {
		query += fmt.Sprintf(` AND p.status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}

	query += ` ORDER BY p.created_at DESC`
	query += fmt.Sprintf(` LIMIT $%d`, argIdx)
	args = append(args, limit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query PRs: %w", err)
	}
	defer rows.Close()

	var prs []models.PullRequest
	for rows.Next() {
		var p models.PullRequest
		if err := rows.Scan(
			&p.ID, &p.RepoID, &p.Number, &p.AuthorID, &p.AuthorName,
			&p.Title, &p.Body, &p.SourceBranch, &p.TargetBranch, &p.Status,
			&p.Intent, &p.DiffStats, &p.ReviewSummary, &p.MergeStrategy,
			&p.MergedByID, &p.MergedAt, &p.ClosedAt, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan PR: %w", err)
		}
		prs = append(prs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PRs: %w", err)
	}
	return prs, nil
}

func (s *Service) Update(ctx context.Context, repoID uuid.UUID, number int, req models.UpdatePullRequestRequest) (*models.PullRequest, error) {
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
	if req.TargetBranch != nil {
		if err := git.ValidateBranchName(*req.TargetBranch); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidBranch, err)
		}
		setClauses = append(setClauses, fmt.Sprintf("target_branch = $%d", argIdx))
		args = append(args, *req.TargetBranch)
		argIdx++
	}
	if req.Status != nil {
		switch *req.Status {
		case "draft", "open":
			// valid transitions
		case "closed":
			setClauses = append(setClauses, fmt.Sprintf("closed_at = $%d", argIdx))
			args = append(args, time.Now())
			argIdx++
		default:
			return nil, ErrInvalidStatus
		}
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *req.Status)
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE pull_requests SET %s
		WHERE repo_id = $1 AND number = $2 AND status NOT IN ('merged')
		RETURNING id, repo_id, number, author_id,
		          (SELECT username FROM users WHERE id = author_id),
		          title, body, source_branch, target_branch, status,
		          intent, diff_stats, review_summary, merge_strategy,
		          merged_by, merged_at, closed_at, created_at, updated_at`,
		strings.Join(setClauses, ", "))

	pr := &models.PullRequest{}
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&pr.ID, &pr.RepoID, &pr.Number, &pr.AuthorID, &pr.AuthorName,
		&pr.Title, &pr.Body, &pr.SourceBranch, &pr.TargetBranch, &pr.Status,
		&pr.Intent, &pr.DiffStats, &pr.ReviewSummary, &pr.MergeStrategy,
		&pr.MergedByID, &pr.MergedAt, &pr.ClosedAt, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update PR: %w", err)
	}
	return pr, nil
}

func (s *Service) Merge(ctx context.Context, repoID uuid.UUID, number int, mergerID uuid.UUID, ownerName, repoName string, req models.MergePullRequestRequest) (*models.PullRequest, error) {
	// Get the PR
	pr, err := s.GetByNumber(ctx, repoID, number)
	if err != nil {
		return nil, err
	}

	if pr.Status == "merged" {
		return nil, ErrAlreadyMerged
	}
	if pr.Status != "open" {
		return nil, ErrNotOpen
	}

	strategy := req.Strategy
	if strategy == "" {
		strategy = "merge"
	}
	switch strategy {
	case "merge", "squash", "rebase":
		// valid
	default:
		return nil, fmt.Errorf("%w: strategy must be merge, squash, or rebase", ErrInvalidStatus)
	}

	// Check branch protection rules
	rule, err := s.prot.Check(ctx, repoID, pr.TargetBranch)
	if err != nil {
		return nil, fmt.Errorf("check branch protection: %w", err)
	}
	if rule != nil {
		if rule.RequiredReviews > 0 {
			var approvalCount int
			err := s.db.QueryRow(ctx,
				`SELECT COUNT(DISTINCT author_id) FROM reviews WHERE pr_id = $1 AND type = 'approval'`,
				pr.ID,
			).Scan(&approvalCount)
			if err != nil {
				return nil, fmt.Errorf("count approvals: %w", err)
			}
			if approvalCount < rule.RequiredReviews {
				return nil, fmt.Errorf("%w: need %d, have %d", ErrInsufficientReviews, rule.RequiredReviews, approvalCount)
			}
		}
		if rule.RequireLinear && strategy == "merge" {
			return nil, ErrLinearRequired
		}
	}

	// Get merger info — hard error if lookup fails (merger must exist)
	var mergerName, mergerEmail string
	if err := s.db.QueryRow(ctx, `SELECT username, email FROM users WHERE id = $1`, mergerID).Scan(&mergerName, &mergerEmail); err != nil {
		return nil, fmt.Errorf("lookup merger: %w", err)
	}

	message := req.Message
	if message == "" {
		message = fmt.Sprintf("Merge pull request #%d from %s into %s", pr.Number, pr.SourceBranch, pr.TargetBranch)
	}

	// Perform git merge
	if err := s.git.MergeBranches(ownerName, repoName, pr.TargetBranch, pr.SourceBranch, strategy, message, mergerName, mergerEmail); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMergeFailed, err)
	}

	// Update PR in database
	now := time.Now()
	err = s.db.QueryRow(ctx, `
		UPDATE pull_requests
		SET status = 'merged', merge_strategy = $3, merged_by = $4, merged_at = $5, closed_at = $5, updated_at = $5
		WHERE repo_id = $1 AND number = $2
		RETURNING id, repo_id, number, author_id,
		          (SELECT username FROM users WHERE id = author_id),
		          title, body, source_branch, target_branch, status,
		          intent, diff_stats, review_summary, merge_strategy,
		          merged_by, merged_at, closed_at, created_at, updated_at`,
		repoID, number, strategy, mergerID, now,
	).Scan(
		&pr.ID, &pr.RepoID, &pr.Number, &pr.AuthorID, &pr.AuthorName,
		&pr.Title, &pr.Body, &pr.SourceBranch, &pr.TargetBranch, &pr.Status,
		&pr.Intent, &pr.DiffStats, &pr.ReviewSummary, &pr.MergeStrategy,
		&pr.MergedByID, &pr.MergedAt, &pr.ClosedAt, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update merged PR: %w", err)
	}

	pr.MergedByName = mergerName
	return pr, nil
}

func (s *Service) GetDiff(ctx context.Context, repoID uuid.UUID, number int, ownerName, repoName string) (*models.PRDiffResponse, error) {
	pr, err := s.GetByNumber(ctx, repoID, number)
	if err != nil {
		return nil, err
	}

	return s.git.CompareBranches(ownerName, repoName, pr.TargetBranch, pr.SourceBranch)
}

func isValidPRStatus(s string) bool {
	switch s {
	case "draft", "open", "merged", "closed":
		return true
	}
	return false
}
