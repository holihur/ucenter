package security

import (
	"strings"
	"testing"
)

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrong password", hash) {
		t.Fatal("wrong password must not verify")
	}
	other, _ := HashPassword("correct horse battery staple")
	if hash == other {
		t.Fatal("hashes must use unique salts")
	}
}

func TestVerifyPKCE(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	s256 := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if !VerifyPKCE(verifier, s256, "S256") {
		t.Fatal("valid S256 challenge rejected")
	}
	if VerifyPKCE("wrong-verifier", s256, "S256") {
		t.Fatal("invalid S256 verifier accepted")
	}
	if !VerifyPKCE(verifier, verifier, "plain") {
		t.Fatal("valid plain challenge rejected")
	}
	if VerifyPKCE(verifier, "other", "plain") {
		t.Fatal("invalid plain verifier accepted")
	}
	if VerifyPKCE(verifier, s256, "unknown") {
		t.Fatal("unknown challenge method accepted")
	}
}

func TestClientSecrets(t *testing.T) {
	secret, err := GenerateClientSecret()
	if err != nil {
		t.Fatalf("GenerateClientSecret: %v", err)
	}
	if !strings.HasPrefix(secret, "ucs_") {
		t.Fatalf("unexpected secret prefix: %s", secret)
	}
	hash := HashClientSecret(secret)
	if !VerifyClientSecret(secret, hash) {
		t.Fatal("valid client secret rejected")
	}
	if VerifyClientSecret("ucs_wrong", hash) {
		t.Fatal("invalid client secret accepted")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, hashes, err := GenerateRecoveryCodes(8)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if len(codes) != 8 || len(hashes) != 8 {
		t.Fatalf("expected 8 codes, got %d/%d", len(codes), len(hashes))
	}
	remaining, ok := ConsumeRecoveryCode(codes[3], hashes)
	if !ok {
		t.Fatal("valid recovery code rejected")
	}
	if len(remaining) != 7 {
		t.Fatalf("expected 7 remaining codes, got %d", len(remaining))
	}
	if _, ok := ConsumeRecoveryCode(codes[3], remaining); ok {
		t.Fatal("recovery code must only be usable once")
	}
}
