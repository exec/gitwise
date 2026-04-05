package models

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestIssue_JSON_Roundtrip(t *testing.T) {
	milestoneID := uuid.New()
	issue := Issue{
		ID:          uuid.New(),
		RepoID:      uuid.New(),
		Number:      42,
		AuthorID:    uuid.New(),
		AuthorName:  "alice",
		Title:       "Fix login bug",
		Body:        "Login fails when...",
		Status:      "open",
		Labels:      []string{"bug", "auth"},
		Assignees:   []uuid.UUID{uuid.New()},
		MilestoneID: &milestoneID,
		LinkedPRs:   []uuid.UUID{},
		Priority:    "high",
		Metadata:    json.RawMessage(`{"severity":"p1"}`),
	}

	data, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Issue
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Number != 42 {
		t.Errorf("Number = %d", decoded.Number)
	}
	if decoded.Status != "open" {
		t.Errorf("Status = %q", decoded.Status)
	}
	if len(decoded.Labels) != 2 {
		t.Errorf("Labels = %v", decoded.Labels)
	}
	if decoded.Priority != "high" {
		t.Errorf("Priority = %q", decoded.Priority)
	}
	if decoded.MilestoneID == nil {
		t.Error("MilestoneID should not be nil")
	}
}

func TestCreateIssueRequest_JSON(t *testing.T) {
	jsonStr := `{"title":"New issue","body":"Description","labels":["bug"],"priority":"medium","assignees":["alice","bob"]}`
	var req CreateIssueRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Title != "New issue" {
		t.Errorf("Title = %q", req.Title)
	}
	if len(req.Assignees) != 2 {
		t.Errorf("Assignees = %v", req.Assignees)
	}
	if req.Priority != "medium" {
		t.Errorf("Priority = %q", req.Priority)
	}
}

func TestUpdateIssueRequest_Partial(t *testing.T) {
	jsonStr := `{"status":"closed"}`
	var req UpdateIssueRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Status == nil || *req.Status != "closed" {
		t.Errorf("Status = %v", req.Status)
	}
	if req.Title != nil {
		t.Error("Title should be nil")
	}
}

func TestIssue_ClosedAtOmitted(t *testing.T) {
	issue := Issue{Status: "open"}
	data, _ := json.Marshal(issue)
	var m map[string]any
	json.Unmarshal(data, &m)
	if _, ok := m["closed_at"]; ok {
		t.Error("closed_at should be omitted when nil")
	}
}
