package mirror

import (
	"bytes"
	"testing"
)

func TestSealOpenRoundtrip(t *testing.T) {
	c, err := NewCrypto("test-secret-value-goes-here")
	if err != nil {
		t.Fatalf("NewCrypto: %v", err)
	}
	plaintext := []byte("ghp_abcdef1234567890")
	ct, nonce, err := c.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(ct, plaintext) {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := c.Open(ct, nonce)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Open = %q, want %q", got, plaintext)
	}
}

func TestSealGeneratesUniqueNonces(t *testing.T) {
	c, _ := NewCrypto("secret")
	seen := map[string]bool{}
	for i := 0; i < 10000; i++ {
		_, nonce, err := c.Seal([]byte("x"))
		if err != nil {
			t.Fatal(err)
		}
		key := string(nonce)
		if seen[key] {
			t.Fatalf("duplicate nonce at iteration %d", i)
		}
		seen[key] = true
	}
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	c, _ := NewCrypto("secret")
	ct, nonce, _ := c.Seal([]byte("hello"))
	ct[0] ^= 0xFF // flip a byte
	_, err := c.Open(ct, nonce)
	if err == nil {
		t.Fatal("expected error on tampered ciphertext")
	}
}

func TestOpenRejectsWrongNonce(t *testing.T) {
	c, _ := NewCrypto("secret")
	ct, _, _ := c.Seal([]byte("hello"))
	_, err := c.Open(ct, make([]byte, 12))
	if err == nil {
		t.Fatal("expected error on wrong nonce")
	}
}

func TestNewCryptoRejectsEmptySecret(t *testing.T) {
	if _, err := NewCrypto(""); err == nil {
		t.Fatal("expected error on empty secret")
	}
}

func TestOpenRejectsTooShortCiphertext(t *testing.T) {
	c, _ := NewCrypto("secret")
	_, err := c.Open([]byte{0x00}, make([]byte, 12))
	if err == nil {
		t.Fatal("expected error on too-short ciphertext")
	}
}
