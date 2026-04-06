package team

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrNotFound       = errors.New("team not found")
	ErrDuplicate      = errors.New("team already exists")
	ErrInvalidName    = errors.New("team name is required (max 100 chars)")
	ErrInvalidPerm    = errors.New("permission must be read, triage, write, or admin")
	ErrNotOrgOwner    = errors.New("only org owners can manage teams")
	ErrMemberNotFound = errors.New("user not found or not an org member")
	ErrRepoNotFound   = errors.New("repository not found or not owned by this org")
)

var validPermissions = map[string]int{
	"read":   1,
	"triage": 2,
	"write":  3,
	"admin":  4,
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

// IsOrgOwner checks whether the given user is an owner of the org.
func (s *Service) IsOrgOwner(ctx context.Context, orgID, userID uuid.UUID) (bool, error) {
	var role string
	err := s.db.QueryRow(ctx, `
		SELECT role FROM org_members WHERE org_id = $1 AND user_id = $2`,
		orgID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check org owner: %w", err)
	}
	return role == "owner", nil
}

// Create creates a new team in the given organization.
func (s *Service) Create(ctx context.Context, orgID uuid.UUID, req models.CreateTeamRequest) (*models.OrgTeam, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 100 {
		return nil, ErrInvalidName
	}

	perm := strings.TrimSpace(req.Permission)
	if perm == "" {
		perm = "read"
	}
	if _, ok := validPermissions[perm]; !ok {
		return nil, ErrInvalidPerm
	}

	team := &models.OrgTeam{
		ID:          uuid.New(),
		OrgID:       orgID,
		Name:        name,
		Description: req.Description,
		Permission:  perm,
	}

	err := s.db.QueryRow(ctx, `
		INSERT INTO org_teams (id, org_id, name, description, permission)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`,
		team.ID, team.OrgID, team.Name, team.Description, team.Permission,
	).Scan(&team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("create team: %w", err)
	}
	return team, nil
}

// Get returns a team by org ID and team name (slug).
func (s *Service) Get(ctx context.Context, orgID uuid.UUID, teamName string) (*models.OrgTeam, error) {
	var t models.OrgTeam
	err := s.db.QueryRow(ctx, `
		SELECT t.id, t.org_id, t.name, t.description, t.permission,
		       (SELECT count(*) FROM org_team_members WHERE team_id = t.id) AS member_count,
		       (SELECT count(*) FROM org_team_repos WHERE team_id = t.id) AS repo_count,
		       t.created_at, t.updated_at
		FROM org_teams t
		WHERE t.org_id = $1 AND t.name = $2`, orgID, teamName,
	).Scan(&t.ID, &t.OrgID, &t.Name, &t.Description, &t.Permission,
		&t.MemberCount, &t.RepoCount, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	return &t, nil
}

// List returns all teams for an organization.
func (s *Service) List(ctx context.Context, orgID uuid.UUID) ([]models.OrgTeam, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id, t.org_id, t.name, t.description, t.permission,
		       (SELECT count(*) FROM org_team_members WHERE team_id = t.id) AS member_count,
		       (SELECT count(*) FROM org_team_repos WHERE team_id = t.id) AS repo_count,
		       t.created_at, t.updated_at
		FROM org_teams t
		WHERE t.org_id = $1
		ORDER BY t.name ASC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	var teams []models.OrgTeam
	for rows.Next() {
		var t models.OrgTeam
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.Description, &t.Permission,
			&t.MemberCount, &t.RepoCount, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		teams = append(teams, t)
	}
	if teams == nil {
		teams = []models.OrgTeam{}
	}
	return teams, nil
}

// Update modifies a team's name, description, or permission.
func (s *Service) Update(ctx context.Context, orgID uuid.UUID, teamName string, req models.UpdateTeamRequest) (*models.OrgTeam, error) {
	// First get the team
	existing, err := s.Get(ctx, orgID, teamName)
	if err != nil {
		return nil, err
	}

	setClauses := []string{"updated_at = now()"}
	args := []any{existing.ID}
	argIdx := 2

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 100 {
			return nil, ErrInvalidName
		}
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, name)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.Permission != nil {
		perm := strings.TrimSpace(*req.Permission)
		if _, ok := validPermissions[perm]; !ok {
			return nil, ErrInvalidPerm
		}
		setClauses = append(setClauses, fmt.Sprintf("permission = $%d", argIdx))
		args = append(args, perm)
		argIdx++
	}

	query := fmt.Sprintf(`UPDATE org_teams SET %s WHERE id = $1
		RETURNING id, org_id, name, description, permission, created_at, updated_at`,
		strings.Join(setClauses, ", "))

	var t models.OrgTeam
	err = s.db.QueryRow(ctx, query, args...).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Description, &t.Permission,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("update team: %w", err)
	}

	// Fill in counts
	t.MemberCount = existing.MemberCount
	t.RepoCount = existing.RepoCount
	return &t, nil
}

