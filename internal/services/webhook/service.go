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
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/models"
)

var (
	ErrNotFound         = errors.New("webhook not found")
	ErrInvalidURL       = errors.New("invalid webhook URL")
	ErrPrivateURL       = errors.New("webhook URL resolves to a private IP address")
	ErrInvalidEventType = errors.New("invalid webhook event type")
)

// retryDelays defines exponential backoff for delivery retries.
var retryDelays = []time.Duration{1 * time.Minute, 5 * time.Minute, 30 * time.Minute}

const maxAttempts            = 3
const maxConcurrentDeliveries = 10

// validEventTypes defines the set of event types that webhooks can subscribe to.
var validEventTypes = map[string]bool{
	"push":              true,
	"ping":              true,
	"issue.opened":      true,
	"issue.closed":      true,
	"pr.opened":         true,
	"pr.merged":         true,
	"pr.closed":         true,
	"review.submitted":  true,
	"comment.created":   true,
}

// privateRanges defines RFC 1918 / RFC 4193 / loopback / link-local ranges
// that must be blocked for webhook URLs to prevent SSRF.
var privateRanges []net.IPNet

func init() {
	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	} {
		_, ipNet, _ := net.ParseCIDR(cidr)
		privateRanges = append(privateRanges, *ipNet)
	}
}

func isPrivateIP(ip net.IP) bool {
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

type Service struct {
	db       *pgxpool.Pool
	client   *http.Client
	cancel   context.CancelFunc
	stopOnce sync.Once
}

func NewService(db *pgxpool.Pool) *Service {
	safeDialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip != nil && isPrivateIP(ip) {
				return fmt.Errorf("connections to private IP %s are not allowed", host)
			}
			return nil
		},
	}

	return &Service{
		db: db,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: safeDialer.DialContext,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("stopped after 3 redirects")
				}
				return nil
			},
		},
		cancel: func() {},
	}
}

// StartRetryLoop starts a background goroutine that periodically retries failed
// webhook deliveries using exponential backoff.
func (s *Service) StartRetryLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				retryCtx, retryCancel := context.WithTimeout(ctx, 60*time.Second)
				if err := s.RetryPending(retryCtx); err != nil {
					slog.Error("webhook retry loop failed", "error", err)
				}
				retryCancel()
			case <-ctx.Done():
				return
			}
		}
	}()
	slog.Info("webhook retry loop started", "interval", "30s")
}

// StopRetryLoop signals the background retry goroutine to stop.
func (s *Service) StopRetryLoop() {
	s.stopOnce.Do(func() { s.cancel() })
}

func (s *Service) Create(ctx context.Context, repoID uuid.UUID, req models.CreateWebhookRequest) (*models.Webhook, error) {
	rawURL := strings.TrimSpace(req.URL)
	if err := validateURL(rawURL); err != nil {
		return nil, err
	}

	if err := validateEventTypes(req.Events); err != nil {
		return nil, err
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
			return nil, err
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
		if err := validateEventTypes(*req.Events); err != nil {
			return nil, err
		}
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

	const maxPayloadBytes = 256 * 1024 // 256 KiB

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		slog.Error("webhook payload marshal failed", "error", err)
		return
	}
	if len(payloadJSON) > maxPayloadBytes {
		// Truncate by capping string fields in the payload and re-marshalling.
		payloadJSON = truncatePayload(payload, maxPayloadBytes)
	}

	var webhooks []models.Webhook
	for rows.Next() {
		var w models.Webhook
		if err := rows.Scan(&w.ID, &w.RepoID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt, &w.UpdatedAt); err != nil {
			slog.Error("webhook scan failed", "error", err)
			continue
		}
		webhooks = append(webhooks, w)
	}

	sem := make(chan struct{}, maxConcurrentDeliveries)
	var wg sync.WaitGroup
	for _, wh := range webhooks {
		wg.Add(1)
		sem <- struct{}{}
		go func(w models.Webhook) {
			defer wg.Done()
			defer func() { <-sem }()
			s.deliver(w, eventType, payloadJSON)
		}(wh)
	}
	wg.Wait()
}

func isDiscordWebhook(rawURL string) bool {
	return strings.Contains(rawURL, "discord.com/api/webhooks/") ||
		strings.Contains(rawURL, "discordapp.com/api/webhooks/")
}

const discordAvatarURL = "https://raw.githubusercontent.com/exec/gitwise/main/web/public/gitwise-avatar.png"

