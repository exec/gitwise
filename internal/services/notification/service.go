package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/websocket"
)

var ErrNotFound = errors.New("notification not found")

type Service struct {
	db  *pgxpool.Pool
	hub *websocket.Hub
}

func NewService(db *pgxpool.Pool, hub ...*websocket.Hub) *Service {
	s := &Service{db: db}
	if len(hub) > 0 {
		s.hub = hub[0]
	}
	return s
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, notifType, title, body, link string) (*models.Notification, error) {
	n := &models.Notification{
		ID:       uuid.New(),
		UserID:   userID,
		Type:     notifType,
		Title:    title,
		Body:     body,
		Link:     link,
		Read:     false,
		Metadata: json.RawMessage(`{}`),
	}

	err := s.db.QueryRow(ctx, `
		INSERT INTO notifications (id, user_id, type, title, body, link, read, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`,
		n.ID, n.UserID, n.Type, n.Title, n.Body, n.Link, n.Read, n.Metadata,
	).Scan(&n.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert notification: %w", err)
	}

	if s.hub != nil {
		msg, _ := json.Marshal(map[string]any{
			"type": "notification",
			"data": n,
		})
		s.hub.SendToUser(n.UserID, msg)
	}

	return n, nil
}

func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit int) ([]models.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	query := `
		SELECT id, user_id, type, title, body, link, read, metadata, created_at
		FROM notifications
		WHERE user_id = $1`

	args := []any{userID}
	argIdx := 2

	if unreadOnly {
		query += fmt.Sprintf(` AND read = $%d`, argIdx)
		args = append(args, false)
		argIdx++
	}

	query += ` ORDER BY created_at DESC`
	query += fmt.Sprintf(` LIMIT $%d`, argIdx)
	args = append(args, limit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query notifications: %w", err)
	}
	defer rows.Close()

	var notifications []models.Notification
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body,
			&n.Link, &n.Read, &n.Metadata, &n.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}

	return notifications, nil
}

func (s *Service) MarkRead(ctx context.Context, notifID, userID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE notifications SET read = true
		WHERE id = $1 AND user_id = $2`,
		notifID, userID,
	)
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Check if the notification exists at all
		var exists bool
		err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notifications WHERE id = $1)`, notifID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check notification exists: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
		// Notification exists but belongs to another user — treat as not found
		return ErrNotFound
	}
	return nil
}

func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `
		UPDATE notifications SET read = true
		WHERE user_id = $1 AND read = false`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}
	return nil
}

// GetByID returns a single notification by ID, scoped to a user.
func (s *Service) GetByID(ctx context.Context, notifID, userID uuid.UUID) (*models.Notification, error) {
	n := &models.Notification{}
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, type, title, body, link, read, metadata, created_at
		FROM notifications
		WHERE id = $1 AND user_id = $2`,
		notifID, userID,
	).Scan(
		&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body,
		&n.Link, &n.Read, &n.Metadata, &n.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query notification: %w", err)
	}
	return n, nil
}
