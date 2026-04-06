package commit

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	gitSvc "github.com/gitwise-io/gitwise/internal/git"
)

// Indexer indexes git commits into the commit_metadata table for contribution tracking.
type Indexer struct {
	db  *pgxpool.Pool
	git *gitSvc.Service
}

func NewIndexer(db *pgxpool.Pool, git *gitSvc.Service) *Indexer {
	return &Indexer{db: db, git: git}
}

// IndexRepo walks all commits in a repo and upserts them into commit_metadata,
// matching author emails to user IDs. Safe to call multiple times — existing commits are skipped.
func (idx *Indexer) IndexRepo(ctx context.Context, repoID uuid.UUID, owner, repoName string) (int, error) {
	repo, err := idx.git.OpenRepo(owner, repoName)
	if err != nil {
		return 0, fmt.Errorf("open repo: %w", err)
	}

	// Walk all branches to collect all reachable commits
	seen := make(map[plumbing.Hash]bool)
	var commits []*object.Commit

	commitIter, err := repo.CommitObjects()
	if err != nil {
		return 0, fmt.Errorf("commit objects: %w", err)
	}

	err = commitIter.ForEach(func(c *object.Commit) error {
		if seen[c.Hash] {
			return nil
		}
		seen[c.Hash] = true
		commits = append(commits, c)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("iterate commits: %w", err)
	}

	if len(commits) == 0 {
		return 0, nil
	}

	// Batch upsert
	indexed := 0
	for _, c := range commits {
		var parents []string
		for _, p := range c.ParentHashes {
			parents = append(parents, p.String())
		}

		// Look up user by email
		var authorID *uuid.UUID
		var uid uuid.UUID
		if err := idx.db.QueryRow(ctx, `SELECT id FROM users WHERE email = $1 LIMIT 1`, c.Author.Email).Scan(&uid); err == nil {
			authorID = &uid
		}

		_, err := idx.db.Exec(ctx, `
			INSERT INTO commit_metadata (sha, repo_id, message, author_email, author_id, parent_shas, committed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (repo_id, sha) DO NOTHING`,
			c.Hash.String(), repoID, c.Message, c.Author.Email, authorID, parents, c.Author.When,
		)
		if err != nil {
			slog.Warn("failed to index commit", "sha", c.Hash.String()[:7], "error", err)
			continue
		}
		indexed++
	}

	slog.Info("indexed commits", "repo", owner+"/"+repoName, "total", len(commits), "new", indexed)
	return indexed, nil
}

// IndexAll indexes all repos in the database.
func (idx *Indexer) IndexAll(ctx context.Context) error {
	rows, err := idx.db.Query(ctx, `
		SELECT r.id, u.username, r.name
		FROM repositories r
		JOIN users u ON r.owner_id = u.id`)
	if err != nil {
		return fmt.Errorf("list repos: %w", err)
	}
	defer rows.Close()

	type repoInfo struct {
		id    uuid.UUID
		owner string
		name  string
	}
	var repos []repoInfo
	for rows.Next() {
		var ri repoInfo
		if err := rows.Scan(&ri.id, &ri.owner, &ri.name); err != nil {
			continue
		}
		repos = append(repos, ri)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate repos: %w", err)
	}

	for _, ri := range repos {
		if _, err := idx.IndexRepo(ctx, ri.id, ri.owner, ri.name); err != nil {
			slog.Warn("failed to index repo", "repo", ri.owner+"/"+ri.name, "error", err)
		}
	}
	return nil
}
