package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAPIToken_JSON_WithToken(t *testing.T) {
	token := APIToken{
		ID:     uuid.New(),
		UserID: uuid.New(),
		Name:   "CI Token",
		Token:  "gw_abc123",
		Scopes: []string{"read", "write"},
	}

	data, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded APIToken
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Token != "gw_abc123" {
		t.Errorf("Token = %q", decoded.Token)
	}
	if len(decoded.Scopes) != 2 {
		t.Errorf("Scopes = %v", decoded.Scopes)
	}
}

func TestAPIToken_JSON_TokenOmitted(t *testing.T) {
	token := APIToken{
		ID:   uuid.New(),
		Name: "Readonly",
	}

	data, _ := json.Marshal(token)
	var m map[string]any
	json.Unmarshal(data, &m)
	if _, ok := m["token"]; ok {
		t.Error("token should be omitted when empty")
	}
}

func TestCreateTokenRequest_JSON(t *testing.T) {
	expiry := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	jsonStr := `{"name":"Deploy","scopes":["push"],"expires_at":"2025-12-31T00:00:00Z"}`

	var req CreateTokenRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Name != "Deploy" {
		t.Errorf("Name = %q", req.Name)
	}
	if req.ExpiresAt == nil || !req.ExpiresAt.Equal(expiry) {
		t.Errorf("ExpiresAt = %v", req.ExpiresAt)
	}
}

func TestCreateTokenRequest_NoExpiry(t *testing.T) {
	jsonStr := `{"name":"Permanent","scopes":["read"]}`
	var req CreateTokenRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.ExpiresAt != nil {
		t.Error("ExpiresAt should be nil")
	}
}
