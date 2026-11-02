package security

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// NewTOTPSecret creates a new TOTP secret for an account.
func NewTOTPSecret(issuer, account string) (*otp.Key, error) {
	return totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: account,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
}

func VerifyTOTP(secret, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" || secret == "" {
		return false
	}
	ok, err := totp.ValidateCustom(code, secret, timeNow(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// GenerateRecoveryCodes returns human-friendly codes and their hashes.
func GenerateRecoveryCodes(n int) ([]string, []string, error) {
	codes := make([]string, 0, n)
	hashes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		raw, err := RandomToken(6)
		if err != nil {
			return nil, nil, err
		}
		code := fmt.Sprintf("%s-%s", strings.ToUpper(raw[:4]), strings.ToUpper(raw[4:8]))
		codes = append(codes, code)
		hashes = append(hashes, HashToken(code))
	}
	return codes, hashes, nil
}

// ConsumeRecoveryCode checks a recovery code against stored hashes and returns
// the remaining hashes when it matches.
func ConsumeRecoveryCode(code string, hashes []string) ([]string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	sum := sha256.Sum256([]byte(normalized))
	digest := hex.EncodeToString(sum[:])
	remaining := make([]string, 0, len(hashes))
	matched := false
	for _, h := range hashes {
		if !matched && subtle.ConstantTimeCompare([]byte(h), []byte(digest)) == 1 {
			matched = true
			continue
		}
		remaining = append(remaining, h)
	}
	return remaining, matched
}
