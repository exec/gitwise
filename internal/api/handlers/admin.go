package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/commit"
	"github.com/gitwise-io/gitwise/internal/services/user"
)

// AdminHandler handles admin panel API endpoints.
type AdminHandler struct {
	db            *pgxpool.Pool
	users         *user.Service
	commitIndexer *commit.Indexer
	reindexing    atomic.Bool
}

func NewAdminHandler(db *pgxpool.Pool, users *user.Service, commitIndexer *commit.Indexer) *AdminHandler {
	return &AdminHandler{
		db:            db,
		users:         users,
		commitIndexer: commitIndexer,
	}
}

// ListUsers returns a paginated, searchable list of all users.
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	offset := 0
	if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
		offset = o
	}

	type adminUser struct {
		ID        uuid.UUID `json:"id"`
		Username  string    `json:"username"`
		Email     string    `json:"email"`
		FullName  string    `json:"full_name"`
		IsAdmin   bool      `json:"is_admin"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		RepoCount int       `json:"repo_count"`
	}

	var rows []adminUser
	var total int

	if search != "" {
		pattern := "%" + search + "%"
		err := h.db.QueryRow(ctx, `
			SELECT COUNT(*) FROM users
			WHERE username ILIKE $1 OR email ILIKE $1 OR full_name ILIKE $1`, pattern,
		).Scan(&total)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "server_error", "failed to count users")
			return
		}

		dbRows, err := h.db.Query(ctx, `
			SELECT u.id, u.username, u.email, u.full_name, u.is_admin, u.created_at, u.updated_at,
			       COALESCE((SELECT COUNT(*) FROM repositories WHERE owner_id = u.id), 0) AS repo_count
			FROM users u
			WHERE u.username ILIKE $1 OR u.email ILIKE $1 OR u.full_name ILIKE $1
			ORDER BY u.created_at DESC
			LIMIT $2 OFFSET $3`, pattern, limit, offset,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "server_error", "failed to list users")
			return
		}
		defer dbRows.Close()

		for dbRows.Next() {
			var u adminUser
			if err := dbRows.Scan(&u.ID, &u.Username, &u.Email, &u.FullName, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt, &u.RepoCount); err != nil {
				writeError(w, http.StatusInternalServerError, "server_error", "failed to scan user")
				return
			}
			rows = append(rows, u)
		}
		if err := dbRows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, "server_error", "failed to iterate users")
			return
		}
	} else {
		err := h.db.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&total)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "server_error", "failed to count users")
			return
		}

		dbRows, err := h.db.Query(ctx, `
			SELECT u.id, u.username, u.email, u.full_name, u.is_admin, u.created_at, u.updated_at,
			       COALESCE((SELECT COUNT(*) FROM repositories WHERE owner_id = u.id), 0) AS repo_count
			FROM users u
			ORDER BY u.created_at DESC
			LIMIT $1 OFFSET $2`, limit, offset,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "server_error", "failed to list users")
			return
		}
		defer dbRows.Close()

		for dbRows.Next() {
			var u adminUser
			if err := dbRows.Scan(&u.ID, &u.Username, &u.Email, &u.FullName, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt, &u.RepoCount); err != nil {
				writeError(w, http.StatusInternalServerError, "server_error", "failed to scan user")
				return
			}
			rows = append(rows, u)
		}
		if err := dbRows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, "server_error", "failed to iterate users")
			return
		}
	}

	if rows == nil {
		rows = []adminUser{}
	}

	writeJSONMeta(w, http.StatusOK, rows, &models.ResponseMeta{Total: total})
}

// GetUser returns detailed info about a specific user.
func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid user ID")
		return
	}

	u, err := h.users.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}

	// Augment with repo count
	var repoCount int
	h.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM repositories WHERE owner_id = $1`, id).Scan(&repoCount)

	type adminUserDetail struct {
		ID        uuid.UUID `json:"id"`
		Username  string    `json:"username"`
		Email     string    `json:"email"`
		FullName  string    `json:"full_name"`
		Bio       string    `json:"bio"`
		AvatarURL string    `json:"avatar_url"`
		IsAdmin   bool      `json:"is_admin"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		RepoCount int       `json:"repo_count"`
	}

	writeJSON(w, http.StatusOK, adminUserDetail{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		FullName:  u.FullName,
		Bio:       u.Bio,
		AvatarURL: u.AvatarURL,
		IsAdmin:   u.IsAdmin,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		RepoCount: repoCount,
	})
}

// UpdateUser allows admins to toggle admin status or disable accounts.
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid user ID")
		return
	}

	currentUserID := middleware.GetUserID(r.Context())
	if currentUserID != nil && *currentUserID == id {
		writeError(w, http.StatusBadRequest, "self_modification", "cannot modify your own admin account")
		return
	}

	var req struct {
		IsAdmin *bool `json:"is_admin"`
	}
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if req.IsAdmin == nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "no fields to update")
		return
	}

	setClauses := []string{"updated_at = now()"}
	args := []any{id}
	argIdx := 2

	if req.IsAdmin != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_admin = $%d", argIdx))
		args = append(args, *req.IsAdmin)
		argIdx++
	}

	query := fmt.Sprintf(`UPDATE users SET %s WHERE id = $1
		RETURNING id, username, email, full_name, avatar_url, bio, is_admin, created_at, updated_at`,
		strings.Join(setClauses, ", "))

	var u struct {
		ID        uuid.UUID `json:"id"`
		Username  string    `json:"username"`
		Email     string    `json:"email"`
		FullName  string    `json:"full_name"`
		AvatarURL string    `json:"avatar_url"`
		Bio       string    `json:"bio"`
		IsAdmin   bool      `json:"is_admin"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	err = h.db.QueryRow(r.Context(), query, args...).Scan(
		&u.ID, &u.Username, &u.Email, &u.FullName, &u.AvatarURL, &u.Bio, &u.IsAdmin,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}

	writeJSON(w, http.StatusOK, u)
}

