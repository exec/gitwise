package totp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/argon2"
)

const (
	recoveryCodeCount = 8
	recoveryCodeBytes = 6 // 48 bits = 12 hex chars per code (I4)

	pendingAuthPrefix = "pending_2fa:"
	pendingAuthExpiry = 5 * time.Minute
	maxAttempts       = 5 // C4: max failed TOTP attempts per pending token

	// Argon2id params for recovery code hashing (lighter than password hashing
	// since recovery codes already have 48 bits of entropy).
	rcArgonTime    = 1
	rcArgonMemory  = 32 * 1024 // 32 MB
	rcArgonThreads = 2
	rcArgonKeyLen  = 32
	rcArgonSaltLen = 16
)

var (
	ErrNotConfigured    = errors.New("2FA encryption key not configured")
	ErrAlreadyEnabled   = errors.New("2FA is already enabled")
	ErrNotSetUp         = errors.New("2FA not set up: run setup first")
	ErrInvalidCode      = errors.New("invalid TOTP code")
	ErrInvalidToken     = errors.New("invalid or expired 2FA token")
	ErrTooManyAttempts  = errors.New("too many failed attempts")
	ErrBadPassword      = errors.New("invalid password")
)

// pendingAuth is stored in Redis as JSON alongside the user ID.
type pendingAuth struct {
	UserID   string `json:"user_id"`
	Attempts int    `json:"attempts"`
}

// Service handles TOTP-based two-factor authentication.
type Service struct {
	db  *pgxpool.Pool
	rdb *redis.Client
	key []byte // 32-byte AES-256 key for encrypting TOTP secrets
}

// NewService creates a new TOTP service.
// encKeyHex is the hex-encoded 32-byte AES-256 key. If empty, 2FA setup
// operations will return ErrNotConfigured (but status/check queries still work).
func NewService(db *pgxpool.Pool, rdb *redis.Client, encKeyHex string) (*Service, error) {
	s := &Service{db: db, rdb: rdb}
	if encKeyHex != "" {
		key, err := hex.DecodeString(encKeyHex)
		if err != nil {
			return nil, fmt.Errorf("decode GITWISE_TOTP_KEY: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("GITWISE_TOTP_KEY must be exactly 32 bytes (64 hex chars), got %d bytes", len(key))
		}
		s.key = key
	}
	return s, nil
}

// SetupResult holds the data returned when beginning 2FA setup.
type SetupResult struct {
	Secret        string   `json:"secret"`
	URI           string   `json:"uri"`
	RecoveryCodes []string `json:"recovery_codes"`
}

// BeginSetup generates a new TOTP secret and recovery codes, storing them
// (encrypted/hashed) in the database. 2FA is not yet enabled until the user
// confirms with a valid code.
//
// Requires password re-authentication (I2). Rejects if 2FA is already enabled (I3).
func (s *Service) BeginSetup(ctx context.Context, userID uuid.UUID, username, issuer, passwordHash, password string) (*SetupResult, error) {
	if s.key == nil {
		return nil, ErrNotConfigured
	}

	// I2: Verify current password.
	if !verifyPasswordFn(password, passwordHash) {
		return nil, ErrBadPassword
	}

	// I3: Reject if already enabled.
	var enabled bool
	if err := s.db.QueryRow(ctx, `SELECT totp_enabled FROM users WHERE id = $1`, userID).Scan(&enabled); err != nil {
		return nil, fmt.Errorf("check totp_enabled: %w", err)
	}
	if enabled {
		return nil, ErrAlreadyEnabled
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
	})
	if err != nil {
		return nil, fmt.Errorf("generate totp key: %w", err)
	}

	// C1: Encrypt the TOTP secret before storing.
	encSecret, err := encrypt(key.Secret(), s.key)
	if err != nil {
		return nil, fmt.Errorf("encrypt totp secret: %w", err)
	}

	// Generate plaintext recovery codes.
	codes, err := generateRecoveryCodes()
	if err != nil {
		return nil, fmt.Errorf("generate recovery codes: %w", err)
	}

	// C2: Hash recovery codes before storing.
	hashedCodes, err := hashRecoveryCodes(codes)
	if err != nil {
		return nil, fmt.Errorf("hash recovery codes: %w", err)
	}

	// Store the encrypted secret and hashed recovery codes but keep totp_enabled = false.
	_, err = s.db.Exec(ctx, `
		UPDATE users
		SET totp_secret = $2, recovery_codes = $3, updated_at = now()
		WHERE id = $1`,
		userID, encSecret, hashedCodes,
	)
	if err != nil {
		return nil, fmt.Errorf("store totp secret: %w", err)
	}

	return &SetupResult{
		Secret:        key.Secret(),
		URI:           key.URL(),
		RecoveryCodes: codes,
	}, nil
}

