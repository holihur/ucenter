package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/models"
	"ucenter/internal/security"
)

const webauthnSessionTTL = 5 * time.Minute

type webauthnUser struct {
	user        *models.User
	credentials []models.WebAuthnCredential
}

func (u *webauthnUser) WebAuthnID() []byte   { id := u.user.ID; return id[:] }
func (u *webauthnUser) WebAuthnName() string { return u.user.Email }
func (u *webauthnUser) WebAuthnDisplayName() string {
	if u.user.Name != "" {
		return u.user.Name
	}
	return u.user.Email
}

func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(u.credentials))
	for _, c := range u.credentials {
		transports := make([]protocol.AuthenticatorTransport, 0, len(c.Transports))
		for _, t := range c.Transports {
			transports = append(transports, protocol.AuthenticatorTransport(t))
		}
		out = append(out, webauthn.Credential{
			ID: c.CredentialID, PublicKey: c.PublicKey, AttestationType: c.AttestationType,
			Transport: transports,
			Flags: webauthn.CredentialFlags{
				UserPresent: true, UserVerified: true,
				BackupEligible: c.BackupEligible, BackupState: c.BackupState,
			},
			Authenticator: webauthn.Authenticator{AAGUID: c.AAGUID, SignCount: c.SignCount},
		})
	}
	return out
}

