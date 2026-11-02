package api

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/mail"
	"ucenter/internal/models"
	"ucenter/internal/security"
)

func (a *API) issueEmailToken(ctx context.Context, tenantID, userID primitive.ObjectID, kind string, ttl time.Duration) (string, error) {
	raw, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	raw = "uct_" + raw
	token := &models.EmailToken{
		TenantID:  tenantID,
		UserID:    userID,
		Type:      kind,
		TokenHash: security.HashToken(raw),
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}
	if err := a.Store.CreateEmailToken(ctx, token); err != nil {
		return "", err
	}
	return raw, nil
}

func (a *API) sendMail(ctx context.Context, tenant *models.Tenant, to, kind, subject, text string) {
	htmlBody := "<div style=\"font-family:system-ui,sans-serif;line-height:1.6;color:#111\"><p>" +
		strings.ReplaceAll(html.EscapeString(text), "\n", "<br>") + "</p></div>"
	if err := a.Mailer.Send(ctx, mail.Message{TenantID: tenant.ID, To: to, Subject: subject, Text: text, HTML: htmlBody, Kind: kind}); err != nil {
		log.Printf("mail send failed (%s to %s): %v", kind, to, err)
	}
}

func (a *API) emailLink(path string, token, tenantSlug string) string {
	return fmt.Sprintf("%s%s?token=%s&tenant=%s", a.Cfg.BaseURL, path, url.QueryEscape(token), url.QueryEscape(tenantSlug))
}

func (a *API) sendInviteEmail(ctx context.Context, tenant *models.Tenant, user *models.User) {
	raw, err := a.issueEmailToken(ctx, tenant.ID, user.ID, models.EmailTokenInvite, 7*24*time.Hour)
	if err != nil {
		log.Printf("invite token: %v", err)
		return
	}
	link := a.emailLink("/accept-invite", raw, tenant.Slug)
	text := fmt.Sprintf("Hello %s,\n\nYou have been invited to %s.\nSet your password using this link (valid for 7 days):\n%s\n\nIf you were not expecting this invitation, you can ignore this email.", user.Name, tenant.Name, link)
	a.sendMail(ctx, tenant, user.Email, models.EmailTokenInvite, "You have been invited to "+tenant.Name, text)
}

func (a *API) sendVerificationEmail(ctx context.Context, tenant *models.Tenant, user *models.User) {
	raw, err := a.issueEmailToken(ctx, tenant.ID, user.ID, models.EmailTokenVerifyEmail, 24*time.Hour)
	if err != nil {
		log.Printf("verify token: %v", err)
		return
	}
	link := a.emailLink("/verify-email", raw, tenant.Slug)
	text := fmt.Sprintf("Hello %s,\n\nPlease verify your email address for %s:\n%s\n\nThis link expires in 24 hours.", user.Name, tenant.Name, link)
	a.sendMail(ctx, tenant, user.Email, models.EmailTokenVerifyEmail, "Verify your email for "+tenant.Name, text)
}

func (a *API) sendPasswordResetEmail(ctx context.Context, tenant *models.Tenant, user *models.User) {
	raw, err := a.issueEmailToken(ctx, tenant.ID, user.ID, models.EmailTokenResetPassword, 30*time.Minute)
	if err != nil {
		log.Printf("reset token: %v", err)
		return
	}
	link := a.emailLink("/reset-password", raw, tenant.Slug)
	text := fmt.Sprintf("Hello %s,\n\nSomeone requested a password reset for your %s account.\nReset your password using this link (valid for 30 minutes):\n%s\n\nIf you did not request this, you can ignore this email.", user.Name, tenant.Name, link)
	a.sendMail(ctx, tenant, user.Email, models.EmailTokenResetPassword, "Reset your "+tenant.Name+" password", text)
}

func (a *API) sendMFARecoveryEmail(ctx context.Context, tenant *models.Tenant, user *models.User) {
	raw, err := a.issueEmailToken(ctx, tenant.ID, user.ID, models.EmailTokenMFA, 15*time.Minute)
	if err != nil {
		log.Printf("mfa recovery token: %v", err)
		return
	}
	link := a.emailLink("/mfa-recovery/confirm", raw, tenant.Slug)
	text := fmt.Sprintf("Hello %s,\n\nA request was made to reset two-factor authentication for your %s account.\nUse this link to disable MFA (valid for 15 minutes):\n%s\n\nIf you did not request this, change your password immediately.", user.Name, tenant.Name, link)
	a.sendMail(ctx, tenant, user.Email, models.EmailTokenMFA, "Two-factor recovery for "+tenant.Name, text)
}

