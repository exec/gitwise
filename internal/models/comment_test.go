package models

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestComment_JSON(t *testing.T) {
	issueID := uuid.New()
	c := Comment{
		ID:         uuid.New(),
		RepoID:     uuid.New(),
		IssueID:    &issueID,
		PRID:       nil,
		AuthorID:   uuid.New(),
		AuthorName: "bob",
		Body:       "LGTM!",
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Comment
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Body != "LGTM!" {
		t.Errorf("Body = %q", decoded.Body)
	}
	if decoded.IssueID == nil {
		t.Error("IssueID should not be nil")
	}
}

func TestComment_PRIDOmitted(t *testing.T) {
	c := Comment{Body: "test"}
	data, _ := json.Marshal(c)
	var m map[string]any
	json.Unmarshal(data, &m)
	if _, ok := m["pr_id"]; ok {
		t.Error("pr_id should be omitted when nil")
	}
}

func TestCreateCommentRequest_JSON(t *testing.T) {
	jsonStr := `{"body":"Nice work!"}`
	var req CreateCommentRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Body != "Nice work!" {
		t.Errorf("Body = %q", req.Body)
	}
}

func TestUpdateCommentRequest_JSON(t *testing.T) {
	jsonStr := `{"body":"Updated comment"}`
	var req UpdateCommentRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Body != "Updated comment" {
		t.Errorf("Body = %q", req.Body)
	}
}
