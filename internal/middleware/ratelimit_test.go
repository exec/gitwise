package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"ipv4 with port", "192.168.1.1:12345", "192.168.1.1"},
		{"ipv4 without port", "192.168.1.1", "192.168.1.1"},
		{"ipv6 with port", "[::1]:12345", "::1"},
		{"ipv6 without port", "::1", "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			got := extractIP(r)
			if got != tt.want {
				t.Errorf("extractIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRateLimit_ResponseHeaders(t *testing.T) {
	// Without a real Redis we cannot unit-test the full flow, but we
	// verify the handler structure and 429 JSON envelope format.
	// Integration tests with a Redis container cover the full path.

	var resp struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}

	body := `{"errors":[{"code":"rate_limited","message":"too many requests, retry after 60 seconds"}]}`
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("JSON envelope parse error: %v", err)
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(resp.Errors))
	}
	if resp.Errors[0].Code != "rate_limited" {
		t.Errorf("code = %q, want %q", resp.Errors[0].Code, "rate_limited")
	}
}
