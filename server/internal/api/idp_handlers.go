package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/idp"
	"ucenter/internal/models"
	"ucenter/internal/security"
)

const idpStateTTL = 10 * time.Minute

func (a *API) idpConfig(p *models.IdentityProvider) (idp.Config, error) {
	if p.ConfigEnc == "" {
		return idp.Config{}, nil
	}
	raw, err := a.Cipher.Decrypt(p.ConfigEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypt provider config: %w", err)
	}
	cfg := idp.Config{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("decode provider config: %w", err)
	}
	return cfg, nil
}

func (a *API) configuredIdP(ctx context.Context, tenantID primitive.ObjectID, typ string) (*models.IdentityProvider, idp.Config, error) {
	rec, err := a.Store.FindIdentityProvider(ctx, tenantID, typ)
	if err != nil {
		return nil, nil, err
	}
	if !rec.Enabled {
		return nil, nil, fmt.Errorf("provider is disabled")
	}
	cfg, err := a.idpConfig(rec)
	if err != nil {
		return nil, nil, err
	}
	if _, err := idp.New(typ, cfg); err != nil {
		return nil, nil, err
	}
	return rec, cfg, nil
}

func (a *API) listIdentityProviders(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := a.currentTenantID(c)
	stored, _ := a.Store.ListIdentityProviders(ctx, tenantID)
	byType := map[string]*models.IdentityProvider{}
	for i := range stored {
		byType[stored[i].Type] = &stored[i]
	}
	items := make([]gin.H, 0, len(idp.Catalog))
	for _, def := range idp.Catalog {
		rec := byType[def.Type]
		item := gin.H{
			"type": def.Type, "name": def.Name, "category": def.Category, "mode": def.Mode,
			"description": def.Description, "fields": def.Fields,
			"enabled": false, "configured": false, "autoProvision": false,
			"defaultRoles": []string{"member"}, "trustEmail": false, "config": gin.H{},
		}
		if rec != nil {
			cfg, _ := a.idpConfig(rec)
			masked := gin.H{}
			for _, field := range def.Fields {
				value := cfg[field.Key]
				if field.Secret && value != "" {
					value = "••••••••"
				}
				masked[field.Key] = value
			}
			item["name"] = rec.Name
			item["enabled"] = rec.Enabled
			item["configured"] = len(cfg) > 0
			item["autoProvision"] = rec.AutoProvision
			item["defaultRoles"] = rec.DefaultRoles
			item["trustEmail"] = rec.TrustEmail
			item["config"] = masked
		}
		items = append(items, item)
	}
	ok(c, gin.H{"items": items})
}

type idpConfigRequest struct {
	Name          string            `json:"name"`
	Enabled       bool              `json:"enabled"`
	Config        map[string]string `json:"config"`
	AutoProvision bool              `json:"autoProvision"`
	DefaultRoles  []string          `json:"defaultRoles"`
	TrustEmail    bool              `json:"trustEmail"`
}

