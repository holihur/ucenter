package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/store"
)

func apiTokenView(t *models.APIToken) gin.H {
	return gin.H{
		"id":         t.ID,
		"name":       t.Name,
		"prefix":     t.Prefix,
		"scopes":     t.Scopes,
		"expiresAt":  t.ExpiresAt,
		"lastUsedAt": t.LastUsedAt,
		"revoked":    t.Revoked,
		"createdAt":  t.CreatedAt,
	}
}

func (a *API) listAPITokens(c *gin.Context) {
	b := a.currentBundle(c)
	tokens, err := a.Store.ListAPITokens(c.Request.Context(), b.Tenant.ID, b.User.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load API tokens")
		return
	}
	items := make([]gin.H, 0, len(tokens))
	for i := range tokens {
		items = append(items, apiTokenView(&tokens[i]))
	}
	ok(c, gin.H{"items": items})
}

type createAPITokenRequest struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expiresInDays"`
}

func (a *API) createAPIToken(c *gin.Context) {
	var body createAPITokenRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "A token name is required")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 80 {
		fail(c, http.StatusBadRequest, "invalid_name", "Token name must be between 1 and 80 characters")
		return
	}
	allowed := map[string]bool{}
	for _, p := range a.currentPermissions(c) {
		allowed[p] = true
	}
	scopes := body.Scopes
	if len(scopes) == 0 {
		scopes = append([]string{}, a.currentPermissions(c)...)
	}
	seen := map[string]bool{}
	clean := []string{}
	for _, scope := range scopes {
		if !allowed[scope] {
			fail(c, http.StatusForbidden, "invalid_scope", "You cannot grant a token a permission you do not have: "+scope)
			return
		}
		if !seen[scope] {
			seen[scope] = true
			clean = append(clean, scope)
		}
	}
	if len(clean) == 0 {
		fail(c, http.StatusBadRequest, "no_scopes", "Your account has no permissions available for an API token")
		return
	}
	if body.ExpiresInDays == 0 {
		body.ExpiresInDays = 90
	}
	if body.ExpiresInDays < 1 || body.ExpiresInDays > 365 {
		fail(c, http.StatusBadRequest, "invalid_expiry", "Expiry must be between 1 and 365 days")
		return
	}
	raw, hash, prefix, err := security.GenerateAPIToken()
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create token")
		return
	}
	b := a.currentBundle(c)
	expires := time.Now().AddDate(0, 0, body.ExpiresInDays)
	token := &models.APIToken{
		TenantID:  b.Tenant.ID,
		UserID:    b.User.ID,
		Name:      body.Name,
		TokenHash: hash,
		Prefix:    prefix,
		Scopes:    clean,
		ExpiresAt: &expires,
	}
	if err := a.Store.CreateAPIToken(c.Request.Context(), token); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create token")
		return
	}
	a.audit(c, "mcp.token.created", "api_token", token.ID.Hex(), map[string]interface{}{"name": token.Name, "scopes": clean})
	c.JSON(http.StatusCreated, gin.H{"token": raw, "item": apiTokenView(token)})
}

func (a *API) revokeAPIToken(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Token not found")
		return
	}
	b := a.currentBundle(c)
	if err := a.Store.RevokeAPIToken(c.Request.Context(), b.Tenant.ID, b.User.ID, id); err != nil {
		if err == store.ErrNotFound {
			fail(c, http.StatusNotFound, "not_found", "Token not found")
			return
		}
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to revoke token")
		return
	}
	a.audit(c, "mcp.token.revoked", "api_token", id.Hex(), nil)
	ok(c, gin.H{"ok": true})
}

// mcpAuth resolves a bearer API token into a tenant-scoped MCP session.
func (a *API) mcpAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(raw), "bearer ") {
			raw = strings.TrimSpace(raw[7:])
		} else {
			raw = strings.TrimSpace(c.GetHeader("X-API-Key"))
		}
		if raw == "" || !strings.HasPrefix(raw, "ucm_") {
			c.Header("WWW-Authenticate", `Bearer realm="ucenter-mcp"`)
			fail(c, http.StatusUnauthorized, "invalid_token", "A UCenter API token is required")
			return
		}
		ctx := c.Request.Context()
		token, err := a.Store.FindAPITokenByHash(ctx, security.HashToken(raw))
		if err != nil {
			c.Header("WWW-Authenticate", `Bearer realm="ucenter-mcp", error="invalid_token"`)
			fail(c, http.StatusUnauthorized, "invalid_token", "The API token is invalid or expired")
			return
		}
		user, err := a.Store.FindUserByID(ctx, token.UserID)
		if err != nil || user.Status != models.StatusActive {
			fail(c, http.StatusUnauthorized, "invalid_token", "The token owner is not active")
			return
		}
		tenant, err := a.Store.FindTenantByID(ctx, token.TenantID)
		if err != nil || tenant.Status != models.StatusActive {
			fail(c, http.StatusUnauthorized, "invalid_token", "The tenant is not active")
			return
		}
		roles := []string{}
		if m := user.Membership(tenant.ID); m != nil {
			roles = m.Roles
		} else if !user.PlatformAdmin {
			fail(c, http.StatusForbidden, "forbidden", "The token owner is no longer a member of this tenant")
			return
		}
		perms := a.Svc.Permissions(ctx, tenant.ID, roles)
		if user.PlatformAdmin {
			perms = append(append([]string{}, models.TenantPermissions...), "platform:manage")
		}
		allowed := map[string]bool{}
		for _, p := range perms {
			allowed[p] = true
		}
		scopes := []string{}
		for _, s := range token.Scopes {
			if allowed[s] {
				scopes = append(scopes, s)
			}
		}
		if len(scopes) == 0 {
			fail(c, http.StatusForbidden, "no_scopes", "This token no longer has any valid permissions")
			return
		}
		auth := &MCPAuth{Token: token, User: user, Tenant: tenant, Roles: roles, Scopes: scopes}
		c.Request = c.Request.WithContext(context.WithValue(ctx, mcpAuthKey{}, auth))
		go a.Store.TouchAPIToken(context.Background(), token.ID)
		c.Next()
	}
}
