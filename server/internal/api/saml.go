package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/idp"
	"ucenter/internal/models"
	"ucenter/internal/security"
)

func (a *API) samlURLs(tenant *models.Tenant) (metadataURL, acsURL url.URL, entityID string) {
	base, _ := url.Parse(a.Cfg.BaseURL)
	metadataURL = *base
	metadataURL.Path = "/t/" + tenant.Slug + "/saml/metadata"
	acsURL = *base
	acsURL.Path = "/t/" + tenant.Slug + "/saml/acs"
	return metadataURL, acsURL, metadataURL.String()
}

func parseSAMLCert(keyPEM, certPEM string) (*rsa.PrivateKey, *x509.Certificate, error) {
	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("invalid SP key PEM")
	}
	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err == nil {
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, fmt.Errorf("SP key is not RSA")
		}
		key = rsaKey
	} else if parsed, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); err == nil {
		key = parsed
	} else {
		return nil, nil, fmt.Errorf("parse SP key: %w", err)
	}
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return nil, nil, fmt.Errorf("invalid SP certificate PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return key, cert, nil
}

func generateSAMLKeyPair(tenant *models.Tenant) (keyPEM, certPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: tenant.Slug + " UCenter SAML SP"},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})),
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), nil
}

func (a *API) samlServiceProvider(tenant *models.Tenant, cfg idp.Config) (*saml.ServiceProvider, error) {
	metaXML := strings.TrimSpace(cfg["idpMetadataXML"])
	if metaXML == "" {
		return nil, fmt.Errorf("IdP metadata XML is required")
	}
	metadata, err := samlsp.ParseMetadata([]byte(metaXML))
	if err != nil {
		return nil, fmt.Errorf("parse IdP metadata: %w", err)
	}
	key, cert, err := parseSAMLCert(cfg["spKey"], cfg["spCert"])
	if err != nil {
		return nil, err
	}
	metadataURL, acsURL, defaultEntityID := a.samlURLs(tenant)
	entityID := strings.TrimSpace(cfg["entityId"])
	if entityID == "" {
		entityID = defaultEntityID
	}
	nameIDFormat := saml.EmailAddressNameIDFormat
	if v := strings.TrimSpace(cfg["nameIDFormat"]); v != "" {
		nameIDFormat = saml.NameIDFormat(v)
	}
	sp := &saml.ServiceProvider{
		EntityID:              entityID,
		Key:                   key,
		Certificate:           cert,
		MetadataURL:           metadataURL,
		AcsURL:                acsURL,
		SLOURL:                url.URL{Scheme: metadataURL.Scheme, Host: metadataURL.Host, Path: "/t/" + tenant.Slug + "/saml/slo"},
		IDPMetadata:           metadata,
		AuthnNameIDFormat:     nameIDFormat,
		AllowIDPInitiated:     true,
		DefaultRedirectURI:    "/admin",
		MetadataValidDuration: 24 * time.Hour,
	}
	return sp, nil
}

func (a *API) saveSAMLProvider(c *gin.Context, tenant *models.Tenant, cfg idp.Config, body idpConfigRequest) {
	// Generate a stable SP key/certificate on first save.
	if cfg["spKey"] == "" || cfg["spCert"] == "" {
		keyPEM, certPEM, err := generateSAMLKeyPair(tenant)
		if err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "Unable to generate SAML signing material")
			return
		}
		cfg["spKey"] = keyPEM
		cfg["spCert"] = certPEM
	}
	if _, err := a.samlServiceProvider(tenant, cfg); err != nil {
		fail(c, http.StatusBadRequest, "invalid_saml_config", err.Error())
		return
	}
	raw, _ := json.Marshal(cfg)
	encrypted, err := a.Cipher.Encrypt(string(raw))
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to encrypt SAML configuration")
		return
	}
	roles := normalizeRoles(body.DefaultRoles)
	if len(body.DefaultRoles) == 0 {
		roles = []string{"member"}
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "SAML 2.0"
	}
	rec := &models.IdentityProvider{
		TenantID: tenant.ID, Type: "saml", Name: name, Enabled: body.Enabled,
		ConfigEnc: encrypted, AutoProvision: body.AutoProvision,
		DefaultRoles: roles, TrustEmail: body.TrustEmail,
	}
	if err := a.Store.UpsertIdentityProvider(c.Request.Context(), rec); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to save SAML provider")
		return
	}
	a.audit(c, "idp.updated", "identity_provider", "saml", map[string]interface{}{"enabled": body.Enabled})
	ok(c, gin.H{"ok": true})
}