func (a *API) putIdentityProvider(c *gin.Context) {
	typ := strings.ToLower(strings.TrimSpace(c.Param("type")))
	def, found := idp.DefinitionFor(typ)
	if !found {
		fail(c, http.StatusNotFound, "not_found", "Unknown identity provider")
		return
	}
	var body idpConfigRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid provider configuration")
		return
	}
	ctx := c.Request.Context()
	tenantID := a.currentTenantID(c)
	if typ == "saml" {
		tenant, err := a.Store.FindTenantByID(ctx, tenantID)
		if err != nil {
			fail(c, http.StatusNotFound, "not_found", "Tenant not found")
			return
		}
		existing, err := a.Store.FindIdentityProvider(ctx, tenantID, typ)
		existingCfg := idp.Config{}
		if err == nil {
			existingCfg, _ = a.idpConfig(existing)
		}
		merged := idp.Config{}
		for k, v := range existingCfg {
			merged[k] = v
		}
		for k, v := range body.Config {
			v = strings.TrimSpace(v)
			if v == "••••••••" || v == "********" {
				continue
			}
			merged[k] = v
		}
		a.saveSAMLProvider(c, tenant, merged, body)
		return
	}
	existing, err := a.Store.FindIdentityProvider(ctx, tenantID, typ)
	existingCfg := idp.Config{}
	if err == nil {
		existingCfg, _ = a.idpConfig(existing)
	}
	merged := idp.Config{}
	for k, v := range existingCfg {
		merged[k] = v
	}
	for k, v := range body.Config {
		v = strings.TrimSpace(v)
		if v == "••••••••" || v == "********" {
			continue
		}
		merged[k] = v
	}
	if _, err := idp.New(typ, merged); err != nil {
		fail(c, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	for _, field := range def.Fields {
		if field.Required && merged[field.Key] == "" {
			fail(c, http.StatusBadRequest, "missing_field", fmt.Sprintf("%s is required", field.Label))
			return
		}
	}
	raw, _ := json.Marshal(merged)
	encrypted, err := a.Cipher.Encrypt(string(raw))
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to encrypt provider configuration")
		return
	}
	roles := normalizeRoles(body.DefaultRoles)
	if len(body.DefaultRoles) == 0 {
		roles = []string{"member"}
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = def.Name
	}
	rec := &models.IdentityProvider{
		TenantID: tenantID, Type: typ, Name: name, Enabled: body.Enabled,
		ConfigEnc: encrypted, AutoProvision: body.AutoProvision,
		DefaultRoles: roles, TrustEmail: body.TrustEmail,
	}
	if err := a.Store.UpsertIdentityProvider(ctx, rec); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to save identity provider")
		return
	}
	a.audit(c, "idp.updated", "identity_provider", typ, map[string]interface{}{"enabled": body.Enabled})
	ok(c, gin.H{"ok": true})
}

func (a *API) deleteIdentityProvider(c *gin.Context) {
	typ := strings.ToLower(strings.TrimSpace(c.Param("type")))
	if err := a.Store.DeleteIdentityProvider(c.Request.Context(), a.currentTenantID(c), typ); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to remove identity provider")
		return
	}
	a.audit(c, "idp.deleted", "identity_provider", typ, nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) publicIdentityProviders(c *gin.Context) {
	tenant, err := a.Store.FindTenantBySlug(c.Request.Context(), strings.ToLower(c.Param("slug")))
	if err != nil || tenant.Status != models.StatusActive {
		fail(c, http.StatusNotFound, "tenant_not_found", "Organization not found")
		return
	}
	stored, _ := a.Store.ListIdentityProviders(c.Request.Context(), tenant.ID)
	items := []gin.H{}
	for i := range stored {
		rec := &stored[i]
		if !rec.Enabled {
			continue
		}
		cfg := mustConfig(a, rec)
		if rec.Type == "saml" {
			if strings.TrimSpace(cfg["idpMetadataXML"]) == "" {
				continue
			}
		} else if _, err := idp.New(rec.Type, cfg); err != nil {
			continue
		}
		def, _ := idp.DefinitionFor(rec.Type)
		items = append(items, gin.H{"type": rec.Type, "name": rec.Name, "mode": def.Mode})
	}
	ok(c, gin.H{"items": items})
}

func mustConfig(a *API, rec *models.IdentityProvider) idp.Config {
	cfg, _ := a.idpConfig(rec)
	return cfg
}

func (a *API) startIdentityProvider(c *gin.Context) {
	ctx := c.Request.Context()
	tenant, err := a.Store.FindTenantBySlug(ctx, strings.ToLower(c.Param("slug")))
	if err != nil || tenant.Status != models.StatusActive {
		fail(c, http.StatusNotFound, "tenant_not_found", "Organization not found")
		return
	}
	typ := strings.ToLower(strings.TrimSpace(c.Param("type")))
	if typ == "saml" {
		a.startSAML(c, tenant)
		return
	}
	rec, cfg, err := a.configuredIdP(ctx, tenant.ID, typ)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=unavailable")
		return
	}
	provider, err := idp.New(typ, cfg)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=config")
		return
	}
	rawState, err := security.RandomToken(32)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to start sign-in")
		return
	}
	state := &models.IdPState{
		ID:        security.HashToken(rawState),
		TenantID:  tenant.ID,
		Provider:  typ,
		ReturnTo:  safeReturnTo(c.Query("return_to")),
		RequestID: strings.TrimSpace(c.Query("request")),
		ExpiresAt: time.Now().Add(idpStateTTL),
		CreatedAt: time.Now(),
	}
	if c.Query("bind") == "1" {
		if b, err := a.Svc.SessionFromRaw(ctx, a.sessionCookie(c)); err == nil && b.Tenant.ID == tenant.ID {
			uid := b.User.ID
			state.BindUserID = &uid
		}
	}
	if err := a.Store.CreateIdPState(ctx, state); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to start sign-in")
		return
	}
	redirectURI := fmt.Sprintf("%s/t/%s/idp/%s/callback", a.Cfg.BaseURL, tenant.Slug, typ)
	c.Redirect(http.StatusFound, provider.AuthURL(rawState, redirectURI))
	_ = rec
}

