package handlers

import (
	"encoding/base64"
	"errors"
	"net/http"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/services/totp"
	"github.com/gitwise-io/gitwise/internal/services/user"
)

// TwoFactorHandler provides HTTP handlers for 2FA setup, enable, disable, and verification.
type TwoFactorHandler struct {
	totp     *totp.Service
	users    *user.Service
	sessions *middleware.SessionManager
}

// NewTwoFactorHandler creates a new TwoFactorHandler.
func NewTwoFactorHandler(totpSvc *totp.Service, users *user.Service, sessions *middleware.SessionManager) *TwoFactorHandler {
	return &TwoFactorHandler{totp: totpSvc, users: users, sessions: sessions}
}

// Setup generates a TOTP secret and returns the provisioning URI, QR code, and recovery codes.
// I2: Requires current_password for re-authentication.
// I3: Rejects if 2FA is already enabled.
func (h *TwoFactorHandler) Setup(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
	}
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if req.CurrentPassword == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "current_password is required")
		return
	}

	u, err := h.users.GetByIDWithPassword(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get user")
		return
	}

	if u.Password == "" {
		writeError(w, http.StatusBadRequest, "no_password", "set a password before enabling 2FA (OAuth-only accounts must add a password first)")
		return
	}

	result, err := h.totp.BeginSetup(r.Context(), *userID, u.Username, "Gitwise", u.Password, req.CurrentPassword)
	if errors.Is(err, totp.ErrBadPassword) {
		writeError(w, http.StatusForbidden, "bad_password", "incorrect password")
		return
	}
	if errors.Is(err, totp.ErrAlreadyEnabled) {
		writeError(w, http.StatusConflict, "already_enabled", "2FA is already enabled; disable it first")
		return
	}
	if errors.Is(err, totp.ErrNotConfigured) {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "2FA is not configured on this server")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to set up 2FA")
		return
	}

	// Generate QR code as base64-encoded PNG.
	qrPNG, err := qrcode.Encode(result.URI, qrcode.Medium, 256)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to generate QR code")
		return
	}
	qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

	writeJSON(w, http.StatusOK, map[string]any{
		"secret":         result.Secret,
		"uri":            result.URI,
		"qr_code":        "data:image/png;base64," + qrBase64,
		"recovery_codes": result.RecoveryCodes,
	})
}

// Enable verifies a TOTP code and activates 2FA.
func (h *TwoFactorHandler) Enable(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "code is required")
		return
	}

	err := h.totp.Enable(r.Context(), *userID, req.Code)
	if errors.Is(err, totp.ErrInvalidCode) {
		writeError(w, http.StatusBadRequest, "invalid_code", "invalid TOTP code")
		return
	}
	if errors.Is(err, totp.ErrNotSetUp) {
		writeError(w, http.StatusBadRequest, "not_setup", "2FA not set up: run setup first")
		return
	}
	if errors.Is(err, totp.ErrNotConfigured) {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "2FA is not configured on this server")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to enable 2FA")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "enabled"})
}

// Disable verifies a TOTP or recovery code and deactivates 2FA.
func (h *TwoFactorHandler) Disable(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "code is required")
		return
	}

	err := h.totp.Disable(r.Context(), *userID, req.Code)
	if errors.Is(err, totp.ErrInvalidCode) {
		writeError(w, http.StatusBadRequest, "invalid_code", "invalid code")
		return
	}
	if errors.Is(err, totp.ErrNotConfigured) {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "2FA is not configured on this server")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to disable 2FA")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

// Status returns whether 2FA is enabled for the current user.
func (h *TwoFactorHandler) Status(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	enabled, err := h.totp.IsEnabled(r.Context(), *userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to check 2FA status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
}

// Challenge is called by the client after every login attempt (step 1) to
// determine whether 2FA verification is required. This design keeps the Login
// response body identical for 2FA and non-2FA users (enumeration defence).
//
// The pending-2FA token is read from the HttpOnly gw_pending_2fa cookie set by
// Login. If the cookie is absent or empty the account has no pending 2FA step
// (either no 2FA is enabled, or the session was already issued by Login).
func (h *TwoFactorHandler) Challenge(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(middleware.Pending2FACookie)
	if err != nil || cookie.Value == "" {
		// No pending 2FA — the full session was already set by Login.
		writeJSON(w, http.StatusOK, map[string]bool{"requires_2fa": false})
		return
	}

	// Validate the token exists in Redis (do not consume yet).
	_, valErr := h.totp.ValidatePendingAuth(r.Context(), cookie.Value)
	if errors.Is(valErr, totp.ErrInvalidToken) || errors.Is(valErr, totp.ErrTooManyAttempts) {
		// Expired or already exhausted — treat as no active 2FA challenge.
		h.sessions.ClearPending2FACookie(w)
		writeJSON(w, http.StatusOK, map[string]bool{"requires_2fa": false})
		return
	}
	if valErr != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to check 2FA challenge")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"requires_2fa": true})
}

// Verify2FA completes a login that requires 2FA verification.
// The pending token is read from the gw_pending_2fa HttpOnly cookie (set by
// Login) rather than the request body to prevent token leakage in URLs/logs.
// I5: Token is looked up (not consumed) first; only consumed after successful verification.
// C4: Attempt count is tracked; after 5 failures the token is deleted.
func (h *TwoFactorHandler) Verify2FA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "code is required")
		return
	}

	// Read the pending token from the HttpOnly cookie (not the request body).
	cookie, err := r.Cookie(middleware.Pending2FACookie)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusBadRequest, "missing_token", "no pending 2FA session; please log in again")
		return
	}
	pendingToken := cookie.Value

	// I5: Look up user ID without consuming the token.
	userID, err := h.totp.ValidatePendingAuth(r.Context(), pendingToken)
	if errors.Is(err, totp.ErrInvalidToken) {
		h.sessions.ClearPending2FACookie(w)
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid or expired 2FA token")
		return
	}
	if errors.Is(err, totp.ErrTooManyAttempts) {
		h.sessions.ClearPending2FACookie(w)
		writeError(w, http.StatusTooManyRequests, "too_many_attempts", "too many failed attempts; please log in again")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to verify 2FA token")
		return
	}

	// Verify the TOTP code.
	valid, err := h.totp.Verify(r.Context(), userID, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to verify 2FA code")
		return
	}
	if !valid {
		// C4: Increment attempt counter on failure.
		h.totp.IncrementAttempts(r.Context(), pendingToken)
		writeError(w, http.StatusUnauthorized, "invalid_code", "invalid 2FA code")
		return
	}

	// I5: Only consume the pending token after successful verification.
	h.totp.ConsumePendingAuth(r.Context(), pendingToken)

	// Clear the pending-2FA cookie and create the full session.
	h.sessions.ClearPending2FACookie(w)
	if err := h.sessions.Create(r.Context(), w, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create session")
		return
	}

	u, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get user")
		return
	}

	writeJSON(w, http.StatusOK, u)
}
