package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"ucenter/internal/models"
	"ucenter/internal/observability"
)

func (a *API) Router() *gin.Engine {
	r := gin.New()
	// Do not trust X-Forwarded-For unless explicit proxies are configured;
	// otherwise clients could spoof their IP in audit logs and rate limits.
	_ = r.SetTrustedProxies(a.Cfg.TrustedProxies)
	r.Use(gin.Recovery())
	r.Use(observability.RequestLogger(slog.Default()))
	if a.Metrics != nil {
		r.Use(a.Metrics.Middleware())
	}
	r.Use(a.securityHeaders())
	r.Use(a.cors())

	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := a.Store.Client.Ping(ctx, readpref.Primary()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "mongo": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready", "mongo": true})
	})
	if a.Metrics != nil {
		r.GET("/metrics", a.metricsAuth(), gin.WrapH(a.Metrics.Handler()))
	}

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
	})
	r.GET("/.well-known/openid-configuration", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "This identity provider is multi-tenant. Use /t/{tenant}/.well-known/openid-configuration.",
			"tenants": a.Cfg.BaseURL + "/api/public/tenants",
		})
	})

	public := r.Group("/api/public")
	public.GET("/config", a.publicConfig)
	public.GET("/tenant-by-host", a.tenantByHost)
	public.POST("/signup", a.selfSignup)
	public.GET("/tenants", a.listPublicTenants)
	public.GET("/tenants/:slug", a.getPublicTenant)
	public.GET("/tenants/:slug/identity-providers", a.publicIdentityProviders)
	public.GET("/permissions", a.listPermissions)

	auth := r.Group("/api/auth")
	auth.POST("/login", a.login)
	auth.POST("/register", a.register)
	auth.POST("/forgot-password", a.forgotPassword)
	auth.POST("/reset-password", a.resetPassword)
	auth.POST("/verify-email", a.verifyEmail)
	auth.POST("/mfa-recovery", a.requestMFARecovery)
	auth.POST("/mfa-recovery/confirm", a.confirmMFARecovery)
	auth.POST("/accept-invite", a.acceptInvite)
	auth.POST("/webauthn/begin", a.beginWebAuthnLogin)
	auth.POST("/webauthn/finish", a.finishWebAuthnLogin)
	auth.POST("/logout", a.logout)
	auth.GET("/request/:id", a.getAuthRequest)
	auth.GET("/me", a.RequireAuth(), a.me)
	auth.GET("/sessions", a.RequireAuth(), a.mySessions)
	auth.DELETE("/sessions/:id", a.RequireAuth(), a.revokeMySession)
	auth.POST("/request/:id/complete", a.RequireAuth(), a.completeAuthRequest)
	auth.POST("/request/:id/consent", a.RequireAuth(), a.consentAuthRequest)

	me := r.Group("/api/me", a.RequireAuth())
	me.PATCH("", a.updateProfile)
	me.POST("/password", a.changePassword)
	me.POST("/mfa/setup", a.setupMFA)
	me.POST("/mfa/enable", a.enableMFA)
	me.POST("/mfa/disable", a.disableMFA)
	me.POST("/mfa/recovery-codes", a.regenerateRecoveryCodes)
	me.GET("/identities", a.myIdentities)
	me.DELETE("/identities/:id", a.unlinkIdentity)
	me.POST("/webauthn/register/begin", a.beginWebAuthnRegistration)
	me.POST("/webauthn/register/finish", a.finishWebAuthnRegistration)
	me.GET("/webauthn/credentials", a.webauthnCredentials)
	me.DELETE("/webauthn/credentials/:id", a.deleteWebAuthnCredential)
	me.POST("/step-up/webauthn/begin", a.beginWebAuthnStepUp)
	me.POST("/step-up/webauthn/finish", a.finishWebAuthnStepUp)

	admin := r.Group("/api/admin", a.RequireAuth(), a.RequireAdmin())
	admin.GET("/context", a.adminContext)
	admin.GET("/overview", a.overview)
	admin.GET("/permissions", a.listPermissions)

	admin.GET("/users", a.RequirePermission("users:read"), a.listUsers)
	admin.POST("/users", a.RequirePermission("users:write"), a.createUser)
	admin.GET("/users/:id", a.RequirePermission("users:read"), a.getUser)
	admin.PATCH("/users/:id", a.RequirePermission("users:write"), a.updateUser)
	admin.DELETE("/users/:id", a.RequirePermission("users:write"), a.deleteUser)
	admin.POST("/users/:id/reset-password", a.RequirePermission("users:write"), a.resetUserPassword)
	admin.POST("/users/:id/revoke-sessions", a.RequirePermission("sessions:write"), a.revokeUserSessions)
	admin.POST("/users/:id/reset-mfa", a.RequirePermission("users:write"), a.resetUserMFA)
	admin.POST("/users/:id/send-invite", a.RequirePermission("users:write"), a.sendUserInvite)
	admin.GET("/mail-outbox", a.RequirePermission("tenant:write"), a.mailOutbox)
	admin.GET("/identity-providers", a.RequirePermission("tenant:read"), a.listIdentityProviders)
	admin.PUT("/identity-providers/:type", a.RequirePermission("tenant:write"), a.putIdentityProvider)
	admin.DELETE("/identity-providers/:type", a.RequirePermission("tenant:write"), a.deleteIdentityProvider)

	admin.GET("/roles", a.RequirePermission("roles:read"), a.listRoles)
	admin.POST("/roles", a.RequirePermission("roles:write"), a.createRole)
	admin.PATCH("/roles/:id", a.RequirePermission("roles:write"), a.updateRole)
	admin.DELETE("/roles/:id", a.RequirePermission("roles:write"), a.deleteRole)

	admin.GET("/clients", a.RequirePermission("clients:read"), a.listClients)
	admin.POST("/clients", a.RequirePermission("clients:write"), a.createClient)
	admin.PATCH("/clients/:id", a.RequirePermission("clients:write"), a.updateClient)
	admin.DELETE("/clients/:id", a.RequirePermission("clients:write"), a.deleteClient)
	admin.POST("/clients/:id/rotate-secret", a.RequirePermission("clients:write"), a.rotateClientSecret)

	admin.GET("/sessions", a.RequirePermission("sessions:read"), a.listSessions)
	admin.DELETE("/sessions/:id", a.RequirePermission("sessions:write"), a.revokeSession)

	admin.GET("/audit", a.RequirePermission("audit:read"), a.listAudit)
	admin.GET("/settings", a.RequirePermission("tenant:read"), a.getSettings)
	admin.PATCH("/settings", a.RequirePermission("tenant:write"), a.updateSettings)
	admin.POST("/signing-keys/rotate", a.RequirePermission("tenant:write"), a.rotateSigningKey)

	admin.GET("/scim", a.RequirePermission("tenant:read"), a.getSCIMSettings)
	admin.PATCH("/scim", a.RequirePermission("tenant:write"), a.updateSCIMSettings)
	admin.POST("/scim/token", a.RequirePermission("tenant:write"), a.rotateSCIMToken)

	admin.GET("/api-tokens", a.listAPITokens)
	admin.POST("/api-tokens", a.createAPIToken)
	admin.DELETE("/api-tokens/:id", a.revokeAPIToken)

	admin.GET("/export", a.RequirePermission("tenant:write"), a.exportTenant)

	admin.GET("/tenants", a.RequirePermission("platform:manage"), a.listTenants)
	admin.POST("/tenants", a.RequirePermission("platform:manage"), a.createTenant)
	admin.PATCH("/tenants/:id", a.RequirePermission("platform:manage"), a.updateTenant)
	admin.DELETE("/tenants/:id", a.RequirePermission("platform:manage"), a.softDeleteTenant)
	admin.POST("/tenants/:id/restore", a.RequirePermission("platform:manage"), a.restoreTenant)
	admin.POST("/tenants/:id/purge", a.RequirePermission("platform:manage"), a.purgeTenant)

	r.Any("/mcp", a.mcpAuth(), a.mcpHTTPHandler())

	scim := r.Group("/t/:slug/scim/v2", a.scimAuth())
	scim.GET("/ServiceProviderConfig", a.scimServiceProviderConfig)
	scim.GET("/Schemas", a.scimSchemas)
	scim.GET("/Users", a.scimListUsers)
	scim.POST("/Users", a.scimCreateUser)
	scim.GET("/Users/:id", a.scimGetUser)
	scim.PUT("/Users/:id", a.scimUpdateUser)
	scim.PATCH("/Users/:id", a.scimPatchUser)
	scim.DELETE("/Users/:id", a.scimDeleteUser)
	scim.GET("/Groups", a.scimListGroups)
	scim.POST("/Groups", a.scimCreateGroup)
	scim.GET("/Groups/:id", a.scimGetGroup)
	scim.PUT("/Groups/:id", a.scimUpdateGroup)
	scim.PATCH("/Groups/:id", a.scimPatchGroup)
	scim.DELETE("/Groups/:id", a.scimDeleteGroup)

	r.GET("/t/:slug/saml/metadata", a.samlMetadata)
	r.GET("/t/:slug/saml/login", func(c *gin.Context) {
		tenant, err := a.Store.FindTenantBySlug(c.Request.Context(), strings.ToLower(c.Param("slug")))
		if err != nil || tenant.Status != models.StatusActive {
			fail(c, http.StatusNotFound, "tenant_not_found", "Organization not found")
			return
		}
		a.startSAML(c, tenant)
	})
	r.POST("/t/:slug/saml/acs", a.samlACS)
	r.GET("/t/:slug/idp/:type/start", a.startIdentityProvider)
	r.GET("/t/:slug/idp/:type/callback", a.callbackIdentityProvider)

	oidc := r.Group("/t/:slug")
	oidc.GET("/.well-known/openid-configuration", a.discovery)
	oidc.GET("/oidc/jwks", a.jwks)
	oidc.GET("/oidc/authorize", a.authorize)
	oidc.POST("/oidc/token", a.token)
	oidc.GET("/oidc/userinfo", a.userinfo)
	oidc.POST("/oidc/userinfo", a.userinfo)
	oidc.POST("/oidc/revoke", a.revokeToken)
	oidc.POST("/oidc/introspect", a.introspect)
	oidc.GET("/oidc/end_session", a.endSession)
	oidc.POST("/oidc/end_session", a.endSession)
	oidc.GET("", a.tenantEntry)
	oidc.GET("/", a.tenantEntry)

	a.mountStatic(r)
	return r
}

func (a *API) metricsAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(a.Cfg.MetricsToken)
		if expected == "" {
			c.Next()
			return
		}
		provided := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(provided), "bearer ") {
			provided = strings.TrimSpace(provided[7:])
		}
		if provided == "" {
			provided = strings.TrimSpace(c.GetHeader("X-Metrics-Token"))
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			fail(c, http.StatusUnauthorized, "unauthorized", "Metrics token is required")
			return
		}
		c.Next()
	}
}

func (a *API) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Content-Security-Policy", "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
	}
}

func (a *API) cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && len(a.Cfg.AllowedOrigins) > 0 {
			allowed := false
			for _, o := range a.Cfg.AllowedOrigins {
				if strings.TrimSpace(o) == origin {
					allowed = true
					break
				}
			}
			if allowed {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Tenant-ID, X-Requested-With")
				c.Header("Access-Control-Max-Age", "600")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
