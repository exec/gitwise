package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/websocket"
)

var (
	ErrNotFound           = errors.New("notification not found")
	ErrNotificationSkipped = errors.New("notification skipped: type disabled by user preference")
)

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

// IsTypeEnabled checks whether the given notification type is enabled for a user.
// If the user has no preferences row, all types default to enabled.
func (s *Service) IsTypeEnabled(ctx context.Context, userID uuid.UUID, notifType string) (bool, error) {
	prefs, err := s.GetPreferences(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("check notification preference: %w", err)
	}

	switch notifType {
	case "pr_review":
		return prefs.PRReview, nil
	case "pr_merged":
		return prefs.PRMerged, nil
	case "pr_comment":
		return prefs.PRComment, nil
	case "issue_comment":
		return prefs.IssueComment, nil
	case "mention":
		return prefs.Mention, nil
	default:
		// Unknown types are always enabled
		return true, nil
	}
}

// GetPreferences returns notification preferences for a user.
// If no row exists, returns defaults (all enabled).
func (s *Service) GetPreferences(ctx context.Context, userID uuid.UUID) (*models.NotificationPreferences, error) {
	prefs := &models.NotificationPreferences{UserID: userID}
	err := s.db.QueryRow(ctx, `
		SELECT pr_review, pr_merged, pr_comment, issue_comment, mention, updated_at
		FROM notification_preferences
		WHERE user_id = $1`,
		userID,
	).Scan(&prefs.PRReview, &prefs.PRMerged, &prefs.PRComment, &prefs.IssueComment, &prefs.Mention, &prefs.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		// Return defaults — all enabled
		prefs.PRReview = true
		prefs.PRMerged = true
		prefs.PRComment = true
		prefs.IssueComment = true
		prefs.Mention = true
		return prefs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query notification preferences: %w", err)
	}
	return prefs, nil
}

// UpdatePreferences upserts notification preferences for a user.
func (s *Service) UpdatePreferences(ctx context.Context, userID uuid.UUID, req *models.UpdateNotificationPreferencesRequest) (*models.NotificationPreferences, error) {
	// Single atomic upsert using COALESCE to merge partial updates with
	// existing values (or defaults). No read-then-write race condition.
	var prefs models.NotificationPreferences
	prefs.UserID = userID

	err := s.db.QueryRow(ctx, `
		INSERT INTO notification_preferences (user_id, pr_review, pr_merged, pr_comment, issue_comment, mention, updated_at)
		VALUES ($1, COALESCE($2, TRUE), COALESCE($3, TRUE), COALESCE($4, TRUE), COALESCE($5, TRUE), COALESCE($6, TRUE), now())
		ON CONFLICT (user_id) DO UPDATE SET
			pr_review = COALESCE($2, notification_preferences.pr_review),
			pr_merged = COALESCE($3, notification_preferences.pr_merged),
			pr_comment = COALESCE($4, notification_preferences.pr_comment),
			issue_comment = COALESCE($5, notification_preferences.issue_comment),
			mention = COALESCE($6, notification_preferences.mention),
			updated_at = now()
		RETURNING pr_review, pr_merged, pr_comment, issue_comment, mention, updated_at`,
		userID, req.PRReview, req.PRMerged, req.PRComment, req.IssueComment, req.Mention,
	).Scan(&prefs.PRReview, &prefs.PRMerged, &prefs.PRComment, &prefs.IssueComment, &prefs.Mention, &prefs.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert notification preferences: %w", err)
	}

	return &prefs, nil
}

// WatchRepo adds a user as a watcher of a repository.
func (s *Service) WatchRepo(ctx context.Context, userID, repoID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO repo_watchers (user_id, repo_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, repo_id) DO NOTHING`,
		userID, repoID,
	)
	if err != nil {
		return fmt.Errorf("watch repo: %w", err)
	}
	return nil
}

// UnwatchRepo removes a user as a watcher of a repository.
func (s *Service) UnwatchRepo(ctx context.Context, userID, repoID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM repo_watchers
		WHERE user_id = $1 AND repo_id = $2`,
		userID, repoID,
	)
	if err != nil {
		return fmt.Errorf("unwatch repo: %w", err)
	}
	return nil
}

