package mirror

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
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

// SyncResult captures the outcome of a single sync, as persisted in repo_mirror_runs
// and returned to API callers (manual sync, admin page).
type SyncResult struct {
	RepoID      uuid.UUID
	RunID       uuid.UUID
	Status      models.MirrorStatus
	RefsChanged int
	Duration    time.Duration
	Error       string
}

// SyncNow runs a single sync for the given repo, blocking until done.
// Grabs the per-repo mutex so concurrent SyncNow/RunDue calls on the same
// repo queue sequentially.
func (s *Service) SyncNow(ctx context.Context, repoID uuid.UUID, trigger models.MirrorTrigger) (*SyncResult, error) {
	lock := s.lockFor(repoID)
	lock.Lock()
	defer lock.Unlock()

	// Load direction, target, and encrypted PAT in a single query.
	var (
		direction        models.MirrorDirection
		owner, repo      string
		ciphertext, nonce []byte
	)
	err := s.db.QueryRow(ctx, `
		SELECT direction, github_owner, github_repo, pat_ciphertext, pat_nonce
		FROM repo_mirrors WHERE repo_id = $1`, repoID,
	).Scan(&direction, &owner, &repo, &ciphertext, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMirrorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mirror: load for sync: %w", err)
	}

	runID, err := s.startRun(ctx, repoID, trigger)
	if err != nil {
		return nil, fmt.Errorf("mirror: start run: %w", err)
	}
	start := nowUTC()

	pat, err := s.decryptPAT(ciphertext, nonce)
	if err != nil {
		return s.finishRun(ctx, runID, repoID, 0, models.MirrorFailed, err.Error(), start)
	}

	remoteURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	localPath := s.repoPath(repoID)

	var outcome SyncOutcome
	switch direction {
	case models.MirrorPush:
		outcome, err = s.remote.PushMirror(ctx, localPath, remoteURL, pat)
	case models.MirrorPull:
		outcome, err = s.remote.FetchMirror(ctx, localPath, remoteURL, pat)
		if err == nil {
			if branch, lsErr := s.remote.LsRemoteDefault(ctx, remoteURL, pat); lsErr == nil {
				_ = s.remote.SetHead(ctx, localPath, branch)
			}
		}
	default:
		err = fmt.Errorf("mirror: unknown direction %q", direction)
	}

	if err != nil {
		return s.finishRun(ctx, runID, repoID, outcome.RefsChanged, models.MirrorFailed, err.Error(), start)
	}
	return s.finishRun(ctx, runID, repoID, outcome.RefsChanged, models.MirrorSuccess, "", start)
}

