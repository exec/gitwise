package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/gitwise-io/gitwise/internal/git"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/comment"
	"github.com/gitwise-io/gitwise/internal/services/issue"
	"github.com/gitwise-io/gitwise/internal/services/pull"
	"github.com/gitwise-io/gitwise/internal/services/repo"
)

// ImportStatus tracks the state of an import job.
type ImportStatus struct {
	ID       string `json:"id"`
	Status   string `json:"status"`   // running, completed, failed
	Progress string `json:"progress"` // human-readable progress message
	RepoName string `json:"repo_name"`
	Error    string `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// GitHubImportRequest is the input for a GitHub import.
type GitHubImportRequest struct {
	Token      string `json:"token"`
	RepoURL    string `json:"repo_url"`
	Visibility string `json:"visibility"`
}

// GitLabImportRequest is the input for a GitLab import.
type GitLabImportRequest struct {
	Token       string `json:"token"`
	ProjectURL  string `json:"project_url"`
	InstanceURL string `json:"instance_url"`
	Visibility  string `json:"visibility"`
}

// externalIssue is an intermediate representation of an imported issue.
type externalIssue struct {
	Number    int
	Title     string
	Body      string
	State     string // open, closed
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  *time.Time
	Author    string // external username or email
	Comments  []externalComment
	IsPR      bool // GitHub uses the same number sequence for issues and PRs
}

// externalPR is an intermediate representation of an imported pull request.
type externalPR struct {
	Number       int
	Title        string
	Body         string
	State        string // open, closed, merged
	SourceBranch string
	TargetBranch string
	Labels       []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MergedAt     *time.Time
	ClosedAt     *time.Time
	Author       string
	Comments     []externalComment
}

// externalComment is an intermediate representation of an imported comment.
type externalComment struct {
	Body      string
	Author    string
	CreatedAt time.Time
}

// Service manages repository imports from external platforms.
type Service struct {
	db         *pgxpool.Pool
	rdb        *redis.Client
	gitSvc     *git.Service
	repoSvc    *repo.Service
	issueSvc   *issue.Service
	pullSvc    *pull.Service
	commentSvc *comment.Service
}

// NewService creates a new import service.
func NewService(
	db *pgxpool.Pool,
	rdb *redis.Client,
	gitSvc *git.Service,
	repoSvc *repo.Service,
	issueSvc *issue.Service,
	pullSvc *pull.Service,
	commentSvc *comment.Service,
) *Service {
	return &Service{
		db:         db,
		rdb:        rdb,
		gitSvc:     gitSvc,
		repoSvc:    repoSvc,
		issueSvc:   issueSvc,
		pullSvc:    pullSvc,
		commentSvc: commentSvc,
	}
}

const (
	statusKeyPrefix = "import:"
	statusTTL       = 24 * time.Hour
)

// GetStatus retrieves the status of an import job from Redis.
func (s *Service) GetStatus(ctx context.Context, jobID string) (*ImportStatus, error) {
	data, err := s.rdb.Get(ctx, statusKeyPrefix+jobID).Result()
	if err != nil {
		return nil, fmt.Errorf("get import status: %w", err)
	}
	var status ImportStatus
	if err := json.Unmarshal([]byte(data), &status); err != nil {
		return nil, fmt.Errorf("unmarshal import status: %w", err)
	}
	return &status, nil
}

func (s *Service) setStatus(ctx context.Context, status *ImportStatus) {
	data, err := json.Marshal(status)
	if err != nil {
		slog.Error("failed to marshal import status", "error", err)
		return
	}
	if err := s.rdb.Set(ctx, statusKeyPrefix+status.ID, string(data), statusTTL).Err(); err != nil {
		slog.Error("failed to set import status", "error", err)
	}
}

// StartGitHubImport kicks off an asynchronous GitHub import.
func (s *Service) StartGitHubImport(ctx context.Context, userID uuid.UUID, req GitHubImportRequest) (string, error) {
	owner, repoName, err := parseGitHubURL(req.RepoURL)
	if err != nil {
		return "", fmt.Errorf("parse github url: %w", err)
	}

	jobID := uuid.New().String()
	status := &ImportStatus{
		ID:       jobID,
		Status:   "running",
		Progress: "Starting GitHub import...",
		RepoName: repoName,
	}
	s.setStatus(ctx, status)

	go s.runGitHubImport(userID, jobID, req.Token, owner, repoName, req.Visibility)

	return jobID, nil
}

// StartGitLabImport kicks off an asynchronous GitLab import.
func (s *Service) StartGitLabImport(ctx context.Context, userID uuid.UUID, req GitLabImportRequest) (string, error) {
	instanceURL := req.InstanceURL
	if instanceURL == "" {
		instanceURL = "https://gitlab.com"
	}

	namespace, projectName, err := parseGitLabURL(req.ProjectURL, instanceURL)
	if err != nil {
		return "", fmt.Errorf("parse gitlab url: %w", err)
	}

	jobID := uuid.New().String()
	status := &ImportStatus{
		ID:       jobID,
		Status:   "running",
		Progress: "Starting GitLab import...",
		RepoName: projectName,
	}
	s.setStatus(ctx, status)

	go s.runGitLabImport(userID, jobID, req.Token, instanceURL, namespace, projectName, req.Visibility)

	return jobID, nil
}

func (s *Service) runGitHubImport(userID uuid.UUID, jobID, token, owner, repoName, visibility string) {
	ctx := context.Background()
	log := slog.With("job_id", jobID, "source", "github", "repo", owner+"/"+repoName)
	status := &ImportStatus{ID: jobID, Status: "running", RepoName: repoName}
	var warnings []string

	defer func() {
		if r := recover(); r != nil {
			status.Status = "failed"
			status.Error = fmt.Sprintf("panic: %v", r)
			s.setStatus(ctx, status)
		}
	}()

	gh := newGitHubClient(token)

	// 1. Fetch repo metadata
	status.Progress = "Fetching repository metadata..."
	s.setStatus(ctx, status)

	meta, err := gh.getRepo(ctx, owner, repoName)
	if err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("Failed to fetch repo metadata: %v", err)
		s.setStatus(ctx, status)
		return
	}

	// 2. Clone the repo
	status.Progress = "Cloning git repository..."
	s.setStatus(ctx, status)

	cloneURL := fmt.Sprintf("https://%s@github.com/%s/%s.git", token, owner, repoName)
	if err := s.cloneBare(ctx, userID, repoName, cloneURL); err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("Failed to clone repository: %v", err)
		s.setStatus(ctx, status)
		return
	}

	// 3. Create repo in DB
	status.Progress = "Creating repository in database..."
	s.setStatus(ctx, status)

	if visibility == "" {
		if meta.Private {
			visibility = "private"
		} else {
			visibility = "public"
		}
	}

	var ownerName string
	if err := s.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&ownerName); err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("Failed to look up user: %v", err)
		s.setStatus(ctx, status)
		return
	}

	dbRepo, err := s.createRepoInDB(ctx, userID, ownerName, repoName, meta.Description, meta.DefaultBranch, visibility, meta.Topics)
	if err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("Failed to create repo in database: %v", err)
		s.setStatus(ctx, status)
		return
	}

	// 4. Import issues
	status.Progress = "Importing issues..."
	s.setStatus(ctx, status)

	issues, err := gh.listIssues(ctx, owner, repoName)
	if err != nil {
		log.Warn("failed to import issues", "error", err)
		warnings = append(warnings, fmt.Sprintf("Failed to import issues: %v", err))
	} else {
		for i, iss := range issues {
			if iss.IsPR {
				continue // GitHub returns PRs in the issues endpoint; skip them
			}
			status.Progress = fmt.Sprintf("Importing issues (%d/%d)...", i+1, len(issues))
			s.setStatus(ctx, status)

			if err := s.importIssue(ctx, dbRepo.ID, userID, iss); err != nil {
				log.Warn("failed to import issue", "number", iss.Number, "error", err)
				warnings = append(warnings, fmt.Sprintf("Issue #%d: %v", iss.Number, err))
			}
		}
	}

	// 5. Import PRs
	status.Progress = "Importing pull requests..."
	s.setStatus(ctx, status)

	prs, err := gh.listPullRequests(ctx, owner, repoName)
	if err != nil {
		log.Warn("failed to import pull requests", "error", err)
		warnings = append(warnings, fmt.Sprintf("Failed to import pull requests: %v", err))
	} else {
		for i, pr := range prs {
			status.Progress = fmt.Sprintf("Importing pull requests (%d/%d)...", i+1, len(prs))
			s.setStatus(ctx, status)

			if err := s.importPR(ctx, dbRepo.ID, userID, ownerName, repoName, pr); err != nil {
				log.Warn("failed to import PR", "number", pr.Number, "error", err)
				warnings = append(warnings, fmt.Sprintf("PR #%d: %v", pr.Number, err))
			}
		}
	}

	status.Status = "completed"
	status.Progress = "Import complete"
	status.Warnings = warnings
	s.setStatus(ctx, status)
	log.Info("github import completed", "repo", repoName, "warnings", len(warnings))
}

func (s *Service) runGitLabImport(userID uuid.UUID, jobID, token, instanceURL, namespace, projectName, visibility string) {
	ctx := context.Background()
	log := slog.With("job_id", jobID, "source", "gitlab", "project", namespace+"/"+projectName)
	status := &ImportStatus{ID: jobID, Status: "running", RepoName: projectName}
	var warnings []string

	defer func() {
		if r := recover(); r != nil {
			status.Status = "failed"
			status.Error = fmt.Sprintf("panic: %v", r)
			s.setStatus(ctx, status)
		}
	}()

	gl := newGitLabClient(token, instanceURL)

	// 1. Fetch project metadata
	status.Progress = "Fetching project metadata..."
	s.setStatus(ctx, status)

	meta, err := gl.getProject(ctx, namespace, projectName)
	if err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("Failed to fetch project metadata: %v", err)
		s.setStatus(ctx, status)
		return
	}

	// 2. Clone the repo
	status.Progress = "Cloning git repository..."
	s.setStatus(ctx, status)

	// Use the HTTP clone URL with token auth
	cloneURL := fmt.Sprintf("%s/%s/%s.git", instanceURL, namespace, projectName)
	cloneURL = strings.Replace(cloneURL, "://", fmt.Sprintf("://oauth2:%s@", token), 1)
	if err := s.cloneBare(ctx, userID, projectName, cloneURL); err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("Failed to clone repository: %v", err)
		s.setStatus(ctx, status)
		return
	}

	// 3. Create repo in DB
	status.Progress = "Creating repository in database..."
	s.setStatus(ctx, status)

	if visibility == "" {
		switch meta.Visibility {
		case "private", "internal":
			visibility = "private"
		default:
			visibility = "public"
		}
	}

	var ownerName string
	if err := s.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&ownerName); err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("Failed to look up user: %v", err)
		s.setStatus(ctx, status)
		return
	}

	dbRepo, err := s.createRepoInDB(ctx, userID, ownerName, projectName, meta.Description, meta.DefaultBranch, visibility, meta.Topics)
	if err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("Failed to create repo in database: %v", err)
		s.setStatus(ctx, status)
		return
	}

	// 4. Import issues
	status.Progress = "Importing issues..."
	s.setStatus(ctx, status)

	issues, err := gl.listIssues(ctx, meta.ID)
	if err != nil {
		log.Warn("failed to import issues", "error", err)
		warnings = append(warnings, fmt.Sprintf("Failed to import issues: %v", err))
	} else {
		for i, iss := range issues {
			status.Progress = fmt.Sprintf("Importing issues (%d/%d)...", i+1, len(issues))
			s.setStatus(ctx, status)

			if err := s.importIssue(ctx, dbRepo.ID, userID, iss); err != nil {
				log.Warn("failed to import issue", "number", iss.Number, "error", err)
				warnings = append(warnings, fmt.Sprintf("Issue #%d: %v", iss.Number, err))
			}
		}
	}

	// 5. Import merge requests as PRs
	status.Progress = "Importing merge requests..."
	s.setStatus(ctx, status)

	mrs, err := gl.listMergeRequests(ctx, meta.ID)
	if err != nil {
		log.Warn("failed to import merge requests", "error", err)
		warnings = append(warnings, fmt.Sprintf("Failed to import merge requests: %v", err))
	} else {
		for i, mr := range mrs {
			status.Progress = fmt.Sprintf("Importing merge requests (%d/%d)...", i+1, len(mrs))
			s.setStatus(ctx, status)

			if err := s.importPR(ctx, dbRepo.ID, userID, ownerName, projectName, mr); err != nil {
				log.Warn("failed to import MR", "number", mr.Number, "error", err)
				warnings = append(warnings, fmt.Sprintf("MR !%d: %v", mr.Number, err))
			}
		}
	}

	status.Status = "completed"
	status.Progress = "Import complete"
	status.Warnings = warnings
	s.setStatus(ctx, status)
	log.Info("gitlab import completed", "project", projectName, "warnings", len(warnings))
}

// cloneBare clones a remote repository as a bare repo into the gitwise repos directory.
func (s *Service) cloneBare(ctx context.Context, userID uuid.UUID, repoName, cloneURL string) error {
	var ownerName string
	if err := s.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&ownerName); err != nil {
		return fmt.Errorf("lookup owner: %w", err)
	}

	destPath := s.gitSvc.RepoPath(ownerName, repoName)

	cmd := exec.CommandContext(ctx, "git", "clone", "--bare", cloneURL, destPath)
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone --bare: %w: %s", err, string(output))
	}
	return nil
}

// createRepoInDB creates the repository record in the database. The bare repo
// must already exist on disk (created by cloneBare).
func (s *Service) createRepoInDB(ctx context.Context, ownerID uuid.UUID, ownerName, repoName, description, defaultBranch, visibility string, topics []string) (*models.Repository, error) {
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	if topics == nil {
		topics = []string{}
	}

	repoID := uuid.New()
	now := time.Now()

	dbRepo := &models.Repository{
		ID:            repoID,
		OwnerID:       ownerID,
		OwnerName:     ownerName,
		Name:          repoName,
		Description:   description,
		DefaultBranch: defaultBranch,
		Visibility:    visibility,
		LanguageStats: json.RawMessage(`{}`),
		Topics:        topics,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO repositories (id, owner_id, owner_type, name, description, default_branch, visibility, language_stats, topics, created_at, updated_at)
		VALUES ($1, $2, 'user', $3, $4, $5, $6, $7, $8, $9, $10)`,
		dbRepo.ID, dbRepo.OwnerID, dbRepo.Name, dbRepo.Description, dbRepo.DefaultBranch,
		dbRepo.Visibility, dbRepo.LanguageStats, dbRepo.Topics, dbRepo.CreatedAt, dbRepo.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert repo: %w", err)
	}

	// Initialize the counter for issue/PR number sequence
	s.db.Exec(ctx, `INSERT INTO repo_counters (repo_id) VALUES ($1) ON CONFLICT DO NOTHING`, repoID)

	return dbRepo, nil
}

