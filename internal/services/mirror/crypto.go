package mirror

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// Crypto seals and opens PATs at rest using AES-256-GCM.
// The key is derived from GITWISE_SECRET via HKDF-SHA256 with a fixed info label,
// so rotating GITWISE_SECRET invalidates all stored PATs.
type Crypto struct {
	aead cipher.AEAD
}

func NewCrypto(secret string) (*Crypto, error) {
	if secret == "" {
		return nil, errors.New("mirror crypto: empty secret")
	}
	h := hkdf.New(sha256.New, []byte(secret), nil, []byte("gitwise-mirror-pat-v1"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(h, key); err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Crypto{aead: aead}, nil
}

func (c *Crypto) Seal(plaintext []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, c.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = c.aead.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func (c *Crypto) Open(ciphertext, nonce []byte) ([]byte, error) {
	if len(nonce) != c.aead.NonceSize() {
		return nil, errors.New("mirror crypto: bad nonce size")
	}
	return c.aead.Open(nil, nonce, ciphertext, nil)
}
