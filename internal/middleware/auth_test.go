package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestGetUserID_NoValue(t *testing.T) {
	ctx := context.Background()
	id := GetUserID(ctx)
	if id != nil {
		t.Errorf("expected nil, got %v", id)
	}
}

func TestGetUserID_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserIDKey, "not-a-uuid")
	id := GetUserID(ctx)
	if id != nil {
		t.Errorf("expected nil for wrong type, got %v", id)
	}
}

func TestGetUserID_Valid(t *testing.T) {
	uid := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, uid)
	id := GetUserID(ctx)
	if id == nil {
		t.Fatal("expected non-nil")
	}
	if *id != uid {
		t.Errorf("got %v, want %v", *id, uid)
	}
}

func TestRequireAuth_Unauthenticated(t *testing.T) {
	handler := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	var resp struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(resp.Errors))
	}
	if resp.Errors[0].Code != "unauthorized" {
		t.Errorf("error code = %q, want %q", resp.Errors[0].Code, "unauthorized")
	}
}

func TestRequireAuth_Authenticated(t *testing.T) {
	var called bool
	handler := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	uid := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, uid)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Error("next handler was not called")
	}
}

func TestRequireAuth_ContentType(t *testing.T) {
	handler := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}
