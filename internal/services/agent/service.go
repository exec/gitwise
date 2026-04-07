package agent

import (
	"context"
	"encoding/json"
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
	ErrNotFound       = errors.New("agent not found")
	ErrDuplicate      = errors.New("agent slug already exists")
	ErrInvalidName    = errors.New("agent name is required")
	ErrInvalidSlug    = errors.New("invalid agent slug")
	ErrForbidden      = errors.New("access denied")
	ErrAlreadyInstalled = errors.New("agent already installed on this repo")
	ErrNotInstalled   = errors.New("agent not installed on this repo")
	ErrTaskNotFound   = errors.New("agent task not found")
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,98}[a-z0-9]$`)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

// ListAvailable returns all official agents plus agents authored by the given user.
func (s *Service) ListAvailable(ctx context.Context, userID *uuid.UUID) ([]models.Agent, error) {
	var rows pgx.Rows
	var err error

	if userID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT id, name, slug, description, is_official, author_id, config, created_at, updated_at
			FROM agents
			WHERE is_official = true OR author_id = $1
			ORDER BY is_official DESC, name ASC`, *userID)
	} else {
		rows, err = s.db.Query(ctx, `
			SELECT id, name, slug, description, is_official, author_id, config, created_at, updated_at
			FROM agents
			WHERE is_official = true
			ORDER BY name ASC`)
	}
	if err != nil {
		return nil, fmt.Errorf("query agents: %w", err)
	}
	defer rows.Close()

	return scanAgents(rows)
}

// Create creates a new custom agent.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, req models.CreateAgentRequest) (*models.Agent, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 100 {
		return nil, ErrInvalidName
	}

	slug := strings.TrimSpace(req.Slug)
	if !slugRe.MatchString(slug) {
		return nil, ErrInvalidSlug
	}

	cfg := req.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}

	agent := &models.Agent{
		ID:          uuid.New(),
		Name:        name,
		Slug:        slug,
		Description: req.Description,
		IsOfficial:  false,
		AuthorID:    &userID,
		Config:      cfg,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO agents (id, name, slug, description, is_official, author_id, config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		agent.ID, agent.Name, agent.Slug, agent.Description, agent.IsOfficial,
		agent.AuthorID, agent.Config, agent.CreatedAt, agent.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("insert agent: %w", err)
	}

	return agent, nil
}

// GetBySlug returns an agent by its slug.
func (s *Service) GetBySlug(ctx context.Context, slug string) (*models.Agent, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, name, slug, description, is_official, author_id, config, created_at, updated_at
		FROM agents
		WHERE slug = $1`, slug)

	agent, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query agent: %w", err)
	}
	return agent, nil
}

// Update updates a custom agent (only the author can update).
func (s *Service) Update(ctx context.Context, slug string, userID uuid.UUID, req models.UpdateAgentRequest) (*models.Agent, error) {
	agent, err := s.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}

	if agent.IsOfficial || agent.AuthorID == nil || *agent.AuthorID != userID {
		return nil, ErrForbidden
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 100 {
			return nil, ErrInvalidName
		}
		agent.Name = name
	}
	if req.Description != nil {
		agent.Description = *req.Description
	}
	if req.Config != nil {
		agent.Config = *req.Config
	}
	agent.UpdatedAt = time.Now()

	_, err = s.db.Exec(ctx, `
		UPDATE agents SET name = $1, description = $2, config = $3, updated_at = $4
		WHERE slug = $5`,
		agent.Name, agent.Description, agent.Config, agent.UpdatedAt, slug)
	if err != nil {
		return nil, fmt.Errorf("update agent: %w", err)
	}

	return agent, nil
}

// Delete deletes a custom agent (only the author can delete).
func (s *Service) Delete(ctx context.Context, slug string, userID uuid.UUID) error {
	agent, err := s.GetBySlug(ctx, slug)
	if err != nil {
		return err
	}

	if agent.IsOfficial || agent.AuthorID == nil || *agent.AuthorID != userID {
		return ErrForbidden
	}

	_, err = s.db.Exec(ctx, `DELETE FROM agents WHERE id = $1`, agent.ID)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
}

// ListForRepo returns all agents installed on a repository.
func (s *Service) ListForRepo(ctx context.Context, repoID uuid.UUID) ([]models.RepoAgent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT ra.id, ra.repo_id, ra.agent_id, a.name, a.slug,
			ra.enabled, ra.config, ra.instructions, ra.trigger_events,
			ra.created_at, ra.updated_at
		FROM repo_agents ra
		JOIN agents a ON a.id = ra.agent_id
		WHERE ra.repo_id = $1
		ORDER BY a.name ASC`, repoID)
	if err != nil {
		return nil, fmt.Errorf("query repo agents: %w", err)
	}
	defer rows.Close()

	return scanRepoAgents(rows)
}

