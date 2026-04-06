package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gitwise-io/gitwise/internal/pagination"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

type Event struct {
	ID        uuid.UUID       `json:"id"`
	RepoID    *uuid.UUID      `json:"repo_id,omitempty"`
	ActorID   uuid.UUID       `json:"actor_id"`
	ActorName string          `json:"actor_name"`
	EventType string          `json:"event_type"`
	RefType   string          `json:"ref_type"`
	RefID     *uuid.UUID      `json:"ref_id,omitempty"`
	RefNumber *int            `json:"ref_number,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	RepoName  string          `json:"repo_name,omitempty"`
	RepoOwner string          `json:"repo_owner,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s *Service) Record(ctx context.Context, repoID *uuid.UUID, actorID uuid.UUID, eventType, refType string, refID *uuid.UUID, refNumber *int, payload map[string]any) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO activity_events (repo_id, actor_id, event_type, ref_type, ref_id, ref_number, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		repoID, actorID, eventType, refType, refID, refNumber, payloadJSON,
	)
	if err != nil {
		return fmt.Errorf("insert activity event: %w", err)
	}
	return nil
}

func (s *Service) ListByRepo(ctx context.Context, repoID uuid.UUID, cursor string, limit int) ([]Event, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `
		SELECT e.id, e.repo_id, e.actor_id, u.username,
		       e.event_type, e.ref_type, e.ref_id, e.ref_number,
		       e.payload, r.name, COALESCE(ru.username, ro.name), e.created_at
		FROM activity_events e
		JOIN users u ON u.id = e.actor_id
		LEFT JOIN repositories r ON r.id = e.repo_id
		LEFT JOIN users ru ON ru.id = r.owner_id AND r.owner_type = 'user'
		LEFT JOIN organizations ro ON ro.id = r.owner_id AND r.owner_type = 'org'
		WHERE e.repo_id = $1`

	args := []any{repoID}
	argIdx := 2

	if cursor != "" {
		cursorTime, cursorID, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		query += fmt.Sprintf(` AND (e.created_at, e.id) < ($%d, $%d)`, argIdx, argIdx+1)
		args = append(args, cursorTime, cursorID)
		argIdx += 2
	}

	query += ` ORDER BY e.created_at DESC, e.id DESC`
	query += fmt.Sprintf(` LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	return s.queryEvents(ctx, query, args, limit)
}

func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]Event, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `
		SELECT e.id, e.repo_id, e.actor_id, u.username,
		       e.event_type, e.ref_type, e.ref_id, e.ref_number,
		       e.payload, r.name, COALESCE(ru.username, ro.name), e.created_at
		FROM activity_events e
		JOIN users u ON u.id = e.actor_id
		LEFT JOIN repositories r ON r.id = e.repo_id
		LEFT JOIN users ru ON ru.id = r.owner_id AND r.owner_type = 'user'
		LEFT JOIN organizations ro ON ro.id = r.owner_id AND r.owner_type = 'org'
		WHERE e.actor_id = $1`

	args := []any{userID}
	argIdx := 2

	if cursor != "" {
		cursorTime, cursorID, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		query += fmt.Sprintf(` AND (e.created_at, e.id) < ($%d, $%d)`, argIdx, argIdx+1)
		args = append(args, cursorTime, cursorID)
		argIdx += 2
	}

	query += ` ORDER BY e.created_at DESC, e.id DESC`
	query += fmt.Sprintf(` LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	return s.queryEvents(ctx, query, args, limit)
}

func (s *Service) queryEvents(ctx context.Context, query string, args []any, limit int) ([]Event, string, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query activity events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var ev Event
		var repoName, repoOwner *string
		if err := rows.Scan(
			&ev.ID, &ev.RepoID, &ev.ActorID, &ev.ActorName,
			&ev.EventType, &ev.RefType, &ev.RefID, &ev.RefNumber,
			&ev.Payload, &repoName, &repoOwner, &ev.CreatedAt,
		); err != nil {
			return nil, "", fmt.Errorf("scan activity event: %w", err)
		}
		if repoName != nil {
			ev.RepoName = *repoName
		}
		if repoOwner != nil {
			ev.RepoOwner = *repoOwner
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate activity events: %w", err)
	}

	var nextCursor string
	if len(events) > limit {
		events = events[:limit]
		last := events[limit-1]
		nextCursor = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}

	return events, nextCursor, nil
}
