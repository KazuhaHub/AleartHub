// Package secretbox is AES-256-GCM authenticated encryption for secrets at rest
// (TOTP secrets, later IdP client secrets). KEK is a 32-byte key persisted in the
// keys dir. SPEC-SAFETY / ARCHITECTURE §8: secrets must be encrypted at rest.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

var ErrKeySize = errors.New("secretbox: KEK must be 32 bytes")

// Seal returns nonce||ciphertext||tag.
func Seal(kek, plaintext []byte) ([]byte, error) {
	if len(kek) != 32 {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func Open(kek, ct []byte) ([]byte, error) {
	if len(kek) != 32 {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ct) < gcm.NonceSize() {
		return nil, errors.New("secretbox: ciphertext too short")
	}
	nonce, body := ct[:gcm.NonceSize()], ct[gcm.NonceSize():]
	return gcm.Open(nil, nonce, body, nil)
}