func (a *API) forgotPassword(c *gin.Context) {
	var body struct {
		Tenant string `json:"tenant"`
		Email  string `json:"email"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Tenant and email are required")
		return
	}
	body.Email = security.NormalizeEmail(body.Email)
	key := a.clientIP(c) + "|forgot|" + body.Tenant + "|" + body.Email
	if !a.mailLimiter.allow(key) {
		fail(c, http.StatusTooManyRequests, "rate_limited", "Too many requests. Please wait a few minutes and try again.")
		return
	}
	ctx := c.Request.Context()
	tenant, err := a.Store.FindTenantBySlug(ctx, strings.ToLower(strings.TrimSpace(body.Tenant)))
	if err == nil && tenant.Status == models.StatusActive {
		if user, err := a.Store.FindUserByEmail(ctx, body.Email); err == nil && user.Status == models.StatusActive && user.Membership(tenant.ID) != nil {
			a.sendPasswordResetEmail(ctx, tenant, user)
			a.Svc.Audit(ctx, tenant.ID, nil, "auth.password.reset.requested", "user", user.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
		}
	}
	ok(c, gin.H{"ok": true})
}

func (a *API) resetPassword(c *gin.Context) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Token == "" || body.Password == "" {
		fail(c, http.StatusBadRequest, "invalid_body", "Token and new password are required")
		return
	}
	ctx := c.Request.Context()
	token, err := a.Store.ConsumeEmailToken(ctx, security.HashToken(body.Token), models.EmailTokenResetPassword)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_token", "This reset link is invalid or has expired")
		return
	}
	tenant, err := a.Store.FindTenantByID(ctx, token.TenantID)
	if err != nil || tenant.Status != models.StatusActive {
		fail(c, http.StatusBadRequest, "invalid_token", "This reset link is no longer valid")
		return
	}
	user, err := a.Store.FindUserByID(ctx, token.UserID)
	if err != nil || user.Status != models.StatusActive {
		fail(c, http.StatusBadRequest, "invalid_token", "This reset link is no longer valid")
		return
	}
	if err := validatePassword(tenant, body.Password); err != nil {
		fail(c, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	hash, err := security.HashPassword(body.Password)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to reset password")
		return
	}
	if err := a.Store.UpdateUser(ctx, user.ID, bson.M{"password_hash": hash}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to reset password")
		return
	}
	_ = a.Store.RevokeAllUserSessions(ctx, tenant.ID, user.ID)
	_ = a.Store.RevokeUserRefreshTokens(ctx, tenant.ID, user.ID)
	_ = a.Store.BumpTokenVersion(ctx, user.ID)
	a.Svc.Audit(ctx, tenant.ID, user, "user.password.reset", "user", user.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"via": "email"})
	ok(c, gin.H{"ok": true})
}

func (a *API) verifyEmail(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Token == "" {
		fail(c, http.StatusBadRequest, "invalid_body", "Token is required")
		return
	}
	ctx := c.Request.Context()
	token, err := a.Store.ConsumeEmailToken(ctx, security.HashToken(body.Token), models.EmailTokenVerifyEmail)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_token", "This verification link is invalid or has expired")
		return
	}
	if err := a.Store.UpdateUser(ctx, token.UserID, bson.M{"email_verified": true}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to verify email")
		return
	}
	if tenant, err := a.Store.FindTenantByID(ctx, token.TenantID); err == nil {
		if user, err := a.Store.FindUserByID(ctx, token.UserID); err == nil {
			a.Svc.Audit(ctx, tenant.ID, user, "user.email.verified", "user", user.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"via": "email"})
		}
	}
	ok(c, gin.H{"ok": true})
}

func (a *API) requestMFARecovery(c *gin.Context) {
	var body struct {
		Tenant string `json:"tenant"`
		Email  string `json:"email"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Tenant and email are required")
		return
	}
	body.Email = security.NormalizeEmail(body.Email)
	key := a.clientIP(c) + "|mfa|" + body.Tenant + "|" + body.Email
	if !a.mailLimiter.allow(key) {
		fail(c, http.StatusTooManyRequests, "rate_limited", "Too many requests. Please wait a few minutes and try again.")
		return
	}
	ctx := c.Request.Context()
	tenant, err := a.Store.FindTenantBySlug(ctx, strings.ToLower(strings.TrimSpace(body.Tenant)))
	if err == nil && tenant.Status == models.StatusActive {
		if user, err := a.Store.FindUserByEmail(ctx, body.Email); err == nil && user.Status == models.StatusActive && user.MFA.Enabled && user.Membership(tenant.ID) != nil {
			a.sendMFARecoveryEmail(ctx, tenant, user)
			a.Svc.Audit(ctx, tenant.ID, nil, "auth.mfa.recovery.requested", "user", user.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
		}
	}
	ok(c, gin.H{"ok": true})
}

func (a *API) confirmMFARecovery(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Token == "" {
		fail(c, http.StatusBadRequest, "invalid_body", "Token is required")
		return
	}
	ctx := c.Request.Context()
	token, err := a.Store.ConsumeEmailToken(ctx, security.HashToken(body.Token), models.EmailTokenMFA)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_token", "This recovery link is invalid or has expired")
		return
	}
	tenant, err := a.Store.FindTenantByID(ctx, token.TenantID)
	if err != nil || tenant.Status != models.StatusActive {
		fail(c, http.StatusBadRequest, "invalid_token", "This recovery link is no longer valid")
		return
	}
	if err := a.Store.UpdateUser(ctx, token.UserID, bson.M{
		"mfa.enabled": false, "mfa.secret": "", "mfa.recovery_codes": []string{}, "mfa.enabled_at": nil,
	}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to reset MFA")
		return
	}
	_ = a.Store.RevokeAllUserSessions(ctx, tenant.ID, token.UserID)
	_ = a.Store.RevokeUserRefreshTokens(ctx, tenant.ID, token.UserID)
	_ = a.Store.BumpTokenVersion(ctx, token.UserID)
	user, _ := a.Store.FindUserByID(ctx, token.UserID)
	a.Svc.Audit(ctx, tenant.ID, user, "user.mfa.recovery", "user", token.UserID.Hex(), a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"via": "email"})
	ok(c, gin.H{"ok": true})
}

