package models

import (
	"time"

	"github.com/google/uuid"
)

type BranchProtection struct {
	ID              uuid.UUID `json:"id"`
	RepoID          uuid.UUID `json:"repo_id"`
	BranchPattern   string    `json:"branch_pattern"`
	RequiredReviews int       `json:"required_reviews"`
	RequireLinear   bool      `json:"require_linear"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CreateBranchProtectionRequest struct {
	BranchPattern   string `json:"branch_pattern"`
	RequiredReviews int    `json:"required_reviews"`
	RequireLinear   bool   `json:"require_linear"`
}

type UpdateBranchProtectionRequest struct {
	RequiredReviews *int  `json:"required_reviews,omitempty"`
	RequireLinear   *bool `json:"require_linear,omitempty"`
}
