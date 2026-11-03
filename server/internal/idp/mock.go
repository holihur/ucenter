package idp

import (
	"context"
	"net/url"
	"strings"
)

type mockProvider struct{ cfg Config }

func (m *mockProvider) AuthURL(state, redirectURI string) string {
	subject := m.cfg["subject"]
	if subject == "" {
		subject = "mock-user"
	}
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	q.Set("code", "mock:"+subject)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}

func (m *mockProvider) Exchange(_ context.Context, code, _ string) (*Identity, error) {
	subject := strings.TrimPrefix(code, "mock:")
	if subject == "" || subject == code {
		subject = "mock-user"
	}
	email := m.cfg["email"]
	name := m.cfg["name"]
	if name == "" {
		name = "Mock User"
	}
	return &Identity{
		Subject: subject,
		Email:   email,
		Name:    name,
		Raw:     map[string]interface{}{"provider": "mock", "subject": subject},
	}, nil
}
