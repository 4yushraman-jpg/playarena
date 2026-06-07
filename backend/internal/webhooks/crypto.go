package webhooks

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

const rawSecretBytes = 32 // 256-bit raw secret

// GenerateSecret produces a cryptographically random 32-byte webhook secret
// and returns the hex-encoded string to show the caller once.
func GenerateSecret() (rawSecret string, err error) {
	b := make([]byte, rawSecretBytes)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// EncryptSecret encrypts the raw webhook secret with AES-256-GCM.
// key must be exactly 32 bytes. The returned ciphertext has the 12-byte GCM
// nonce prepended: nonce || ciphertext || tag.
func EncryptSecret(key []byte, rawSecret string) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("webhook secret key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(rawSecret), nil)
	return ciphertext, nil
}

// DecryptSecret decrypts ciphertext produced by EncryptSecret.
// key must be exactly 32 bytes.
func DecryptSecret(key []byte, ciphertext []byte) (string, error) {
	if len(key) != 32 {
		return "", errors.New("webhook secret key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
