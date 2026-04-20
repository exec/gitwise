package mirror

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrInvalidDirection = errors.New("mirror: invalid direction")
	ErrInvalidTarget    = errors.New("mirror: github_owner and github_repo required")
	ErrInvalidInterval  = errors.New("mirror: interval must be >= 0")
	ErrMirrorNotFound   = errors.New("mirror: not configured for repo")
	ErrPATRequired      = errors.New("mirror: PAT required for push direction")
)

type Service struct {
	db       *pgxpool.Pool
	crypto   *Crypto
	remote   *Remote
	reposDir string // absolute path; bare repos live at <reposDir>/<repo-id>.git

	mu    sync.Mutex
	locks map[uuid.UUID]*sync.Mutex // per-repo serialization for sync operations
}

func NewService(db *pgxpool.Pool, crypto *Crypto, remote *Remote, reposDir string) *Service {
	return &Service{
		db:       db,
		crypto:   crypto,
		remote:   remote,
		reposDir: reposDir,
		locks:    map[uuid.UUID]*sync.Mutex{},
	}
}

func (s *Service) Configure(ctx context.Context, repoID uuid.UUID, req models.ConfigureMirrorRequest) (*models.RepoMirror, error) {
	if req.Direction != models.MirrorPush && req.Direction != models.MirrorPull {
		return nil, ErrInvalidDirection
	}
	if req.GithubOwner == "" || req.GithubRepo == "" {
		return nil, ErrInvalidTarget
	}
	if req.IntervalSeconds < 0 {
		return nil, ErrInvalidInterval
	}
	if req.Direction == models.MirrorPush && req.PAT == "" {
		// For push, we need write access. If no existing PAT stored, require one.
		existing, _ := s.Get(ctx, repoID)
		if existing == nil || !existing.HasPAT {
			return nil, ErrPATRequired
		}
	}

	var ciphertext, nonce []byte
	var err error
	switch {
	case req.ClearPAT:
		ciphertext, nonce = nil, nil
	case req.PAT != "":
		ciphertext, nonce, err = s.crypto.Seal([]byte(req.PAT))
		if err != nil {
			return nil, fmt.Errorf("mirror: seal pat: %w", err)
		}
	}

	// Upsert. Positional args 1..9.
	//   $1 repo_id           $6 pat_nonce
	//   $2 direction         $7 interval_seconds
	//   $3 github_owner      $8 auto_push
	//   $4 github_repo       $9 clear_pat (bool)
	//   $5 pat_ciphertext
	var row models.RepoMirror
	err = s.db.QueryRow(ctx, `
		INSERT INTO repo_mirrors
		    (repo_id, direction, github_owner, github_repo,
		     pat_ciphertext, pat_nonce, interval_seconds, auto_push,
		     last_status, next_run_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending', now(), now())
		ON CONFLICT (repo_id) DO UPDATE SET
		    direction        = EXCLUDED.direction,
		    github_owner     = EXCLUDED.github_owner,
		    github_repo      = EXCLUDED.github_repo,
		    pat_ciphertext   = CASE
		                         WHEN $9::bool THEN NULL
		                         WHEN $5::bytea IS NOT NULL THEN EXCLUDED.pat_ciphertext
		                         ELSE repo_mirrors.pat_ciphertext
		                       END,
		    pat_nonce        = CASE
		                         WHEN $9::bool THEN NULL
		                         WHEN $6::bytea IS NOT NULL THEN EXCLUDED.pat_nonce
		                         ELSE repo_mirrors.pat_nonce
		                       END,
		    interval_seconds = EXCLUDED.interval_seconds,
		    auto_push        = EXCLUDED.auto_push,
		    updated_at       = now()
		RETURNING repo_id, direction, github_owner, github_repo,
		          (pat_ciphertext IS NOT NULL), interval_seconds, auto_push,
		          last_status, COALESCE(last_error, ''), last_synced_at, next_run_at,
		          created_at, updated_at`,
		repoID, req.Direction, req.GithubOwner, req.GithubRepo,
		ciphertext, nonce, req.IntervalSeconds, req.AutoPush,
		req.ClearPAT,
	).Scan(&row.RepoID, &row.Direction, &row.GithubOwner, &row.GithubRepo,
		&row.HasPAT, &row.IntervalSeconds, &row.AutoPush,
		&row.LastStatus, &row.LastError, &row.LastSyncedAt, &row.NextRunAt,
		&row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("mirror: upsert: %w", err)
	}
	return &row, nil
}

func (s *Service) Get(ctx context.Context, repoID uuid.UUID) (*models.RepoMirror, error) {
	var row models.RepoMirror
	err := s.db.QueryRow(ctx, `
		SELECT repo_id, direction, github_owner, github_repo,
		       (pat_ciphertext IS NOT NULL), interval_seconds, auto_push,
		       last_status, COALESCE(last_error, ''), last_synced_at, next_run_at,
		       created_at, updated_at
		FROM repo_mirrors WHERE repo_id = $1`, repoID,
	).Scan(&row.RepoID, &row.Direction, &row.GithubOwner, &row.GithubRepo,
		&row.HasPAT, &row.IntervalSeconds, &row.AutoPush,
		&row.LastStatus, &row.LastError, &row.LastSyncedAt, &row.NextRunAt,
		&row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMirrorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mirror: get: %w", err)
	}
	return &row, nil
}

func (s *Service) Remove(ctx context.Context, repoID uuid.UUID) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM repo_mirrors WHERE repo_id = $1`, repoID); err != nil {
		return fmt.Errorf("mirror: remove: %w", err)
	}
	return nil
}

func (s *Service) ListRuns(ctx context.Context, repoID uuid.UUID, limit int) ([]models.RepoMirrorRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, started_at, finished_at, status, trigger,
		       refs_changed, COALESCE(error, ''), duration_ms
		FROM repo_mirror_runs WHERE repo_id = $1
		ORDER BY started_at DESC LIMIT $2`, repoID, limit)
	if err != nil {
		return nil, fmt.Errorf("mirror: list runs: %w", err)
	}
	defer rows.Close()

	var out []models.RepoMirrorRun
	for rows.Next() {
		var r models.RepoMirrorRun
		if err := rows.Scan(&r.ID, &r.RepoID, &r.StartedAt, &r.FinishedAt, &r.Status, &r.Trigger,
			&r.RefsChanged, &r.Error, &r.DurationMs); err != nil {
			return nil, fmt.Errorf("mirror: scan run: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mirror: list runs: %w", err)
	}
	return out, nil
}

func (s *Service) decryptPAT(ciphertext, nonce []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}
	pt, err := s.crypto.Open(ciphertext, nonce)
	if err != nil {
		return "", fmt.Errorf("mirror: decrypt pat: %w", err)
	}
	return string(pt), nil
}

func (s *Service) lockFor(repoID uuid.UUID) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.locks[repoID]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.locks[repoID] = m
	return m
}

func (s *Service) repoPath(repoID uuid.UUID) string {
	return fmt.Sprintf("%s/%s.git", s.reposDir, repoID.String())
}

// nowUTC is used by later tasks for deterministic timestamping.
func nowUTC() time.Time { return time.Now().UTC() }