func (a *API) samlMetadata(c *gin.Context) {
	tenant, err := a.Store.FindTenantBySlug(c.Request.Context(), strings.ToLower(c.Param("slug")))
	if err != nil || tenant.Status != models.StatusActive {
		fail(c, http.StatusNotFound, "tenant_not_found", "Organization not found")
		return
	}
	rec, err := a.Store.FindIdentityProvider(c.Request.Context(), tenant.ID, "saml")
	if err != nil {
		fail(c, http.StatusNotFound, "saml_not_configured", "SAML is not configured for this tenant")
		return
	}
	cfg, err := a.idpConfig(rec)
	if err != nil || strings.TrimSpace(cfg["idpMetadataXML"]) == "" {
		fail(c, http.StatusNotFound, "saml_not_configured", "SAML is not configured for this tenant")
		return
	}
	sp, err := a.samlServiceProvider(tenant, cfg)
	if err != nil {
		fail(c, http.StatusInternalServerError, "saml_error", err.Error())
		return
	}
	body, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		fail(c, http.StatusInternalServerError, "saml_error", "Unable to render metadata")
		return
	}
	c.Data(http.StatusOK, "application/samlmetadata+xml; charset=utf-8", body)
}

func (a *API) configuredSAML(ctx context.Context, tenantID primitive.ObjectID) (*models.IdentityProvider, idp.Config, error) {
	rec, err := a.Store.FindIdentityProvider(ctx, tenantID, "saml")
	if err != nil || !rec.Enabled {
		return nil, nil, fmt.Errorf("SAML is not enabled")
	}
	cfg, err := a.idpConfig(rec)
	if err != nil {
		return nil, nil, err
	}
	return rec, cfg, nil
}

func (a *API) startSAML(c *gin.Context, tenant *models.Tenant) {
	ctx := c.Request.Context()
	rec, cfg, err := a.configuredSAML(ctx, tenant.ID)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=unavailable")
		return
	}
	sp, err := a.samlServiceProvider(tenant, cfg)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=config")
		return
	}
	location := sp.GetSSOBindingLocation(saml.HTTPRedirectBinding)
	if location == "" {
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=config")
		return
	}
	req, err := sp.MakeAuthenticationRequest(location, saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error=config")
		return
	}
	rawState, err := security.RandomToken(32)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to start SAML sign-in")
		return
	}
	state := &models.IdPState{
		ID: security.HashToken(rawState), TenantID: tenant.ID, Provider: "saml",
		ReturnTo: safeReturnTo(c.Query("return_to")), RequestID: strings.TrimSpace(c.Query("request")),
		SAMLRequestID: req.ID, ExpiresAt: time.Now().Add(idpStateTTL), CreatedAt: time.Now(),
	}
	if c.Query("bind") == "1" {
		if b, err := a.Svc.SessionFromRaw(ctx, a.sessionCookie(c)); err == nil && b.Tenant.ID == tenant.ID {
			uid := b.User.ID
			state.BindUserID = &uid
		}
	}
	if err := a.Store.CreateIdPState(ctx, state); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to start SAML sign-in")
		return
	}
	redirectURL, err := req.Redirect(rawState, sp)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to build SAML request")
		return
	}
	_ = rec
	c.Redirect(http.StatusFound, redirectURL.String())
}