func (a *API) acceptInvite(c *gin.Context) {
	var body struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Token == "" || body.Password == "" {
		fail(c, http.StatusBadRequest, "invalid_body", "Token and password are required")
		return
	}
	ctx := c.Request.Context()
	token, err := a.Store.ConsumeEmailToken(ctx, security.HashToken(body.Token), models.EmailTokenInvite)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_token", "This invitation is invalid or has expired")
		return
	}
	tenant, err := a.Store.FindTenantByID(ctx, token.TenantID)
	if err != nil || tenant.Status != models.StatusActive {
		fail(c, http.StatusBadRequest, "invalid_token", "This invitation is no longer valid")
		return
	}
	user, err := a.Store.FindUserByID(ctx, token.UserID)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_token", "This invitation is no longer valid")
		return
	}
	if err := validatePassword(tenant, body.Password); err != nil {
		fail(c, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	hash, err := security.HashPassword(body.Password)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to accept invitation")
		return
	}
	update := bson.M{"password_hash": hash, "status": models.StatusActive, "email_verified": true}
	if strings.TrimSpace(body.Name) != "" {
		update["name"] = strings.TrimSpace(body.Name)
	}
	if err := a.Store.UpdateUser(ctx, user.ID, update); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to accept invitation")
		return
	}
	a.Svc.Audit(ctx, tenant.ID, user, "user.invite.accepted", "user", user.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) sendUserInvite(c *gin.Context) {
	target, err := a.findTenantUser(c)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "User not found")
		return
	}
	tenant := a.currentTenant(c)
	a.sendInviteEmail(c.Request.Context(), tenant, target)
	a.audit(c, "user.invite.sent", "user", target.ID.Hex(), nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) mailOutbox(c *gin.Context) {
	b := a.currentBundle(c)
	messages, err := a.Store.ListMailOutbox(c.Request.Context(), b.Tenant.ID, 20)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load mail outbox")
		return
	}
	ok(c, gin.H{"items": messages, "driver": a.Cfg.MailDriver})
}