func (a *API) callbackIdentityProvider(c *gin.Context) {
	ctx := c.Request.Context()
	tenant, err := a.Store.FindTenantBySlug(ctx, strings.ToLower(c.Param("slug")))
	if err != nil || tenant.Status != models.StatusActive {
		fail(c, http.StatusNotFound, "tenant_not_found", "Organization not found")
		return
	}
	typ := strings.ToLower(strings.TrimSpace(c.Param("type")))
	state, err := a.Store.ConsumeIdPState(ctx, security.HashToken(c.Query("state")))
	if err != nil || state.TenantID != tenant.ID || state.Provider != typ {
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=state")
		return
	}
	rec, cfg, err := a.configuredIdP(ctx, tenant.ID, typ)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=unavailable")
		return
	}
	provider, err := idp.New(typ, cfg)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=config")
		return
	}
	redirectURI := fmt.Sprintf("%s/t/%s/idp/%s/callback", a.Cfg.BaseURL, tenant.Slug, typ)
	identity, err := provider.Exchange(ctx, c.Query("code"), redirectURI)
	if err != nil {
		a.Svc.Audit(ctx, tenant.ID, nil, "auth.idp.failed", "identity_provider", typ, a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"error": err.Error()})
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=exchange")
		return
	}
	if state.BindUserID != nil {
		user, err := a.Store.FindUserByID(ctx, *state.BindUserID)
		if err != nil || user.Membership(tenant.ID) == nil {
			c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=bind")
			return
		}
		if err := a.linkIdPIdentity(ctx, tenant, rec, user, identity, typ); err != nil {
			c.Redirect(http.StatusFound, "/account?idp_error=linked")
			return
		}
		a.Svc.Audit(ctx, tenant.ID, user, "user.idp.linked", "identity_provider", typ, a.clientIP(c), c.Request.UserAgent(), nil)
		c.Redirect(http.StatusFound, "/account?idp_linked=1")
		return
	}
	user, err := a.resolveIdPUser(ctx, tenant, rec, identity, typ)
	if err != nil {
		a.Svc.Audit(ctx, tenant.ID, nil, "auth.idp.denied", "identity_provider", typ, a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"error": err.Error()})
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error="+url.QueryEscape(err.Error()))
		return
	}
	if user.MFA.Enabled {
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=mfa_required")
		return
	}
	sess, raw, err := a.Svc.StartSession(ctx, tenant, user, false, a.clientIP(c), c.Request.UserAgent())
	if err != nil {
		fail(c, http.StatusInternalServerError, "session_error", "Unable to create a session")
		return
	}
	a.setSessionCookie(c, raw, sess.ExpiresAt)
	a.Svc.Audit(ctx, tenant.ID, user, "auth.login.success", "session", sess.ID, a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"provider": typ})
	target := safeReturnTo(state.ReturnTo)
	if state.RequestID != "" {
		target = "/consent?request=" + url.QueryEscape(state.RequestID)
	}
	c.Redirect(http.StatusFound, target)
}