// startRun inserts a 'running' row in repo_mirror_runs and flips the mirror's
// last_status atomically. Returns the new run's id.
func (s *Service) startRun(ctx context.Context, repoID uuid.UUID, trigger models.MirrorTrigger) (uuid.UUID, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO repo_mirror_runs (repo_id, status, trigger)
		VALUES ($1, 'running', $2) RETURNING id`, repoID, trigger,
	).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE repo_mirrors
		SET last_status = 'running', updated_at = now()
		WHERE repo_id = $1`, repoID); err != nil {
		return uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// finishRun writes the terminal state of a run row and updates the mirror's
// last_status, last_error, last_synced_at, next_run_at. Also prunes history to
// the last 50 runs per repo.
//
// Uses a detached context so a cancelled caller context (e.g. server shutdown
// mid-sync, HTTP client disconnect) does not abort the cleanup writes — otherwise
// the 'running' row would linger until ReapStuck (T6) collects it. The detached
// context is bounded by a 30s timeout so a wedged DB still unblocks the caller.
func (s *Service) finishRun(
	ctx context.Context, runID, repoID uuid.UUID,
	refsChanged int, status models.MirrorStatus, errMsg string,
	start time.Time,
) (*SyncResult, error) {
	dur := time.Since(start)
	durMs := int(dur.Milliseconds())

	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	if _, err := s.db.Exec(writeCtx, `
		UPDATE repo_mirror_runs
		SET finished_at = now(), status = $1, refs_changed = $2, error = $3, duration_ms = $4
		WHERE id = $5`, status, refsChanged, errPtr, durMs, runID); err != nil {
		return nil, fmt.Errorf("mirror: finish run: %w", err)
	}
	if _, err := s.db.Exec(writeCtx, `
		UPDATE repo_mirrors
		SET last_status    = $1,
		    last_error     = $2,
		    last_synced_at = CASE WHEN $1 = 'success' THEN now() ELSE last_synced_at END,
		    next_run_at    = CASE
		                       WHEN interval_seconds > 0 THEN now() + (interval_seconds || ' seconds')::interval
		                       ELSE NULL
		                     END,
		    updated_at     = now()
		WHERE repo_id = $3`, status, errPtr, repoID); err != nil {
		return nil, fmt.Errorf("mirror: update mirror state: %w", err)
	}
	// Retention: keep last 50 runs per repo. Ignore error — not critical.
	_, _ = s.db.Exec(writeCtx, `
		DELETE FROM repo_mirror_runs
		WHERE repo_id = $1
		  AND id NOT IN (
		    SELECT id FROM repo_mirror_runs WHERE repo_id = $1
		    ORDER BY started_at DESC LIMIT 50
		  )`, repoID)

	return &SyncResult{
		RepoID:      repoID,
		RunID:       runID,
		Status:      status,
		RefsChanged: refsChanged,
		Duration:    dur,
		Error:       errMsg,
	}, nil
}

const (
	// runDueSyncTimeout is the per-sync timeout applied to each RunDue goroutine.
	runDueSyncTimeout = 10 * time.Minute
	// runDueMaxInflight caps concurrent goroutines spawned by RunDue.
	runDueMaxInflight = 5
)

// runDueMu is a tryLock guard so that if a previous RunDue tick's goroutines
// are still running, the new tick skips entirely rather than piling on.
var runDueMu sync.Mutex

// RunDue dispatches all mirrors whose next_run_at has passed, up to 50 per call.
// Each mirror is synced concurrently, bounded by runDueMaxInflight. Each sync is
// wrapped in a per-sync timeout (runDueSyncTimeout). If a previous RunDue call
// has not completed, the new tick is skipped.
// The call blocks until every dispatched sync has finished.
func (s *Service) RunDue(ctx context.Context) []SyncResult {
	if !runDueMu.TryLock() {
		slog.Warn("mirror: RunDue skipped — previous tick still running")
		return nil
	}
	defer runDueMu.Unlock()

	rows, err := s.db.Query(ctx, `
		SELECT repo_id FROM repo_mirrors
		WHERE interval_seconds > 0
		  AND last_status <> 'running'
		  AND (next_run_at IS NULL OR next_run_at <= now())
		LIMIT 50`)
	if err != nil {
		slog.Error("mirror: RunDue query failed", "error", err)
		return nil
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			slog.Warn("mirror: RunDue scan failed", "error", err)
			continue
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		slog.Warn("mirror: RunDue iterate failed", "error", err)
	}

	if len(ids) == 0 {
		return nil
	}

	sem := make(chan struct{}, runDueMaxInflight)
	var wg sync.WaitGroup
	resultsCh := make(chan SyncResult, len(ids))
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(id uuid.UUID) {
			defer wg.Done()
			defer func() { <-sem }()
			// Each sync gets its own bounded timeout.
			syncCtx, cancel := context.WithTimeout(ctx, runDueSyncTimeout)
			defer cancel()
			r, err := s.SyncNow(syncCtx, id, models.MirrorTriggerScheduled)
			switch {
			case err != nil:
				// Surface failures in the aggregate result so the worker can log them.
				resultsCh <- SyncResult{RepoID: id, Status: models.MirrorFailed, Error: err.Error()}
			case r != nil:
				resultsCh <- *r
			}
		}(id)
	}
	wg.Wait()
	close(resultsCh)

	results := make([]SyncResult, 0, len(ids))
	for r := range resultsCh {
		results = append(results, r)
	}
	return results
}

