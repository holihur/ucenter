package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/service"
	"ucenter/internal/store"
)

func (a *API) listPublicTenants(c *gin.Context) {
	tenants, err := a.Store.ListTenants(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load organizations")
		return
	}
	out := []gin.H{}
	for i := range tenants {
		if tenants[i].Status != models.StatusActive {
			continue
		}
		out = append(out, publicTenant(&tenants[i]))
	}
	ok(c, gin.H{"items": out})
}

func (a *API) getPublicTenant(c *gin.Context) {
	t, err := a.Store.FindTenantBySlug(c.Request.Context(), strings.ToLower(c.Param("slug")))
	if err != nil || t.Status != models.StatusActive {
		fail(c, http.StatusNotFound, "tenant_not_found", "Organization not found")
		return
	}
	ok(c, publicTenant(t))
}

func (a *API) listPermissions(c *gin.Context) {
	ok(c, gin.H{"items": models.AllPermissions})
}

type loginRequest struct {
	Tenant    string `json:"tenant"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	TOTP      string `json:"totp"`
	RequestID string `json:"requestId"`
}

func (a *API) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "A tenant, email and password are required")
		return
	}
	req.Email = security.NormalizeEmail(req.Email)
	req.Tenant = strings.ToLower(strings.TrimSpace(req.Tenant))
	if req.Email == "" || req.Password == "" || req.Tenant == "" {
		fail(c, http.StatusBadRequest, "invalid_body", "A tenant, email and password are required")
		return
	}
	key := a.clientIP(c) + "|" + req.Tenant + "|" + req.Email
	if ok, _ := a.limiter.Allow(c.Request.Context(), key); !ok {
		a.Metrics.Login("rate_limited")
		fail(c, http.StatusTooManyRequests, "rate_limited", "Too many sign-in attempts. Please wait a few minutes and try again.")
		return
	}

	ctx := c.Request.Context()
	tenant, err := a.Store.FindTenantBySlug(ctx, req.Tenant)
	if err != nil {
		fail(c, http.StatusNotFound, "tenant_not_found", "Organization not found")
		return
	}
	if !tenant.Settings.AllowPasswordLogin {
		fail(c, http.StatusForbidden, "password_login_disabled", "Password sign-in is disabled for this organization")
		return
	}

	user, err := a.Svc.LoginPassword(ctx, tenant, req.Email, req.Password)
	if err != nil {
		a.Metrics.Login("failure")
		a.Svc.Audit(ctx, tenant.ID, nil, "auth.login.failed", "user", req.Email, a.clientIP(c), c.Request.UserAgent(),
			map[string]interface{}{"reason": err.Error()})
		mapServiceError(c, err)
		return
	}

	webauthnCount, _ := a.Store.CountWebAuthnCredentials(ctx, tenant.ID, user.ID)
	mfaMethods := []string{}
	if user.MFA.Enabled {
		mfaMethods = append(mfaMethods, "totp")
	}
	if webauthnCount > 0 {
		mfaMethods = append(mfaMethods, "webauthn")
	}
	mfaUsed := false
	if len(mfaMethods) > 0 {
		if strings.TrimSpace(req.TOTP) != "" && user.MFA.Enabled {
			remaining, valid := a.Svc.VerifyMFA(user, req.TOTP)
			if !valid {
				a.Svc.Audit(ctx, tenant.ID, user, "auth.mfa.failed", "user", user.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
				fail(c, http.StatusUnauthorized, "invalid_mfa", "The verification code is invalid")
				return
			}
			mfaUsed = true
			if len(remaining) != len(user.MFA.RecoveryCodes) {
				_ = a.Store.UpdateUser(ctx, user.ID, bson.M{"mfa.recovery_codes": remaining})
				user.MFA.RecoveryCodes = remaining
			}
		} else {
			pending, err := a.createMFAPending(ctx, tenant, user, req.RequestID)
			if err != nil {
				fail(c, http.StatusInternalServerError, "internal_error", "Unable to start MFA")
				return
			}
			a.Metrics.Login("mfa_required")
			c.JSON(http.StatusOK, gin.H{
				"mfaRequired": true,
				"mfaToken":    pending,
				"mfaMethods":  mfaMethods,
				"tenant":      publicTenant(tenant),
			})
			return
		}
	}

	sess, raw, err := a.Svc.StartSession(ctx, tenant, user, mfaUsed, a.clientIP(c), c.Request.UserAgent())
	if err != nil {
		log.Printf("login session error for %s: %v", user.Email, err)
		fail(c, http.StatusInternalServerError, "session_error", "Unable to create a session")
		return
	}
	a.setSessionCookie(c, raw, sess.ExpiresAt)
	_ = a.limiter.Reset(ctx, key)

	var authReq *models.AuthRequest
	var client *models.Client
	if req.RequestID != "" {
		authReq, client, err = a.attachRequest(c, tenant, user, req.RequestID)
		if err != nil {
			fail(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}

	a.Metrics.Login("success")
	a.Svc.Audit(ctx, tenant.ID, user, "auth.login.success", "session", sess.ID, a.clientIP(c), c.Request.UserAgent(),
		map[string]interface{}{"mfa": mfaUsed})

	next := ""
	if authReq != nil {
		next, _ = a.nextStep(c, tenant, user, authReq, client)
	}
	c.JSON(http.StatusOK, gin.H{
		"mfaRequired":      false,
		"mfaSetupRequired": tenant.Settings.RequireMFA && !user.MFA.Enabled && webauthnCount == 0,
		"user":             user,
		"tenant":           publicTenant(tenant),
		"next":             next,
		"requestId":        req.RequestID,
	})
}

type registerRequest struct {
	Tenant    string `json:"tenant"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Name      string `json:"name"`
	RequestID string `json:"requestId"`
}

