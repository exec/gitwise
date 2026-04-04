package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrNotFound       = errors.New("user not found")
	ErrDuplicateUser  = errors.New("username or email already exists")
	ErrInvalidInput   = errors.New("invalid input")
	ErrBadCredentials = errors.New("invalid credentials")
	ErrTokenNotFound  = errors.New("token not found")
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,37}[a-zA-Z0-9])?$`)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, req models.CreateUserRequest) (*models.User, error) {
	if !usernameRe.MatchString(req.Username) {
		return nil, fmt.Errorf("%w: username must be 1-39 alphanumeric characters", ErrInvalidInput)
	}
	if len(req.Password) < 8 {
		return nil, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidInput)
	}
	if len(req.Password) > 128 {
		return nil, fmt.Errorf("%w: password must be at most 128 characters", ErrInvalidInput)
	}
	if !strings.Contains(req.Email, "@") {
		return nil, fmt.Errorf("%w: invalid email address", ErrInvalidInput)
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &models.User{
		ID:       uuid.New(),
		Username: strings.ToLower(req.Username),
		Email:    strings.ToLower(req.Email),
		Password: hash,
		FullName: req.FullName,
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO users (id, username, email, password, full_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		user.ID, user.Username, user.Email, user.Password, user.FullName, time.Now(),
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrDuplicateUser
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}

	user.Password = ""
	return user, nil
}

func (s *Service) Authenticate(ctx context.Context, login, password string) (*models.User, error) {
	login = strings.ToLower(login)
	user := &models.User{}
	err := s.db.QueryRow(ctx, `
		SELECT id, username, email, password, full_name, avatar_url, bio, is_admin, created_at, updated_at
		FROM users WHERE username = $1 OR email = $1`, login,
	).Scan(
		&user.ID, &user.Username, &user.Email, &user.Password,
		&user.FullName, &user.AvatarURL, &user.Bio, &user.IsAdmin,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBadCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}

	if !verifyPassword(password, user.Password) {
		return nil, ErrBadCredentials
	}

	user.Password = ""
	return user, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	user := &models.User{}
	err := s.db.QueryRow(ctx, `
		SELECT id, username, email, full_name, avatar_url, bio, is_admin, created_at, updated_at
		FROM users WHERE id = $1`, id,
	).Scan(
		&user.ID, &user.Username, &user.Email,
		&user.FullName, &user.AvatarURL, &user.Bio, &user.IsAdmin,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	return user, nil
}

func (s *Service) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	user := &models.User{}
	err := s.db.QueryRow(ctx, `
		SELECT id, username, email, full_name, avatar_url, bio, is_admin, created_at, updated_at
		FROM users WHERE username = $1`, strings.ToLower(username),
	).Scan(
		&user.ID, &user.Username, &user.Email,
		&user.FullName, &user.AvatarURL, &user.Bio, &user.IsAdmin,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	return user, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req models.UpdateUserRequest) (*models.User, error) {
	setClauses := []string{"updated_at = now()"}
	args := []any{id}
	argIdx := 2

	if req.FullName != nil {
		setClauses = append(setClauses, fmt.Sprintf("full_name = $%d", argIdx))
		args = append(args, *req.FullName)
		argIdx++
	}
	if req.Bio != nil {
		setClauses = append(setClauses, fmt.Sprintf("bio = $%d", argIdx))
		args = append(args, *req.Bio)
		argIdx++
	}
	if req.AvatarURL != nil {
		setClauses = append(setClauses, fmt.Sprintf("avatar_url = $%d", argIdx))
		args = append(args, *req.AvatarURL)
		argIdx++
	}

	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $1 RETURNING id, username, email, full_name, avatar_url, bio, is_admin, created_at, updated_at",
		strings.Join(setClauses, ", "))

	user := &models.User{}
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&user.ID, &user.Username, &user.Email,
		&user.FullName, &user.AvatarURL, &user.Bio, &user.IsAdmin,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return user, nil
}

// CreateToken generates a new API token for a user.
func (s *Service) CreateToken(ctx context.Context, userID uuid.UUID, req models.CreateTokenRequest) (*models.APIToken, error) {
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	tokenStr := "gw_" + hex.EncodeToString(rawToken)
	hash := sha256.Sum256([]byte(tokenStr))
	tokenHash := hex.EncodeToString(hash[:])

	token := &models.APIToken{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      req.Name,
		Token:     tokenStr,
		Scopes:    req.Scopes,
		ExpiresAt: req.ExpiresAt,
		CreatedAt: time.Now(),
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO api_tokens (id, user_id, name, token_hash, scopes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		token.ID, token.UserID, token.Name, tokenHash, token.Scopes, token.ExpiresAt, token.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert token: %w", err)
	}

	return token, nil
}

// ValidateToken checks a raw API token and returns the user.
func (s *Service) ValidateToken(ctx context.Context, rawToken string) (*models.User, error) {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	var userID uuid.UUID
	var expiresAt *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT user_id, expires_at FROM api_tokens WHERE token_hash = $1`, tokenHash,
	).Scan(&userID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query token: %w", err)
	}

	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return nil, ErrTokenNotFound
	}

	// Update last_used timestamp
	s.db.Exec(ctx, `UPDATE api_tokens SET last_used = now() WHERE token_hash = $1`, tokenHash)

	return s.GetByID(ctx, userID)
}

// ListTokens returns all tokens for a user (without the raw token).
func (s *Service) ListTokens(ctx context.Context, userID uuid.UUID) ([]models.APIToken, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, name, scopes, expires_at, last_used, created_at
		FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query tokens: %w", err)
	}
	defer rows.Close()

	var tokens []models.APIToken
	for rows.Next() {
		var t models.APIToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Scopes, &t.ExpiresAt, &t.LastUsed, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tokens: %w", err)
	}
	return tokens, nil
}

// DeleteToken removes an API token.
func (s *Service) DeleteToken(ctx context.Context, userID uuid.UUID, tokenID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM api_tokens WHERE id = $1 AND user_id = $2`, tokenID, userID)
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenNotFound
	}
	return nil
}
