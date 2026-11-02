package token

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/security"
	"ucenter/internal/store"
)

func TestSigningKeyEncryptionAndRotation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	uri := "mongodb://127.0.0.1:27017"
	dbName := fmt.Sprintf("ucenter_token_test_%d", time.Now().UnixNano())
	st, err := store.New(ctx, uri, dbName)
	if err != nil {
		t.Skipf("mongo unavailable: %v", err)
	}
	defer func() {
		_ = st.DB.Drop(context.Background())
		_ = st.Close(context.Background())
	}()
	if err := st.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}

	cipher1, _ := security.NewCipher("test-key-one")
	mgr := NewManager(st, cipher1)
	tenantID := primitive.NewObjectID()

	key, err := mgr.KeyFor(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	var rec bson.M
	if err := st.DB.Collection("signing_keys").FindOne(ctx, bson.M{"kid": key.Kid}).Decode(&rec); err != nil {
		t.Fatal(err)
	}
	if v, ok := rec["private_pem"]; ok && v != "" {
		t.Fatal("private PEM must not be stored in plaintext")
	}
	if !strings.HasPrefix(fmt.Sprint(rec["private_pem_enc"]), "enc:v1:") {
		t.Fatal("private PEM is not encrypted")
	}

	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://issuer.test",
			Subject:   "user-1",
			Audience:  jwt.ClaimStrings{"client-1"},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		TenantID: tenantID.Hex(),
	}
	oldToken, err := mgr.Sign(ctx, tenantID, claims)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Verify(ctx, oldToken, tenantID, "https://issuer.test"); err != nil {
		t.Fatalf("old token must verify: %v", err)
	}

	rotated, err := mgr.Rotate(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Kid == key.Kid {
		t.Fatal("rotation must produce a new kid")
	}
	jwks, err := mgr.JWKS(ctx, tenantID)
	if err != nil || len(jwks) < 2 {
		t.Fatalf("JWKS must overlap old and new keys: %v %v", len(jwks), err)
	}
	if _, err := mgr.Verify(ctx, oldToken, tenantID, "https://issuer.test"); err != nil {
		t.Fatalf("old token must verify during key overlap: %v", err)
	}
	newToken, err := mgr.Sign(ctx, tenantID, claims)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mgr.Verify(ctx, newToken, tenantID, "https://issuer.test")
	if err != nil || parsed.TenantID != tenantID.Hex() {
		t.Fatalf("new token must verify: %v", err)
	}

	cipher2, _ := security.NewCipher("test-key-two")
	broken := NewManager(st, cipher2)
	if _, err := broken.KeyFor(ctx, tenantID); err == nil {
		t.Fatal("manager with the wrong encryption key must fail to load keys")
	}
}
