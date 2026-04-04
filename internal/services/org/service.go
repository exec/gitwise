package org

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var ErrNotFound = errors.New("organization not found")

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
