package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/gitwise-io/gitwise/internal/config"
)

const (
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
	githubUserURL      = "https://api.github.com/user"

	statePrefix = "oauth_state:"
	stateTTL    = 10 * time.Minute
)

// GitHubUser represents the relevant fields from GitHub's user API.
type GitHubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// Service handles OAuth2 flows for external providers.
type Service struct {
	cfg   config.GitHubOAuthConfig
	base  string // base URL for callback
	redis *redis.Client
}

// NewService creates a new OAuth service.
func NewService(cfg config.GitHubOAuthConfig, baseURL string, rdb *redis.Client) *Service {
	return &Service{
		cfg:   cfg,
		base:  strings.TrimRight(baseURL, "/"),
		redis: rdb,
	}
}

// GenerateState creates a cryptographically random state token and stores it
// in Redis with a 10-minute TTL for CSRF protection.
func (s *Service) GenerateState(ctx context.Context) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	state := hex.EncodeToString(b)

	if err := s.redis.Set(ctx, statePrefix+state, "1", stateTTL).Err(); err != nil {
		return "", fmt.Errorf("store oauth state: %w", err)
	}
	return state, nil
}

// ValidateState checks that the state token exists in Redis and deletes it
// (single-use). Returns true if valid.
func (s *Service) ValidateState(ctx context.Context, state string) bool {
	res, err := s.redis.Del(ctx, statePrefix+state).Result()
	return err == nil && res == 1
}

// GetGitHubAuthURL returns the GitHub authorization URL with the given state.
func (s *Service) GetGitHubAuthURL(state string) string {
	params := url.Values{
		"client_id":    {s.cfg.ClientID},
		"redirect_uri": {s.base + "/api/v1/auth/github/callback"},
		"scope":        {"user:email"},
		"state":        {state},
	}
	return githubAuthorizeURL + "?" + params.Encode()
}

// ExchangeGitHubCode exchanges an authorization code for an access token
// and fetches the GitHub user profile.
func (s *Service) ExchangeGitHubCode(ctx context.Context, code string) (*GitHubUser, string, error) {
	// Step 1: Exchange code for access token.
	tokenReqBody := url.Values{
		"client_id":     {s.cfg.ClientID},
		"client_secret": {s.cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {s.base + "/api/v1/auth/github/callback"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubTokenURL, strings.NewReader(tokenReqBody.Encode()))
	if err != nil {
		return nil, "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("exchange code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read token response: %w", err)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, "", fmt.Errorf("decode token response: %w", err)
	}
	if tokenResp.Error != "" {
		return nil, "", fmt.Errorf("github token error: %s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	if tokenResp.AccessToken == "" {
		return nil, "", fmt.Errorf("github returned empty access token")
	}

	// Step 2: Fetch user profile.
	ghUser, err := s.fetchGitHubUser(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, "", err
	}

	return ghUser, tokenResp.AccessToken, nil
}

// fetchGitHubUser calls the GitHub user API with the given access token.
func (s *Service) fetchGitHubUser(ctx context.Context, accessToken string) (*GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubUserURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch github user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github user API returned %d", resp.StatusCode)
	}

	var ghUser GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("decode github user: %w", err)
	}

	// If no public email, fetch from the emails endpoint.
	if ghUser.Email == "" {
		ghUser.Email = s.fetchPrimaryEmail(ctx, accessToken, ghUser.Login)
	}

	return &ghUser, nil
}

// fetchPrimaryEmail fetches the user's primary email from the GitHub emails API.
// Falls back to {login}@users.noreply.github.com if unavailable.
func (s *Service) fetchPrimaryEmail(ctx context.Context, accessToken, login string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return login + "@users.noreply.github.com"
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return login + "@users.noreply.github.com"
	}
	defer resp.Body.Close()

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return login + "@users.noreply.github.com"
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email
		}
	}

	return login + "@users.noreply.github.com"
}

// ProviderID returns the string representation of the GitHub user ID.
func ProviderID(ghID int) string {
	return strconv.Itoa(ghID)
}