// ReapStuck marks any run that has been 'running' longer than
// max(interval_seconds, 600s) * 3 as failed, and flips the corresponding
// mirror's last_status back to 'failed' so the scheduler picks it up again.
// next_run_at is intentionally not advanced — the sync will re-run on the
// next tick since next_run_at was already in the past when the stuck run
// began. Returns the number of mirrors reset (not runs reaped; these match
// in normal operation since one run-per-repo is in flight at a time).
func (s *Service) ReapStuck(ctx context.Context) (int, error) {
	tag, err := s.db.Exec(ctx, `
		WITH stuck AS (
		    SELECT mr.id, mr.repo_id
		    FROM repo_mirror_runs mr
		    JOIN repo_mirrors rm ON rm.repo_id = mr.repo_id
		    WHERE mr.status = 'running'
		      AND mr.started_at < now() - (GREATEST(rm.interval_seconds, 600) * 3 || ' seconds')::interval
		), upd_runs AS (
		    UPDATE repo_mirror_runs SET
		        status      = 'failed',
		        error       = 'abandoned (process may have crashed)',
		        finished_at = now()
		    FROM stuck
		    WHERE repo_mirror_runs.id = stuck.id
		    RETURNING repo_mirror_runs.repo_id
		)
		UPDATE repo_mirrors SET
		    last_status = 'failed',
		    last_error  = 'abandoned (process may have crashed)',
		    updated_at  = now()
		WHERE repo_id IN (SELECT repo_id FROM upd_runs)`)
	if err != nil {
		return 0, fmt.Errorf("mirror: reap stuck: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// InitialClone initializes a bare repo and mirror-fetches from GitHub.
// Caller is responsible for:
//  1. Creating the repo row and the repo_mirrors row (direction='pull') first.
//  2. Rolling back the repo row on error if they want clean failure semantics.
//
// Runs synchronously. Intended for the repo-creation flow where the user sees
// a blocking spinner.
func (s *Service) InitialClone(ctx context.Context, repoID uuid.UUID) error {
	lock := s.lockFor(repoID)
	lock.Lock()
	defer lock.Unlock()

	// Load target + encrypted PAT in one query (same pattern as SyncNow).
	var (
		owner, repo       string
		ciphertext, nonce []byte
	)
	err := s.db.QueryRow(ctx, `
		SELECT github_owner, github_repo, pat_ciphertext, pat_nonce
		FROM repo_mirrors WHERE repo_id = $1`, repoID,
	).Scan(&owner, &repo, &ciphertext, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMirrorNotFound
	}
	if err != nil {
		return fmt.Errorf("mirror: load for initial clone: %w", err)
	}

	runID, err := s.startRun(ctx, repoID, models.MirrorTriggerInitialClone)
	if err != nil {
		return fmt.Errorf("mirror: start initial clone run: %w", err)
	}
	start := nowUTC()

	pat, err := s.decryptPAT(ciphertext, nonce)
	if err != nil {
		_, _ = s.finishRun(ctx, runID, repoID, 0, models.MirrorFailed, err.Error(), start)
		return fmt.Errorf("mirror: initial clone decrypt: %w", err)
	}

	localPath := s.repoPath(repoID)
	remoteURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)

	// 1. init --bare
	if err := s.remote.InitBare(ctx, localPath); err != nil {
		_, _ = s.finishRun(ctx, runID, repoID, 0, models.MirrorFailed, "init: "+err.Error(), start)
		return fmt.Errorf("mirror: init bare: %w", err)
	}
	// 2. fetch mirror; on failure, remove the bare dir we just created so
	//    the caller's rollback path doesn't leave an orphan on disk.
	outcome, err := s.remote.FetchMirror(ctx, localPath, remoteURL, pat)
	if err != nil {
		_ = os.RemoveAll(localPath)
		_, _ = s.finishRun(ctx, runID, repoID, outcome.RefsChanged, models.MirrorFailed, err.Error(), start)
		return fmt.Errorf("mirror: initial fetch: %w", err)
	}
	// 3. set HEAD to remote's default branch (best effort)
	if branch, lsErr := s.remote.LsRemoteDefault(ctx, remoteURL, pat); lsErr == nil {
		_ = s.remote.SetHead(ctx, localPath, branch)
	}

	if _, err := s.finishRun(ctx, runID, repoID, outcome.RefsChanged, models.MirrorSuccess, "", start); err != nil {
		return fmt.Errorf("mirror: finish initial clone: %w", err)
	}
	return nil
}

// MirrorRow is the admin-view projection: a mirror plus the owner/repo name
// for display.
type MirrorRow struct {
	models.RepoMirror
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
}

// ListAll returns every configured mirror in the instance, joined with the
// repo's owner and name. Admin endpoint only.
func (s *Service) ListAll(ctx context.Context) ([]MirrorRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT rm.repo_id, rm.direction, rm.github_owner, rm.github_repo,
		       (rm.pat_ciphertext IS NOT NULL), rm.interval_seconds, rm.auto_push,
		       rm.last_status, COALESCE(rm.last_error, ''),
		       rm.last_synced_at, rm.next_run_at, rm.created_at, rm.updated_at,
		       LOWER(COALESCE(u.username, o.name)) AS owner_name,
		       r.name AS repo_name
		FROM repo_mirrors rm
		JOIN repositories r ON r.id = rm.repo_id
		LEFT JOIN users u         ON r.owner_id = u.id AND r.owner_type = 'user'
		LEFT JOIN organizations o ON r.owner_id = o.id AND r.owner_type = 'org'
		ORDER BY rm.last_synced_at DESC NULLS LAST`)
	if err != nil {
		return nil, fmt.Errorf("mirror: list all: %w", err)
	}
	defer rows.Close()

	var out []MirrorRow
	for rows.Next() {
		var m MirrorRow
		if err := rows.Scan(&m.RepoID, &m.Direction, &m.GithubOwner, &m.GithubRepo,
			&m.HasPAT, &m.IntervalSeconds, &m.AutoPush,
			&m.LastStatus, &m.LastError, &m.LastSyncedAt, &m.NextRunAt,
			&m.CreatedAt, &m.UpdatedAt,
			&m.RepoOwner, &m.RepoName,
		); err != nil {
			return nil, fmt.Errorf("mirror: scan list all: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mirror: list all: %w", err)
	}
	return out, nil
}
