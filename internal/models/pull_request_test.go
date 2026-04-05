package models

import (
	"encoding/json"
	"testing"
)

func TestPRIntent_JSON(t *testing.T) {
	intent := PRIntent{
		Type:       "feature",
		Scope:      "auth",
		Components: []string{"login", "signup"},
	}

	data, err := json.Marshal(intent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PRIntent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != "feature" {
		t.Errorf("Type = %q", decoded.Type)
	}
	if decoded.Scope != "auth" {
		t.Errorf("Scope = %q", decoded.Scope)
	}
	if len(decoded.Components) != 2 {
		t.Errorf("Components = %v", decoded.Components)
	}
}

func TestCreatePullRequestRequest_JSON(t *testing.T) {
	jsonStr := `{
		"title": "Add auth",
		"body": "Implements authentication",
		"source_branch": "feature/auth",
		"target_branch": "main",
		"draft": true,
		"intent": {"type": "feature", "scope": "auth", "components": ["login"]}
	}`

	var req CreatePullRequestRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Title != "Add auth" {
		t.Errorf("Title = %q", req.Title)
	}
	if req.SourceBranch != "feature/auth" {
		t.Errorf("SourceBranch = %q", req.SourceBranch)
	}
	if !req.Draft {
		t.Error("Draft should be true")
	}
	if req.Intent == nil {
		t.Fatal("Intent should not be nil")
	}
	if req.Intent.Type != "feature" {
		t.Errorf("Intent.Type = %q", req.Intent.Type)
	}
}

func TestMergePullRequestRequest_JSON(t *testing.T) {
	jsonStr := `{"strategy":"squash","message":"Squash merge","delete_branch":true}`
	var req MergePullRequestRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Strategy != "squash" {
		t.Errorf("Strategy = %q", req.Strategy)
	}
	if !req.DeleteBranch {
		t.Error("DeleteBranch should be true")
	}
}

func TestUpdatePullRequestRequest_Partial(t *testing.T) {
	jsonStr := `{"title":"Updated title"}`
	var req UpdatePullRequestRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Title == nil || *req.Title != "Updated title" {
		t.Errorf("Title = %v", req.Title)
	}
	if req.Body != nil {
		t.Error("Body should be nil")
	}
	if req.Status != nil {
		t.Error("Status should be nil")
	}
}

func TestPRDiffResponse_JSON(t *testing.T) {
	resp := PRDiffResponse{
		Commits: []Commit{{SHA: "abc", Message: "test"}},
		Files:   []DiffFile{{Path: "main.go", Status: "modified"}},
	}
	resp.Stats.TotalCommits = 1
	resp.Stats.TotalFiles = 1

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PRDiffResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Stats.TotalCommits != 1 {
		t.Errorf("TotalCommits = %d", decoded.Stats.TotalCommits)
	}
}