func (a *API) register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "All fields are required")
		return
	}
	req.Email = security.NormalizeEmail(req.Email)
	req.Tenant = strings.ToLower(strings.TrimSpace(req.Tenant))
	req.Name = strings.TrimSpace(req.Name)
	if req.Email == "" || req.Password == "" || req.Name == "" || req.Tenant == "" {
		fail(c, http.StatusBadRequest, "invalid_body", "All fields are required")
		return
	}
	ctx := c.Request.Context()
	tenant, err := a.Store.FindTenantBySlug(ctx, req.Tenant)
	if err != nil {
		fail(c, http.StatusNotFound, "tenant_not_found", "Organization not found")
		return
	}
	if tenant.Status != models.StatusActive {
		fail(c, http.StatusForbidden, "tenant_disabled", "This organization is disabled")
		return
	}
	if !tenant.Settings.AllowRegistration {
		fail(c, http.StatusForbidden, "registration_disabled", "Self-registration is disabled for this organization")
		return
	}
	if !emailAllowed(tenant, req.Email) {
		fail(c, http.StatusBadRequest, "email_domain_not_allowed", "This email domain is not allowed for this organization")
		return
	}
	if err := validatePassword(tenant, req.Password); err != nil {
		fail(c, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	if _, err := a.Store.FindUserByEmail(ctx, req.Email); err == nil {
		fail(c, http.StatusConflict, "email_in_use", "An account with this email already exists. Try signing in instead.")
		return
	}
	hash, err := security.HashPassword(req.Password)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	}
	now := time.Now()
	user := &models.User{
		Email:        req.Email,
		Username:     strings.Split(req.Email, "@")[0],
		Name:         req.Name,
		PasswordHash: hash,
		Status:       models.StatusActive,
		Memberships: []models.Membership{{
			TenantID: tenant.ID,
			Roles:    []string{"member"},
			JoinedAt: now,
		}},
	}
	if err := a.Store.CreateUser(ctx, user); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			fail(c, http.StatusConflict, "email_in_use", "An account with this email already exists")
			return
		}
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	}
	sess, raw, err := a.Svc.StartSession(ctx, tenant, user, false, a.clientIP(c), c.Request.UserAgent())
	if err != nil {
		fail(c, http.StatusInternalServerError, "session_error", "Unable to create a session")
		return
	}
	a.setSessionCookie(c, raw, sess.ExpiresAt)
	a.sendVerificationEmail(ctx, tenant, user)
	a.Svc.Audit(ctx, tenant.ID, user, "user.registered", "user", user.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)

	var authReq *models.AuthRequest
	var client *models.Client
	if req.RequestID != "" {
		authReq, client, err = a.attachRequest(c, tenant, user, req.RequestID)
		if err != nil {
			fail(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	next := ""
	if authReq != nil {
		next, _ = a.nextStep(c, tenant, user, authReq, client)
	}
	c.JSON(http.StatusCreated, gin.H{
		"user":      user,
		"tenant":    publicTenant(tenant),
		"next":      next,
		"requestId": req.RequestID,
	})
}

func (a *API) logout(c *gin.Context) {
	raw := a.sessionCookie(c)
	if raw != "" {
		if b, err := a.Svc.SessionFromRaw(c.Request.Context(), raw); err == nil {
			a.Svc.Audit(c.Request.Context(), b.Tenant.ID, b.User, "auth.logout", "session", b.Session.ID,
				a.clientIP(c), c.Request.UserAgent(), nil)
		}
	}
	a.Svc.EndSession(c.Request.Context(), raw)
	a.clearSessionCookie(c)
	ok(c, gin.H{"ok": true})
}

func (a *API) me(c *gin.Context) {
	b := a.currentBundle(c)
	a.respondMe(c, b)
}

func (a *API) adminContext(c *gin.Context) {
	b := a.currentBundle(c)
	tenant := a.currentTenant(c)
	roles := a.currentRoles(c)
	perms := a.currentPermissions(c)
	out := a.mePayload(c, b)
	out["tenant"] = publicTenant(tenant)
	out["roles"] = roles
	out["permissions"] = perms
	ok(c, out)
}

func (a *API) respondMe(c *gin.Context, b *service.SessionBundle) {
	ok(c, a.mePayload(c, b))
}

func (a *API) mePayload(c *gin.Context, b *service.SessionBundle) gin.H {
	ctx := c.Request.Context()
	roles := []string{}
	if m := b.User.Membership(b.Tenant.ID); m != nil {
		roles = m.Roles
	}
	perms := a.Svc.Permissions(ctx, b.Tenant.ID, roles)
	if b.User.PlatformAdmin {
		perms = append(append([]string{}, models.TenantPermissions...), "platform:manage")
	}
	memberships := []gin.H{}
	for _, m := range b.User.Memberships {
		t, err := a.Store.FindTenantByID(ctx, m.TenantID)
		if err != nil {
			continue
		}
		memberships = append(memberships, gin.H{
			"tenant": publicTenant(t),
			"roles":  m.Roles,
		})
	}
	return gin.H{
		"user":             b.User,
		"tenant":           publicTenant(b.Tenant),
		"roles":            roles,
		"permissions":      perms,
		"memberships":      memberships,
		"isPlatformAdmin":  b.User.PlatformAdmin,
		"mfaSetupRequired": b.Tenant.Settings.RequireMFA && !b.User.MFA.Enabled,
	}
}

func (a *API) mySessions(c *gin.Context) {
	b := a.currentBundle(c)
	sessions, err := a.Store.ListSessions(c.Request.Context(), b.Tenant.ID, 100)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load sessions")
		return
	}
	out := []gin.H{}
	for _, s := range sessions {
		if s.UserID != b.User.ID {
			continue
		}
		out = append(out, gin.H{
			"id":        s.ID,
			"ip":        s.IP,
			"userAgent": s.UserAgent,
			"createdAt": s.CreatedAt,
			"expiresAt": s.ExpiresAt,
			"mfa":       s.MFA,
			"current":   s.ID == b.Session.ID,
		})
	}
	ok(c, gin.H{"items": out})
}

func (a *API) revokeMySession(c *gin.Context) {
	b := a.currentBundle(c)
	id := c.Param("id")
	sessions, err := a.Store.ListSessions(c.Request.Context(), b.Tenant.ID, 500)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to revoke session")
		return
	}
	found := false
	for _, s := range sessions {
		if s.ID == id && s.UserID == b.User.ID {
			found = true
			break
		}
	}
	if !found {
		fail(c, http.StatusNotFound, "not_found", "Session not found")
		return
	}
	if err := a.Store.RevokeSession(c.Request.Context(), id); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to revoke session")
		return
	}
	if id == b.Session.ID {
		a.clearSessionCookie(c)
	}
	a.Svc.Audit(c.Request.Context(), b.Tenant.ID, b.User, "session.revoked", "session", id, a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) attachRequest(c *gin.Context, tenant *models.Tenant, user *models.User, requestID string) (*models.AuthRequest, *models.Client, error) {
	ctx := c.Request.Context()
	req, err := a.Store.FindAuthRequest(ctx, requestID)
	if err != nil {
		return nil, nil, errors.New("authorization request not found or expired")
	}
	if req.TenantID != tenant.ID {
		return nil, nil, errors.New("authorization request belongs to a different organization")
	}
	if time.Now().After(req.ExpiresAt) {
		return nil, nil, errors.New("authorization request has expired")
	}
	client, err := a.Store.FindClientByClientID(ctx, tenant.ID, req.ClientID)
	if err != nil || !client.Enabled {
		return nil, nil, errors.New("the requesting application is not available")
	}
	if req.Status == models.RequestCompleted {
		return nil, nil, errors.New("authorization request has already been completed")
	}
	if err := a.Store.UpdateAuthRequest(ctx, req.ID, bson.M{
		"status": models.RequestAuthenticated, "user_id": user.ID,
	}); err != nil {
		return nil, nil, err
	}
	req.Status = models.RequestAuthenticated
	req.UserID = &user.ID
	return req, client, nil
}

