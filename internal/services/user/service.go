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
	ErrTooManyPins    = errors.New("maximum 6 pinned repos allowed")
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,37}[a-zA-Z0-9])?$`)

// OrgNameChecker can check if an org name exists. Used to prevent
// username/org-name collisions without a circular import.
type OrgNameChecker interface {
	NameExists(ctx context.Context, name string) (bool, error)
}

type Service struct {
	db       *pgxpool.Pool
	orgCheck OrgNameChecker
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

// SetOrgNameChecker sets the org name checker for cross-namespace validation.
func (s *Service) SetOrgNameChecker(checker OrgNameChecker) {
	s.orgCheck = checker
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
	if !strings.Contains(req.Email, "@") || len(req.Email) > 254 {
		return nil, fmt.Errorf("%w: invalid email address", ErrInvalidInput)
	}

	// Check for org name collision
	if s.orgCheck != nil {
		exists, err := s.orgCheck.NameExists(ctx, req.Username)
		if err != nil {
			return nil, fmt.Errorf("check org name conflict: %w", err)
		}
		if exists {
			return nil, ErrDuplicateUser
		}
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

// FindOrCreateByOAuth looks up an existing user linked to the given OAuth
// provider+providerID. If none exists, it looks for a user with a matching
// email and links the account. If no user exists at all, it creates one
// (with NULL password) and links the OAuth account.
func (s *Service) FindOrCreateByOAuth(ctx context.Context, provider, providerID, email, username, fullName, avatarURL, accessToken string) (*models.User, error) {
	email = strings.ToLower(email)
	username = strings.ToLower(username)

	// 1. Check if an oauth_account already exists for this provider+id.
	var existingUserID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT user_id FROM oauth_accounts
		WHERE provider = $1 AND provider_id = $2`, provider, providerID,
	).Scan(&existingUserID)
	if err == nil {
		// Update access token.
		s.db.Exec(ctx, `UPDATE oauth_accounts SET access_token = $1
			WHERE provider = $2 AND provider_id = $3`, accessToken, provider, providerID)
		return s.GetByID(ctx, existingUserID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("lookup oauth account: %w", err)
	}

	// 2. Check if a user with this email already exists.
	// Only auto-link if the user has no password (OAuth-only account).
	// Password-protected accounts require the user to link manually to
	// prevent account takeover via GitHub email spoofing.
	existingUser := &models.User{}
	var hasPassword bool
	err = s.db.QueryRow(ctx, `
		SELECT id, username, email, (password IS NOT NULL AND password != '') AS has_pw,
		       full_name, avatar_url, bio, is_admin, created_at, updated_at
		FROM users WHERE email = $1`, email,
	).Scan(
		&existingUser.ID, &existingUser.Username, &existingUser.Email, &hasPassword,
		&existingUser.FullName, &existingUser.AvatarURL, &existingUser.Bio,
		&existingUser.IsAdmin, &existingUser.CreatedAt, &existingUser.UpdatedAt,
	)
	if err == nil && !hasPassword {
		// Safe to auto-link: this is an OAuth-only account with the same email.
		_, err = s.db.Exec(ctx, `
			INSERT INTO oauth_accounts (id, user_id, provider, provider_id, access_token, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (provider, provider_id) DO UPDATE SET access_token = $5`,
			uuid.New(), existingUser.ID, provider, providerID, accessToken, time.Now(),
		)
		if err != nil {
			return nil, fmt.Errorf("link oauth account: %w", err)
		}
		return existingUser, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("lookup user by email: %w", err)
	}
	// If hasPassword is true, fall through to create a new account —
	// the user can link their GitHub later from settings.

	// 3. Create a new user inside a transaction to prevent race conditions.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin oauth tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Resolve username conflicts by appending numeric suffix (capped at 100).
	finalUsername := username
	for i := 1; i <= 100; i++ {
		var exists bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)`, finalUsername).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("check username: %w", err)
		}
		if !exists {
			break
		}
		finalUsername = fmt.Sprintf("%s%d", username, i)
		if i == 100 {
			return nil, fmt.Errorf("%w: could not find available username", ErrInvalidInput)
		}
	}

	now := time.Now()
	newUser := &models.User{
		ID:        uuid.New(),
		Username:  finalUsername,
		Email:     email,
		FullName:  fullName,
		AvatarURL: avatarURL,
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, username, email, password, full_name, avatar_url, created_at, updated_at)
		VALUES ($1, $2, $3, NULL, $4, $5, $6, $6)`,
		newUser.ID, newUser.Username, newUser.Email, newUser.FullName, newUser.AvatarURL, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create oauth user: %w", err)
	}

	// Link oauth account (ON CONFLICT handles concurrent race).
	_, err = tx.Exec(ctx, `
		INSERT INTO oauth_accounts (id, user_id, provider, provider_id, access_token, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (provider, provider_id) DO UPDATE SET access_token = $5, user_id = $2`,
		uuid.New(), newUser.ID, provider, providerID, accessToken, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create oauth link: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit oauth tx: %w", err)
	}

	return newUser, nil
}

