package ucenter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientLoginAndCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "uc_session", Value: "token", Path: "/"})
			_ = json.NewEncoder(w).Encode(map[string]any{"mfaRequired": false, "user": map[string]string{"email": "a@b.test"}})
		case "/api/auth/me":
			if r.Header.Get("Cookie") != "uc_session=token" {
				t.Errorf("missing session cookie: %q", r.Header.Get("Cookie"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tenant": map[string]string{"slug": "acme"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := New(server.URL)
	login, err := c.Login(context.Background(), "acme", "a@b.test", "pw")
	if err != nil || login.MFARequired {
		t.Fatalf("login: %+v %v", login, err)
	}
	if c.Cookie != "uc_session=token" {
		t.Fatalf("cookie not captured: %q", c.Cookie)
	}
	me, err := c.Me(context.Background())
	if err != nil || me["tenant"] == nil {
		t.Fatalf("me: %v %v", me, err)
	}
}

func TestClientTenantHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tenant-ID") != "tenant-1" {
			t.Errorf("missing tenant header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 0})
	}))
	defer server.Close()
	c := New(server.URL)
	c.TenantID = "tenant-1"
	if _, err := c.ListUsers(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}
