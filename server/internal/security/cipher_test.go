package security

import (
	"strings"
	"testing"
)

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher("test-encryption-key")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := c.Encrypt("super-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encrypted, "enc:v1:") {
		t.Fatalf("unexpected ciphertext prefix: %s", encrypted)
	}
	plain, err := c.Decrypt(encrypted)
	if err != nil || plain != "super-secret" {
		t.Fatalf("round trip failed: %q %v", plain, err)
	}
}

func TestCipherWrongKeyAndTamper(t *testing.T) {
	c1, _ := NewCipher("key-one")
	c2, _ := NewCipher("key-two")
	encrypted, _ := c1.Encrypt("secret")
	if _, err := c2.Decrypt(encrypted); err == nil {
		t.Fatal("decrypt with a different key must fail")
	}
	tampered := encrypted[:len(encrypted)-2] + "AA"
	if _, err := c1.Decrypt(tampered); err == nil {
		t.Fatal("tampered ciphertext must fail authentication")
	}
	if _, err := c1.Decrypt("not-encrypted"); err == nil {
		t.Fatal("plaintext input must be rejected")
	}
}