// DeleteUser removes a user and all their data (repos, issues, PRs, etc.).
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid user ID")
		return
	}

	currentUserID := middleware.GetUserID(r.Context())
	if currentUserID != nil && *currentUserID == id {
		writeError(w, http.StatusBadRequest, "self_deletion", "cannot delete your own account from admin panel")
		return
	}

	// Verify user exists
	_, err = h.users.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}

	// Delete user (cascades to repos, issues, PRs, etc. via FK constraints)
	tag, err := h.db.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		slog.Error("admin: failed to delete user", "user_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete user")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// GetStats returns system-wide statistics.
func (h *AdminHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type stats struct {
		UserCount    int    `json:"user_count"`
		RepoCount    int    `json:"repo_count"`
		CommitCount  int    `json:"commit_count"`
		IssueCount   int    `json:"issue_count"`
		PRCount      int    `json:"pr_count"`
		DiskUsage    string `json:"disk_usage"`
	}

	var s stats

	err := h.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM repositories),
			(SELECT COUNT(*) FROM commit_metadata),
			(SELECT COUNT(*) FROM issues),
			(SELECT COUNT(*) FROM pull_requests),
			pg_size_pretty(pg_database_size(current_database()))
	`).Scan(&s.UserCount, &s.RepoCount, &s.CommitCount, &s.IssueCount, &s.PRCount, &s.DiskUsage)
	if err != nil {
		slog.Error("admin: failed to get stats", "error", err)
		s.DiskUsage = "unknown"
	}

	writeJSON(w, http.StatusOK, s)
}

// ListJobs returns recent webhook deliveries across all repos.
func (h *AdminHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	type deliveryRow struct {
		ID             uuid.UUID       `json:"id"`
		WebhookID      uuid.UUID       `json:"webhook_id"`
		EventType      string          `json:"event_type"`
		ResponseStatus *int            `json:"response_status"`
		Success        bool            `json:"success"`
		Attempts       int             `json:"attempts"`
		Duration       int             `json:"duration_ms"`
		DeliveredAt    time.Time       `json:"delivered_at"`
		WebhookURL     string          `json:"webhook_url"`
		RepoName       string          `json:"repo_name"`
		OwnerName      string          `json:"owner_name"`
	}

	rows, err := h.db.Query(ctx, `
		SELECT d.id, d.webhook_id, d.event_type, d.response_status, d.success,
		       d.attempts, d.duration_ms, d.delivered_at,
		       w.url AS webhook_url,
		       COALESCE(r.name, '') AS repo_name,
		       COALESCE(u.username, o.name, '') AS owner_name
		FROM webhook_deliveries d
		JOIN webhooks w ON w.id = d.webhook_id
		LEFT JOIN repositories r ON r.id = w.repo_id
		LEFT JOIN users u ON u.id = r.owner_id AND r.owner_type = 'user'
		LEFT JOIN organizations o ON o.id = r.owner_id AND r.owner_type = 'org'
		ORDER BY d.delivered_at DESC
		LIMIT $1`, limit,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list deliveries")
		return
	}
	defer rows.Close()

	var deliveries []deliveryRow
	for rows.Next() {
		var d deliveryRow
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.EventType, &d.ResponseStatus,
			&d.Success, &d.Attempts, &d.Duration, &d.DeliveredAt,
			&d.WebhookURL, &d.RepoName, &d.OwnerName); err != nil {
			writeError(w, http.StatusInternalServerError, "server_error", "failed to scan delivery")
			return
		}
		deliveries = append(deliveries, d)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to iterate deliveries")
		return
	}

	if deliveries == nil {
		deliveries = []deliveryRow{}
	}

	writeJSON(w, http.StatusOK, deliveries)
}

// ReindexCommits triggers commit re-indexing for all repos.
func (h *AdminHandler) ReindexCommits(w http.ResponseWriter, r *http.Request) {
	if !h.reindexing.CompareAndSwap(false, true) {
		writeError(w, http.StatusConflict, "already_running", "reindex already in progress")
		return
	}
	go func() {
		defer h.reindexing.Store(false)
		if err := h.commitIndexer.IndexAll(context.Background()); err != nil {
			slog.Error("admin: commit reindex failed", "error", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "indexing started"})
}

