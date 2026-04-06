package org

import (
	"context"
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
	ErrNotFound      = errors.New("organization not found")
	ErrDuplicate     = errors.New("organization name already taken")
	ErrInvalidName   = errors.New("invalid organization name")
	ErrNameConflict  = errors.New("name conflicts with an existing username")
	ErrForbidden     = errors.New("access denied")
	ErrLastOwner     = errors.New("cannot remove the last owner")
	ErrMemberNotFound = errors.New("member not found")
	ErrInvalidRole   = errors.New("invalid role: must be 'owner' or 'member'")
)

var orgNameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,37}[a-zA-Z0-9])?$`)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) GetByName(ctx context.Context, name string) (*models.Organization, error) {
	var o models.Organization
	err := s.db.QueryRow(ctx, `
		SELECT id, name, display_name, description, avatar_url, created_at, updated_at
		FROM organizations
		WHERE name = $1`, name).Scan(
		&o.ID, &o.Name, &o.DisplayName, &o.Description, &o.AvatarURL,
		&o.CreatedAt, &o.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get org by name: %w", err)
	}
	return &o, nil
}

func (s *Service) ListMembers(ctx context.Context, orgID uuid.UUID) ([]models.OrgMember, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.id, u.username, u.full_name, u.avatar_url, om.role
		FROM org_members om
		JOIN users u ON u.id = om.user_id
		WHERE om.org_id = $1
		ORDER BY om.role ASC, u.username ASC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list org members: %w", err)
	}
	defer rows.Close()

	var members []models.OrgMember
	for rows.Next() {
		var m models.OrgMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.FullName, &m.AvatarURL, &m.Role); err != nil {
			return nil, fmt.Errorf("scan org member: %w", err)
		}
		members = append(members, m)
	}
	if members == nil {
		members = []models.OrgMember{}
	}
	return members, nil
}

func (s *Service) ListRepos(ctx context.Context, orgID uuid.UUID, viewerID *uuid.UUID, limit int) ([]models.Repository, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	// If the viewer is a member of the org, show private repos too.
	var isMember bool
	if viewerID != nil {
		err := s.db.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM org_members WHERE org_id = $1 AND user_id = $2)`,
			orgID, *viewerID).Scan(&isMember)
		if err != nil {
			return nil, fmt.Errorf("check membership: %w", err)
		}
	}

	query := `
		SELECT r.id, r.owner_id, o.name, r.name, r.description, r.default_branch,
		       r.visibility, r.language_stats, r.topics, r.stars_count, r.forks_count,
		       r.created_at, r.updated_at
		FROM repositories r
		JOIN organizations o ON o.id = r.owner_id
		WHERE r.owner_id = $1
		  AND (r.visibility = 'public' OR $2)
		ORDER BY r.updated_at DESC
		LIMIT $3`

	rows, err := s.db.Query(ctx, query, orgID, isMember, limit)
	if err != nil {
		return nil, fmt.Errorf("list org repos: %w", err)
	}
	defer rows.Close()

	var repos []models.Repository
	for rows.Next() {
		var r models.Repository
		if err := rows.Scan(
			&r.ID, &r.OwnerID, &r.OwnerName, &r.Name, &r.Description,
			&r.DefaultBranch, &r.Visibility, &r.LanguageStats, &r.Topics,
			&r.StarsCount, &r.ForksCount, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan org repo: %w", err)
		}
		repos = append(repos, r)
	}
	if repos == nil {
		repos = []models.Repository{}
	}
	return repos, nil
}

// Create creates a new organization and makes the creator an owner.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, req models.CreateOrgRequest) (*models.Organization, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !orgNameRe.MatchString(name) {
		return nil, fmt.Errorf("%w: must be 1-39 alphanumeric characters, hyphens, dots, or underscores", ErrInvalidName)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check for username conflict inside transaction to prevent TOCTOU race
	var usernameExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE username = $1 FOR SHARE)`, name).Scan(&usernameExists); err != nil {
		return nil, fmt.Errorf("check username conflict: %w", err)
	}
	if usernameExists {
		return nil, ErrNameConflict
	}

	o := &models.Organization{
		ID:          uuid.New(),
		Name:        name,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Description: strings.TrimSpace(req.Description),
	}
	now := time.Now()
	o.CreatedAt = now
	o.UpdatedAt = now

	_, err = tx.Exec(ctx, `
		INSERT INTO organizations (id, name, display_name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		o.ID, o.Name, o.DisplayName, o.Description, o.CreatedAt, o.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("insert org: %w", err)
	}

	// Make creator the owner
	_, err = tx.Exec(ctx, `
		INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, 'owner')`,
		o.ID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert org owner: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return o, nil
}

