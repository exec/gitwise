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

// Verify2FA completes a login that requires 2FA verification.
// The client must present the pending_token from the login response plus a TOTP code.
// I5: Token is looked up (not consumed) first; only consumed after successful verification.
// C4: Attempt count is tracked; after 5 failures the token is deleted.
func (h *TwoFactorHandler) Verify2FA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PendingToken string `json:"pending_token"`
		Code         string `json:"code"`
	}
	if handleDecodeError(w, decodeJSON(r, &req)) {
		return
	}

	if req.PendingToken == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "pending_token and code are required")
		return
	}

	// I5: Look up user ID without consuming the token.
	userID, err := h.totp.ValidatePendingAuth(r.Context(), req.PendingToken)
	if errors.Is(err, totp.ErrInvalidToken) {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid or expired 2FA token")
		return
	}
	if errors.Is(err, totp.ErrTooManyAttempts) {
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
		h.totp.IncrementAttempts(r.Context(), req.PendingToken)
		writeError(w, http.StatusUnauthorized, "invalid_code", "invalid 2FA code")
		return
	}

	// I5: Only consume the pending token after successful verification.
	h.totp.ConsumePendingAuth(r.Context(), req.PendingToken)

	// Create the full session now that 2FA is verified.
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