// Delete removes a team.
func (s *Service) Delete(ctx context.Context, orgID uuid.UUID, teamName string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM org_teams WHERE org_id = $1 AND name = $2`, orgID, teamName)
	if err != nil {
		return fmt.Errorf("delete team: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AddMember adds a user to a team. The user must be a member of the org.
func (s *Service) AddMember(ctx context.Context, orgID uuid.UUID, teamName string, username string) error {
	team, err := s.Get(ctx, orgID, teamName)
	if err != nil {
		return err
	}

	// Look up the user and verify org membership
	var userID uuid.UUID
	err = s.db.QueryRow(ctx, `
		SELECT u.id FROM users u
		JOIN org_members om ON om.user_id = u.id AND om.org_id = $1
		WHERE u.username = $2`, orgID, username).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMemberNotFound
	}
	if err != nil {
		return fmt.Errorf("look up user for team: %w", err)
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO org_team_members (team_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, team.ID, userID)
	if err != nil {
		return fmt.Errorf("add team member: %w", err)
	}
	return nil
}

// RemoveMember removes a user from a team.
func (s *Service) RemoveMember(ctx context.Context, orgID uuid.UUID, teamName string, username string) error {
	team, err := s.Get(ctx, orgID, teamName)
	if err != nil {
		return err
	}

	var userID uuid.UUID
	err = s.db.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, username).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMemberNotFound
	}
	if err != nil {
		return fmt.Errorf("look up user: %w", err)
	}

	tag, err := s.db.Exec(ctx, `
		DELETE FROM org_team_members WHERE team_id = $1 AND user_id = $2`, team.ID, userID)
	if err != nil {
		return fmt.Errorf("remove team member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

// ListMembers returns all members of a team.
func (s *Service) ListMembers(ctx context.Context, orgID uuid.UUID, teamName string) ([]models.OrgTeamMember, error) {
	team, err := s.Get(ctx, orgID, teamName)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT u.id, u.username, u.full_name, u.avatar_url
		FROM org_team_members tm
		JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = $1
		ORDER BY u.username ASC`, team.ID)
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	defer rows.Close()

	var members []models.OrgTeamMember
	for rows.Next() {
		var m models.OrgTeamMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.FullName, &m.AvatarURL); err != nil {
			return nil, fmt.Errorf("scan team member: %w", err)
		}
		members = append(members, m)
	}
	if members == nil {
		members = []models.OrgTeamMember{}
	}
	return members, nil
}

