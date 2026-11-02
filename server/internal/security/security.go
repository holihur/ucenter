package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory  = 32 * 1024
	argonTime    = 3
	argonThreads = 2
	argonKeyLen  = 32
)

var ErrInvalidHash = errors.New("invalid password hash format")

// HashPassword derives an Argon2id hash with a random salt.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory uint32
	var iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func RandomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func RandomHex(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func VerifyPKCE(verifier, challenge, method string) bool {
	if challenge == "" {
		return true
	}
	if verifier == "" {
		return false
	}
	switch method {
	case "S256":
		sum := sha256.Sum256([]byte(verifier))
		return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) == 1
	case "", "plain":
		return subtle.ConstantTimeCompare([]byte(verifier), []byte(challenge)) == 1
	default:
		return false
	}
}

// NormalizeEmail lowercases and trims an email address.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Client secrets are high-entropy random values, so a fast digest is
// sufficient and avoids the cost of a password KDF on every token request.
func HashClientSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func VerifyClientSecret(secret, hash string) bool {
	if secret == "" || hash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(HashClientSecret(secret)), []byte(hash)) == 1
}

func GenerateClientSecret() (string, error) {
	raw, err := RandomToken(32)
	if err != nil {
		return "", err
	}
	return "ucs_" + raw, nil
}

func GenerateAPIToken() (raw, hash, prefix string, err error) {
	token, err := RandomToken(32)
	if err != nil {
		return "", "", "", err
	}
	raw = "ucm_" + token
	return raw, HashToken(raw), raw[:12], nil
}
