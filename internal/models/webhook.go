package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Webhook struct {
	ID     uuid.UUID `json:"id"`
	RepoID uuid.UUID `json:"repo_id"`
	URL    string    `json:"url"`
	Secret string    `json:"-"`
	Events []string  `json:"events"`
	Active bool      `json:"active"`
	Timestamps
}

type CreateWebhookRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
	Active *bool    `json:"active,omitempty"`
}

type UpdateWebhookRequest struct {
	URL    *string   `json:"url,omitempty"`
	Secret *string   `json:"secret,omitempty"`
	Events *[]string `json:"events,omitempty"`
	Active *bool     `json:"active,omitempty"`
}

type WebhookDelivery struct {
	ID             uuid.UUID       `json:"id"`
	WebhookID      uuid.UUID       `json:"webhook_id"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	ResponseStatus *int            `json:"response_status,omitempty"`
	Success        bool            `json:"success"`
	Attempts       int             `json:"attempts"`
	Duration       int             `json:"duration_ms"`
	DeliveredAt    time.Time       `json:"delivered_at"`
}
