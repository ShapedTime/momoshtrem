// Package secret encrypts small secrets at rest (Trakt's OAuth tokens) with
// AES-256-GCM under SECRET_KEY.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// version is the first byte of every sealed value, so the format can change
// without guessing what an old row holds.
const version byte = 1

// MinKeyLength is the shortest SECRET_KEY that isn't 64 hex characters.
const MinKeyLength = 32

var (
	// ErrInvalidKey means SECRET_KEY is missing or too short.
	ErrInvalidKey = errors.New("SECRET_KEY must be 64 hex characters (openssl rand -hex 32) or at least 32 characters")
	// ErrDecrypt means a sealed value can't be opened: another key, another
	// purpose (AAD), or damaged bytes.
	ErrDecrypt = errors.New("can't decrypt: SECRET_KEY changed or the data is damaged")
)

// Box seals and opens values with one key. It is safe for concurrent use.
type Box struct {
	aead cipher.AEAD
}

// New derives the key from SECRET_KEY: 64 hex characters are its 32 bytes;
// anything else of at least MinKeyLength characters is hashed with SHA-256.
func New(secretKey string) (*Box, error) {
	key, err := deriveKey(secretKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	return &Box{aead: aead}, nil
}

func deriveKey(secretKey string) ([]byte, error) {
	if len(secretKey) == 64 {
		if key, err := hex.DecodeString(secretKey); err == nil {
			return key, nil
		}
	}
	if len(secretKey) < MinKeyLength {
		return nil, ErrInvalidKey
	}
	sum := sha256.Sum256([]byte(secretKey))
	return sum[:], nil
}

// Seal encrypts plaintext for one purpose, aad (e.g. "trakt:access"):
// version, a random 12-byte nonce, then the ciphertext and its tag.
func (b *Box) Seal(plaintext []byte, aad string) []byte {
	out := make([]byte, 1+b.aead.NonceSize(), 1+b.aead.NonceSize()+len(plaintext)+b.aead.Overhead())
	out[0] = version
	nonce := out[1:]
	_, _ = rand.Read(nonce) // never fails (crypto/rand crashes the program instead)
	return b.aead.Seal(out, nonce, plaintext, []byte(aad))
}

// Open decrypts what Seal sealed for the same aad. Any mismatch is
// ErrDecrypt.
func (b *Box) Open(sealed []byte, aad string) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < 1+n+b.aead.Overhead() || sealed[0] != version {
		return nil, ErrDecrypt
	}
	plaintext, err := b.aead.Open(nil, sealed[1:1+n], sealed[1+n:], []byte(aad))
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}