func (s *Service) Authenticate(ctx context.Context, login, password string) (*models.User, error) {
	login = strings.ToLower(login)
	user := &models.User{}
	err := s.db.QueryRow(ctx, `
		SELECT id, username, email, COALESCE(password, ''), full_name, avatar_url, bio, is_admin, created_at, updated_at
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

// GetByIDWithPassword returns a user by ID including the password hash.
// Used by the TOTP service for password re-authentication during 2FA setup.
func (s *Service) GetByIDWithPassword(ctx context.Context, id uuid.UUID) (*models.User, error) {
	user := &models.User{}
	err := s.db.QueryRow(ctx, `
		SELECT id, username, email, COALESCE(password, ''), full_name, avatar_url, bio, is_admin, created_at, updated_at
		FROM users WHERE id = $1`, id,
	).Scan(
		&user.ID, &user.Username, &user.Email, &user.Password,
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
		if len(*req.FullName) > 100 {
			return nil, fmt.Errorf("%w: full name must be at most 100 characters", ErrInvalidInput)
		}
		setClauses = append(setClauses, fmt.Sprintf("full_name = $%d", argIdx))
		args = append(args, *req.FullName)
		argIdx++
	}
	if req.Bio != nil {
		if len(*req.Bio) > 500 {
			return nil, fmt.Errorf("%w: bio must be at most 500 characters", ErrInvalidInput)
		}
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
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 100 {
		return nil, fmt.Errorf("%w: token name must be 1-100 characters", ErrInvalidInput)
	}
	req.Name = name

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

	u, err := s.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Update last_used timestamp only after all validation succeeds.
	s.db.Exec(ctx, `UPDATE api_tokens SET last_used = now() WHERE token_hash = $1`, tokenHash)

	return u, nil
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

// GetContributions returns per-day contribution counts for a user within a date range.
// Contributions are aggregated from commits, issues, and pull requests.
func (s *Service) GetContributions(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]models.DayCount, error) {
	query := `
		SELECT d::date AS day, COALESCE(SUM(cnt), 0)::int AS count
		FROM generate_series($2::date, $3::date, '1 day'::interval) d
		LEFT JOIN (
			SELECT committed_at::date AS day, COUNT(*) AS cnt
			FROM commit_metadata WHERE author_id = $1
			  AND committed_at >= $2 AND committed_at < ($3::date + '1 day'::interval)
			GROUP BY 1
			UNION ALL
			SELECT created_at::date AS day, COUNT(*) AS cnt
			FROM issues WHERE author_id = $1
			  AND created_at >= $2 AND created_at < ($3::date + '1 day'::interval)
			GROUP BY 1
			UNION ALL
			SELECT created_at::date AS day, COUNT(*) AS cnt
			FROM pull_requests WHERE author_id = $1
			  AND created_at >= $2 AND created_at < ($3::date + '1 day'::interval)
			GROUP BY 1
		) sub ON sub.day = d::date
		GROUP BY 1
		ORDER BY 1`

	rows, err := s.db.Query(ctx, query, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query contributions: %w", err)
	}
	defer rows.Close()

	var days []models.DayCount
	for rows.Next() {
		var dc models.DayCount
		var day time.Time
		if err := rows.Scan(&day, &dc.Count); err != nil {
			return nil, fmt.Errorf("scan contribution: %w", err)
		}
		dc.Date = day.Format("2006-01-02")
		days = append(days, dc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contributions: %w", err)
	}
	return days, nil
}

// ListPinnedRepos returns a user's pinned repositories with full repo details.
func (s *Service) ListPinnedRepos(ctx context.Context, userID uuid.UUID) ([]models.PinnedRepo, error) {
	query := `
		SELECT p.position,
		       r.id, r.owner_id, COALESCE(u.username, o.name) AS owner_name,
		       r.name, r.description, r.default_branch,
		       r.visibility, r.language_stats, r.topics, r.stars_count, r.forks_count,
		       r.created_at, r.updated_at
		FROM pinned_repos p
		JOIN repositories r ON r.id = p.repo_id
		LEFT JOIN users u ON u.id = r.owner_id AND r.owner_type = 'user'
		LEFT JOIN organizations o ON o.id = r.owner_id AND r.owner_type = 'org'
		WHERE p.user_id = $1
		ORDER BY p.position`

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query pinned repos: %w", err)
	}
	defer rows.Close()

	var pinned []models.PinnedRepo
	for rows.Next() {
		var pr models.PinnedRepo
		if err := rows.Scan(
			&pr.Position,
			&pr.Repository.ID, &pr.Repository.OwnerID, &pr.Repository.OwnerName,
			&pr.Repository.Name, &pr.Repository.Description, &pr.Repository.DefaultBranch,
			&pr.Repository.Visibility, &pr.Repository.LanguageStats, &pr.Repository.Topics,
			&pr.Repository.StarsCount, &pr.Repository.ForksCount,
			&pr.Repository.CreatedAt, &pr.Repository.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan pinned repo: %w", err)
		}
		pinned = append(pinned, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pinned repos: %w", err)
	}
	return pinned, nil
}

// SetPinnedRepos replaces all pinned repos for a user. Maximum 6 repos.
func (s *Service) SetPinnedRepos(ctx context.Context, userID uuid.UUID, repoIDs []uuid.UUID) error {
	if len(repoIDs) > 6 {
		return ErrTooManyPins
	}

	// Validate all repos are accessible to this user (public or owned)
	if len(repoIDs) > 0 {
		var invalidCount int
		err := s.db.QueryRow(ctx, `
			SELECT COUNT(*) FROM unnest($1::uuid[]) AS rid(id)
			WHERE NOT EXISTS (
				SELECT 1 FROM repositories r
				WHERE r.id = rid.id AND (r.visibility = 'public' OR r.owner_id = $2)
			)`, repoIDs, userID).Scan(&invalidCount)
		if err != nil {
			return fmt.Errorf("validate pinned repos: %w", err)
		}
		if invalidCount > 0 {
			return fmt.Errorf("%w: one or more repos are not accessible", ErrInvalidInput)
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM pinned_repos WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete pinned repos: %w", err)
	}

	for i, repoID := range repoIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO pinned_repos (user_id, repo_id, position) VALUES ($1, $2, $3)`,
			userID, repoID, i,
		); err != nil {
			return fmt.Errorf("insert pinned repo: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit pinned repos: %w", err)
	}
	return nil
}