func (a *API) nextStep(c *gin.Context, tenant *models.Tenant, user *models.User, req *models.AuthRequest, client *models.Client) (string, error) {
	if client.Trusted {
		return "complete", nil
	}
	consent, err := a.Store.FindConsent(c.Request.Context(), tenant.ID, user.ID, client.ClientID)
	if err != nil {
		return "consent", nil
	}
	if service.ScopeSubset(service.SplitScopes(req.Scope), consent.Scopes) {
		return "complete", nil
	}
	return "consent", nil
}

func (a *API) getAuthRequest(c *gin.Context) {
	ctx := c.Request.Context()
	req, err := a.Store.FindAuthRequest(ctx, c.Param("id"))
	if err != nil {
		fail(c, http.StatusNotFound, "request_not_found", "This sign-in request is invalid or has expired")
		return
	}
	tenant, err := a.Store.FindTenantByID(ctx, req.TenantID)
	if err != nil || tenant.Status != models.StatusActive {
		fail(c, http.StatusBadRequest, "tenant_unavailable", "The organization is unavailable")
		return
	}
	client, err := a.Store.FindClientByClientID(ctx, tenant.ID, req.ClientID)
	if err != nil || !client.Enabled {
		fail(c, http.StatusBadRequest, "client_unavailable", "The requesting application is unavailable")
		return
	}
	status := req.Status
	if time.Now().After(req.ExpiresAt) && status != models.RequestCompleted {
		status = models.RequestExpired
	}
	var sessionUser *models.User
	b, err := a.Svc.SessionFromRaw(ctx, a.sessionCookie(c))
	if err == nil && b.Tenant.ID == tenant.ID && !promptHas(req.Prompt, "login") && status != models.RequestExpired && status != models.RequestCompleted {
		sessionUser = b.User
		if req.Status == models.RequestPending {
			_ = a.Store.UpdateAuthRequest(ctx, req.ID, bson.M{"status": models.RequestAuthenticated, "user_id": b.User.ID})
			req.UserID = &b.User.ID
		}
		status = models.RequestAuthenticated
	}
	payload := gin.H{
		"id":        req.ID,
		"status":    status,
		"prompt":    req.Prompt,
		"loginHint": req.LoginHint,
		"expiresAt": req.ExpiresAt,
		"tenant":    publicTenant(tenant),
		"client": gin.H{
			"clientId": client.ClientID,
			"name":     client.Name,
			"logoUrl":  client.LogoURL,
			"trusted":  client.Trusted,
		},
		"scopes": describeScopes(req.Scope),
	}
	if sessionUser != nil {
		payload["user"] = sessionUser
	}
	ok(c, payload)
}