func (a *API) resolveIdPUser(ctx context.Context, tenant *models.Tenant, rec *models.IdentityProvider, identity *idp.Identity, typ string) (*models.User, error) {
	link, err := a.Store.FindUserIdentity(ctx, tenant.ID, typ, identity.Subject)
	if err != nil && identity.UnionID != "" {
		link, err = a.Store.FindUserIdentityByUnionID(ctx, tenant.ID, typ, identity.UnionID)
	}
	if err == nil {
		user, err := a.Store.FindUserByID(ctx, link.UserID)
		if err == nil && user.Status == models.StatusActive && user.Membership(tenant.ID) != nil {
			a.Store.TouchUserIdentity(ctx, link.ID)
			return user, nil
		}
		return nil, fmt.Errorf("linked_user_unavailable")
	}
	if identity.Email != "" {
		if existing, err := a.Store.FindUserByEmail(ctx, security.NormalizeEmail(identity.Email)); err == nil && existing.Membership(tenant.ID) != nil {
			if !rec.TrustEmail {
				return nil, fmt.Errorf("email_exists_use_password")
			}
			if err := a.linkIdPIdentity(ctx, tenant, rec, existing, identity, typ); err != nil {
				return nil, fmt.Errorf("link_failed")
			}
			return existing, nil
		}
	}
	if !rec.AutoProvision {
		return nil, fmt.Errorf("account_not_linked")
	}
	if err := a.checkUserQuotaFor(ctx, tenant); err != nil {
		return nil, fmt.Errorf("quota_exceeded")
	}
	email := security.NormalizeEmail(identity.Email)
	emailVerified := rec.TrustEmail && identity.EmailVerified
	if email == "" {
		hash := security.HashToken(typ + "|" + identity.Subject)
		email = fmt.Sprintf("idp_%s_%s@%s.idp.local", typ, hash[:12], tenant.Slug)
		emailVerified = false
	}
	name := identity.Name
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	randomPassword, _ := security.RandomToken(32)
	hash, _ := security.HashPassword(randomPassword)
	roles := normalizeRoles(rec.DefaultRoles)
	user := &models.User{
		Email:         email,
		EmailVerified: emailVerified,
		Username:      strings.Split(email, "@")[0],
		Name:          name,
		Picture:       identity.Avatar,
		PasswordHash:  hash,
		Status:        models.StatusActive,
		Memberships:   []models.Membership{{TenantID: tenant.ID, Roles: roles, JoinedAt: time.Now()}},
	}
	if err := a.Store.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("create_user_failed")
	}
	if err := a.linkIdPIdentity(ctx, tenant, rec, user, identity, typ); err != nil {
		return nil, fmt.Errorf("link_failed")
	}
	a.Svc.Audit(ctx, tenant.ID, user, "user.idp.provisioned", "user", user.ID.Hex(), "", "", map[string]interface{}{"provider": typ})
	return user, nil
}

func (a *API) linkIdPIdentity(ctx context.Context, tenant *models.Tenant, rec *models.IdentityProvider, user *models.User, identity *idp.Identity, typ string) error {
	link := &models.UserIdentity{
		TenantID: tenant.ID, UserID: user.ID, Provider: typ,
		Subject: identity.Subject, UnionID: identity.UnionID,
		Email: identity.Email, Name: identity.Name, Avatar: identity.Avatar,
	}
	if err := a.Store.LinkUserIdentity(ctx, link); err != nil {
		return err
	}
	return nil
}

func (a *API) myIdentities(c *gin.Context) {
	b := a.currentBundle(c)
	links, err := a.Store.ListUserIdentities(c.Request.Context(), b.Tenant.ID, b.User.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load linked accounts")
		return
	}
	items := []gin.H{}
	for _, link := range links {
		def, _ := idp.DefinitionFor(link.Provider)
		items = append(items, gin.H{"id": link.ID, "provider": link.Provider, "providerName": def.Name, "subject": link.Subject, "email": link.Email, "name": link.Name, "createdAt": link.CreatedAt, "lastLoginAt": link.LastLoginAt})
	}
	ok(c, gin.H{"items": items})
}

func (a *API) unlinkIdentity(c *gin.Context) {
	b := a.currentBundle(c)
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Linked account not found")
		return
	}
	if err := a.Store.DeleteUserIdentity(c.Request.Context(), b.Tenant.ID, b.User.ID, id); err != nil {
		fail(c, http.StatusNotFound, "not_found", "Linked account not found")
		return
	}
	a.Svc.Audit(c.Request.Context(), b.Tenant.ID, b.User, "user.idp.unlinked", "user", b.User.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"ok": true})
}

func safeReturnTo(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.HasPrefix(value, "/login") {
		return "/admin"
	}
	return value
}