// Update updates an organization's display_name, description, and avatar_url.
func (s *Service) Update(ctx context.Context, orgName string, req models.UpdateOrgRequest) (*models.Organization, error) {
	setClauses := []string{"updated_at = now()"}
	args := []any{orgName}
	argIdx := 2

	if req.DisplayName != nil {
		setClauses = append(setClauses, fmt.Sprintf("display_name = $%d", argIdx))
		args = append(args, strings.TrimSpace(*req.DisplayName))
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, strings.TrimSpace(*req.Description))
		argIdx++
	}
	if req.AvatarURL != nil {
		setClauses = append(setClauses, fmt.Sprintf("avatar_url = $%d", argIdx))
		args = append(args, strings.TrimSpace(*req.AvatarURL))
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE organizations SET %s
		WHERE name = $1
		RETURNING id, name, display_name, description, avatar_url, created_at, updated_at`,
		strings.Join(setClauses, ", "))

	var o models.Organization
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&o.ID, &o.Name, &o.DisplayName, &o.Description, &o.AvatarURL,
		&o.CreatedAt, &o.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update org: %w", err)
	}
	return &o, nil
}

// Delete deletes an organization and all its members (cascading).
func (s *Service) Delete(ctx context.Context, orgName string) error {
	// Delete org-owned repos first (no FK cascade since owner_id is polymorphic)
	_, err := s.db.Exec(ctx, `
		DELETE FROM repositories WHERE owner_id = (
			SELECT id FROM organizations WHERE name = $1
		) AND owner_type = 'org'`, orgName)
	if err != nil {
		return fmt.Errorf("delete org repos: %w", err)
	}

	tag, err := s.db.Exec(ctx, `DELETE FROM organizations WHERE name = $1`, orgName)
	if err != nil {
		return fmt.Errorf("delete org: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IsOwner checks whether a user is an owner of the named organization.
func (s *Service) IsOwner(ctx context.Context, orgName string, userID uuid.UUID) (bool, error) {
	var isOwner bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM org_members om
			JOIN organizations o ON o.id = om.org_id
			WHERE o.name = $1 AND om.user_id = $2 AND om.role = 'owner'
		)`, orgName, userID).Scan(&isOwner)
	if err != nil {
		return false, fmt.Errorf("check org owner: %w", err)
	}
	return isOwner, nil
}

// IsMember checks whether a user is a member (any role) of the named organization.
func (s *Service) IsMember(ctx context.Context, orgName string, userID uuid.UUID) (bool, error) {
	var isMember bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM org_members om
			JOIN organizations o ON o.id = om.org_id
			WHERE o.name = $1 AND om.user_id = $2
		)`, orgName, userID).Scan(&isMember)
	if err != nil {
		return false, fmt.Errorf("check org member: %w", err)
	}
	return isMember, nil
}

// AddMember adds a user to an organization or updates their role.
func (s *Service) AddMember(ctx context.Context, orgName, username, role string) error {
	role = strings.ToLower(strings.TrimSpace(role))
	if role != "owner" && role != "member" {
		return ErrInvalidRole
	}

	// Look up org
	o, err := s.GetByName(ctx, orgName)
	if err != nil {
		return err
	}

	// Look up user by username
	var userID uuid.UUID
	err = s.db.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, strings.ToLower(username)).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (org_id, user_id) DO UPDATE SET role = $3`,
		o.ID, userID, role,
	)
	if err != nil {
		return fmt.Errorf("upsert org member: %w", err)
	}
	return nil
}

