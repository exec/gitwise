package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gitwise-io/gitwise/internal/models"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"hello": "world"})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var resp models.APIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data == nil {
		t.Error("expected non-nil data")
	}
	if resp.Errors != nil {
		t.Error("expected nil errors")
	}
}

func TestWriteJSON_Created(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]int{"id": 1})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestWriteJSONMeta(t *testing.T) {
	rec := httptest.NewRecorder()
	meta := &models.ResponseMeta{Total: 42, NextCursor: "abc123"}
	writeJSONMeta(rec, http.StatusOK, []string{"a", "b"}, meta)

	var resp models.APIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Meta == nil {
		t.Fatal("expected non-nil meta")
	}
	if resp.Meta.Total != 42 {
		t.Errorf("meta.Total = %d, want 42", resp.Meta.Total)
	}
	if resp.Meta.NextCursor != "abc123" {
		t.Errorf("meta.NextCursor = %q, want %q", resp.Meta.NextCursor, "abc123")
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "bad_request", "something went wrong")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var resp models.APIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(resp.Errors))
	}
	if resp.Errors[0].Code != "bad_request" {
		t.Errorf("code = %q, want %q", resp.Errors[0].Code, "bad_request")
	}
	if resp.Errors[0].Message != "something went wrong" {
		t.Errorf("message = %q", resp.Errors[0].Message)
	}
}

func TestWriteFieldError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeFieldError(rec, http.StatusUnprocessableEntity, "validation", "required", "name")

	var resp models.APIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(resp.Errors))
	}
	if resp.Errors[0].Field != "name" {
		t.Errorf("field = %q, want %q", resp.Errors[0].Field, "name")
	}
}

func TestDecodeJSON_ValidBody(t *testing.T) {
	body := `{"username":"alice","email":"alice@example.com","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	var result models.CreateUserRequest
	err := decodeJSON(req, &result)
	if err != nil {
		t.Fatalf("decodeJSON error: %v", err)
	}
	if result.Username != "alice" {
		t.Errorf("Username = %q, want %q", result.Username, "alice")
	}
	if result.Email != "alice@example.com" {
		t.Errorf("Email = %q", result.Email)
	}
}

func TestDecodeJSON_InvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not json"))
	var result models.CreateUserRequest
	err := decodeJSON(req, &result)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDecodeJSON_UnknownFields(t *testing.T) {
	body := `{"username":"alice","unknown_field":"value"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	var result models.CreateUserRequest
	err := decodeJSON(req, &result)
	if err == nil {
		t.Error("expected error for unknown fields")
	}
}

func TestDecodeJSON_TooLarge(t *testing.T) {
	// Create a valid-looking JSON body larger than 1MB.
	// Start with a valid JSON key then pad the value.
	padding := bytes.Repeat([]byte("a"), maxBodySize)
	body := append([]byte(`{"username":"`), padding...)
	body = append(body, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	var result models.CreateUserRequest
	err := decodeJSON(req, &result)
	if err == nil {
		t.Error("expected error for too-large body")
	}
	// The error may be ErrBodyTooLarge or a JSON parse error triggered by MaxBytesReader
	// Either way, the request should be rejected
}

func TestDecodeJSON_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	var result models.CreateUserRequest
	err := decodeJSON(req, &result)
	if err == nil {
		t.Error("expected error for empty body")
	}
}

func TestHandleDecodeError_Nil(t *testing.T) {
	rec := httptest.NewRecorder()
	wrote := handleDecodeError(rec, nil)
	if wrote {
		t.Error("expected false for nil error")
	}
}

func TestHandleDecodeError_BodyTooLarge(t *testing.T) {
	rec := httptest.NewRecorder()
	wrote := handleDecodeError(rec, ErrBodyTooLarge)
	if !wrote {
		t.Error("expected true for ErrBodyTooLarge")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandleDecodeError_OtherError(t *testing.T) {
	rec := httptest.NewRecorder()
	wrote := handleDecodeError(rec, io.ErrUnexpectedEOF)
	if !wrote {
		t.Error("expected true for other error")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestWriteUserJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteUserJSON(rec, map[string]string{"name": "test"})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWriteReposJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteReposJSON(rec, []string{"repo1", "repo2"})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
