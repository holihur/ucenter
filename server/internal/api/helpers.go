package api

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/webauthn"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/billing"
	"ucenter/internal/config"
	"ucenter/internal/mail"
	"ucenter/internal/models"
	"ucenter/internal/observability"
	"ucenter/internal/ratelimit"
	"ucenter/internal/security"
	"ucenter/internal/service"
	"ucenter/internal/store"
)

const (
	ctxBundle      = "bundle"
	ctxTenant      = "tenant"
	ctxTenantID    = "tenantID"
	ctxPermissions = "permissions"
	ctxRoles       = "roles"
	ctxTokenAuth   = "tokenAuth"
	ctxTokenScopes = "tokenScopes"
)

type API struct {
	Store        *store.Store
	Svc          *service.Service
	Cfg          config.Config
	Mailer       mail.Mailer
	Cipher       *security.Cipher
	Metrics      *observability.Metrics
	WebAuthn     *webauthn.WebAuthn
	Billing      billing.Provider
	limiter      ratelimit.Limiter
	mailLimiter  ratelimit.Limiter
	tokenLimiter ratelimit.Limiter
}

func New(s *store.Store, svc *service.Service, cfg config.Config, cipher *security.Cipher, metrics *observability.Metrics) *API {
	limiter := ratelimit.Limiter(ratelimit.NewMemory(10, 5*time.Minute))
	mailLimiter := ratelimit.Limiter(ratelimit.NewMemory(5, 15*time.Minute))
	tokenLimiter := ratelimit.Limiter(ratelimit.NewMemory(120, time.Minute))
	if cfg.RedisURL != "" {
		client, err := ratelimit.Connect(context.Background(), cfg.RedisURL)
		if err != nil {
			slog.Warn("redis_unavailable_using_memory_rate_limits", "error", err)
		} else {
			limiter = ratelimit.NewRedis(client, 10, 5*time.Minute, cfg.RedisPrefix+"login:", slog.Default())
			mailLimiter = ratelimit.NewRedis(client, 5, 15*time.Minute, cfg.RedisPrefix+"mail:", slog.Default())
			tokenLimiter = ratelimit.NewRedis(client, 120, time.Minute, cfg.RedisPrefix+"token:", slog.Default())
		}
	}
	provider := billing.Provider(billing.MockProvider{})
	if cfg.BillingProvider == "stripe" && cfg.StripeSecretKey != "" {
		provider = billing.NewStripe(cfg.StripeSecretKey, cfg.StripeSuccessURL, cfg.StripeCancelURL)
	}
	wa, waErr := newWebAuthn(cfg)
	if waErr != nil {
		slog.Warn("webauthn_disabled", "error", waErr)
	}
	return &API{
		Store:        s,
		Svc:          svc,
		Cfg:          cfg,
		Mailer:       mail.New(cfg, s),
		Cipher:       cipher,
		Metrics:      metrics,
		WebAuthn:     wa,
		Billing:      provider,
		limiter:      limiter,
		mailLimiter:  mailLimiter,
		tokenLimiter: tokenLimiter,
	}
}

func fail(c *gin.Context, status int, code, message string) {
	if status >= 500 {
		log.Printf("request failed: %s %s -> %d %s: %s", c.Request.Method, c.Request.URL.Path, status, code, message)
	}
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, data)
}

func (a *API) setSessionCookie(c *gin.Context, raw string, expires time.Time) {
	ttl := int(time.Until(expires).Seconds())
	if ttl < 1 {
		ttl = 1
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(a.Cfg.CookieName, raw, ttl, "/", "", a.Cfg.CookieSecure, true)
}

func (a *API) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(a.Cfg.CookieName, "", -1, "/", "", a.Cfg.CookieSecure, true)
}

func (a *API) sessionCookie(c *gin.Context) string {
	v, err := c.Cookie(a.Cfg.CookieName)
	if err != nil {
		return ""
	}
	return v
}

func (a *API) currentBundle(c *gin.Context) *service.SessionBundle {
	v, exists := c.Get(ctxBundle)
	if !exists {
		return nil
	}
	b, _ := v.(*service.SessionBundle)
	return b
}