func (a *API) completeAuthRequest(c *gin.Context) {
	b := a.currentBundle(c)
	ctx := c.Request.Context()
	req, err := a.Store.FindAuthRequest(ctx, c.Param("id"))
	if err != nil {
		fail(c, http.StatusNotFound, "request_not_found", "This sign-in request is invalid or has expired")
		return
	}
	if req.TenantID != b.Tenant.ID {
		fail(c, http.StatusForbidden, "wrong_tenant", "This request belongs to a different organization")
		return
	}
	if time.Now().After(req.ExpiresAt) {
		fail(c, http.StatusBadRequest, "request_expired", "This sign-in request has expired")
		return
	}
	if promptHas(req.Prompt, "login") && b.Session.AuthTime.Before(req.CreatedAt) {
		fail(c, http.StatusUnauthorized, "login_required", "Please sign in again to continue")
		return
	}
	if req.UserID != nil && *req.UserID != b.User.ID {
		fail(c, http.StatusForbidden, "wrong_user", "This request belongs to a different user")
		return
	}
	if req.Status == models.RequestPending {
		_ = a.Store.UpdateAuthRequest(ctx, req.ID, bson.M{"status": models.RequestAuthenticated, "user_id": b.User.ID})
		req.Status = models.RequestAuthenticated
		req.UserID = &b.User.ID
	}
	client, err := a.Store.FindClientByClientID(ctx, b.Tenant.ID, req.ClientID)
	if err != nil || !client.Enabled {
		fail(c, http.StatusBadRequest, "client_unavailable", "The requesting application is unavailable")
		return
	}
	redirectURL, err := a.Svc.CompleteAuthRequest(ctx, req, b.Tenant, b.User, b.Session, false)
	if errors.Is(err, service.ErrConsentRequired) {
		ok(c, gin.H{
			"next": "consent",
			"client": gin.H{
				"clientId": client.ClientID,
				"name":     client.Name,
				"logoUrl":  client.LogoURL,
			},
			"scopes": describeScopes(req.Scope),
		})
		return
	}
	if err != nil {
		fail(c, http.StatusBadRequest, "authorize_failed", err.Error())
		return
	}
	ok(c, gin.H{"next": "redirect", "redirectUrl": redirectURL})
}