// importIssue creates an issue in the database from an external issue.
func (s *Service) importIssue(ctx context.Context, repoID, authorID uuid.UUID, ext externalIssue) error {
	// Reserve the next number from the shared issue/PR counter
	var number int
	if err := s.db.QueryRow(ctx, `SELECT next_repo_number($1)`, repoID).Scan(&number); err != nil {
		return fmt.Errorf("get next number: %w", err)
	}

	status := "open"
	if ext.State == "closed" {
		status = "closed"
	}

	labels := ext.Labels
	if labels == nil {
		labels = []string{}
	}

	body := ext.Body
	if ext.Author != "" {
		body = fmt.Sprintf("*Originally posted by @%s on %s*\n\n%s", ext.Author, ext.CreatedAt.Format("Jan 2, 2006"), body)
	}

	issueID := uuid.New()
	now := time.Now()

	_, err := s.db.Exec(ctx, `
		INSERT INTO issues (id, repo_id, number, author_id, title, body, status, labels, assignees, linked_prs, priority, metadata, closed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		issueID, repoID, number, authorID, ext.Title, body, status,
		labels, []uuid.UUID{}, []uuid.UUID{}, "none", json.RawMessage(`{}`),
		ext.ClosedAt, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert issue: %w", err)
	}

	// Import comments
	for _, c := range ext.Comments {
		commentBody := c.Body
		if c.Author != "" {
			commentBody = fmt.Sprintf("*Originally posted by @%s on %s*\n\n%s", c.Author, c.CreatedAt.Format("Jan 2, 2006"), commentBody)
		}

		_, err := s.db.Exec(ctx, `
			INSERT INTO comments (id, repo_id, issue_id, author_id, body, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			uuid.New(), repoID, issueID, authorID, commentBody, time.Now(), time.Now(),
		)
		if err != nil {
			slog.Warn("failed to import issue comment", "issue", ext.Number, "error", err)
		}
	}

	return nil
}

