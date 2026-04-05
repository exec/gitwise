package models

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestRepository_JSON_Roundtrip(t *testing.T) {
	repo := Repository{
		ID:            uuid.New(),
		OwnerID:       uuid.New(),
		OwnerName:     "alice",
		Name:          "myrepo",
		Description:   "A test repo",
		DefaultBranch: "main",
		Visibility:    "public",
		LanguageStats: json.RawMessage(`{"Go": 80, "TypeScript": 20}`),
		Topics:        []string{"go", "web"},
		StarsCount:    42,
		ForksCount:    5,
	}

	data, err := json.Marshal(repo)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Repository
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Name != "myrepo" {
		t.Errorf("Name = %q", decoded.Name)
	}
	if decoded.Visibility != "public" {
		t.Errorf("Visibility = %q", decoded.Visibility)
	}
	if len(decoded.Topics) != 2 {
		t.Errorf("Topics len = %d, want 2", len(decoded.Topics))
	}
	if decoded.StarsCount != 42 {
		t.Errorf("StarsCount = %d", decoded.StarsCount)
	}
}

func TestCreateRepoRequest_JSON(t *testing.T) {
	jsonStr := `{"name":"newrepo","description":"desc","visibility":"private","default_branch":"develop","topics":["api"],"auto_init":true}`
	var req CreateRepoRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Name != "newrepo" {
		t.Errorf("Name = %q", req.Name)
	}
	if req.Visibility != "private" {
		t.Errorf("Visibility = %q", req.Visibility)
	}
	if req.DefaultBranch != "develop" {
		t.Errorf("DefaultBranch = %q", req.DefaultBranch)
	}
	if !req.AutoInit {
		t.Error("AutoInit should be true")
	}
	if len(req.Topics) != 1 || req.Topics[0] != "api" {
		t.Errorf("Topics = %v", req.Topics)
	}
}

func TestUpdateRepoRequest_JSON_Partial(t *testing.T) {
	jsonStr := `{"description":"updated desc"}`
	var req UpdateRepoRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Description == nil || *req.Description != "updated desc" {
		t.Errorf("Description = %v", req.Description)
	}
	if req.Visibility != nil {
		t.Error("Visibility should be nil")
	}
}

func TestRepository_OmitEmptyCloneURL(t *testing.T) {
	repo := Repository{Name: "test"}
	data, _ := json.Marshal(repo)
	var m map[string]any
	json.Unmarshal(data, &m)
	if _, ok := m["clone_url"]; ok {
		t.Error("clone_url should be omitted when empty")
	}
	if _, ok := m["ssh_url"]; ok {
		t.Error("ssh_url should be omitted when empty")
	}
}