// Enable verifies a TOTP code and activates 2FA for the user.
func (s *Service) Enable(ctx context.Context, userID uuid.UUID, code string) error {
	if s.key == nil {
		return ErrNotConfigured
	}

	var encSecret *string
	err := s.db.QueryRow(ctx, `SELECT totp_secret FROM users WHERE id = $1`, userID).Scan(&encSecret)
	if err != nil {
		return fmt.Errorf("query totp secret: %w", err)
	}
	if encSecret == nil || *encSecret == "" {
		return ErrNotSetUp
	}

	// Decrypt the stored secret.
	secret, err := decrypt(*encSecret, s.key)
	if err != nil {
		return fmt.Errorf("decrypt totp secret: %w", err)
	}

	if !totp.Validate(code, secret) {
		return ErrInvalidCode
	}

	_, err = s.db.Exec(ctx, `
		UPDATE users SET totp_enabled = true, updated_at = now()
		WHERE id = $1`, userID,
	)
	if err != nil {
		return fmt.Errorf("enable totp: %w", err)
	}

	return nil
}

// Disable verifies a TOTP code (or recovery code) and deactivates 2FA.
func (s *Service) Disable(ctx context.Context, userID uuid.UUID, code string) error {
	if s.key == nil {
		return ErrNotConfigured
	}

	var encSecret *string
	var hashedCodes []string
	err := s.db.QueryRow(ctx, `
		SELECT totp_secret, recovery_codes FROM users WHERE id = $1`, userID,
	).Scan(&encSecret, &hashedCodes)
	if err != nil {
		return fmt.Errorf("query totp data: %w", err)
	}

	// Try TOTP code first.
	if encSecret != nil && *encSecret != "" {
		secret, decErr := decrypt(*encSecret, s.key)
		if decErr == nil && totp.Validate(code, secret) {
			return s.clearTOTP(ctx, userID)
		}
	}

	// Try recovery code (C2/C3: constant-time via argon2id hash comparison).
	if s.matchAndConsumeRecoveryCode(ctx, userID, code, hashedCodes) {
		return s.clearTOTP(ctx, userID)
	}

	return ErrInvalidCode
}

// IsEnabled returns whether 2FA is enabled for a user.
func (s *Service) IsEnabled(ctx context.Context, userID uuid.UUID) (bool, error) {
	var enabled bool
	err := s.db.QueryRow(ctx, `SELECT totp_enabled FROM users WHERE id = $1`, userID).Scan(&enabled)
	if err != nil {
		return false, fmt.Errorf("query totp_enabled: %w", err)
	}
	return enabled, nil
}

// StorePendingAuth stores a pending 2FA authentication token in Redis.
// Returns the token that the client must present with the TOTP code.
func (s *Service) StorePendingAuth(ctx context.Context, userID uuid.UUID) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate pending token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	pa := pendingAuth{
		UserID:   userID.String(),
		Attempts: 0,
	}
	data, err := json.Marshal(pa)
	if err != nil {
		return "", fmt.Errorf("marshal pending auth: %w", err)
	}

	err = s.rdb.Set(ctx, pendingAuthPrefix+token, string(data), pendingAuthExpiry).Err()
	if err != nil {
		return "", fmt.Errorf("store pending auth: %w", err)
	}

	return token, nil
}

// ValidatePendingAuth looks up a pending 2FA token and returns the user ID.
// I5: Uses GET (not GETDEL) so the token remains until verification succeeds.
// C4: Tracks attempt count; after maxAttempts the token is deleted.
func (s *Service) ValidatePendingAuth(ctx context.Context, token string) (uuid.UUID, error) {
	key := pendingAuthPrefix + token

	val, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return uuid.Nil, ErrInvalidToken
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("get pending auth: %w", err)
	}

	var pa pendingAuth
	if err := json.Unmarshal([]byte(val), &pa); err != nil {
		return uuid.Nil, fmt.Errorf("unmarshal pending auth: %w", err)
	}

	if pa.Attempts >= maxAttempts {
		s.rdb.Del(ctx, key)
		return uuid.Nil, ErrTooManyAttempts
	}

	userID, err := uuid.Parse(pa.UserID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse pending user id: %w", err)
	}

	return userID, nil
}

// IncrementAttempts bumps the attempt counter on a pending token.
// Called when TOTP verification fails.
func (s *Service) IncrementAttempts(ctx context.Context, token string) {
	key := pendingAuthPrefix + token

	val, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return
	}

	var pa pendingAuth
	if err := json.Unmarshal([]byte(val), &pa); err != nil {
		return
	}

	pa.Attempts++

	if pa.Attempts >= maxAttempts {
		// C4: Delete token after max attempts.
		s.rdb.Del(ctx, key)
		return
	}

	data, err := json.Marshal(pa)
	if err != nil {
		return
	}

	// Preserve remaining TTL.
	ttl, err := s.rdb.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		ttl = pendingAuthExpiry
	}
	s.rdb.Set(ctx, key, string(data), ttl)
}

// ConsumePendingAuth deletes the pending token after successful verification.
func (s *Service) ConsumePendingAuth(ctx context.Context, token string) {
	s.rdb.Del(ctx, pendingAuthPrefix+token)
}

