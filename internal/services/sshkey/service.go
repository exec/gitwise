package sshkey

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/ssh"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrInvalidKey   = errors.New("invalid SSH public key")
	ErrDuplicateKey = errors.New("SSH key already exists")
	ErrKeyNotFound  = errors.New("SSH key not found")
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

// Add parses, validates, and stores a new SSH public key for the given user.
func (s *Service) Add(ctx context.Context, userID uuid.UUID, req models.CreateSSHKeyRequest) (*models.SSHKey, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidKey)
	}

	pubKeyStr := strings.TrimSpace(req.PublicKey)
	if pubKeyStr == "" {
		return nil, fmt.Errorf("%w: public key is required", ErrInvalidKey)
	}

	parsed, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKeyStr))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}

	fingerprint := Fingerprint(parsed)
	keyType := parsed.Type()

	_ = comment // comment is available but title is already required

	key := &models.SSHKey{
		ID:          uuid.New(),
		UserID:      userID,
		Name:        title,
		Fingerprint: fingerprint,
		KeyType:     keyType,
		CreatedAt:   time.Now(),
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO ssh_keys (id, user_id, name, fingerprint, public_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		key.ID, key.UserID, key.Name, key.Fingerprint, pubKeyStr, key.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateKey
		}
		return nil, fmt.Errorf("insert ssh key: %w", err)
	}

	return key, nil
}

// List returns all SSH keys for a user.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]models.SSHKey, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, name, fingerprint, public_key, created_at
		FROM ssh_keys WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query ssh keys: %w", err)
	}
	defer rows.Close()

	var keys []models.SSHKey
	for rows.Next() {
		var k models.SSHKey
		var pubKey string
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.Fingerprint, &pubKey, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ssh key: %w", err)
		}
		k.KeyType = parseKeyType(pubKey)
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ssh keys: %w", err)
	}
	return keys, nil
}

// Delete removes an SSH key owned by the given user.
func (s *Service) Delete(ctx context.Context, userID uuid.UUID, keyID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM ssh_keys WHERE id = $1 AND user_id = $2`, keyID, userID)
	if err != nil {
		return fmt.Errorf("delete ssh key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrKeyNotFound
	}
	return nil
}

// LookupByFingerprint finds the username associated with an SSH key fingerprint.
// Used by the SSH server's PublicKeyHandler.
func (s *Service) LookupByFingerprint(ctx context.Context, fingerprint string) (string, error) {
	var username string
	err := s.db.QueryRow(ctx, `
		SELECT u.username FROM ssh_keys sk
		JOIN users u ON u.id = sk.user_id
		WHERE sk.fingerprint = $1`, fingerprint,
	).Scan(&username)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrKeyNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lookup ssh key: %w", err)
	}
	return username, nil
}

// Fingerprint computes the SHA256 fingerprint of an SSH public key,
// formatted as "SHA256:<base64>" (matching ssh-keygen -l output).
func Fingerprint(key ssh.PublicKey) string {
	hash := sha256.Sum256(key.Marshal())
	b64 := base64.RawStdEncoding.EncodeToString(hash[:])
	return "SHA256:" + b64
}

// parseKeyType extracts the key type from a stored authorized_keys line.
func parseKeyType(pubKeyStr string) string {
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKeyStr))
	if err != nil {
		parts := strings.Fields(pubKeyStr)
		if len(parts) > 0 {
			return parts[0]
		}
		return "unknown"
	}
	return parsed.Type()
}
