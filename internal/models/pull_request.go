package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PullRequest struct {
	ID            uuid.UUID       `json:"id"`
	RepoID        uuid.UUID       `json:"repo_id"`
	Number        int             `json:"number"`
	AuthorID      uuid.UUID       `json:"author_id"`
	AuthorName    string          `json:"author_name"` // populated via join
	Title         string          `json:"title"`
	Body          string          `json:"body"`
	SourceBranch  string          `json:"source_branch"`
	TargetBranch  string          `json:"target_branch"`
	Status        string          `json:"status"` // draft, open, merged, closed
	Intent        json.RawMessage `json:"intent"`
	DiffStats     json.RawMessage `json:"diff_stats"`
	ReviewSummary json.RawMessage `json:"review_summary"`
	MergeStrategy *string         `json:"merge_strategy,omitempty"`
	MergedByID    *uuid.UUID      `json:"merged_by,omitempty"`
	MergedByName  string          `json:"merged_by_name,omitempty"` // populated via join
	MergedAt      *time.Time      `json:"merged_at,omitempty"`
	ClosedAt      *time.Time      `json:"closed_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type PRIntent struct {
	Type       string   `json:"type"`       // feature, bugfix, refactor, chore
	Scope      string   `json:"scope"`
	Components []string `json:"components"`
}

type CreatePullRequestRequest struct {
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	SourceBranch string    `json:"source_branch"`
	TargetBranch string    `json:"target_branch"`
	Draft        bool      `json:"draft"`
	Intent       *PRIntent `json:"intent,omitempty"`
}

type UpdatePullRequestRequest struct {
	Title        *string   `json:"title,omitempty"`
	Body         *string   `json:"body,omitempty"`
	TargetBranch *string   `json:"target_branch,omitempty"`
	Status       *string   `json:"status,omitempty"` // draft, open, closed (not merged — use merge endpoint)
	Intent       *PRIntent `json:"intent,omitempty"`
}

type MergePullRequestRequest struct {
	Strategy     string `json:"strategy"`      // merge, squash, rebase
	Message      string `json:"message"`       // custom merge commit message (optional)
	DeleteBranch bool   `json:"delete_branch"` // delete source branch after merge
}

// PRDiffResponse is returned by the PR diff endpoint.
type PRDiffResponse struct {
	Commits []Commit   `json:"commits"`
	Files   []DiffFile `json:"files"`
	Stats   struct {
		TotalCommits   int `json:"total_commits"`
		TotalFiles     int `json:"total_files"`
		TotalAdditions int `json:"total_additions"`
		TotalDeletions int `json:"total_deletions"`
	} `json:"stats"`
}
