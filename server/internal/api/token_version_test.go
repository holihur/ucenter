package api

import (
	"testing"

	"ucenter/internal/models"
	"ucenter/internal/token"
)

func TestTokenVersionRevocation(t *testing.T) {
	user := &models.User{TokenVersion: 3}
	if !tokenVersionValid(&token.Claims{TokenVersion: 3}, user) {
		t.Fatal("current token version must be accepted")
	}
	if tokenVersionValid(&token.Claims{TokenVersion: 2}, user) {
		t.Fatal("older token version must be rejected after a bump")
	}
	if tokenVersionValid(&token.Claims{TokenVersion: 4}, user) {
		t.Fatal("unknown future token version must be rejected")
	}
}