func (a *API) currentTenant(c *gin.Context) *models.Tenant {
	v, _ := c.Get(ctxTenant)
	t, _ := v.(*models.Tenant)
	return t
}

func (a *API) currentTenantID(c *gin.Context) primitive.ObjectID {
	v, _ := c.Get(ctxTenantID)
	id, _ := v.(primitive.ObjectID)
	return id
}

func (a *API) currentPermissions(c *gin.Context) []string {
	v, _ := c.Get(ctxPermissions)
	p, _ := v.([]string)
	return p
}

func (a *API) currentRoles(c *gin.Context) []string {
	v, _ := c.Get(ctxRoles)
	r, _ := v.([]string)
	return r
}

// RequireAdminAuth accepts either the admin session cookie or a tenant-scoped
// API bearer token (ucm_...). API tokens carry a subset of the owner's scopes
// and are restricted to the tenant they were created for.
func (a *API) RequireAdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if b, err := a.Svc.SessionFromRaw(ctx, a.sessionCookie(c)); err == nil {
			c.Set(ctxBundle, b)
			c.Next()
			return
		}
		raw := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
			fail(c, http.StatusUnauthorized, "unauthenticated", "Authentication required")
			return
		}
		tokenValue := strings.TrimSpace(raw[7:])
		token, err := a.Store.FindAPITokenByHash(ctx, security.HashToken(tokenValue))
		if err != nil {
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
		if user.Membership(tenant.ID) == nil && !user.PlatformAdmin {
			fail(c, http.StatusForbidden, "forbidden", "The token owner is no longer a member of this tenant")
			return
		}
		synthetic := &models.Session{
			ID: "api-token:" + token.Prefix, TenantID: tenant.ID, UserID: user.ID,
			AuthTime: token.CreatedAt, CreatedAt: token.CreatedAt,
		}
		c.Set(ctxBundle, &service.SessionBundle{Session: synthetic, User: user, Tenant: tenant})
		c.Set(ctxTokenAuth, true)
		c.Set(ctxTokenScopes, token.Scopes)
		go a.Store.TouchAPIToken(context.Background(), token.ID)
		c.Next()
	}
}

// RequireAuth loads the session cookie and rejects unauthenticated requests.
func (a *API) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		b, err := a.Svc.SessionFromRaw(c.Request.Context(), a.sessionCookie(c))
		if err != nil {
			fail(c, http.StatusUnauthorized, "unauthenticated", "Your session has expired. Please sign in again.")
			return
		}
		c.Set(ctxBundle, b)
		c.Next()
	}
}

// RequireAdmin resolves the active tenant and verifies the caller is a member
// (or a platform admin) before permission checks run.
func (a *API) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		b := a.currentBundle(c)
		if b == nil {
			fail(c, http.StatusUnauthorized, "unauthenticated", "Authentication required")
			return
		}
		tenant := b.Tenant
		tokenAuth, _ := c.Get(ctxTokenAuth)
		if b.User.PlatformAdmin && tokenAuth != true {
			if raw := strings.TrimSpace(c.GetHeader("X-Tenant-ID")); raw != "" {
				t, err := a.resolveTenant(c, raw)
				if err != nil {
					fail(c, http.StatusNotFound, "tenant_not_found", "Tenant not found")
					return
				}
				tenant = t
			}
		} else if b.User.Membership(tenant.ID) == nil {
			fail(c, http.StatusForbidden, "forbidden", "You do not have access to this tenant")
			return
		}

		roles := []string{}
		if m := b.User.Membership(tenant.ID); m != nil {
			roles = m.Roles
		}
		perms := a.Svc.Permissions(c.Request.Context(), tenant.ID, roles)
		if b.User.PlatformAdmin {
			perms = append(append([]string{}, models.TenantPermissions...), "platform:manage")
		}
		if tokenAuth, _ := c.Get(ctxTokenAuth); tokenAuth == true {
			scopes, _ := c.Get(ctxTokenScopes)
			allowed := map[string]bool{}
			for _, p := range perms {
				allowed[p] = true
			}
			filtered := []string{}
			for _, s := range scopes.([]string) {
				if allowed[s] {
					filtered = append(filtered, s)
				}
			}
			perms = filtered
		}
		c.Set(ctxTenant, tenant)
		c.Set(ctxTenantID, tenant.ID)
		c.Set(ctxRoles, roles)
		c.Set(ctxPermissions, perms)
		c.Next()
	}
}