// Install installs an agent on a repository.
func (s *Service) Install(ctx context.Context, repoID uuid.UUID, agentSlug string, req models.InstallAgentRequest) (*models.RepoAgent, error) {
	agent, err := s.GetBySlug(ctx, agentSlug)
	if err != nil {
		return nil, err
	}

	cfg := req.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}

	triggerEvents := req.TriggerEvents
	if triggerEvents == nil {
		triggerEvents = []string{"push"}
	}

	ra := &models.RepoAgent{
		ID:            uuid.New(),
		RepoID:        repoID,
		AgentID:       agent.ID,
		AgentName:     agent.Name,
		AgentSlug:     agent.Slug,
		Enabled:       true,
		Config:        cfg,
		Instructions:  req.Instructions,
		TriggerEvents: triggerEvents,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO repo_agents (id, repo_id, agent_id, enabled, config, instructions, trigger_events, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		ra.ID, ra.RepoID, ra.AgentID, ra.Enabled, ra.Config, ra.Instructions,
		ra.TriggerEvents, ra.CreatedAt, ra.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrAlreadyInstalled
		}
		return nil, fmt.Errorf("insert repo agent: %w", err)
	}

	return ra, nil
}

// UpdateConfig updates a repo agent's configuration.
func (s *Service) UpdateConfig(ctx context.Context, repoID uuid.UUID, agentSlug string, req models.UpdateRepoAgentRequest) (*models.RepoAgent, error) {
	ra, err := s.GetRepoAgent(ctx, repoID, agentSlug)
	if err != nil {
		return nil, err
	}

	if req.Enabled != nil {
		ra.Enabled = *req.Enabled
	}
	if req.Config != nil {
		ra.Config = *req.Config
	}
	if req.Instructions != nil {
		ra.Instructions = *req.Instructions
	}
	if req.TriggerEvents != nil {
		ra.TriggerEvents = *req.TriggerEvents
	}
	ra.UpdatedAt = time.Now()

	_, err = s.db.Exec(ctx, `
		UPDATE repo_agents
		SET enabled = $1, config = $2, instructions = $3, trigger_events = $4, updated_at = $5
		WHERE id = $6`,
		ra.Enabled, ra.Config, ra.Instructions, ra.TriggerEvents, ra.UpdatedAt, ra.ID)
	if err != nil {
		return nil, fmt.Errorf("update repo agent: %w", err)
	}

	return ra, nil
}

// Uninstall removes an agent from a repository.
func (s *Service) Uninstall(ctx context.Context, repoID uuid.UUID, agentSlug string) error {
	agent, err := s.GetBySlug(ctx, agentSlug)
	if err != nil {
		return err
	}

	tag, err := s.db.Exec(ctx, `
		DELETE FROM repo_agents WHERE repo_id = $1 AND agent_id = $2`,
		repoID, agent.ID)
	if err != nil {
		return fmt.Errorf("delete repo agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotInstalled
	}
	return nil
}

// GetRepoAgent returns a specific agent installation for a repo.
func (s *Service) GetRepoAgent(ctx context.Context, repoID uuid.UUID, agentSlug string) (*models.RepoAgent, error) {
	row := s.db.QueryRow(ctx, `
		SELECT ra.id, ra.repo_id, ra.agent_id, a.name, a.slug,
			ra.enabled, ra.config, ra.instructions, ra.trigger_events,
			ra.created_at, ra.updated_at
		FROM repo_agents ra
		JOIN agents a ON a.id = ra.agent_id
		WHERE ra.repo_id = $1 AND a.slug = $2`, repoID, agentSlug)

	ra, err := scanRepoAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotInstalled
	}
	if err != nil {
		return nil, fmt.Errorf("query repo agent: %w", err)
	}
	return ra, nil
}

