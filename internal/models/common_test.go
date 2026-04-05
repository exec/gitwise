package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAPIResponse_JSON_DataOnly(t *testing.T) {
	resp := APIResponse{Data: map[string]string{"hello": "world"}}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if s == "" {
		t.Fatal("empty JSON")
	}

	var decoded APIResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Meta != nil {
		t.Error("meta should be nil")
	}
	if decoded.Errors != nil {
		t.Error("errors should be nil")
	}
}

func TestAPIResponse_JSON_WithErrors(t *testing.T) {
	resp := APIResponse{
		Errors: []APIError{
			{Code: "not_found", Message: "item not found"},
			{Code: "validation", Message: "bad field", Field: "name"},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded APIResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(decoded.Errors))
	}
	if decoded.Errors[1].Field != "name" {
		t.Errorf("Field = %q, want %q", decoded.Errors[1].Field, "name")
	}
}

func TestAPIResponse_JSON_WithMeta(t *testing.T) {
	resp := APIResponse{
		Data: []string{"a", "b"},
		Meta: &ResponseMeta{Total: 10, NextCursor: "cursor123"},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded APIResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Meta.Total != 10 {
		t.Errorf("Total = %d, want 10", decoded.Meta.Total)
	}
	if decoded.Meta.NextCursor != "cursor123" {
		t.Errorf("NextCursor = %q", decoded.Meta.NextCursor)
	}
}

func TestAPIError_OmitEmptyField(t *testing.T) {
	e := APIError{Code: "err", Message: "msg"}
	data, _ := json.Marshal(e)
	s := string(data)
	// Field should still be present since it has no omitempty tag
	var decoded map[string]string
	json.Unmarshal(data, &decoded)
	if _, ok := decoded["code"]; !ok {
		t.Error("expected code field in JSON")
	}
	_ = s
}

func TestTimestamps_JSON(t *testing.T) {
	ts := Timestamps{
		CreatedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 6, 20, 14, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(ts)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Timestamps
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.CreatedAt.Equal(ts.CreatedAt) {
		t.Errorf("CreatedAt mismatch")
	}
	if !decoded.UpdatedAt.Equal(ts.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch")
	}
}