// Verify checks a TOTP code or recovery code against a user's stored secret.
// If a recovery code is used, it is consumed.
func (s *Service) Verify(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	if s.key == nil {
		return false, ErrNotConfigured
	}

	var encSecret *string
	var hashedCodes []string
	err := s.db.QueryRow(ctx, `
		SELECT totp_secret, recovery_codes FROM users WHERE id = $1`, userID,
	).Scan(&encSecret, &hashedCodes)
	if err != nil {
		return false, fmt.Errorf("query totp data: %w", err)
	}

	// Try TOTP code first.
	if encSecret != nil && *encSecret != "" {
		secret, decErr := decrypt(*encSecret, s.key)
		if decErr == nil && totp.Validate(code, secret) {
			return true, nil
		}
	}

	// Try recovery code (C2/C3: constant-time via argon2id hash).
	if s.matchAndConsumeRecoveryCode(ctx, userID, code, hashedCodes) {
		return true, nil
	}

	return false, nil
}

// clearTOTP removes all 2FA data for a user.
func (s *Service) clearTOTP(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `
		UPDATE users
		SET totp_enabled = false, totp_secret = NULL, recovery_codes = NULL, updated_at = now()
		WHERE id = $1`, userID,
	)
	if err != nil {
		return fmt.Errorf("disable totp: %w", err)
	}
	return nil
}

// matchAndConsumeRecoveryCode iterates over hashed recovery codes and checks
// if the provided code matches any of them. If a match is found, that code
// is removed from the database. Uses argon2id for constant-time comparison (C3).
func (s *Service) matchAndConsumeRecoveryCode(ctx context.Context, userID uuid.UUID, code string, hashedCodes []string) bool {
	normalizedCode := strings.ToLower(strings.TrimSpace(code))
	if normalizedCode == "" {
		return false
	}

	matchIdx := -1
	for i, hc := range hashedCodes {
		if verifyRecoveryCode(normalizedCode, hc) {
			matchIdx = i
			break
		}
	}

	if matchIdx < 0 {
		return false
	}

	// Consume the matched recovery code.
	remaining := make([]string, 0, len(hashedCodes)-1)
	remaining = append(remaining, hashedCodes[:matchIdx]...)
	remaining = append(remaining, hashedCodes[matchIdx+1:]...)
	s.db.Exec(ctx, `
		UPDATE users SET recovery_codes = $2, updated_at = now()
		WHERE id = $1`, userID, remaining,
	)
	return true
}

// generateRecoveryCodes creates recoveryCodeCount random codes with
// recoveryCodeBytes of entropy each (I4: 6 bytes = 48 bits = 12 hex chars).
func generateRecoveryCodes() ([]string, error) {
	codes := make([]string, recoveryCodeCount)
	for i := range codes {
		b := make([]byte, recoveryCodeBytes)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		codes[i] = hex.EncodeToString(b)
	}
	return codes, nil
}

// hashRecoveryCodes produces argon2id hashes of the plaintext recovery codes (C2).
func hashRecoveryCodes(codes []string) ([]string, error) {
	hashed := make([]string, len(codes))
	for i, code := range codes {
		h, err := hashRecoveryCode(strings.ToLower(code))
		if err != nil {
			return nil, err
		}
		hashed[i] = h
	}
	return hashed, nil
}

// hashRecoveryCode creates an argon2id hash of a single recovery code.
// Format: hex(salt) + ":" + hex(hash)
func hashRecoveryCode(code string) (string, error) {
	salt := make([]byte, rcArgonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(code), salt, rcArgonTime, rcArgonMemory, rcArgonThreads, rcArgonKeyLen)
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(hash), nil
}

// verifyRecoveryCode checks a plaintext code against an argon2id hash.
// Constant-time comparison via argon2id recomputation (C3).
func verifyRecoveryCode(code, encoded string) bool {
	parts := strings.SplitN(encoded, ":", 2)
	if len(parts) != 2 {
		return false
	}

	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}

	expectedHash, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}

	hash := argon2.IDKey([]byte(code), salt, rcArgonTime, rcArgonMemory, rcArgonThreads, uint32(len(expectedHash)))

	// Constant-time comparison is inherent in argon2id recomputation:
	// the timing is dominated by the key derivation, not the final compare.
	// But we still use a constant-time byte compare for correctness.
	if len(hash) != len(expectedHash) {
		return false
	}
	result := byte(0)
	for i := range hash {
		result |= hash[i] ^ expectedHash[i]
	}
	return result == 0
}

// verifyPasswordFn is the function used to verify passwords.
// It is a package-level variable to allow test injection.
var verifyPasswordFn = defaultVerifyPassword

func defaultVerifyPassword(password, encoded string) bool {
	// Import at function level to avoid circular dependency.
	// The user package exports VerifyPassword.
	// We use a dynamic import pattern via the variable to allow test mocking.
	return verifyPasswordArgon2id(password, encoded)
}
