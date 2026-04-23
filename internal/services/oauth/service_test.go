package oauth

import (
	"strings"
	"testing"

	"github.com/gitwise-io/gitwise/internal/config"
)

func newTestService(baseURL string) *Service {
	return &Service{
		cfg:  config.GitHubOAuthConfig{ClientID: "test-id", ClientSecret: "test-secret"},
		base: strings.TrimRight(baseURL, "/"),
	}
}

// TestCallbackURI verifies that callbackURI always produces the expected
// allowlisted path and rejects disallowed base-URL schemes.
func TestCallbackURI(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		wantSuffix string
		wantErr    bool
	}{
		{
			name:       "https base URL",
			baseURL:    "https://git.example.com",
			wantSuffix: "/api/v1/auth/github/callback",
		},
		{
			name:       "http base URL (dev)",
			baseURL:    "http://localhost:3000",
			wantSuffix: "/api/v1/auth/github/callback",
		},
		{
			name:    "javascript scheme — rejected",
			baseURL: "javascript:alert(1)",
			wantErr: true,
		},
		{
			name:    "data scheme — rejected",
			baseURL: "data:text/html,<h1>",
			wantErr: true,
		},
		{
			name:       "trailing slash in base — stripped",
			baseURL:    "https://git.example.com/",
			wantSuffix: "/api/v1/auth/github/callback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(tt.baseURL)
			uri, err := svc.callbackURI()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for base URL %q, got nil (uri=%q)", tt.baseURL, uri)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.HasSuffix(uri, tt.wantSuffix) {
				t.Errorf("callbackURI() = %q, want suffix %q", uri, tt.wantSuffix)
			}
			// Must not have query string or fragment appended.
			if strings.ContainsAny(uri, "?#") {
				t.Errorf("callbackURI() %q must not contain query/fragment", uri)
			}
		})
	}
}

// TestGetGitHubAuthURL verifies the auth URL contains the correct redirect_uri.
func TestGetGitHubAuthURL(t *testing.T) {
	svc := newTestService("https://git.example.com")
	authURL := svc.GetGitHubAuthURL("test-state-123")

	wantRedirect := "redirect_uri=https%3A%2F%2Fgit.example.com%2Fapi%2Fv1%2Fauth%2Fgithub%2Fcallback"
	if !strings.Contains(authURL, wantRedirect) {
		t.Errorf("auth URL does not contain expected redirect_uri\nURL:  %s\nWant: %s", authURL, wantRedirect)
	}
	if !strings.Contains(authURL, "state=test-state-123") {
		t.Errorf("auth URL missing state parameter: %s", authURL)
	}
}