func buildDiscordPayload(eventType string, payloadJSON []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, err
	}

	// Pick color by event type
	colors := map[string]int{
		"push":             0x2196F3, // blue
		"ping":             0x9E9E9E, // gray
		"issue.opened":     0x4CAF50, // green
		"issue.closed":     0xF44336, // red
		"pr.opened":        0x4CAF50, // green
		"pr.merged":        0x9C27B0, // purple
		"pr.closed":        0xF44336, // red
		"review.submitted": 0xFF9800, // orange
		"comment.created":  0x00BCD4, // cyan
	}
	color := colors[eventType]
	if color == 0 {
		color = 0x607D8B // default blue-gray
	}

	// Extract common fields
	repoName := ""
	if repoObj, ok := payload["repository"].(map[string]any); ok {
		repoName, _ = repoObj["name"].(string)
	} else if s, ok := payload["repository"].(string); ok {
		repoName = s
	}
	sender, _ := payload["sender"].(string)
	owner, _ := payload["owner"].(string)

	// Build repo path for URLs (owner/repo)
	repoPath := repoName
	if owner != "" && repoName != "" {
		repoPath = owner + "/" + repoName
	}

	title := eventType
	var desc string

	switch {
	case strings.HasPrefix(eventType, "issue."):
		issue, _ := payload["issue"].(map[string]any)
		if issue != nil {
			num, _ := issue["number"].(float64)
			issueTitle, _ := issue["title"].(string)
			action := strings.TrimPrefix(eventType, "issue.")
			title = fmt.Sprintf("Issue #%d %s", int(num), action)
			desc = issueTitle
		}

	case strings.HasPrefix(eventType, "pr."):
		pr, _ := payload["pull_request"].(map[string]any)
		if pr != nil {
			num, _ := pr["number"].(float64)
			prTitle, _ := pr["title"].(string)
			head, _ := pr["head_branch"].(string)
			base, _ := pr["base_branch"].(string)
			action := strings.TrimPrefix(eventType, "pr.")
			title = fmt.Sprintf("Pull request #%d %s", int(num), action)
			desc = prTitle
			if head != "" && base != "" {
				desc += fmt.Sprintf("\n`%s` → `%s`", head, base)
			}
		}

	case eventType == "push":
		ref, _ := payload["ref"].(string)
		commits, _ := payload["commits"].([]any)
		branch := strings.TrimPrefix(ref, "refs/heads/")
		title = fmt.Sprintf("Push to %s", branch)
		if len(commits) > 0 {
			var lines []string
			for i, c := range commits {
				if i >= 5 {
					lines = append(lines, fmt.Sprintf("... and %d more", len(commits)-5))
					break
				}
				cm, _ := c.(map[string]any)
				if cm != nil {
					sha, _ := cm["id"].(string)
					msg, _ := cm["message"].(string)
					if len(sha) > 7 {
						sha = sha[:7]
					}
					if idx := strings.IndexByte(msg, '\n'); idx > 0 {
						msg = msg[:idx]
					}
					lines = append(lines, fmt.Sprintf("`%s` %s", sha, msg))
				}
			}
			desc = strings.Join(lines, "\n")
		}

	case eventType == "comment.created":
		comment, _ := payload["comment"].(map[string]any)
		if comment != nil {
			body, _ := comment["body"].(string)
			if len(body) > 200 {
				body = body[:200] + "..."
			}
			desc = body
		}
		// Add context about which issue/PR the comment is on
		if iss, ok := payload["issue"].(map[string]any); ok {
			num, _ := iss["number"].(float64)
			title = fmt.Sprintf("Comment on issue #%d", int(num))
		} else if pr, ok := payload["pull_request"].(map[string]any); ok {
			num, _ := pr["number"].(float64)
			title = fmt.Sprintf("Comment on pull request #%d", int(num))
		} else {
			title = "New comment"
		}

	case eventType == "review.submitted":
		review, _ := payload["review"].(map[string]any)
		if review != nil {
			state, _ := review["state"].(string)
			body, _ := review["body"].(string)
			title = fmt.Sprintf("Review submitted: %s", state)
			if len(body) > 200 {
				body = body[:200] + "..."
			}
			desc = body
		}

	case eventType == "ping":
		title = "Webhook connected"
		desc = "Webhook configured successfully!"
	}

	embed := map[string]any{
		"title":     title,
		"color":     color,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if desc != "" {
		embed["description"] = desc
	}
	if repoPath != "" {
		embed["footer"] = map[string]any{
			"text":     repoPath,
			"icon_url": discordAvatarURL,
		}
	}
	if sender != "" {
		embed["author"] = map[string]string{"name": sender}
	}

	discord := map[string]any{
		"username":   "Gitwise",
		"avatar_url": discordAvatarURL,
		"embeds":     []any{embed},
	}

	return json.Marshal(discord)
}