func (a *API) loadWebAuthnUser(ctx context.Context, tenantID, userID primitive.ObjectID) (*webauthnUser, error) {
	user, err := a.Store.FindUserByID(ctx, userID)
	if err != nil || user.Status != models.StatusActive || user.Membership(tenantID) == nil {
		return nil, fmt.Errorf("user is not active in this tenant")
	}
	creds, err := a.Store.ListWebAuthnCredentials(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	return &webauthnUser{user: user, credentials: creds}, nil
}

func (a *API) createMFAPending(ctx context.Context, tenant *models.Tenant, user *models.User, requestID string) (string, error) {
	raw, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	raw = "mfa_" + raw
	pending := &models.MFAPending{
		ID: security.HashToken(raw), TenantID: tenant.ID, UserID: user.ID,
		RequestID: requestID, ExpiresAt: time.Now().Add(webauthnSessionTTL), CreatedAt: time.Now(),
	}
	if err := a.Store.CreateMFAPending(ctx, pending); err != nil {
		return "", err
	}
	return raw, nil
}

func (a *API) createWebAuthnSession(ctx context.Context, tenantID, userID primitive.ObjectID, kind string, data webauthn.SessionData, requestID string) (string, error) {
	raw, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	st := &models.WebAuthnSession{
		ID: security.HashToken(raw), TenantID: tenantID, UserID: userID, Kind: kind,
		Data: string(encoded), RequestID: requestID, ExpiresAt: time.Now().Add(webauthnSessionTTL), CreatedAt: time.Now(),
	}
	if err := a.Store.CreateWebAuthnSession(ctx, st); err != nil {
		return "", err
	}
	return raw, nil
}

func (a *API) beginWebAuthnRegistration(c *gin.Context) {
	if a.WebAuthn == nil {
		fail(c, http.StatusBadRequest, "webauthn_disabled", "Passkey support is not configured")
		return
	}
	b := a.currentBundle(c)
	ctx := c.Request.Context()
	waUser, err := a.loadWebAuthnUser(ctx, b.Tenant.ID, b.User.ID)
	if err != nil {
		fail(c, http.StatusBadRequest, "webauthn_error", err.Error())
		return
	}
	creation, session, err := a.WebAuthn.BeginRegistration(waUser)
	if err != nil {
		fail(c, http.StatusBadRequest, "webauthn_error", err.Error())
		return
	}
	raw, err := a.createWebAuthnSession(ctx, b.Tenant.ID, b.User.ID, "register", *session, "")
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to start passkey registration")
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessionId": raw, "options": creation})
}

func (a *API) finishWebAuthnRegistration(c *gin.Context) {
	if a.WebAuthn == nil {
		fail(c, http.StatusBadRequest, "webauthn_disabled", "Passkey support is not configured")
		return
	}
	b := a.currentBundle(c)
	var body struct {
		SessionID  string          `json:"sessionId"`
		Name       string          `json:"name"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil || body.SessionID == "" || len(body.Credential) == 0 {
		fail(c, http.StatusBadRequest, "invalid_body", "A WebAuthn credential is required")
		return
	}
	ctx := c.Request.Context()
	session, err := a.Store.ConsumeWebAuthnSession(ctx, security.HashToken(body.SessionID))
	if err != nil || session.Kind != "register" || session.TenantID != b.Tenant.ID || session.UserID != b.User.ID {
		fail(c, http.StatusBadRequest, "webauthn_session", "The registration session is invalid or expired")
		return
	}
	waUser, err := a.loadWebAuthnUser(ctx, b.Tenant.ID, b.User.ID)
	if err != nil {
		fail(c, http.StatusBadRequest, "webauthn_error", err.Error())
		return
	}
	var data webauthn.SessionData
	if err := json.Unmarshal([]byte(session.Data), &data); err != nil {
		fail(c, http.StatusBadRequest, "webauthn_session", "The registration session is invalid")
		return
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewReader(body.Credential))
	req.Header.Set("Content-Type", "application/json")
	credential, err := a.WebAuthn.FinishRegistration(waUser, data, req)
	if err != nil {
		fail(c, http.StatusBadRequest, "webauthn_error", err.Error())
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "Passkey"
	}
	transports := make([]string, 0, len(credential.Transport))
	for _, t := range credential.Transport {
		transports = append(transports, string(t))
	}
	stored := &models.WebAuthnCredential{
		TenantID: b.Tenant.ID, UserID: b.User.ID,
		CredentialID: credential.ID, PublicKey: credential.PublicKey,
		AAGUID: credential.Authenticator.AAGUID, SignCount: credential.Authenticator.SignCount,
		AttestationType: credential.AttestationType, Transports: transports, Name: name,
		BackupEligible: credential.Flags.BackupEligible, BackupState: credential.Flags.BackupState,
	}
	if err := a.Store.CreateWebAuthnCredential(ctx, stored); err != nil {
		fail(c, http.StatusConflict, "credential_exists", "This passkey is already registered")
		return
	}
	a.Svc.Audit(ctx, b.Tenant.ID, b.User, "user.webauthn.registered", "user", b.User.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"name": name})
	c.JSON(http.StatusCreated, gin.H{"ok": true, "credential": webauthnCredentialView(stored)})
}

func (a *API) webauthnCredentials(c *gin.Context) {
	b := a.currentBundle(c)
	creds, err := a.Store.ListWebAuthnCredentials(c.Request.Context(), b.Tenant.ID, b.User.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load passkeys")
		return
	}
	items := make([]gin.H, 0, len(creds))
	for i := range creds {
		items = append(items, webauthnCredentialView(&creds[i]))
	}
	ok(c, gin.H{"items": items})
}

func webauthnCredentialView(c *models.WebAuthnCredential) gin.H {
	return gin.H{
		"id": c.ID, "name": c.Name, "createdAt": c.CreatedAt, "lastUsedAt": c.LastUsedAt,
		"transports": c.Transports, "backupEligible": c.BackupEligible, "backupState": c.BackupState,
	}
}

func (a *API) deleteWebAuthnCredential(c *gin.Context) {
	b := a.currentBundle(c)
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Passkey not found")
		return
	}
	if err := a.Store.DeleteWebAuthnCredential(c.Request.Context(), b.Tenant.ID, b.User.ID, id); err != nil {
		fail(c, http.StatusNotFound, "not_found", "Passkey not found")
		return
	}
	a.Svc.Audit(c.Request.Context(), b.Tenant.ID, b.User, "user.webauthn.deleted", "user", b.User.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) beginWebAuthnLogin(c *gin.Context) {
	if a.WebAuthn == nil {
		fail(c, http.StatusBadRequest, "webauthn_disabled", "Passkey support is not configured")
		return
	}
	var body struct {
		MFAToken string `json:"mfaToken"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.MFAToken == "" {
		fail(c, http.StatusBadRequest, "invalid_body", "An MFA token is required")
		return
	}
	ctx := c.Request.Context()
	pending, err := a.Store.ConsumeMFAPending(ctx, security.HashToken(body.MFAToken))
	if err != nil {
		fail(c, http.StatusBadRequest, "mfa_expired", "The sign-in attempt has expired")
		return
	}
	waUser, err := a.loadWebAuthnUser(ctx, pending.TenantID, pending.UserID)
	if err != nil || len(waUser.credentials) == 0 {
		fail(c, http.StatusBadRequest, "webauthn_unavailable", "No passkey is registered for this account")
		return
	}
	assertion, session, err := a.WebAuthn.BeginLogin(waUser)
	if err != nil {
		fail(c, http.StatusBadRequest, "webauthn_error", err.Error())
		return
	}
	raw, err := a.createWebAuthnSession(ctx, pending.TenantID, pending.UserID, "login", *session, pending.RequestID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to start passkey sign-in")
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessionId": raw, "options": assertion})
}

func (a *API) finishWebAuthnLogin(c *gin.Context) {
	if a.WebAuthn == nil {
		fail(c, http.StatusBadRequest, "webauthn_disabled", "Passkey support is not configured")
		return
	}
	var body struct {
		SessionID  string          `json:"sessionId"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil || body.SessionID == "" || len(body.Credential) == 0 {
		fail(c, http.StatusBadRequest, "invalid_body", "A WebAuthn credential is required")
		return
	}
	ctx := c.Request.Context()
	session, err := a.Store.ConsumeWebAuthnSession(ctx, security.HashToken(body.SessionID))
	if err != nil || session.Kind != "login" {
		fail(c, http.StatusBadRequest, "webauthn_session", "The sign-in session is invalid or expired")
		return
	}
	waUser, err := a.loadWebAuthnUser(ctx, session.TenantID, session.UserID)
	if err != nil || len(waUser.credentials) == 0 {
		fail(c, http.StatusBadRequest, "webauthn_unavailable", "No passkey is registered for this account")
		return
	}
	var data webauthn.SessionData
	if err := json.Unmarshal([]byte(session.Data), &data); err != nil {
		fail(c, http.StatusBadRequest, "webauthn_session", "The sign-in session is invalid")
		return
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewReader(body.Credential))
	req.Header.Set("Content-Type", "application/json")
	credential, err := a.WebAuthn.FinishLogin(waUser, data, req)
	if err != nil {
		fail(c, http.StatusUnauthorized, "webauthn_failed", "Passkey verification failed")
		return
	}
	for i := range waUser.credentials {
		if bytes.Equal(waUser.credentials[i].CredentialID, credential.ID) {
			_ = a.Store.UpdateWebAuthnCredential(ctx, waUser.credentials[i].ID, credential.Authenticator.SignCount)
			break
		}
	}
	tenant, err := a.Store.FindTenantByID(ctx, session.TenantID)
	if err != nil {
		fail(c, http.StatusBadRequest, "tenant_not_found", "Organization not found")
		return
	}
	sess, raw, err := a.Svc.StartSession(ctx, tenant, waUser.user, true, a.clientIP(c), c.Request.UserAgent())
	if err != nil {
		fail(c, http.StatusInternalServerError, "session_error", "Unable to create a session")
		return
	}
	a.setSessionCookie(c, raw, sess.ExpiresAt)
	a.Svc.Audit(ctx, tenant.ID, waUser.user, "auth.login.success", "session", sess.ID, a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"method": "webauthn"})
	next := ""
	if session.RequestID != "" {
		if req, client, err := a.attachRequest(c, tenant, waUser.user, session.RequestID); err == nil {
			next, _ = a.nextStep(c, tenant, waUser.user, req, client)
		}
	}
	c.JSON(http.StatusOK, gin.H{"mfaRequired": false, "user": waUser.user, "tenant": publicTenant(tenant), "next": next, "requestId": session.RequestID})
}

func (a *API) beginWebAuthnStepUp(c *gin.Context) {
	if a.WebAuthn == nil {
		fail(c, http.StatusBadRequest, "webauthn_disabled", "Passkey support is not configured")
		return
	}
	b := a.currentBundle(c)
	waUser, err := a.loadWebAuthnUser(c.Request.Context(), b.Tenant.ID, b.User.ID)
	if err != nil || len(waUser.credentials) == 0 {
		fail(c, http.StatusBadRequest, "webauthn_unavailable", "No passkey is registered for this account")
		return
	}
	assertion, session, err := a.WebAuthn.BeginLogin(waUser)
	if err != nil {
		fail(c, http.StatusBadRequest, "webauthn_error", err.Error())
		return
	}
	raw, err := a.createWebAuthnSession(c.Request.Context(), b.Tenant.ID, b.User.ID, "stepup", *session, "")
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to start step-up verification")
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessionId": raw, "options": assertion})
}

func (a *API) finishWebAuthnStepUp(c *gin.Context) {
	if a.WebAuthn == nil {
		fail(c, http.StatusBadRequest, "webauthn_disabled", "Passkey support is not configured")
		return
	}
	b := a.currentBundle(c)
	var body struct {
		SessionID  string          `json:"sessionId"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	session, err := a.Store.ConsumeWebAuthnSession(c.Request.Context(), security.HashToken(body.SessionID))
	if err != nil || session.Kind != "stepup" || session.TenantID != b.Tenant.ID || session.UserID != b.User.ID {
		fail(c, http.StatusBadRequest, "webauthn_session", "The step-up session is invalid or expired")
		return
	}
	waUser, err := a.loadWebAuthnUser(c.Request.Context(), b.Tenant.ID, b.User.ID)
	if err != nil || len(waUser.credentials) == 0 {
		fail(c, http.StatusBadRequest, "webauthn_unavailable", "No passkey is registered for this account")
		return
	}
	var data webauthn.SessionData
	_ = json.Unmarshal([]byte(session.Data), &data)
	req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "/", bytes.NewReader(body.Credential))
	req.Header.Set("Content-Type", "application/json")
	credential, err := a.WebAuthn.FinishLogin(waUser, data, req)
	if err != nil {
		fail(c, http.StatusUnauthorized, "webauthn_failed", "Passkey verification failed")
		return
	}
	for i := range waUser.credentials {
		if bytes.Equal(waUser.credentials[i].CredentialID, credential.ID) {
			_ = a.Store.UpdateWebAuthnCredential(c.Request.Context(), waUser.credentials[i].ID, credential.Authenticator.SignCount)
			break
		}
	}
	_ = a.Store.MarkStepUp(c.Request.Context(), b.Session.ID)
	a.Svc.Audit(c.Request.Context(), b.Tenant.ID, b.User, "user.stepup.verified", "user", b.User.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"method": "webauthn"})
	ok(c, gin.H{"ok": true, "stepUp": true})
}