func (a *API) consentAuthRequest(c *gin.Context) {
	b := a.currentBundle(c)
	ctx := c.Request.Context()
	var body struct {
		Approve bool `json:"approve"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "An approve decision is required")
		return
	}
	req, err := a.Store.FindAuthRequest(ctx, c.Param("id"))
	if err != nil {
		fail(c, http.StatusNotFound, "request_not_found", "This sign-in request is invalid or has expired")
		return
	}
	if req.TenantID != b.Tenant.ID || (req.UserID != nil && *req.UserID != b.User.ID) {
		fail(c, http.StatusForbidden, "wrong_request", "This request belongs to a different session")
		return
	}
	if time.Now().After(req.ExpiresAt) {
		fail(c, http.StatusBadRequest, "request_expired", "This sign-in request has expired")
		return
	}
	if !body.Approve {
		_ = a.Store.UpdateAuthRequest(ctx, req.ID, bson.M{"status": models.RequestDenied})
		a.Svc.Audit(ctx, b.Tenant.ID, b.User, "oidc.authorize.denied", "client", req.ClientID, a.clientIP(c), c.Request.UserAgent(), nil)
		ok(c, gin.H{"next": "redirect", "redirectUrl": service.DenyAuthRequest(ctx, req)})
		return
	}
	redirectURL, err := a.Svc.CompleteAuthRequest(ctx, req, b.Tenant, b.User, b.Session, true)
	if err != nil {
		fail(c, http.StatusBadRequest, "authorize_failed", err.Error())
		return
	}
	ok(c, gin.H{"next": "redirect", "redirectUrl": redirectURL})
}

func (a *API) tenantEntry(c *gin.Context) {
	c.Redirect(http.StatusFound, "/login?tenant="+c.Param("slug"))
}

func emailAllowed(t *models.Tenant, email string) bool {
	if len(t.Settings.AllowedEmailDomains) == 0 {
		return true
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(parts[1])
	for _, d := range t.Settings.AllowedEmailDomains {
		if strings.ToLower(strings.TrimSpace(d)) == domain {
			return true
		}
	}
	return false
}

func validatePassword(t *models.Tenant, password string) error {
	min := t.Settings.PasswordMinLength
	if min < 8 {
		min = 8
	}
	if len(password) < min {
		return errors.New("Password must be at least " + itoa(min) + " characters")
	}
	if len(password) > 200 {
		return errors.New("Password must be at most 200 characters")
	}
	return nil
}

func itoa(n int) string { return strconv.Itoa(n) }

func parseObjectID(raw string) (primitive.ObjectID, bool) {
	id, err := primitive.ObjectIDFromHex(raw)
	return id, err == nil
}
