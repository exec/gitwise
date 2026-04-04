package models

import "github.com/google/uuid"

type User struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Password  string    `json:"-"` // never serialized
	FullName  string    `json:"full_name"`
	AvatarURL string    `json:"avatar_url"`
	Bio       string    `json:"bio"`
	IsAdmin   bool      `json:"is_admin"`
	Timestamps
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type LoginRequest struct {
	Login    string `json:"login"` // username or email
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type UpdateUserRequest struct {
	FullName  *string `json:"full_name,omitempty"`
	Bio       *string `json:"bio,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}
