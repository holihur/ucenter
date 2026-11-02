package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const cipherPrefix = "enc:v1:"

// Cipher provides authenticated encryption for secrets at rest.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher derives a 32-byte key from ENCRYPTION_KEY. The value may be a
// base64-encoded 32-byte key, a hex key, or a passphrase (hashed with SHA-256).
// An empty key is allowed for local development and must not be used in
// production.
func NewCipher(key string) (*Cipher, error) {
	var raw []byte
	key = strings.TrimSpace(key)
	if key == "" {
		sum := sha256.Sum256([]byte("ucenter-development-only-encryption-key"))
		raw = sum[:]
	} else if decoded, err := base64.StdEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		raw = decoded
	} else if decoded, err := base64.RawStdEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		raw = decoded
	} else {
		sum := sha256.Sum256([]byte(key))
		raw = sum[:]
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if c == nil {
		return "", errors.New("cipher is not configured")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	return cipherPrefix + base64.RawStdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

func (c *Cipher) Decrypt(value string) (string, error) {
	if c == nil {
		return "", errors.New("cipher is not configured")
	}
	if !strings.HasPrefix(value, cipherPrefix) {
		return "", errors.New("value is not encrypted")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, cipherPrefix))
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(raw) < c.aead.NonceSize() {
		return "", errors.New("ciphertext is too short")
	}
	nonce, payload := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	plain, err := c.aead.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}

func IsEncrypted(value string) bool { return strings.HasPrefix(value, cipherPrefix) }
