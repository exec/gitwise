package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrNotFound   = errors.New("webhook not found")
	ErrInvalidURL = errors.New("invalid webhook URL")
)

// retryDelays defines exponential backoff for delivery retries.
var retryDelays = []time.Duration{1 * time.Minute, 5 * time.Minute, 30 * time.Minute}

const maxAttempts = 3

type Service struct {
	db     *pgxpool.Pool
	client *http.Client
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{
		db: db,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *Service) Create(ctx context.Context, repoID uuid.UUID, req models.CreateWebhookRequest) (*models.Webhook, error) {
	rawURL := strings.TrimSpace(req.URL)
	if err := validateURL(rawURL); err != nil {
		return nil, ErrInvalidURL
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	now := time.Now()
	w := &models.Webhook{
		ID:     uuid.New(),
		RepoID: repoID,
		URL:    rawURL,
		Secret: req.Secret,
		Events: req.Events,
		Active: active,
		Timestamps: models.Timestamps{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	if w.Events == nil {
		w.Events = []string{}
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO webhooks (id, repo_id, url, secret, events, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		w.ID, w.RepoID, w.URL, w.Secret, w.Events, w.Active, w.CreatedAt, w.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert webhook: %w", err)
	}

	return w, nil
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID) ([]models.Webhook, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, url, secret, events, active, created_at, updated_at
		FROM webhooks
		WHERE repo_id = $1
		ORDER BY created_at DESC`, repoID,
	)
	if err != nil {
		return nil, fmt.Errorf("query webhooks: %w", err)
	}
	defer rows.Close()

	var webhooks []models.Webhook
	for rows.Next() {
		var w models.Webhook
		if err := rows.Scan(&w.ID, &w.RepoID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook: %w", err)
		}
		webhooks = append(webhooks, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate webhooks: %w", err)
	}
	return webhooks, nil
}

func (s *Service) Get(ctx context.Context, repoID, webhookID uuid.UUID) (*models.Webhook, error) {
	w := &models.Webhook{}
	err := s.db.QueryRow(ctx, `
		SELECT id, repo_id, url, secret, events, active, created_at, updated_at
		FROM webhooks
		WHERE id = $1 AND repo_id = $2`, webhookID, repoID,
	).Scan(&w.ID, &w.RepoID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query webhook: %w", err)
	}
	return w, nil
}

func (s *Service) Update(ctx context.Context, repoID, webhookID uuid.UUID, req models.UpdateWebhookRequest) (*models.Webhook, error) {
	setClauses := []string{"updated_at = now()"}
	args := []any{webhookID, repoID}
	argIdx := 3

	if req.URL != nil {
		rawURL := strings.TrimSpace(*req.URL)
		if err := validateURL(rawURL); err != nil {
			return nil, ErrInvalidURL
		}
		setClauses = append(setClauses, fmt.Sprintf("url = $%d", argIdx))
		args = append(args, rawURL)
		argIdx++
	}
	if req.Secret != nil {
		setClauses = append(setClauses, fmt.Sprintf("secret = $%d", argIdx))
		args = append(args, *req.Secret)
		argIdx++
	}
	if req.Events != nil {
		setClauses = append(setClauses, fmt.Sprintf("events = $%d", argIdx))
		args = append(args, *req.Events)
		argIdx++
	}
	if req.Active != nil {
		setClauses = append(setClauses, fmt.Sprintf("active = $%d", argIdx))
		args = append(args, *req.Active)
		argIdx++
	}

	query := fmt.Sprintf(`UPDATE webhooks SET %s WHERE id = $1 AND repo_id = $2
		RETURNING id, repo_id, url, secret, events, active, created_at, updated_at`,
		strings.Join(setClauses, ", "))

	w := &models.Webhook{}
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&w.ID, &w.RepoID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt, &w.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update webhook: %w", err)
	}
	return w, nil
}

func (s *Service) Delete(ctx context.Context, repoID, webhookID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM webhooks WHERE id = $1 AND repo_id = $2`, webhookID, repoID)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListDeliveries(ctx context.Context, webhookID uuid.UUID, limit int) ([]models.WebhookDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, webhook_id, event_type, payload, response_status, success, attempts, duration_ms, delivered_at
		FROM webhook_deliveries
		WHERE webhook_id = $1
		ORDER BY delivered_at DESC
		LIMIT $2`, webhookID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []models.WebhookDelivery
	for rows.Next() {
		var d models.WebhookDelivery
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.EventType, &d.Payload, &d.ResponseStatus, &d.Success, &d.Attempts, &d.Duration, &d.DeliveredAt); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}
		deliveries = append(deliveries, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deliveries: %w", err)
	}
	return deliveries, nil
}

// Dispatch finds all active webhooks for a repo that subscribe to the given event
// and delivers the payload to each.
func (s *Service) Dispatch(ctx context.Context, repoID uuid.UUID, eventType string, payload map[string]any) {
	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, url, secret, events, active, created_at, updated_at
		FROM webhooks
		WHERE repo_id = $1 AND active = TRUE AND $2 = ANY(events)`, repoID, eventType,
	)
	if err != nil {
		slog.Error("webhook dispatch query failed", "repo_id", repoID, "event", eventType, "error", err)
		return
	}
	defer rows.Close()

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		slog.Error("webhook payload marshal failed", "error", err)
		return
	}

	for rows.Next() {
		var w models.Webhook
		if err := rows.Scan(&w.ID, &w.RepoID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt, &w.UpdatedAt); err != nil {
			slog.Error("webhook scan failed", "error", err)
			continue
		}
		go s.deliver(w, eventType, payloadJSON)
	}
}

func (s *Service) deliver(w models.Webhook, eventType string, payloadJSON []byte) {
	deliveryID := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Build request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(payloadJSON))
	if err != nil {
		slog.Error("webhook request build failed", "webhook_id", w.ID, "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitwise-Event", eventType)
	req.Header.Set("X-Gitwise-Delivery", deliveryID.String())
	req.Header.Set("User-Agent", "Gitwise-Hookshot")

	if w.Secret != "" {
		mac := hmac.New(sha256.New, []byte(w.Secret))
		mac.Write(payloadJSON)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Hub-Signature-256", "sha256="+sig)
	}

	reqHeaders, _ := json.Marshal(flattenHeaders(req.Header))

	// Execute request
	start := time.Now()
	resp, err := s.client.Do(req)
	durationMs := int(time.Since(start).Milliseconds())

	var respStatus *int
	var respBody string
	var respHeaders json.RawMessage
	success := false

	if err != nil {
		respBody = err.Error()
	} else {
		defer resp.Body.Close()
		status := resp.StatusCode
		respStatus = &status
		success = status >= 200 && status < 300

		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		respBody = string(bodyBytes)
		rh, _ := json.Marshal(flattenHeaders(resp.Header))
		respHeaders = rh
	}

	var nextRetry *time.Time
	if !success && 0 < len(retryDelays) {
		t := time.Now().Add(retryDelays[0])
		nextRetry = &t
	}

	_, dbErr := s.db.Exec(ctx, `
		INSERT INTO webhook_deliveries
			(id, webhook_id, event_type, payload, request_headers, response_status, response_body, response_headers, duration_ms, success, attempts, next_retry)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		deliveryID, w.ID, eventType, payloadJSON, reqHeaders, respStatus, respBody, respHeaders, durationMs, success, 1, nextRetry,
	)
	if dbErr != nil {
		slog.Error("webhook delivery record failed", "delivery_id", deliveryID, "error", dbErr)
	}
}

// RetryPending finds failed deliveries that are due for retry and re-delivers them.
func (s *Service) RetryPending(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `
		SELECT d.id, d.webhook_id, d.event_type, d.payload, d.attempts,
		       w.url, w.secret
		FROM webhook_deliveries d
		JOIN webhooks w ON w.id = d.webhook_id
		WHERE d.next_retry < now()
		  AND d.success = FALSE
		  AND d.attempts < $1
		  AND w.active = TRUE`, maxAttempts,
	)
	if err != nil {
		return fmt.Errorf("query pending retries: %w", err)
	}
	defer rows.Close()

	type pending struct {
		deliveryID uuid.UUID
		webhookID  uuid.UUID
		eventType  string
		payload    []byte
		attempts   int
		url        string
		secret     string
	}

	var items []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.deliveryID, &p.webhookID, &p.eventType, &p.payload, &p.attempts, &p.url, &p.secret); err != nil {
			slog.Error("scan pending retry", "error", err)
			continue
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate pending retries: %w", err)
	}

	for _, p := range items {
		go s.retryDelivery(p.deliveryID, p.webhookID, p.eventType, p.payload, p.attempts, p.url, p.secret)
	}
	return nil
}

func (s *Service) retryDelivery(deliveryID, webhookID uuid.UUID, eventType string, payload []byte, attempts int, webhookURL, secret string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		slog.Error("retry request build failed", "delivery_id", deliveryID, "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitwise-Event", eventType)
	req.Header.Set("X-Gitwise-Delivery", deliveryID.String())
	req.Header.Set("User-Agent", "Gitwise-Hookshot")

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Hub-Signature-256", "sha256="+sig)
	}

	start := time.Now()
	resp, err := s.client.Do(req)
	durationMs := int(time.Since(start).Milliseconds())

	var respStatus *int
	var respBody string
	success := false
	newAttempts := attempts + 1

	if err != nil {
		respBody = err.Error()
	} else {
		defer resp.Body.Close()
		status := resp.StatusCode
		respStatus = &status
		success = status >= 200 && status < 300
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		respBody = string(bodyBytes)
	}

	var nextRetry *time.Time
	if !success && newAttempts < maxAttempts {
		idx := newAttempts - 1
		if idx >= len(retryDelays) {
			idx = len(retryDelays) - 1
		}
		t := time.Now().Add(retryDelays[idx])
		nextRetry = &t
	}

	_, dbErr := s.db.Exec(ctx, `
		UPDATE webhook_deliveries
		SET response_status = $1, response_body = $2, duration_ms = $3,
		    success = $4, attempts = $5, next_retry = $6
		WHERE id = $7`,
		respStatus, respBody, durationMs, success, newAttempts, nextRetry, deliveryID,
	)
	if dbErr != nil {
		slog.Error("retry delivery update failed", "delivery_id", deliveryID, "error", dbErr)
	}
}

func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	if u.Host == "" {
		return ErrInvalidURL
	}
	return nil
}

func flattenHeaders(h http.Header) map[string]string {
	flat := make(map[string]string, len(h))
	for k, v := range h {
		flat[k] = strings.Join(v, ", ")
	}
	return flat
}