// importPR creates a pull request in the database from an external PR/MR.
func (s *Service) importPR(ctx context.Context, repoID, authorID uuid.UUID, ownerName, repoName string, ext externalPR) error {
	var number int
	if err := s.db.QueryRow(ctx, `SELECT next_repo_number($1)`, repoID).Scan(&number); err != nil {
		return fmt.Errorf("get next number: %w", err)
	}

	status := "open"
	switch ext.State {
	case "closed":
		status = "closed"
	case "merged":
		status = "merged"
	}

	body := ext.Body
	if ext.Author != "" {
		body = fmt.Sprintf("*Originally posted by @%s on %s*\n\n%s", ext.Author, ext.CreatedAt.Format("Jan 2, 2006"), body)
	}

	prID := uuid.New()
	now := time.Now()

	intentJSON := json.RawMessage(`{}`)
	diffStats := json.RawMessage(`{}`)
	reviewSummary := json.RawMessage(`{}`)

	_, err := s.db.Exec(ctx, `
		INSERT INTO pull_requests (id, repo_id, number, author_id, title, body, source_branch, target_branch, status, intent, diff_stats, review_summary, merged_at, closed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		prID, repoID, number, authorID, ext.Title, body,
		ext.SourceBranch, ext.TargetBranch, status,
		intentJSON, diffStats, reviewSummary,
		ext.MergedAt, ext.ClosedAt, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert PR: %w", err)
	}

	// Import comments
	for _, c := range ext.Comments {
		commentBody := c.Body
		if c.Author != "" {
			commentBody = fmt.Sprintf("*Originally posted by @%s on %s*\n\n%s", c.Author, c.CreatedAt.Format("Jan 2, 2006"), commentBody)
		}

		_, err := s.db.Exec(ctx, `
			INSERT INTO comments (id, repo_id, pr_id, author_id, body, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			uuid.New(), repoID, prID, authorID, commentBody, time.Now(), time.Now(),
		)
		if err != nil {
			slog.Warn("failed to import PR comment", "pr", ext.Number, "error", err)
		}
	}

	return nil
}

