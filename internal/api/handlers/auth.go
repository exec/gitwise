package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/oauth"
	"github.com/gitwise-io/gitwise/internal/services/totp"
	"github.com/gitwise-io/gitwise/internal/services/user"
)

type AuthHandler struct {
	users    *user.Service
	sessions *middleware.SessionManager
	oauth    *oauth.Service // nil if GitHub OAuth is not configured
	totp     *totp.Service
}

func NewAuthHandler(users *user.Service, sessions *middleware.SessionManager, oauthSvc *oauth.Service, totpSvc *totp.Service) *AuthHandler {
	return &AuthHandler{users: users, sessions: sessions, oauth: oauthSvc, totp: totpSvc}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUserRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	u, err := h.users.Create(r.Context(), req)
	if errors.Is(err, user.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if errors.Is(err, user.ErrDuplicateUser) {
		writeError(w, http.StatusConflict, "duplicate", "username or email already taken")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create user")
		return
	}

	if err := h.sessions.Create(r.Context(), w, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create session")
		return
	}

	writeJSON(w, http.StatusCreated, u)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	u, err := h.users.Authenticate(r.Context(), req.Login, req.Password)
	if errors.Is(err, user.ErrBadCredentials) {
		writeError(w, http.StatusUnauthorized, "bad_credentials", "invalid username/email or password")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "authentication failed")
		return
	}

	// Check if 2FA is enabled for this user.
	if h.totp != nil {
		enabled, err := h.totp.IsEnabled(r.Context(), u.ID)
		if err != nil {
			slog.Error("failed to check 2fa status during login", "error", err)
			writeError(w, http.StatusInternalServerError, "server_error", "authentication failed")
			return
		}
		if enabled {
			// Don't create a session yet. Return a pending token for 2FA verification.
			pendingToken, err := h.totp.StorePendingAuth(r.Context(), u.ID)
			if err != nil {
				slog.Error("failed to create pending 2fa auth", "error", err)
				writeError(w, http.StatusInternalServerError, "server_error", "authentication failed")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"requires_2fa":  true,
				"pending_token": pendingToken,
			})
			return
		}
	}

	if err := h.sessions.Create(r.Context(), w, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create session")
		return
	}

	writeJSON(w, http.StatusOK, u)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session, sessionID, err := h.sessions.Get(r.Context(), r)
	if err != nil || session == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	h.sessions.Destroy(r.Context(), w, sessionID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	u, err := h.users.GetByID(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get user")
		return
	}

	writeJSON(w, http.StatusOK, u)
}

// Token management

func (h *AuthHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req models.CreateTokenRequest
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	token, err := h.users.CreateToken(r.Context(), *userID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create token")
		return
	}

	writeJSON(w, http.StatusCreated, token)
}

func (h *AuthHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	tokens, err := h.users.ListTokens(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list tokens")
		return
	}

	writeJSON(w, http.StatusOK, tokens)
}

func (h *AuthHandler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	tokenIDStr := chi.URLParam(r, "tokenID")
	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid token ID")
		return
	}

	if err := h.users.DeleteToken(r.Context(), *userID, tokenID); err != nil {
		if errors.Is(err, user.ErrTokenNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListProviders returns the list of enabled OAuth providers.
func (h *AuthHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	providers := []string{}
	if h.oauth != nil {
		providers = append(providers, "github")
	}
	writeJSON(w, http.StatusOK, providers)
}

// GitHubLogin redirects the user to GitHub's OAuth authorization page.
func (h *AuthHandler) GitHubLogin(w http.ResponseWriter, r *http.Request) {
	if h.oauth == nil {
		writeError(w, http.StatusNotFound, "not_configured", "GitHub OAuth is not configured")
		return
	}

	state, err := h.oauth.GenerateState(r.Context())
	if err != nil {
		slog.Error("failed to generate oauth state", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to initiate OAuth flow")
		return
	}

	authURL := h.oauth.GetGitHubAuthURL(state)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// GitHubCallback handles the OAuth callback from GitHub. It validates the
// state parameter, exchanges the authorization code for a token, fetches the
// GitHub user, finds or creates a local user, creates a session, and redirects
// to the frontend.
// C5: If the resolved user has 2FA enabled, redirect to the frontend 2FA challenge
// instead of creating a session immediately.
func (h *AuthHandler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	if h.oauth == nil {
		writeError(w, http.StatusNotFound, "not_configured", "GitHub OAuth is not configured")
		return
	}

	// Validate state to prevent CSRF.
	state := r.URL.Query().Get("state")
	if state == "" || !h.oauth.ValidateState(r.Context(), state) {
		writeError(w, http.StatusBadRequest, "invalid_state", "invalid or expired OAuth state")
		return
	}

	// Check for error from GitHub.
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		desc := r.URL.Query().Get("error_description")
		slog.Warn("github oauth error", "error", errParam, "description", desc)
		writeError(w, http.StatusBadRequest, "oauth_error", "GitHub denied the request: "+desc)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing_code", "missing authorization code")
		return
	}

	// Exchange code for token and fetch GitHub user.
	ghUser, accessToken, err := h.oauth.ExchangeGitHubCode(r.Context(), code)
	if err != nil {
		slog.Error("github oauth exchange failed", "error", err)
		writeError(w, http.StatusBadGateway, "exchange_failed", "failed to authenticate with GitHub")
		return
	}

	// Find or create local user.
	providerID := oauth.ProviderID(ghUser.ID)
	u, err := h.users.FindOrCreateByOAuth(
		r.Context(), "github", providerID,
		ghUser.Email, ghUser.Login, ghUser.Name, ghUser.AvatarURL, accessToken,
	)
	if err != nil {
		slog.Error("oauth find/create user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create or link user")
		return
	}

	// C5: Check if the resolved user has 2FA enabled.
	if h.totp != nil {
		enabled, err := h.totp.IsEnabled(r.Context(), u.ID)
		if err != nil {
			slog.Error("failed to check 2fa status during github callback", "error", err)
			writeError(w, http.StatusInternalServerError, "server_error", "authentication failed")
			return
		}
		if enabled {
			// Store a pending auth token and redirect to the frontend 2FA challenge.
			pendingToken, err := h.totp.StorePendingAuth(r.Context(), u.ID)
			if err != nil {
				slog.Error("failed to create pending 2fa auth for github user", "error", err)
				writeError(w, http.StatusInternalServerError, "server_error", "authentication failed")
				return
			}
			http.Redirect(w, r, "/login?pending_2fa="+pendingToken, http.StatusTemporaryRedirect)
			return
		}
	}

	// Create session.
	if err := h.sessions.Create(r.Context(), w, u.ID); err != nil {
		slog.Error("oauth session creation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create session")
		return
	}

	// Redirect to frontend root. The SPA will pick up the session via fetchMe().
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}