func (a *API) RequirePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, p := range a.currentPermissions(c) {
			if p == perm {
				c.Next()
				return
			}
		}
		fail(c, http.StatusForbidden, "forbidden", "You do not have permission to perform this action")
	}
}

func (a *API) resolveTenant(c *gin.Context, raw string) (*models.Tenant, error) {
	if id, err := primitive.ObjectIDFromHex(raw); err == nil {
		return a.Store.FindTenantByID(c.Request.Context(), id)
	}
	return a.Store.FindTenantBySlug(c.Request.Context(), strings.ToLower(raw))
}

func (a *API) clientIP(c *gin.Context) string {
	return c.ClientIP()
}

func (a *API) audit(c *gin.Context, action, targetType, targetID string, metadata map[string]interface{}) {
	b := a.currentBundle(c)
	var actor *models.User
	if b != nil {
		actor = b.User
	}
	a.Svc.Audit(c.Request.Context(), a.currentTenantID(c), actor, action, targetType, targetID, a.clientIP(c), c.Request.UserAgent(), metadata)
}

func mapServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		fail(c, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
	case errors.Is(err, service.ErrMFARequired):
		fail(c, http.StatusUnauthorized, "mfa_required", "A verification code is required")
	case errors.Is(err, service.ErrInvalidMFA):
		fail(c, http.StatusUnauthorized, "invalid_mfa", "The verification code is invalid")
	case errors.Is(err, service.ErrAccountDisabled):
		fail(c, http.StatusForbidden, "account_disabled", "This account has been disabled")
	case errors.Is(err, service.ErrNoMembership):
		fail(c, http.StatusForbidden, "not_a_member", "This account does not have access to the requested organization")
	case errors.Is(err, service.ErrTenantDisabled):
		fail(c, http.StatusForbidden, "tenant_disabled", "This organization is disabled")
	case errors.Is(err, store.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, store.ErrDuplicate):
		fail(c, http.StatusConflict, "conflict", "A record with this value already exists")
	case errors.Is(err, service.ErrInvalidScope):
		fail(c, http.StatusBadRequest, "invalid_scope", err.Error())
	case errors.Is(err, service.ErrInvalidClient):
		fail(c, http.StatusUnauthorized, "invalid_client", "Invalid client credentials")
	default:
		fail(c, http.StatusBadRequest, "bad_request", err.Error())
	}
}

func publicTenant(t *models.Tenant) gin.H {
	return gin.H{
		"id":                 t.ID,
		"slug":               t.Slug,
		"name":               t.Name,
		"status":             t.Status,
		"branding":           t.Branding,
		"allowRegistration":  t.Settings.AllowRegistration,
		"allowPasswordLogin": t.Settings.AllowPasswordLogin,
		"requireMfa":         t.Settings.RequireMFA,
	}
}

var scopeDescriptions = map[string]string{
	"openid":         "Verify your identity",
	"profile":        "Read your name, username and profile picture",
	"email":          "Read your email address and verification status",
	"offline_access": "Stay signed in and refresh access without you present",
	"roles":          "Read your roles in this organization",
}

func describeScopes(scope string) []gin.H {
	out := []gin.H{}
	for _, s := range service.SplitScopes(scope) {
		desc, known := scopeDescriptions[s]
		if !known {
			desc = "Access " + s
		}
		out = append(out, gin.H{"key": s, "description": desc})
	}
	return out
}

func promptHas(prompt, want string) bool {
	for _, p := range strings.Fields(prompt) {
		if p == want {
			return true
		}
	}
	return false
}
