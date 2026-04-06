package models

import (
	"time"

	"github.com/google/uuid"
)

// NotificationPreferences controls which notification types a user receives.
// All fields default to true (enabled).
type NotificationPreferences struct {
	UserID       uuid.UUID `json:"user_id"`
	PRReview     bool      `json:"pr_review"`
	PRMerged     bool      `json:"pr_merged"`
	PRComment    bool      `json:"pr_comment"`
	IssueComment bool      `json:"issue_comment"`
	Mention      bool      `json:"mention"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UpdateNotificationPreferencesRequest is the request body for updating preferences.
type UpdateNotificationPreferencesRequest struct {
	PRReview     *bool `json:"pr_review,omitempty"`
	PRMerged     *bool `json:"pr_merged,omitempty"`
	PRComment    *bool `json:"pr_comment,omitempty"`
	IssueComment *bool `json:"issue_comment,omitempty"`
	Mention      *bool `json:"mention,omitempty"`
}

// RepoWatcher represents a user watching a repository.
type RepoWatcher struct {
	UserID    uuid.UUID `json:"user_id"`
	RepoID    uuid.UUID `json:"repo_id"`
	CreatedAt time.Time `json:"created_at"`
}