// ListTasks returns agent tasks for a repository.
func (s *Service) ListTasks(ctx context.Context, repoID uuid.UUID, limit, offset int) ([]models.AgentTask, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, agent_id, trigger_event, trigger_ref, status,
			provider, input_tokens, output_tokens, duration_ms, result, error,
			created_at, completed_at
		FROM agent_tasks
		WHERE repo_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, repoID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

// GetTask returns a single agent task by ID.
func (s *Service) GetTask(ctx context.Context, taskID uuid.UUID) (*models.AgentTask, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, repo_id, agent_id, trigger_event, trigger_ref, status,
			provider, input_tokens, output_tokens, duration_ms, result, error,
			created_at, completed_at
		FROM agent_tasks
		WHERE id = $1`, taskID)

	task, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query task: %w", err)
	}
	return task, nil
}

// CreateTask creates a new agent task record.
func (s *Service) CreateTask(ctx context.Context, task *models.AgentTask) (*models.AgentTask, error) {
	if task.ID == uuid.Nil {
		task.ID = uuid.New()
	}
	if task.Status == "" {
		task.Status = "queued"
	}
	if task.Result == nil {
		task.Result = json.RawMessage(`{}`)
	}
	task.CreatedAt = time.Now()

	_, err := s.db.Exec(ctx, `
		INSERT INTO agent_tasks (id, repo_id, agent_id, trigger_event, trigger_ref, status,
			provider, input_tokens, output_tokens, duration_ms, result, error, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		task.ID, task.RepoID, task.AgentID, task.TriggerEvent, task.TriggerRef,
		task.Status, task.Provider, task.InputTokens, task.OutputTokens,
		task.DurationMs, task.Result, task.Error, task.CreatedAt, task.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	return task, nil
}

// UpdateTaskStatus updates the status and result of an agent task.
func (s *Service) UpdateTaskStatus(ctx context.Context, taskID uuid.UUID, status string, result json.RawMessage) error {
	if result == nil {
		result = json.RawMessage(`{}`)
	}

	var completedAt *time.Time
	if status == "completed" || status == "failed" {
		now := time.Now()
		completedAt = &now
	}

	_, err := s.db.Exec(ctx, `
		UPDATE agent_tasks
		SET status = $1, result = $2, completed_at = $3
		WHERE id = $4`,
		status, result, completedAt, taskID)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

// scanAgent scans a single agent from a row.
func scanAgent(row pgx.Row) (*models.Agent, error) {
	var a models.Agent
	err := row.Scan(&a.ID, &a.Name, &a.Slug, &a.Description, &a.IsOfficial,
		&a.AuthorID, &a.Config, &a.CreatedAt, &a.UpdatedAt)
	return &a, err
}

// scanAgents scans multiple agents from rows.
func scanAgents(rows pgx.Rows) ([]models.Agent, error) {
	var agents []models.Agent
	for rows.Next() {
		var a models.Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Slug, &a.Description, &a.IsOfficial,
			&a.AuthorID, &a.Config, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []models.Agent{}
	}
	return agents, rows.Err()
}

// scanRepoAgent scans a single repo agent from a row.
func scanRepoAgent(row pgx.Row) (*models.RepoAgent, error) {
	var ra models.RepoAgent
	err := row.Scan(&ra.ID, &ra.RepoID, &ra.AgentID, &ra.AgentName, &ra.AgentSlug,
		&ra.Enabled, &ra.Config, &ra.Instructions, &ra.TriggerEvents,
		&ra.CreatedAt, &ra.UpdatedAt)
	return &ra, err
}

// scanRepoAgents scans multiple repo agents from rows.
func scanRepoAgents(rows pgx.Rows) ([]models.RepoAgent, error) {
	var agents []models.RepoAgent
	for rows.Next() {
		var ra models.RepoAgent
		if err := rows.Scan(&ra.ID, &ra.RepoID, &ra.AgentID, &ra.AgentName, &ra.AgentSlug,
			&ra.Enabled, &ra.Config, &ra.Instructions, &ra.TriggerEvents,
			&ra.CreatedAt, &ra.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan repo agent: %w", err)
		}
		agents = append(agents, ra)
	}
	if agents == nil {
		agents = []models.RepoAgent{}
	}
	return agents, rows.Err()
}

// scanTask scans a single task from a row.
func scanTask(row pgx.Row) (*models.AgentTask, error) {
	var t models.AgentTask
	err := row.Scan(&t.ID, &t.RepoID, &t.AgentID, &t.TriggerEvent, &t.TriggerRef,
		&t.Status, &t.Provider, &t.InputTokens, &t.OutputTokens, &t.DurationMs,
		&t.Result, &t.Error, &t.CreatedAt, &t.CompletedAt)
	return &t, err
}

// scanTasks scans multiple tasks from rows.
func scanTasks(rows pgx.Rows) ([]models.AgentTask, error) {
	var tasks []models.AgentTask
	for rows.Next() {
		var t models.AgentTask
		if err := rows.Scan(&t.ID, &t.RepoID, &t.AgentID, &t.TriggerEvent, &t.TriggerRef,
			&t.Status, &t.Provider, &t.InputTokens, &t.OutputTokens, &t.DurationMs,
			&t.Result, &t.Error, &t.CreatedAt, &t.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if tasks == nil {
		tasks = []models.AgentTask{}
	}
	return tasks, rows.Err()
}
