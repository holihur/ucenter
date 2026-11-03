// Package ucenter provides a small Go client for the UCenter auth and admin API.
package ucenter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL  string
	HTTP     *http.Client
	Cookie   string
	Token    string
	TenantID string
	Headers  http.Header
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 20 * time.Second},
		Headers: http.Header{},
	}
}

type LoginResult struct {
	MFARequired bool          `json:"mfaRequired"`
	MFAToken    string        `json:"mfaToken,omitempty"`
	MFAMethods  []string      `json:"mfaMethods,omitempty"`
	User        *User         `json:"user,omitempty"`
	Tenant      *PublicTenant `json:"tenant,omitempty"`
	Next        string        `json:"next,omitempty"`
	RequestID   string        `json:"requestId,omitempty"`
}

type User struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	Username      string `json:"username"`
	Status        string `json:"status"`
	PlatformAdmin bool   `json:"platformAdmin"`
	CreatedAt     string `json:"createdAt"`
}

type PublicTenant struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Role struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int64 `json:"page"`
	PageSize int64 `json:"pageSize"`
}

type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%s (%d): %s", e.Code, e.Status, e.Message) }

func (c *Client) Login(ctx context.Context, tenant, email, password string) (*LoginResult, error) {
	var out LoginResult
	if err := c.do(ctx, http.MethodPost, "/api/auth/login", map[string]string{
		"tenant": tenant, "email": email, "password": password,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Me(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.do(ctx, http.MethodGet, "/api/auth/me", nil, &out)
	return out, err
}

func (c *Client) Logout(ctx context.Context) error {
	var out map[string]any
	return c.do(ctx, http.MethodPost, "/api/auth/logout", nil, &out)
}

func (c *Client) ListUsers(ctx context.Context, query url.Values) (*Page[User], error) {
	var out Page[User]
	path := "/api/admin/users"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateUser(ctx context.Context, email, name string, roles []string) (map[string]any, error) {
	var out map[string]any
	body := map[string]any{"email": email, "name": name, "roles": roles}
	if err := c.do(ctx, http.MethodPost, "/api/admin/users", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateTenant(ctx context.Context, slug, name, ownerEmail string) (map[string]any, error) {
	var out map[string]any
	body := map[string]any{"slug": slug, "name": name, "ownerEmail": ownerEmail}
	if err := c.do(ctx, http.MethodPost, "/api/admin/tenants", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, values := range c.Headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	if c.Cookie != "" {
		req.Header.Set("Cookie", c.Cookie)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.TenantID != "" {
		req.Header.Set("X-Tenant-ID", c.TenantID)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if cookie := resp.Header.Get("Set-Cookie"); cookie != "" {
		c.Cookie = strings.Split(cookie, ";")[0]
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		var payload struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &payload)
		return &Error{Status: resp.StatusCode, Code: payload.Error.Code, Message: payload.Error.Message}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}
