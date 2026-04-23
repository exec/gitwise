package sshkey

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"golang.org/x/crypto/ssh"
)

func generateRSAPublicKey(t *testing.T, bits int) ssh.PublicKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("ssh.NewPublicKey RSA: %v", err)
	}
	return pub
}

func generateECDSAPublicKey(t *testing.T, curve elliptic.Curve) ssh.PublicKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}
	pub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("ssh.NewPublicKey ECDSA: %v", err)
	}
	return pub
}

func generateEd25519PublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	rawPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 key: %v", err)
	}
	pub, err := ssh.NewPublicKey(rawPub)
	if err != nil {
		t.Fatalf("ssh.NewPublicKey Ed25519: %v", err)
	}
	return pub
}

// TestValidateKeyStrength covers allowed and rejected key types and sizes.
func TestValidateKeyStrength(t *testing.T) {
	tests := []struct {
		name    string
		keyFn   func(t *testing.T) ssh.PublicKey
		wantErr bool
	}{
		{
			name:    "RSA 4096 — allowed",
			keyFn:   func(t *testing.T) ssh.PublicKey { return generateRSAPublicKey(t, 4096) },
			wantErr: false,
		},
		{
			name:    "RSA 3072 — allowed (minimum)",
			keyFn:   func(t *testing.T) ssh.PublicKey { return generateRSAPublicKey(t, 3072) },
			wantErr: false,
		},
		{
			name:    "RSA 2048 — rejected (below minimum)",
			keyFn:   func(t *testing.T) ssh.PublicKey { return generateRSAPublicKey(t, 2048) },
			wantErr: true,
		},
		{
			name:    "RSA 1024 — rejected (far too small)",
			keyFn:   func(t *testing.T) ssh.PublicKey { return generateRSAPublicKey(t, 1024) },
			wantErr: true,
		},
		{
			name:    "ECDSA P-256 — allowed",
			keyFn:   func(t *testing.T) ssh.PublicKey { return generateECDSAPublicKey(t, elliptic.P256()) },
			wantErr: false,
		},
		{
			name:    "ECDSA P-384 — allowed",
			keyFn:   func(t *testing.T) ssh.PublicKey { return generateECDSAPublicKey(t, elliptic.P384()) },
			wantErr: false,
		},
		{
			name:    "ECDSA P-521 — allowed",
			keyFn:   func(t *testing.T) ssh.PublicKey { return generateECDSAPublicKey(t, elliptic.P521()) },
			wantErr: false,
		},
		{
			name:    "Ed25519 — allowed",
			keyFn:   func(t *testing.T) ssh.PublicKey { return generateEd25519PublicKey(t) },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := tt.keyFn(t)
			err := validateKeyStrength(pub)
			if tt.wantErr && err == nil {
				t.Error("expected error (weak/disallowed key) but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for allowed key: %v", err)
			}
		})
	}
}
