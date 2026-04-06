package models

import (
	"time"

	"github.com/google/uuid"
)

type SSHKey struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Name        string    `json:"name"`
	Fingerprint string    `json:"fingerprint"`
	KeyType     string    `json:"key_type"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateSSHKeyRequest struct {
	Title     string `json:"title"`
	PublicKey string `json:"public_key"`
}