func (a *API) samlACS(c *gin.Context) {
	ctx := c.Request.Context()
	tenant, err := a.Store.FindTenantBySlug(ctx, strings.ToLower(c.Param("slug")))
	if err != nil || tenant.Status != models.StatusActive {
		fail(c, http.StatusNotFound, "tenant_not_found", "Organization not found")
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		c.String(http.StatusBadRequest, "invalid SAML response")
		return
	}
	relayState := c.Request.PostForm.Get("RelayState")
	state, err := a.Store.ConsumeIdPState(ctx, security.HashToken(relayState))
	if err != nil || state.TenantID != tenant.ID || state.Provider != "saml" {
		a.samlFailure(c, tenant, "state", nil)
		return
	}
	rec, cfg, err := a.configuredSAML(ctx, tenant.ID)
	if err != nil {
		a.samlFailure(c, tenant, "unavailable", nil)
		return
	}
	sp, err := a.samlServiceProvider(tenant, cfg)
	if err != nil {
		a.samlFailure(c, tenant, "config", nil)
		return
	}
	requestIDs := []string{state.SAMLRequestID}
	if state.SAMLRequestID == "" {
		requestIDs = []string{""}
	}
	assertion, err := sp.ParseResponse(c.Request, requestIDs)
	if err != nil {
		a.samlFailure(c, tenant, "signature", err)
		return
	}
	identity := samlIdentity(assertion, cfg, rec.TrustEmail)
	if identity.Subject == "" && identity.Email != "" {
		identity.Subject = identity.Email
	}
	if identity.Subject == "" {
		a.samlFailure(c, tenant, "identity", nil)
		return
	}
	if state.BindUserID != nil {
		user, err := a.Store.FindUserByID(ctx, *state.BindUserID)
		if err != nil || user.Membership(tenant.ID) == nil {
			a.samlFailure(c, tenant, "bind", nil)
			return
		}
		if err := a.linkIdPIdentity(ctx, tenant, rec, user, identity, "saml"); err != nil {
			c.Redirect(http.StatusFound, "/account?idp_error=linked")
			return
		}
		a.Svc.Audit(ctx, tenant.ID, user, "user.idp.linked", "identity_provider", "saml", a.clientIP(c), c.Request.UserAgent(), nil)
		c.Redirect(http.StatusFound, "/account?idp_linked=1")
		return
	}
	user, err := a.resolveIdPUser(ctx, tenant, rec, identity, "saml")
	if err != nil {
		a.samlFailure(c, tenant, err.Error(), nil)
		return
	}
	if user.MFA.Enabled {
		a.samlFailure(c, tenant, "mfa_required", nil)
		return
	}
	sess, raw, err := a.Svc.StartSession(ctx, tenant, user, false, a.clientIP(c), c.Request.UserAgent())
	if err != nil {
		a.samlFailure(c, tenant, "session", err)
		return
	}
	a.setSessionCookie(c, raw, sess.ExpiresAt)
	a.Svc.Audit(ctx, tenant.ID, user, "auth.login.success", "session", sess.ID, a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"provider": "saml"})
	target := safeReturnTo(state.ReturnTo)
	if state.RequestID != "" {
		target = "/consent?request=" + url.QueryEscape(state.RequestID)
	}
	c.Redirect(http.StatusFound, target)
}

func samlIdentity(assertion *saml.Assertion, cfg idp.Config, trustEmail bool) *idp.Identity {
	emailAttr := defaultString(cfg["emailAttribute"], "email")
	nameAttr := defaultString(cfg["nameAttribute"], "displayName")
	identity := &idp.Identity{EmailVerified: trustEmail, Raw: map[string]interface{}{}}
	if assertion.Subject != nil && assertion.Subject.NameID != nil {
		identity.Subject = assertion.Subject.NameID.Value
		if identity.Email == "" && strings.Contains(identity.Subject, "@") {
			identity.Email = identity.Subject
		}
	}
	attrs := map[string][]string{}
	for _, stmt := range assertion.AttributeStatements {
		for _, attr := range stmt.Attributes {
			values := []string{}
			for _, value := range attr.Values {
				values = append(values, value.Value)
			}
			attrs[attr.Name] = values
			if attr.FriendlyName != "" {
				attrs[attr.FriendlyName] = values
			}
		}
	}
	if v := firstValue(attrs, emailAttr); v != "" {
		identity.Email = v
	}
	identity.Name = firstValue(attrs, nameAttr)
	for k, v := range attrs {
		identity.Raw[k] = v
	}
	return identity
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstValue(attrs map[string][]string, key string) string {
	if values := attrs[key]; len(values) > 0 {
		return values[0]
	}
	return ""
}

func (a *API) samlFailure(c *gin.Context, tenant *models.Tenant, reason string, err error) {
	meta := map[string]interface{}{"error": reason}
	if err != nil {
		meta["detail"] = err.Error()
	}
	a.Svc.Audit(c.Request.Context(), tenant.ID, nil, "auth.idp.failed", "identity_provider", "saml", a.clientIP(c), c.Request.UserAgent(), meta)
	c.Redirect(http.StatusFound, "/login?tenant="+url.QueryEscape(tenant.Slug)+"&idp_error="+url.QueryEscape(reason))
}
