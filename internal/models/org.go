package models

import "github.com/google/uuid"

type Organization struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description"`
	AvatarURL   string    `json:"avatar_url"`
	Timestamps
}

type OrgMember struct {
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	FullName  string    `json:"full_name"`
	AvatarURL string    `json:"avatar_url"`
	Role      string    `json:"role"`
}

// OrgTeam represents a team within an organization.
type OrgTeam struct {
	ID          uuid.UUID `json:"id"`
	OrgID       uuid.UUID `json:"org_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permission  string    `json:"permission"` // read, triage, write, admin
	MemberCount int       `json:"member_count"`
	RepoCount   int       `json:"repo_count"`
	Timestamps
}

type CreateTeamRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Permission  string `json:"permission"`
}

type UpdateTeamRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Permission  *string `json:"permission,omitempty"`
}

// OrgTeamMember is a user who belongs to a team.
type OrgTeamMember struct {
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	FullName  string    `json:"full_name"`
	AvatarURL string    `json:"avatar_url"`
}

// OrgTeamRepo is a repository assigned to a team.
type OrgTeamRepo struct {
	RepoID      uuid.UUID `json:"repo_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Visibility  string    `json:"visibility"`
}