// AddRepo assigns a repository to a team. The repo must be owned by the org.
func (s *Service) AddRepo(ctx context.Context, orgID uuid.UUID, teamName string, repoName string) error {
	team, err := s.Get(ctx, orgID, teamName)
	if err != nil {
		return err
	}

	// Verify repo is owned by this org
	var repoID uuid.UUID
	err = s.db.QueryRow(ctx, `
		SELECT id FROM repositories WHERE owner_id = $1 AND name = $2`,
		orgID, repoName).Scan(&repoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRepoNotFound
	}
	if err != nil {
		return fmt.Errorf("look up repo for team: %w", err)
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO org_team_repos (team_id, repo_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, team.ID, repoID)
	if err != nil {
		return fmt.Errorf("add team repo: %w", err)
	}
	return nil
}

// RemoveRepo unassigns a repository from a team.
func (s *Service) RemoveRepo(ctx context.Context, orgID uuid.UUID, teamName string, repoName string) error {
	team, err := s.Get(ctx, orgID, teamName)
	if err != nil {
		return err
	}

	var repoID uuid.UUID
	err = s.db.QueryRow(ctx, `
		SELECT id FROM repositories WHERE owner_id = $1 AND name = $2`,
		orgID, repoName).Scan(&repoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRepoNotFound
	}
	if err != nil {
		return fmt.Errorf("look up repo: %w", err)
	}

	tag, err := s.db.Exec(ctx, `
		DELETE FROM org_team_repos WHERE team_id = $1 AND repo_id = $2`, team.ID, repoID)
	if err != nil {
		return fmt.Errorf("remove team repo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRepoNotFound
	}
	return nil
}

// ListRepos returns all repositories assigned to a team.
func (s *Service) ListRepos(ctx context.Context, orgID uuid.UUID, teamName string) ([]models.OrgTeamRepo, error) {
	team, err := s.Get(ctx, orgID, teamName)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT r.id, r.name, r.description, r.visibility
		FROM org_team_repos tr
		JOIN repositories r ON r.id = tr.repo_id
		WHERE tr.team_id = $1
		ORDER BY r.name ASC`, team.ID)
	if err != nil {
		return nil, fmt.Errorf("list team repos: %w", err)
	}
	defer rows.Close()

	var repos []models.OrgTeamRepo
	for rows.Next() {
		var r models.OrgTeamRepo
		if err := rows.Scan(&r.RepoID, &r.Name, &r.Description, &r.Visibility); err != nil {
			return nil, fmt.Errorf("scan team repo: %w", err)
		}
		repos = append(repos, r)
	}
	if repos == nil {
		repos = []models.OrgTeamRepo{}
	}
	return repos, nil
}

// GetUserRepoPermission determines the highest permission level a user has
// on a repository through team membership. Returns empty string if no access.
// Org owners automatically get "admin" on all org repos.
func (s *Service) GetUserRepoPermission(ctx context.Context, userID, repoID uuid.UUID) (string, error) {
	// First check if the repo belongs to an org where the user is an owner
	var orgOwnerRole string
	err := s.db.QueryRow(ctx, `
		SELECT om.role FROM org_members om
		JOIN repositories r ON r.owner_id = om.org_id AND r.owner_type = 'org'
		WHERE r.id = $1 AND om.user_id = $2`, repoID, userID).Scan(&orgOwnerRole)
	if err == nil && orgOwnerRole == "owner" {
		return "admin", nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("check org owner permission: %w", err)
	}

	// Check team-based permissions: find the highest permission across all teams
	// the user belongs to that have this repo assigned.
	var perm string
	err = s.db.QueryRow(ctx, `
		SELECT t.permission FROM org_teams t
		JOIN org_team_members tm ON tm.team_id = t.id
		JOIN org_team_repos tr ON tr.team_id = t.id
		WHERE tm.user_id = $1 AND tr.repo_id = $2
		ORDER BY CASE t.permission
			WHEN 'admin' THEN 4
			WHEN 'write' THEN 3
			WHEN 'triage' THEN 2
			WHEN 'read' THEN 1
		END DESC
		LIMIT 1`, userID, repoID).Scan(&perm)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get team repo permission: %w", err)
	}
	return perm, nil
}

// PermissionLevel returns the numeric level of a permission string.
// Higher is more powerful: read=1, triage=2, write=3, admin=4.
// Returns 0 for unknown permissions.
func PermissionLevel(perm string) int {
	return validPermissions[perm]
}