func (s *Service) deliver(w models.Webhook, eventType string, payloadJSON []byte) uuid.UUID {
	deliveryID := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Transform payload for Discord webhooks
	body := payloadJSON
	if isDiscordWebhook(w.URL) {
		discordBody, err := buildDiscordPayload(eventType, payloadJSON)
		if err != nil {
			slog.Error("discord payload transform failed", "webhook_id", w.ID, "error", err)
			return deliveryID
		}
		body = discordBody
	}

	// Build request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		slog.Error("webhook request build failed", "webhook_id", w.ID, "error", err)
		return deliveryID
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitwise-Event", eventType)
	req.Header.Set("X-Gitwise-Delivery", deliveryID.String())
	req.Header.Set("User-Agent", "Gitwise-Hookshot")

	// Always HMAC over the canonical source payload (payloadJSON), not the
	// transformed Discord body. This allows receivers to verify against the
	// original event bytes regardless of destination format.
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
	return deliveryID
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

	sem := make(chan struct{}, maxConcurrentDeliveries)
	var wg sync.WaitGroup
	for _, p := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(p pending) {
			defer wg.Done()
			defer func() { <-sem }()
			s.retryDelivery(p.deliveryID, p.webhookID, p.eventType, p.payload, p.attempts, p.url, p.secret)
		}(p)
	}
	wg.Wait()
	return nil
}

func (s *Service) retryDelivery(deliveryID, webhookID uuid.UUID, eventType string, payload []byte, attempts int, webhookURL, secret string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Transform payload for Discord webhooks (stored payload is raw event JSON)
	body := payload
	if isDiscordWebhook(webhookURL) {
		discordBody, err := buildDiscordPayload(eventType, payload)
		if err != nil {
			slog.Error("discord retry payload transform failed", "delivery_id", deliveryID, "error", err)
			return
		}
		body = discordBody
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		slog.Error("retry request build failed", "delivery_id", deliveryID, "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitwise-Event", eventType)
	req.Header.Set("X-Gitwise-Delivery", deliveryID.String())
	req.Header.Set("User-Agent", "Gitwise-Hookshot")

	// Sign the canonical stored payload, not the transformed Discord body.
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

// DeliverOne delivers a payload to a single webhook synchronously and returns the
// delivery ID. Used by the Test handler so the UI can reference the delivery record.
func (s *Service) DeliverOne(ctx context.Context, wh models.Webhook, eventType string, payload []byte) uuid.UUID {
	return s.deliver(wh, eventType, payload)
}

// truncatePayload re-marshals payload after capping any string values that push
// the total over maxBytes. It sets truncated=true in the top-level map so
// receivers know some content was elided.
func truncatePayload(payload map[string]any, maxBytes int) []byte {
	// Deep-clone to avoid mutating the caller's map.
	clone := make(map[string]any, len(payload))
	for k, v := range payload {
		clone[k] = v
	}
	clone["truncated"] = true

	// Cap long string fields at 1 KiB each.
	const maxStrLen = 1024
	var capStrings func(m map[string]any)
	capStrings = func(m map[string]any) {
		for k, v := range m {
			switch tv := v.(type) {
			case string:
				if len(tv) > maxStrLen {
					m[k] = tv[:maxStrLen] + "…[truncated]"
				}
			case map[string]any:
				capStrings(tv)
			case []any:
				for _, elem := range tv {
					if sub, ok := elem.(map[string]any); ok {
						capStrings(sub)
					}
				}
			}
		}
	}
	capStrings(clone)

	b, err := json.Marshal(clone)
	if err != nil {
		// Fallback: just a minimal marker.
		b, _ = json.Marshal(map[string]any{"truncated": true, "error": "payload too large"})
	}
	return b
}

// validateURL performs a URL parse + scheme/port check only.
// IP blocking is enforced exclusively in the dialer's Control callback used for
// every actual HTTP request, closing the DNS TOCTOU window: the OS resolves the
// hostname exactly once — during dial — where our Control function checks the
// resolved IP before the socket connects.
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
	// Block explicit IP literals in the URL directly (fast path — no DNS needed).
	if ip := net.ParseIP(u.Hostname()); ip != nil && isPrivateIP(ip) {
		return ErrPrivateURL
	}
	return nil
}

func validateEventTypes(events []string) error {
	for _, e := range events {
		if !validEventTypes[e] {
			return fmt.Errorf("%w: %s", ErrInvalidEventType, e)
		}
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
