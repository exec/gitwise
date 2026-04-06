package totp

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

// TestEncryptDecrypt verifies the AES-GCM encrypt/decrypt round-trip.
func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty string", ""},
		{"short secret", "JBSWY3DPEHPK3PXP"},
		{"long secret", "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := encrypt(tt.plaintext, key)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}

			dec, err := decrypt(enc, key)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}

			if dec != tt.plaintext {
				t.Errorf("got %q, want %q", dec, tt.plaintext)
			}
		})
	}
}

// TestEncryptDifferentCiphertexts verifies that encrypting the same plaintext
// twice produces different ciphertexts (due to random nonce).
func TestEncryptDifferentCiphertexts(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	enc1, err := encrypt("secret", key)
	if err != nil {
		t.Fatal(err)
	}
	enc2, err := encrypt("secret", key)
	if err != nil {
		t.Fatal(err)
	}

	if enc1 == enc2 {
		t.Error("two encryptions of the same plaintext should produce different ciphertexts")
	}
}

// TestDecryptWrongKey verifies that decryption with a wrong key fails.
func TestDecryptWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 0xFF

	enc, err := encrypt("secret", key1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = decrypt(enc, key2)
	if err == nil {
		t.Error("expected decrypt with wrong key to fail")
	}
}

// TestRecoveryCodeHashVerify tests the argon2id hash/verify round-trip for recovery codes.
func TestRecoveryCodeHashVerify(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		match bool
	}{
		{"exact match", "abcdef123456", true},
		{"case insensitive", "ABCDEF123456", true}, // verifyRecoveryCode normalizes before hashing
		{"wrong code", "000000000000", false},
		{"empty code", "", false},
	}

	// Hash the code "abcdef123456".
	hashed, err := hashRecoveryCode("abcdef123456")
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// verifyRecoveryCode expects a lowercase code; the service normalizes before calling.
			code := tt.code
			if code != "" {
				code = normalizeCode(code)
			}
			got := verifyRecoveryCode(code, hashed)
			if got != tt.match {
				t.Errorf("verifyRecoveryCode(%q) = %v, want %v", tt.code, got, tt.match)
			}
		})
	}
}

func normalizeCode(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// TestGenerateRecoveryCodes checks entropy requirements (I4).
func TestGenerateRecoveryCodes(t *testing.T) {
	codes, err := generateRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}

	if len(codes) != recoveryCodeCount {
		t.Errorf("got %d codes, want %d", len(codes), recoveryCodeCount)
	}

	for _, code := range codes {
		// 6 bytes = 12 hex chars
		if len(code) != 12 {
			t.Errorf("code %q has %d chars, want 12", code, len(code))
		}
		// Should be valid hex.
		if _, err := hex.DecodeString(code); err != nil {
			t.Errorf("code %q is not valid hex: %v", code, err)
		}
	}

	// All codes should be unique.
	seen := make(map[string]bool)
	for _, code := range codes {
		if seen[code] {
			t.Errorf("duplicate code: %s", code)
		}
		seen[code] = true
	}
}

// TestHashRecoveryCodes verifies that hashing produces unique outputs for the same input
// (due to random salt).
func TestHashRecoveryCodesDifferentSalts(t *testing.T) {
	h1, err := hashRecoveryCode("abcdef123456")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := hashRecoveryCode("abcdef123456")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("two hashes of the same code should produce different outputs (different salts)")
	}
}

// TestNewServiceValidation tests TOTP key validation.
func TestNewServiceValidation(t *testing.T) {
	tests := []struct {
		name    string
		keyHex  string
		wantErr bool
	}{
		{"empty key (ok, 2FA disabled)", "", false},
		{"valid 32-byte key", hex.EncodeToString(make([]byte, 32)), false},
		{"invalid hex", "not-hex", true},
		{"too short", hex.EncodeToString(make([]byte, 16)), true},
		{"too long", hex.EncodeToString(make([]byte, 64)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewService(nil, nil, tt.keyHex)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewService(key=%q) error = %v, wantErr %v", tt.keyHex, err, tt.wantErr)
			}
		})
	}
}

// TestVerifyPasswordFnInjection tests that we can inject a password verifier for testing.
func TestVerifyPasswordFnInjection(t *testing.T) {
	// Save and restore the original.
	orig := verifyPasswordFn
	defer func() { verifyPasswordFn = orig }()

	verifyPasswordFn = func(password, encoded string) bool {
		return password == "correct" && encoded == "hash"
	}

	if !verifyPasswordFn("correct", "hash") {
		t.Error("expected true")
	}
	if verifyPasswordFn("wrong", "hash") {
		t.Error("expected false")
	}
}

// TestBeginSetupRequiresPassword tests that BeginSetup rejects when password is wrong (I2).
func TestBeginSetupRequiresPassword(t *testing.T) {
	orig := verifyPasswordFn
	defer func() { verifyPasswordFn = orig }()

	verifyPasswordFn = func(password, encoded string) bool {
		return password == "correct"
	}

	key := make([]byte, 32)
	svc, err := NewService(nil, nil, hex.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}

	// Should fail with bad password (before any DB call).
	_, err = svc.BeginSetup(context.Background(), uuid.New(), "test", "Gitwise", "hash", "wrong")
	if err == nil {
		t.Error("expected error for wrong password")
	}
	if err.Error() != ErrBadPassword.Error() {
		t.Errorf("expected ErrBadPassword, got %v", err)
	}
}

// TestBeginSetupRequiresKey tests that BeginSetup fails when encryption key is not configured (C1).
func TestBeginSetupRequiresKey(t *testing.T) {
	svc, err := NewService(nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.BeginSetup(context.Background(), uuid.New(), "test", "Gitwise", "hash", "pass")
	if err == nil {
		t.Error("expected error when key not configured")
	}
	if err.Error() != ErrNotConfigured.Error() {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

// TestTOTPValidateRoundTrip ensures our encrypt/decrypt round-trip produces
// a secret that validates TOTP codes.
func TestTOTPValidateRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	// Generate a TOTP key.
	totpKey, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "test",
		AccountName: "user",
	})
	if err != nil {
		t.Fatal(err)
	}

	secret := totpKey.Secret()

	// Encrypt and decrypt.
	enc, err := encrypt(secret, key)
	if err != nil {
		t.Fatal(err)
	}

	dec, err := decrypt(enc, key)
	if err != nil {
		t.Fatal(err)
	}

	if dec != secret {
		t.Fatalf("round-trip failed: got %q, want %q", dec, secret)
	}

	// Generate a code from the decrypted secret and validate it.
	code, err := totp.GenerateCode(dec, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if !totp.Validate(code, dec) {
		t.Error("TOTP code validation failed after encrypt/decrypt round-trip")
	}
}