// IsWatching checks if a user is watching a repository.
func (s *Service) IsWatching(ctx context.Context, userID, repoID uuid.UUID) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM repo_watchers WHERE user_id = $1 AND repo_id = $2)`,
		userID, repoID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check watching: %w", err)
	}
	return exists, nil
}

// ListRepoWatchers returns all user IDs watching a given repository.
func (s *Service) ListRepoWatchers(ctx context.Context, repoID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.db.Query(ctx, `
		SELECT user_id FROM repo_watchers WHERE repo_id = $1`,
		repoID,
	)
	if err != nil {
		return nil, fmt.Errorf("query repo watchers: %w", err)
	}
	defer rows.Close()

	var userIDs []uuid.UUID
	for rows.Next() {
		var uid uuid.UUID
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("scan watcher: %w", err)
		}
		userIDs = append(userIDs, uid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate watchers: %w", err)
	}
	return userIDs, nil
}

// WatcherCount returns the number of watchers for a repository.
func (s *Service) WatcherCount(ctx context.Context, repoID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM repo_watchers WHERE repo_id = $1`,
		repoID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count watchers: %w", err)
	}
	return count, nil
}

// Create inserts a notification for userID if the given type is enabled in
// their preferences (defaulting to enabled when no preference row exists).
//
// The preference check and the INSERT are collapsed into a single SQL
// statement to eliminate the TOCTOU race that existed when they were two
// separate queries: a user could disable a type between the SELECT and the
// INSERT and still receive the notification.
//
// Returns (nil, ErrNotificationSkipped) when the type is disabled so callers
// can branch cleanly without treating the skip as a hard error.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, notifType, title, body, link string) (*models.Notification, error) {
	// Map the generic notifType string to the matching boolean column in
	// notification_preferences.  Unknown types default to enabled (no pref
	// row → allowed; known type with pref row → respect the column).
	prefColumn := notifTypeToColumn(notifType)

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

	// Single atomic statement: insert only when the pref is enabled (or absent).
	// The CTE returns a boolean so we can distinguish "skipped" from "inserted"
	// without a second round-trip.  If the INSERT is skipped, created_at is NULL
	// and we return ErrNotificationSkipped.
	query := fmt.Sprintf(`
		WITH pref AS (
			SELECT %s AS enabled
			FROM notification_preferences
			WHERE user_id = $2
		),
		allowed AS (
			SELECT COALESCE((SELECT enabled FROM pref), TRUE) AS ok
		),
		ins AS (
			INSERT INTO notifications (id, user_id, type, title, body, link, is_read, metadata)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8
			WHERE (SELECT ok FROM allowed)
			RETURNING created_at
		)
		SELECT (SELECT ok FROM allowed), (SELECT created_at FROM ins)
	`, prefColumn)

	var enabled bool
	var createdAt *time.Time
	err := s.db.QueryRow(ctx, query,
		n.ID, n.UserID, n.Type, n.Title, n.Body, n.Link, n.Read, n.Metadata,
	).Scan(&enabled, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert notification: %w", err)
	}

	if !enabled {
		return nil, ErrNotificationSkipped
	}

	if createdAt != nil {
		n.CreatedAt = *createdAt
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

// notifTypeToColumn maps a notification type string to the corresponding
// boolean column in notification_preferences.  Unknown types are treated as
// always enabled by returning the SQL literal TRUE.
func notifTypeToColumn(notifType string) string {
	switch notifType {
	case "pr_review":
		return "pr_review"
	case "pr_merged":
		return "pr_merged"
	case "pr_comment":
		return "pr_comment"
	case "issue_comment":
		return "issue_comment"
	case "mention":
		return "mention"
	default:
		// Unknown types are always enabled — no column to look up
		return "TRUE"
	}
}

func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit int) ([]models.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	query := `
		SELECT id, user_id, type, title, body, link, is_read, metadata, created_at
		FROM notifications
		WHERE user_id = $1`

	args := []any{userID}
	argIdx := 2

	if unreadOnly {
		query += fmt.Sprintf(` AND is_read = $%d`, argIdx)
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
		UPDATE notifications SET is_read = true
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
		UPDATE notifications SET is_read = true
		WHERE user_id = $1 AND is_read = false`,
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
		SELECT id, user_id, type, title, body, link, is_read, metadata, created_at
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
