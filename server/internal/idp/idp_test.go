package idp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestMockProvider(t *testing.T) {
	p := &mockProvider{cfg: Config{"subject": "u1", "email": "u1@example.com", "name": "User One"}}
	auth := p.AuthURL("state-1", "https://id.example.com/callback")
	u, err := url.Parse(auth)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("state") != "state-1" || !strings.HasPrefix(u.Query().Get("code"), "mock:") {
		t.Fatalf("unexpected auth URL: %s", auth)
	}
	identity, err := p.Exchange(context.Background(), "mock:u1", "")
	if err != nil || identity.Subject != "u1" || identity.Email != "u1@example.com" {
		t.Fatalf("unexpected identity: %#v %v", identity, err)
	}
}

func TestOAuth2AuthURL(t *testing.T) {
	p, err := newOAuth2("google", Config{"clientId": "client", "clientSecret": "secret"})
	if err != nil {
		t.Fatal(err)
	}
	raw := p.AuthURL("state", "https://id.example.com/t/acme/idp/google/callback")
	u, _ := url.Parse(raw)
	q := u.Query()
	if q.Get("client_id") != "client" || q.Get("state") != "state" || q.Get("response_type") != "code" {
		t.Fatalf("unexpected authorize URL: %s", raw)
	}
	if !strings.Contains(q.Get("scope"), "openid") {
		t.Fatalf("missing default scopes: %s", raw)
	}
}

func TestAlipayRSA2Signature(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	p, err := newAlipay(Config{"appId": "app", "privateKey": base64.StdEncoding.EncodeToString(der)})
	if err != nil {
		t.Fatal(err)
	}
	params := map[string]string{"app_id": "app", "method": "alipay.system.oauth.token", "charset": "utf-8"}
	sig, err := p.sign(params)
	if err != nil {
		t.Fatal(err)
	}
	rawSig, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		t.Fatal(err)
	}
	content := "app_id=app&charset=utf-8&method=alipay.system.oauth.token"
	digest := sha256.Sum256([]byte(content))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], rawSig); err != nil {
		t.Fatalf("signature did not verify: %v", err)
	}
}
