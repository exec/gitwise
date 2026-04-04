package models

import "github.com/google/uuid"

// DayCount represents a single day's contribution count.
type DayCount struct {
	Date  string `json:"date"`  // YYYY-MM-DD
	Count int    `json:"count"`
}

// PinnedRepo is a repository pinned to a user's profile.
type PinnedRepo struct {
	Position   int        `json:"position"`
	Repository Repository `json:"repository"`
}

// SetPinnedReposRequest is the body for PUT /api/v1/user/pinned-repos.
type SetPinnedReposRequest struct {
	RepoIDs []uuid.UUID `json:"repo_ids"`
}
