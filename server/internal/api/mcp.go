package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/store"
)

type mcpAuthKey struct{}

// MCPAuth is the tenant-scoped identity behind an MCP request.
type MCPAuth struct {
	Token  *models.APIToken
	User   *models.User
	Tenant *models.Tenant
	Roles  []string
	Scopes []string
}

func (a *MCPAuth) Has(scope string) bool {
	for _, s := range a.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func mcpAuthFromContext(ctx context.Context) *MCPAuth {
	v, _ := ctx.Value(mcpAuthKey{}).(*MCPAuth)
	return v
}

func (a *API) mcpHTTPHandler() gin.HandlerFunc {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return a.buildMCPServer(mcpAuthFromContext(r.Context()))
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return func(c *gin.Context) {
		handler.ServeHTTP(c.Writer, c.Request)
	}
}

func (a *API) mcpAudit(auth *MCPAuth, action, targetType, targetID string, metadata map[string]interface{}) {
	a.Svc.Audit(context.Background(), auth.Tenant.ID, auth.User, action, targetType, targetID, "mcp", "mcp", metadata)
}

func mcpErr(format string, args ...interface{}) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf(format, args...)
}

func (a *API) buildMCPServer(auth *MCPAuth) *mcp.Server {
	if auth == nil {
		return nil
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "ucenter", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: fmt.Sprintf("You are connected to UCenter tenant %q (%s). Use the tools to inspect or manage this tenant. Destructive actions require confirm=true. All operations are audited.", auth.Tenant.Name, auth.Tenant.Slug),
	})

	mcp.AddTool(s, &mcp.Tool{Name: "whoami", Description: "Show the authenticated user, tenant and granted tool scopes."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return nil, map[string]interface{}{
				"user":         map[string]interface{}{"id": auth.User.ID, "email": auth.User.Email, "name": auth.User.Name, "platformAdmin": auth.User.PlatformAdmin},
				"tenant":       map[string]interface{}{"id": auth.Tenant.ID, "slug": auth.Tenant.Slug, "name": auth.Tenant.Name},
				"roles":        auth.Roles,
				"scopes":       auth.Scopes,
				"issuer":       a.Svc.Issuer(auth.Tenant),
				"tokenPrefix":  auth.Token.Prefix,
				"tokenExpires": auth.Token.ExpiresAt,
			}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "get_tenant", Description: "Get the current tenant configuration, branding and security settings."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			if !auth.Has("tenant:read") {
				return mcpErr("missing scope tenant:read")
			}
			return nil, auth.Tenant, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "get_discovery", Description: "Return the OIDC discovery document and issuer for this tenant."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return nil, a.Svc.Discovery(auth.Tenant), nil
		})

	if auth.Has("users:read") {
		mcp.AddTool(s, &mcp.Tool{Name: "list_users", Description: "List tenant users with optional search, status filter and pagination."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
				Search   string `json:"search,omitempty" jsonschema:"search by email, name or username"`
				Status   string `json:"status,omitempty" jsonschema:"active, disabled or invited"`
				Page     int64  `json:"page,omitempty" jsonschema:"1-based page number"`
				PageSize int64  `json:"pageSize,omitempty" jsonschema:"number of users per page, maximum 100"`
			}) (*mcp.CallToolResult, any, error) {
				if in.Page < 1 {
					in.Page = 1
				}
				if in.PageSize < 1 || in.PageSize > 100 {
					in.PageSize = 20
				}
				users, total, err := a.Store.ListUsers(ctx, store.UserFilter{TenantID: auth.Tenant.ID, Search: in.Search, Status: in.Status, Page: in.Page, PageSize: in.PageSize})
				if err != nil {
					return mcpErr("list users: %v", err)
				}
				items := make([]gin.H, 0, len(users))
				for i := range users {
					items = append(items, adminUserView(&users[i], auth.Tenant.ID))
				}
				return nil, map[string]interface{}{"items": items, "total": total, "page": in.Page, "pageSize": in.PageSize}, nil
			})

		mcp.AddTool(s, &mcp.Tool{Name: "get_user", Description: "Get a tenant user by id or email."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
				ID    string `json:"id,omitempty" jsonschema:"user ObjectID"`
				Email string `json:"email,omitempty" jsonschema:"user email"`
			}) (*mcp.CallToolResult, any, error) {
				user, err := a.findMCPUser(ctx, auth, in.ID, in.Email)
				if err != nil {
					return mcpErr("%v", err)
				}
				return nil, adminUserView(user, auth.Tenant.ID), nil
			})
	}

	if auth.Has("users:write") {
		mcp.AddTool(s, &mcp.Tool{Name: "create_user", Description: "Create a user or add an existing identity to this tenant. Returns a one-time temporary password when no password is supplied."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
				Email    string   `json:"email" jsonschema:"user email address"`
				Name     string   `json:"name" jsonschema:"display name"`
				Username string   `json:"username,omitempty" jsonschema:"optional username"`
				Roles    []string `json:"roles,omitempty" jsonschema:"role keys, defaults to member"`
				Password string   `json:"password,omitempty" jsonschema:"optional initial password; generated when empty"`
			}) (*mcp.CallToolResult, any, error) {
				email := security.NormalizeEmail(in.Email)
				if !validEmail(email) {
					return mcpErr("invalid email address")
				}
				if strings.TrimSpace(in.Name) == "" {
					return mcpErr("name is required")
				}
				if err := a.checkUserQuotaFor(ctx, auth.Tenant); err != nil {
					return mcpErr("%v", err)
				}
				roles := normalizeRoles(in.Roles)
				if err := a.validateMCPRoles(ctx, auth, roles); err != nil {
					return mcpErr("%v", err)
				}
				if existing, err := a.Store.FindUserByEmail(ctx, email); err == nil {
					if existing.Membership(auth.Tenant.ID) != nil {
						return mcpErr("user is already a member of this tenant")
					}
					if err := a.Store.SetMembership(ctx, existing.ID, auth.Tenant.ID, roles); err != nil {
						return mcpErr("add membership: %v", err)
					}
					a.mcpAudit(auth, "user.membership.added", "user", existing.ID.Hex(), map[string]interface{}{"roles": roles, "via": "mcp"})
					return nil, map[string]interface{}{"user": adminUserView(existing, auth.Tenant.ID), "tempPassword": ""}, nil
				}
				password := in.Password
				temp := ""
				if password == "" {
					temp = generateTempPassword()
					password = temp
				}
				if err := validatePassword(auth.Tenant, password); err != nil {
					return mcpErr("%v", err)
				}
				hash, err := security.HashPassword(password)
				if err != nil {
					return mcpErr("hash password: %v", err)
				}
				username := strings.TrimSpace(in.Username)
				if username == "" {
					username = strings.Split(email, "@")[0]
				}
				user := &models.User{
					Email:         email,
					EmailVerified: true,
					Username:      username,
					Name:          strings.TrimSpace(in.Name),
					PasswordHash:  hash,
					Status:        models.StatusActive,
					Memberships:   []models.Membership{{TenantID: auth.Tenant.ID, Roles: roles, JoinedAt: time.Now()}},
				}
				if err := a.Store.CreateUser(ctx, user); err != nil {
					return mcpErr("create user: %v", err)
				}
				a.mcpAudit(auth, "user.created", "user", user.ID.Hex(), map[string]interface{}{"email": email, "roles": roles, "via": "mcp"})
				return nil, map[string]interface{}{"user": adminUserView(user, auth.Tenant.ID), "tempPassword": temp}, nil
			})

		mcp.AddTool(s, &mcp.Tool{Name: "update_user", Description: "Update a tenant user's name, username, status or roles."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
				ID       string   `json:"id" jsonschema:"user ObjectID"`
				Name     string   `json:"name,omitempty"`
				Username string   `json:"username,omitempty"`
				Status   string   `json:"status,omitempty" jsonschema:"active or disabled"`
				Roles    []string `json:"roles,omitempty"`
			}) (*mcp.CallToolResult, any, error) {
				id, ok := parseObjectID(in.ID)
				if !ok {
					return mcpErr("invalid user id")
				}
				target, err := a.Store.FindUserByID(ctx, id)
				if err != nil || target.Membership(auth.Tenant.ID) == nil {
					return mcpErr("user not found")
				}
				if target.HasRole(auth.Tenant.ID, "owner") && !auth.User.PlatformAdmin && !auth.User.HasRole(auth.Tenant.ID, "owner") {
					return mcpErr("only an owner can modify another owner")
				}
				update := bson.M{}
				if strings.TrimSpace(in.Name) != "" {
					update["name"] = strings.TrimSpace(in.Name)
				}
				if strings.TrimSpace(in.Username) != "" {
					update["username"] = strings.TrimSpace(in.Username)
				}
				if in.Status != "" {
					if in.Status != models.StatusActive && in.Status != models.StatusDisabled {
						return mcpErr("status must be active or disabled")
					}
					update["status"] = in.Status
				}
				if len(in.Roles) > 0 {
					roles := normalizeRoles(in.Roles)
					if err := a.validateMCPRoles(ctx, auth, roles); err != nil {
						return mcpErr("%v", err)
					}
					if target.HasRole(auth.Tenant.ID, "owner") && !contains(roles, "owner") {
						count, _ := a.Store.CountUsersWithRole(ctx, auth.Tenant.ID, "owner")
						if count <= 1 {
							return mcpErr("the organization must keep at least one owner")
						}
					}
					if err := a.Store.SetMembership(ctx, target.ID, auth.Tenant.ID, roles); err != nil {
						return mcpErr("update roles: %v", err)
					}
				}
				if len(update) > 0 {
					if err := a.Store.UpdateUser(ctx, target.ID, update); err != nil {
						return mcpErr("update user: %v", err)
					}
				}
				if in.Status == models.StatusDisabled || len(in.Roles) > 0 {
					_ = a.Store.RevokeAllUserSessions(ctx, auth.Tenant.ID, target.ID)
					_ = a.Store.RevokeUserRefreshTokens(ctx, auth.Tenant.ID, target.ID)
					_ = a.Store.BumpTokenVersion(ctx, target.ID)
				}
				updated, _ := a.Store.FindUserByID(ctx, target.ID)
				a.mcpAudit(auth, "user.updated", "user", target.ID.Hex(), map[string]interface{}{"via": "mcp"})
				return nil, adminUserView(updated, auth.Tenant.ID), nil
			})
	}

	if auth.Has("roles:read") {
		mcp.AddTool(s, &mcp.Tool{Name: "list_roles", Description: "List tenant roles and their permissions."},
			func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
				roles, err := a.Store.ListRoles(ctx, auth.Tenant.ID)
				if err != nil {
					return mcpErr("list roles: %v", err)
				}
				return nil, map[string]interface{}{"items": roles}, nil
			})
	}

	if auth.Has("roles:write") {
		mcp.AddTool(s, &mcp.Tool{Name: "create_role", Description: "Create a custom tenant role. Permissions must be a subset of your token scopes."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
				Key         string   `json:"key" jsonschema:"lowercase role key"`
				Name        string   `json:"name"`
				Description string   `json:"description,omitempty"`
				Permissions []string `json:"permissions,omitempty"`
			}) (*mcp.CallToolResult, any, error) {
				key := strings.ToLower(strings.TrimSpace(in.Key))
				if !keyRE.MatchString(key) {
					return mcpErr("role key must match ^[a-z][a-z0-9_-]{1,31}$")
				}
				perms := []string{}
				for _, p := range in.Permissions {
					if p == "platform:manage" || !models.PermissionSet[p] {
						return mcpErr("unknown permission: %s", p)
					}
					if !auth.Has(p) {
						return mcpErr("you cannot grant permission you do not have: %s", p)
					}
					perms = append(perms, p)
				}
				role := &models.Role{TenantID: auth.Tenant.ID, Key: key, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), Permissions: perms}
				if role.Name == "" {
					return mcpErr("role name is required")
				}
				if err := a.Store.CreateRole(ctx, role); err != nil {
					return mcpErr("create role: %v", err)
				}
				a.mcpAudit(auth, "role.created", "role", role.ID.Hex(), map[string]interface{}{"key": key, "via": "mcp"})
				return nil, role, nil
			})
	}

	if auth.Has("clients:read") {
		mcp.AddTool(s, &mcp.Tool{Name: "list_clients", Description: "List OIDC clients in this tenant."},
			func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
				clients, err := a.Store.ListClients(ctx, auth.Tenant.ID)
				if err != nil {
					return mcpErr("list clients: %v", err)
				}
				items := make([]gin.H, 0, len(clients))
				for i := range clients {
					items = append(items, clientView(&clients[i]))
				}
				return nil, map[string]interface{}{"items": items}, nil
			})
	}

	if auth.Has("clients:write") {
		mcp.AddTool(s, &mcp.Tool{Name: "create_client", Description: "Register an OIDC client. Returns a one-time client secret for confidential clients."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
				Name                   string   `json:"name"`
				Description            string   `json:"description,omitempty"`
				Type                   string   `json:"type,omitempty" jsonschema:"confidential or public; defaults to confidential"`
				RedirectURIs           []string `json:"redirectUris,omitempty"`
				PostLogoutRedirectURIs []string `json:"postLogoutRedirectUris,omitempty"`
				AllowedScopes          []string `json:"allowedScopes,omitempty"`
				GrantTypes             []string `json:"grantTypes,omitempty"`
				Trusted                bool     `json:"trusted,omitempty"`
				RequirePKCE            *bool    `json:"requirePkce,omitempty"`
			}) (*mcp.CallToolResult, any, error) {
				name := strings.TrimSpace(in.Name)
				if name == "" {
					return mcpErr("name is required")
				}
				if err := a.checkClientQuotaFor(ctx, auth.Tenant); err != nil {
					return mcpErr("%v", err)
				}
				typ := in.Type
				if typ == "" {
					typ = models.ClientTypeConfidential
				}
				if typ != models.ClientTypeConfidential && typ != models.ClientTypePublic {
					return mcpErr("type must be confidential or public")
				}
				for _, u := range append(append([]string{}, in.RedirectURIs...), in.PostLogoutRedirectURIs...) {
					if err := validateRedirectURI(u); err != nil {
						return mcpErr("%v", err)
					}
				}
				scopes := in.AllowedScopes
				if len(scopes) == 0 {
					scopes = []string{"openid", "profile", "email"}
				}
				hasOpenID := false
				for _, s := range scopes {
					if !scopeRE.MatchString(s) {
						return mcpErr("invalid scope: %s", s)
					}
					if s == "openid" {
						hasOpenID = true
					}
				}
				if !hasOpenID {
					return mcpErr("the openid scope is required")
				}
				grants := in.GrantTypes
				if len(grants) == 0 {
					grants = []string{"authorization_code", "refresh_token"}
				}
				for _, g := range grants {
					if g != "authorization_code" && g != "refresh_token" && g != "client_credentials" {
						return mcpErr("unsupported grant type: %s", g)
					}
				}
				if contains(grants, "client_credentials") && typ == models.ClientTypePublic {
					return mcpErr("client_credentials requires a confidential client")
				}
				if contains(grants, "authorization_code") && len(in.RedirectURIs) == 0 {
					return mcpErr("at least one redirect URI is required for the authorization code flow")
				}
				requirePKCE := typ == models.ClientTypePublic
				if in.RequirePKCE != nil {
					requirePKCE = *in.RequirePKCE || typ == models.ClientTypePublic
				}
				suffix, err := security.RandomHex(12)
				if err != nil {
					return mcpErr("generate client id: %v", err)
				}
				client := &models.Client{
					TenantID:               auth.Tenant.ID,
					ClientID:               "app_" + suffix,
					Name:                   name,
					Description:            strings.TrimSpace(in.Description),
					Type:                   typ,
					RedirectURIs:           in.RedirectURIs,
					PostLogoutRedirectURIs: in.PostLogoutRedirectURIs,
					AllowedScopes:          scopes,
					GrantTypes:             grants,
					RequirePKCE:            requirePKCE,
					Trusted:                in.Trusted,
					AccessTokenTTL:         3600,
					IDTokenTTL:             3600,
					RefreshTokenTTL:        2592000,
					Enabled:                true,
				}
				secret := ""
				if typ == models.ClientTypeConfidential {
					secret, err = security.GenerateClientSecret()
					if err != nil {
						return mcpErr("generate secret: %v", err)
					}
					client.ClientSecretHash = security.HashClientSecret(secret)
					encrypted, encErr := a.Cipher.Encrypt(secret)
					if encErr != nil {
						return mcpErr("encrypt secret: %v", encErr)
					}
					client.ClientSecretEnc = encrypted
				}
				if err := a.Store.CreateClient(ctx, client); err != nil {
					return mcpErr("create client: %v", err)
				}
				a.mcpAudit(auth, "client.created", "client", client.ClientID, map[string]interface{}{"name": client.Name, "via": "mcp"})
				return nil, map[string]interface{}{"client": clientView(client), "clientSecret": secret}, nil
			})

		mcp.AddTool(s, &mcp.Tool{Name: "rotate_client_secret", Description: "Rotate a confidential OIDC client secret. The old secret stops working immediately."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
				ClientID string `json:"clientId" jsonschema:"OIDC client_id"`
			}) (*mcp.CallToolResult, any, error) {
				client, err := a.Store.FindClientByClientID(ctx, auth.Tenant.ID, in.ClientID)
				if err != nil {
					return mcpErr("client not found")
				}
				if client.Type != models.ClientTypeConfidential {
					return mcpErr("public clients do not use a client secret")
				}
				secret, err := security.GenerateClientSecret()
				if err != nil {
					return mcpErr("generate secret: %v", err)
				}
				encrypted, encErr := a.Cipher.Encrypt(secret)
				if encErr != nil {
					return mcpErr("encrypt secret: %v", encErr)
				}
				if err := a.Store.UpdateClient(ctx, auth.Tenant.ID, client.ID, bson.M{"client_secret_hash": security.HashClientSecret(secret), "client_secret_enc": encrypted}); err != nil {
					return mcpErr("rotate secret: %v", err)
				}
				a.mcpAudit(auth, "client.secret.rotated", "client", client.ClientID, map[string]interface{}{"via": "mcp"})
				return nil, map[string]interface{}{"clientId": client.ClientID, "clientSecret": secret}, nil
			})
	}

	if auth.Has("sessions:read") {
		mcp.AddTool(s, &mcp.Tool{Name: "list_sessions", Description: "List active tenant sessions."},
			func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
				sessions, err := a.Store.ListSessions(ctx, auth.Tenant.ID, 200)
				if err != nil {
					return mcpErr("list sessions: %v", err)
				}
				ids := make([]primitive.ObjectID, 0, len(sessions))
				for _, s := range sessions {
					ids = append(ids, s.UserID)
				}
				users, _ := a.Store.FindUsersByIDs(ctx, ids)
				items := make([]gin.H, 0, len(sessions))
				for _, s := range sessions {
					u := users[s.UserID]
					items = append(items, gin.H{"id": s.ID, "userEmail": u.Email, "userName": u.Name, "ip": s.IP, "userAgent": s.UserAgent, "mfa": s.MFA, "createdAt": s.CreatedAt, "expiresAt": s.ExpiresAt})
				}
				return nil, map[string]interface{}{"items": items}, nil
			})
	}

	if auth.Has("sessions:write") {
		mcp.AddTool(s, &mcp.Tool{Name: "revoke_session", Description: "Revoke a session by its session id. Requires confirm=true."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
				SessionID string `json:"sessionId" jsonschema:"session id returned by list_sessions"`
				Confirm   bool   `json:"confirm" jsonschema:"must be true to revoke"`
			}) (*mcp.CallToolResult, any, error) {
				if !in.Confirm {
					return mcpErr("confirm must be true")
				}
				sess, err := a.Store.FindSession(ctx, in.SessionID)
				if err != nil || sess.TenantID != auth.Tenant.ID {
					return mcpErr("session not found")
				}
				if err := a.Store.RevokeSession(ctx, in.SessionID); err != nil {
					return mcpErr("revoke session: %v", err)
				}
				a.mcpAudit(auth, "session.revoked", "session", in.SessionID, map[string]interface{}{"via": "mcp"})
				return nil, map[string]interface{}{"ok": true}, nil
			})

		mcp.AddTool(s, &mcp.Tool{Name: "revoke_user_sessions", Description: "Revoke all sessions for a user. Requires confirm=true."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
				UserID  string `json:"userId,omitempty"`
				Email   string `json:"email,omitempty"`
				Confirm bool   `json:"confirm"`
			}) (*mcp.CallToolResult, any, error) {
				if !in.Confirm {
					return mcpErr("confirm must be true")
				}
				user, err := a.findMCPUser(ctx, auth, in.UserID, in.Email)
				if err != nil {
					return mcpErr("%v", err)
				}
				_ = a.Store.RevokeAllUserSessions(ctx, auth.Tenant.ID, user.ID)
				_ = a.Store.RevokeUserRefreshTokens(ctx, auth.Tenant.ID, user.ID)
				_ = a.Store.BumpTokenVersion(ctx, user.ID)
				a.mcpAudit(auth, "sessions.revoked", "user", user.ID.Hex(), map[string]interface{}{"via": "mcp"})
				return nil, map[string]interface{}{"ok": true, "userId": user.ID}, nil
			})
	}

	if auth.Has("audit:read") {
		mcp.AddTool(s, &mcp.Tool{Name: "list_audit", Description: "List audit events with optional search, action filter and pagination."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
				Search   string `json:"search,omitempty"`
				Action   string `json:"action,omitempty"`
				Page     int64  `json:"page,omitempty"`
				PageSize int64  `json:"pageSize,omitempty"`
			}) (*mcp.CallToolResult, any, error) {
				if in.Page < 1 {
					in.Page = 1
				}
				if in.PageSize < 1 || in.PageSize > 200 {
					in.PageSize = 30
				}
				logs, total, err := a.Store.ListAudit(ctx, store.AuditFilter{TenantID: auth.Tenant.ID, Search: in.Search, Action: in.Action, Page: in.Page, PageSize: in.PageSize})
				if err != nil {
					return mcpErr("list audit: %v", err)
				}
				return nil, map[string]interface{}{"items": logs, "total": total, "page": in.Page, "pageSize": in.PageSize}, nil
			})
	}

	if auth.Has("platform:manage") {
		mcp.AddTool(s, &mcp.Tool{Name: "list_tenants", Description: "List all tenants on the platform with user and client counts."},
			func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
				tenants, err := a.Store.ListTenants(ctx)
				if err != nil {
					return mcpErr("list tenants: %v", err)
				}
				items := make([]gin.H, 0, len(tenants))
				for i := range tenants {
					t := &tenants[i]
					users, _ := a.Store.CountUsersInTenant(ctx, t.ID, "")
					clients, _ := a.Store.CountClients(ctx, t.ID)
					items = append(items, gin.H{"id": t.ID, "slug": t.Slug, "name": t.Name, "status": t.Status, "users": users, "clients": clients, "createdAt": t.CreatedAt})
				}
				return nil, map[string]interface{}{"items": items}, nil
			})
	}

	return s
}

