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
