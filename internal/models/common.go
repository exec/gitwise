package models

import "time"

// APIResponse is the standard JSON envelope for all API responses.
type APIResponse struct {
	Data   any            `json:"data,omitempty"`
	Meta   *ResponseMeta  `json:"meta,omitempty"`
	Errors []APIError     `json:"errors,omitempty"`
}

type ResponseMeta struct {
	Total      int    `json:"total,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// Timestamps embedded in most models.
type Timestamps struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