func (a *API) findMCPUser(ctx context.Context, auth *MCPAuth, id, email string) (*models.User, error) {
	var user *models.User
	var err error
	if strings.TrimSpace(id) != "" {
		oid, ok := parseObjectID(id)
		if !ok {
			return nil, fmt.Errorf("invalid user id")
		}
		user, err = a.Store.FindUserByID(ctx, oid)
	} else if strings.TrimSpace(email) != "" {
		user, err = a.Store.FindUserByEmail(ctx, security.NormalizeEmail(email))
	} else {
		return nil, fmt.Errorf("id or email is required")
	}
	if err != nil || user.Membership(auth.Tenant.ID) == nil {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

func (a *API) validateMCPRoles(ctx context.Context, auth *MCPAuth, roles []string) error {
	docs, err := a.Store.RolesByKeys(ctx, auth.Tenant.ID, roles)
	if err != nil {
		return fmt.Errorf("validate roles: %w", err)
	}
	if len(docs) != len(roles) {
		return fmt.Errorf("one or more roles do not exist")
	}
	isOwner := auth.User.PlatformAdmin || auth.User.HasRole(auth.Tenant.ID, "owner")
	for _, role := range docs {
		if role.Key == "owner" && !isOwner {
			return fmt.Errorf("only an owner can grant the owner role")
		}
		if !isOwner {
			for _, p := range role.Permissions {
				if !auth.Has(p) {
					return fmt.Errorf("you cannot grant the %s role because it includes permissions you do not have", role.Name)
				}
			}
		}
	}
	return nil
}