// RemoveMember removes a user from an organization. Prevents removing the last owner.
func (s *Service) RemoveMember(ctx context.Context, orgName, username string) error {
	o, err := s.GetByName(ctx, orgName)
	if err != nil {
		return err
	}

	// Look up user by username
	var userID uuid.UUID
	err = s.db.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, strings.ToLower(username)).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}

	// Check if this user is an owner and if they're the last one
	var memberRole string
	err = s.db.QueryRow(ctx, `
		SELECT role FROM org_members WHERE org_id = $1 AND user_id = $2`, o.ID, userID).Scan(&memberRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMemberNotFound
	}
	if err != nil {
		return fmt.Errorf("get member role: %w", err)
	}

	if memberRole == "owner" {
		var ownerCount int
		if err := s.db.QueryRow(ctx, `
			SELECT COUNT(*) FROM org_members WHERE org_id = $1 AND role = 'owner'`, o.ID).Scan(&ownerCount); err != nil {
			return fmt.Errorf("count owners: %w", err)
		}
		if ownerCount <= 1 {
			return ErrLastOwner
		}
	}

	tag, err := s.db.Exec(ctx, `DELETE FROM org_members WHERE org_id = $1 AND user_id = $2`, o.ID, userID)
	if err != nil {
		return fmt.Errorf("delete org member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

// UpdateMemberRole changes a member's role in an organization.
func (s *Service) UpdateMemberRole(ctx context.Context, orgName, username, role string) error {
	role = strings.ToLower(strings.TrimSpace(role))
	if role != "owner" && role != "member" {
		return ErrInvalidRole
	}

	o, err := s.GetByName(ctx, orgName)
	if err != nil {
		return err
	}

	var userID uuid.UUID
	err = s.db.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, strings.ToLower(username)).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}

	// If demoting an owner, make sure they're not the last one
	var currentRole string
	err = s.db.QueryRow(ctx, `
		SELECT role FROM org_members WHERE org_id = $1 AND user_id = $2`, o.ID, userID).Scan(&currentRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMemberNotFound
	}
	if err != nil {
		return fmt.Errorf("get current role: %w", err)
	}

	if currentRole == "owner" && role == "member" {
		var ownerCount int
		if err := s.db.QueryRow(ctx, `
			SELECT COUNT(*) FROM org_members WHERE org_id = $1 AND role = 'owner'`, o.ID).Scan(&ownerCount); err != nil {
			return fmt.Errorf("count owners: %w", err)
		}
		if ownerCount <= 1 {
			return ErrLastOwner
		}
	}

	_, err = s.db.Exec(ctx, `
		UPDATE org_members SET role = $3 WHERE org_id = $1 AND user_id = $2`, o.ID, userID, role)
	if err != nil {
		return fmt.Errorf("update member role: %w", err)
	}
	return nil
}

// ListUserOrgs returns all organizations a user is a member of.
func (s *Service) ListUserOrgs(ctx context.Context, userID uuid.UUID) ([]models.OrgMembership, error) {
	rows, err := s.db.Query(ctx, `
		SELECT o.id, o.name, o.display_name, o.avatar_url, om.role
		FROM org_members om
		JOIN organizations o ON o.id = om.org_id
		WHERE om.user_id = $1
		ORDER BY o.name ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user orgs: %w", err)
	}
	defer rows.Close()

	var orgs []models.OrgMembership
	for rows.Next() {
		var m models.OrgMembership
		if err := rows.Scan(&m.ID, &m.Name, &m.DisplayName, &m.AvatarURL, &m.Role); err != nil {
			return nil, fmt.Errorf("scan user org: %w", err)
		}
		orgs = append(orgs, m)
	}
	if orgs == nil {
		orgs = []models.OrgMembership{}
	}
	return orgs, nil
}

// ListPublicUserOrgs returns organizations a user belongs to (public info, no role).
// Used for displaying orgs on a user's public profile page.
func (s *Service) ListPublicUserOrgs(ctx context.Context, username string) ([]models.OrgMembership, error) {
	rows, err := s.db.Query(ctx, `
		SELECT o.id, o.name, o.display_name, o.avatar_url, om.role
		FROM org_members om
		JOIN organizations o ON o.id = om.org_id
		JOIN users u ON u.id = om.user_id
		WHERE u.username = $1
		ORDER BY o.name ASC`, username)
	if err != nil {
		return nil, fmt.Errorf("list public user orgs: %w", err)
	}
	defer rows.Close()

	var orgs []models.OrgMembership
	for rows.Next() {
		var m models.OrgMembership
		if err := rows.Scan(&m.ID, &m.Name, &m.DisplayName, &m.AvatarURL, &m.Role); err != nil {
			return nil, fmt.Errorf("scan public user org: %w", err)
		}
		orgs = append(orgs, m)
	}
	if orgs == nil {
		orgs = []models.OrgMembership{}
	}
	return orgs, nil
}

// NameExists returns true if an organization with the given name exists.
func (s *Service) NameExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE name = $1)`, strings.ToLower(name)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check org name: %w", err)
	}
	return exists, nil
}
