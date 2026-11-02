package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"ucenter/internal/security"
)

func (a *API) updateProfile(c *gin.Context) {
	b := a.currentBundle(c)
	var body struct {
		Name     *string `json:"name"`
		Username *string `json:"username"`
		Picture  *string `json:"picture"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	update := bson.M{}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if len(name) < 1 || len(name) > 120 {
			fail(c, http.StatusBadRequest, "invalid_name", "Name must be between 1 and 120 characters")
			return
		}
		update["name"] = name
	}
	if body.Username != nil {
		username := strings.TrimSpace(*body.Username)
		if len(username) > 60 {
			fail(c, http.StatusBadRequest, "invalid_username", "Username must be at most 60 characters")
			return
		}
		update["username"] = username
	}
	if body.Picture != nil {
		picture := strings.TrimSpace(*body.Picture)
		if picture != "" && !strings.HasPrefix(picture, "https://") && !strings.HasPrefix(picture, "http://") {
			fail(c, http.StatusBadRequest, "invalid_picture", "Picture must be an http(s) URL")
			return
		}
		update["picture"] = picture
	}
	if len(update) == 0 {
		ok(c, gin.H{"user": b.User})
		return
	}
	if err := a.Store.UpdateUser(c.Request.Context(), b.User.ID, update); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to update profile")
		return
	}
	user, _ := a.Store.FindUserByID(c.Request.Context(), b.User.ID)
	a.Svc.Audit(c.Request.Context(), b.Tenant.ID, user, "user.profile.updated", "user", b.User.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"user": user})
}

func (a *API) changePassword(c *gin.Context) {
	b := a.currentBundle(c)
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Current and new passwords are required")
		return
	}
	if !security.VerifyPassword(body.CurrentPassword, b.User.PasswordHash) {
		fail(c, http.StatusUnauthorized, "invalid_password", "Your current password is incorrect")
		return
	}
	if err := validatePassword(b.Tenant, body.NewPassword); err != nil {
		fail(c, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	if body.NewPassword == body.CurrentPassword {
		fail(c, http.StatusBadRequest, "same_password", "The new password must be different from the current password")
		return
	}
	hash, err := security.HashPassword(body.NewPassword)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to update password")
		return
	}
	ctx := c.Request.Context()
	if err := a.Store.UpdateUser(ctx, b.User.ID, bson.M{"password_hash": hash}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to update password")
		return
	}
	_ = a.Store.RevokeAllUserSessions(ctx, b.Tenant.ID, b.User.ID)
	_ = a.Store.RevokeUserRefreshTokens(ctx, b.Tenant.ID, b.User.ID)
	_ = a.Store.BumpTokenVersion(ctx, b.User.ID)
	user, _ := a.Store.FindUserByID(ctx, b.User.ID)
	sess, raw, err := a.Svc.StartSession(ctx, b.Tenant, user, b.Session.MFA, a.clientIP(c), c.Request.UserAgent())
	if err == nil {
		a.setSessionCookie(c, raw, sess.ExpiresAt)
	}
	a.Svc.Audit(ctx, b.Tenant.ID, user, "user.password.changed", "user", b.User.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) setupMFA(c *gin.Context) {
	b := a.currentBundle(c)
	if b.User.MFA.Enabled {
		fail(c, http.StatusBadRequest, "mfa_enabled", "Two-factor authentication is already enabled")
		return
	}
	key, err := security.NewTOTPSecret("UCenter "+b.Tenant.Name, b.User.Email)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to generate a TOTP secret")
		return
	}
	if err := a.Store.UpdateUser(c.Request.Context(), b.User.ID, bson.M{"mfa.secret": key.Secret(), "mfa.enabled": false}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to save the TOTP secret")
		return
	}
	ok(c, gin.H{"secret": key.Secret(), "otpauthUrl": key.URL()})
}

func (a *API) enableMFA(c *gin.Context) {
	b := a.currentBundle(c)
	var body struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "A verification code is required")
		return
	}
	if b.User.MFA.Enabled {
		fail(c, http.StatusBadRequest, "mfa_enabled", "Two-factor authentication is already enabled")
		return
	}
	if b.User.MFA.Secret == "" {
		fail(c, http.StatusBadRequest, "mfa_not_started", "Start two-factor setup before verifying a code")
		return
	}
	if !security.VerifyTOTP(b.User.MFA.Secret, body.Code) {
		fail(c, http.StatusUnauthorized, "invalid_mfa", "The verification code is invalid")
		return
	}
	codes, hashes, err := security.GenerateRecoveryCodes(8)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to generate recovery codes")
		return
	}
	now := time.Now()
	ctx := c.Request.Context()
	if err := a.Store.UpdateUser(ctx, b.User.ID, bson.M{
		"mfa.enabled":        true,
		"mfa.recovery_codes": hashes,
		"mfa.enabled_at":     now,
	}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to enable two-factor authentication")
		return
	}
	a.Svc.Audit(ctx, b.Tenant.ID, b.User, "user.mfa.enabled", "user", b.User.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"recoveryCodes": codes})
}

func (a *API) disableMFA(c *gin.Context) {
	b := a.currentBundle(c)
	var body struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Your password is required")
		return
	}
	if !security.VerifyPassword(body.Password, b.User.PasswordHash) {
		fail(c, http.StatusUnauthorized, "invalid_password", "Your password is incorrect")
		return
	}
	ctx := c.Request.Context()
	if err := a.Store.UpdateUser(ctx, b.User.ID, bson.M{
		"mfa.enabled":        false,
		"mfa.secret":         "",
		"mfa.recovery_codes": []string{},
		"mfa.enabled_at":     nil,
	}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to disable two-factor authentication")
		return
	}
	a.Svc.Audit(ctx, b.Tenant.ID, b.User, "user.mfa.disabled", "user", b.User.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) regenerateRecoveryCodes(c *gin.Context) {
	b := a.currentBundle(c)
	var body struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Your password is required")
		return
	}
	if !security.VerifyPassword(body.Password, b.User.PasswordHash) {
		fail(c, http.StatusUnauthorized, "invalid_password", "Your password is incorrect")
		return
	}
	if !b.User.MFA.Enabled {
		fail(c, http.StatusBadRequest, "mfa_disabled", "Two-factor authentication is not enabled")
		return
	}
	codes, hashes, err := security.GenerateRecoveryCodes(8)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to generate recovery codes")
		return
	}
	if err := a.Store.UpdateUser(c.Request.Context(), b.User.ID, bson.M{"mfa.recovery_codes": hashes}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to save recovery codes")
		return
	}
	a.Svc.Audit(c.Request.Context(), b.Tenant.ID, b.User, "user.mfa.recovery_codes.regenerated", "user", b.User.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"recoveryCodes": codes})
}
