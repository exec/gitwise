package models

import (
	"time"

	"github.com/google/uuid"
)

type MirrorDirection string

const (
	MirrorPush MirrorDirection = "push" // Gitwise -> GitHub
	MirrorPull MirrorDirection = "pull" // Gitwise <- GitHub
)

type MirrorStatus string

const (
	MirrorPending MirrorStatus = "pending"
	MirrorRunning MirrorStatus = "running"
	MirrorSuccess MirrorStatus = "success"
	MirrorFailed  MirrorStatus = "failed"
)

type MirrorTrigger string

const (
	MirrorTriggerManual       MirrorTrigger = "manual"
	MirrorTriggerScheduled    MirrorTrigger = "scheduled"
	MirrorTriggerPushEvent    MirrorTrigger = "push_event"
	MirrorTriggerInitialClone MirrorTrigger = "initial_clone"
)

type RepoMirror struct {
	RepoID          uuid.UUID       `json:"repo_id"`
	Direction       MirrorDirection `json:"direction"`
	GithubOwner     string          `json:"github_owner"`
	GithubRepo      string          `json:"github_repo"`
	HasPAT          bool            `json:"has_pat"`
	IntervalSeconds int             `json:"interval_seconds"`
	AutoPush        bool            `json:"auto_push"`
	LastStatus      MirrorStatus    `json:"last_status"`
	LastError       string          `json:"last_error,omitempty"`
	LastSyncedAt    *time.Time      `json:"last_synced_at,omitempty"`
	NextRunAt       *time.Time      `json:"next_run_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type RepoMirrorRun struct {
	ID          uuid.UUID     `json:"id"`
	RepoID      uuid.UUID     `json:"repo_id"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  *time.Time    `json:"finished_at,omitempty"`
	Status      MirrorStatus  `json:"status"`
	Trigger     MirrorTrigger `json:"trigger"`
	RefsChanged *int          `json:"refs_changed,omitempty"`
	Error       string        `json:"error,omitempty"`
	DurationMs  *int          `json:"duration_ms,omitempty"`
}

type ConfigureMirrorRequest struct {
	Direction       MirrorDirection `json:"direction"`
	GithubOwner     string          `json:"github_owner"`
	GithubRepo      string          `json:"github_repo"`
	PAT             string          `json:"pat,omitempty"`      // empty = keep existing
	ClearPAT        bool            `json:"clear_pat,omitempty"`
	IntervalSeconds int             `json:"interval_seconds"`
	AutoPush        bool            `json:"auto_push"`
}
