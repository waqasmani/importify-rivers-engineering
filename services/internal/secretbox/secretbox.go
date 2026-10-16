// Package secretbox encrypts small secrets (provider API keys) at rest with AES-256-GCM.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

// Box seals and opens secrets with a 32-byte key.
type Box struct{ aead cipher.AEAD }

// New creates a Box. The key must be 32 bytes.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, errors.New("secretbox: key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext. The nonce is prepended to the ciphertext.
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plaintext, []byte("rea-secret-v1")), nil
}

// Open decrypts a value produced by Seal.
func (b *Box) Open(sealed []byte) ([]byte, error) {
	ns := b.aead.NonceSize()
	if len(sealed) < ns {
		return nil, errors.New("secretbox: ciphertext too short")
	}
	return b.aead.Open(nil, sealed[:ns], sealed[ns:], []byte("rea-secret-v1"))
}
