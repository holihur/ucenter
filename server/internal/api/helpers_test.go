package api

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	l := newLoginLimiter(2, time.Minute)
	key := "127.0.0.1|acme|user@example.com"
	if !l.allow(key) || !l.allow(key) {
		t.Fatal("first attempts should be allowed")
	}
	if l.allow(key) {
		t.Fatal("attempt over the limit should be rejected")
	}
	l.reset(key)
	if !l.allow(key) {
		t.Fatal("attempt after reset should be allowed")
	}
	if l.allow("other-key") != true {
		t.Fatal("different keys must not share limits")
	}
}