// parseGitHubURL extracts owner and repo from a GitHub URL.
// Accepts: https://github.com/owner/repo or owner/repo
func parseGitHubURL(rawURL string) (string, string, error) {
	rawURL = strings.TrimSuffix(rawURL, ".git")
	rawURL = strings.TrimSuffix(rawURL, "/")

	// Try full URL
	if strings.Contains(rawURL, "github.com") {
		parts := strings.Split(rawURL, "github.com/")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid GitHub URL: %s", rawURL)
		}
		segments := strings.SplitN(parts[1], "/", 3)
		if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
			return "", "", fmt.Errorf("invalid GitHub URL: expected owner/repo")
		}
		return segments[0], segments[1], nil
	}

	// Try owner/repo format
	segments := strings.SplitN(rawURL, "/", 3)
	if len(segments) == 2 && segments[0] != "" && segments[1] != "" {
		return segments[0], segments[1], nil
	}

	return "", "", fmt.Errorf("invalid GitHub URL or owner/repo: %s", rawURL)
}

// parseGitLabURL extracts namespace and project name from a GitLab URL.
// Accepts: https://gitlab.com/namespace/project, https://custom.gitlab.com/namespace/project, or namespace/project
func parseGitLabURL(rawURL, instanceURL string) (string, string, error) {
	rawURL = strings.TrimSuffix(rawURL, ".git")
	rawURL = strings.TrimSuffix(rawURL, "/")

	// Extract the host from instanceURL for comparison
	instanceHost := strings.TrimPrefix(instanceURL, "https://")
	instanceHost = strings.TrimPrefix(instanceHost, "http://")
	instanceHost = strings.TrimSuffix(instanceHost, "/")

	// Try full URL
	if strings.Contains(rawURL, instanceHost) {
		idx := strings.Index(rawURL, instanceHost)
		path := rawURL[idx+len(instanceHost):]
		path = strings.TrimPrefix(path, "/")
		if path == "" {
			return "", "", fmt.Errorf("invalid GitLab URL: no project path")
		}
		// GitLab namespaces can be nested (group/subgroup/project)
		lastSlash := strings.LastIndex(path, "/")
		if lastSlash < 0 {
			return "", "", fmt.Errorf("invalid GitLab URL: expected namespace/project")
		}
		return path[:lastSlash], path[lastSlash+1:], nil
	}

	// Try namespace/project format
	lastSlash := strings.LastIndex(rawURL, "/")
	if lastSlash > 0 {
		return rawURL[:lastSlash], rawURL[lastSlash+1:], nil
	}

	return "", "", fmt.Errorf("invalid GitLab URL or namespace/project: %s", rawURL)
}
