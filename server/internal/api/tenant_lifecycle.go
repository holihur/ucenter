package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"context"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"ucenter/internal/billing"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/store"
)

func (a *API) publicConfig(c *gin.Context) {
	ok(c, gin.H{"allowSelfSignup": a.Cfg.AllowSelfSignup})
}

func (a *API) tenantByHost(c *gin.Context) {
	host := strings.TrimSpace(c.Query("domain"))
	if host == "" {
		host = c.Request.Host
	}
	host = strings.ToLower(strings.Split(host, ":")[0])
	tenant, err := a.Store.FindTenantByDomain(c.Request.Context(), host)
	if err != nil || tenant.Status != models.StatusActive {
		fail(c, http.StatusNotFound, "tenant_not_found", "No tenant is configured for this domain")
		return
	}
	ok(c, publicTenant(tenant))
}

type signupRequest struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	OwnerName  string `json:"ownerName"`
	OwnerEmail string `json:"ownerEmail"`
	Password   string `json:"password"`
}

func (a *API) selfSignup(c *gin.Context) {
	if !a.Cfg.AllowSelfSignup {
		fail(c, http.StatusForbidden, "signup_disabled", "Self-service tenant signup is disabled")
		return
	}
	var body signupRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "All fields are required")
		return
	}
	body.Slug = strings.ToLower(strings.TrimSpace(body.Slug))
	body.Name = strings.TrimSpace(body.Name)
	body.OwnerName = strings.TrimSpace(body.OwnerName)
	body.OwnerEmail = security.NormalizeEmail(body.OwnerEmail)
	if !slugRE.MatchString(body.Slug) || body.Name == "" || !validEmail(body.OwnerEmail) || body.Password == "" {
		fail(c, http.StatusBadRequest, "invalid_body", "A valid organization slug, name, owner email and password are required")
		return
	}
	if ok, _ := a.mailLimiter.Allow(c.Request.Context(), "signup|"+a.clientIP(c)); !ok {
		fail(c, http.StatusTooManyRequests, "rate_limited", "Too many signup attempts. Please try again later.")
		return
	}
	ctx := c.Request.Context()
	tenant := &models.Tenant{Slug: body.Slug, Name: body.Name, Status: models.StatusActive, Settings: models.DefaultSettings()}
	if err := a.Store.CreateTenant(ctx, tenant); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			fail(c, http.StatusConflict, "tenant_exists", "That organization slug is already taken")
			return
		}
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create organization")
		return
	}
	if err := a.Svc.ProvisionTenant(ctx, tenant); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to provision organization")
		return
	}
	if err := validatePassword(tenant, body.Password); err != nil {
		_ = a.Store.PurgeTenant(ctx, tenant.ID)
		fail(c, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	if _, err := a.Store.FindUserByEmail(ctx, body.OwnerEmail); err == nil {
		_ = a.Store.PurgeTenant(ctx, tenant.ID)
		fail(c, http.StatusConflict, "email_in_use", "An account with this email already exists")
		return
	}
	hash, _ := security.HashPassword(body.Password)
	name := body.OwnerName
	if name == "" {
		name = strings.Split(body.OwnerEmail, "@")[0]
	}
	user := &models.User{
		Email: body.OwnerEmail, EmailVerified: true, Username: strings.Split(body.OwnerEmail, "@")[0],
		Name: name, PasswordHash: hash, Status: models.StatusActive,
		Memberships: []models.Membership{{TenantID: tenant.ID, Roles: []string{"owner"}, JoinedAt: time.Now()}},
	}
	if err := a.Store.CreateUser(ctx, user); err != nil {
		_ = a.Store.PurgeTenant(ctx, tenant.ID)
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create owner account")
		return
	}
	sess, raw, err := a.Svc.StartSession(ctx, tenant, user, false, a.clientIP(c), c.Request.UserAgent())
	if err != nil {
		fail(c, http.StatusInternalServerError, "session_error", "Unable to create a session")
		return
	}
	a.setSessionCookie(c, raw, sess.ExpiresAt)
	a.Svc.Audit(ctx, tenant.ID, user, "tenant.signup", "tenant", tenant.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	c.JSON(http.StatusCreated, gin.H{"tenant": publicTenant(tenant), "user": user})
}

func (a *API) checkUserQuota(c *gin.Context) error {
	return a.checkUserQuotaFor(c.Request.Context(), a.currentTenant(c))
}

func (a *API) checkUserQuotaFor(ctx context.Context, tenant *models.Tenant) error {
	limit := billing.EffectiveLimits(tenant.Subscription, tenant.Settings.QuotaUsers, 0).Users
	if limit <= 0 {
		return nil
	}
	count, err := a.Store.CountUsersInTenant(ctx, tenant.ID, "")
	if err != nil {
		return err
	}
	if count >= int64(limit) {
		return fmt.Errorf("user limit reached (%d)", limit)
	}
	return nil
}

func (a *API) checkClientQuota(c *gin.Context) error {
	return a.checkClientQuotaFor(c.Request.Context(), a.currentTenant(c))
}

func (a *API) checkClientQuotaFor(ctx context.Context, tenant *models.Tenant) error {
	limit := billing.EffectiveLimits(tenant.Subscription, 0, tenant.Settings.QuotaClients).Clients
	if limit <= 0 {
		return nil
	}
	count, err := a.Store.CountClients(ctx, tenant.ID)
	if err != nil {
		return err
	}
	if count >= int64(limit) {
		return fmt.Errorf("application limit reached (%d)", limit)
	}
	return nil
}

func (a *API) exportTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenant := a.currentTenant(c)
	users := []gin.H{}
	page := int64(1)
	for {
		list, total, err := a.Store.ListUsers(ctx, store.UserFilter{TenantID: tenant.ID, Page: page, PageSize: 200})
		if err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "Unable to export tenant")
			return
		}
		for i := range list {
			users = append(users, adminUserView(&list[i], tenant.ID))
		}
		if int64(len(users)) >= total || len(list) == 0 {
			break
		}
		page++
	}
	roles, _ := a.Store.ListRoles(ctx, tenant.ID)
	clients, _ := a.Store.ListClients(ctx, tenant.ID)
	clientItems := make([]gin.H, 0, len(clients))
	for i := range clients {
		clientItems = append(clientItems, clientView(&clients[i]))
	}
	body := gin.H{
		"exportedAt": time.Now().UTC(),
		"tenant":     tenant,
		"users":      users,
		"roles":      roles,
		"clients":    clientItems,
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", tenant.Slug+"-export.json"))
	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, body)
}

func (a *API) softDeleteTenant(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Tenant not found")
		return
	}
	tenant, err := a.Store.FindTenantByID(c.Request.Context(), id)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "Tenant not found")
		return
	}
	if tenant.ID == a.currentTenantID(c) {
		fail(c, http.StatusBadRequest, "self_delete", "You cannot disable the tenant you are currently managing")
		return
	}
	now := time.Now()
	purge := now.AddDate(0, 0, 30)
	if err := a.Store.UpdateTenant(c.Request.Context(), id, bson.M{
		"status": models.StatusDisabled, "deleted_at": now, "purge_after": purge,
	}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to delete tenant")
		return
	}
	a.Svc.Audit(c.Request.Context(), id, a.currentBundle(c).User, "tenant.soft_deleted", "tenant", id.Hex(), a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"purgeAfter": purge})
	ok(c, gin.H{"ok": true, "purgeAfter": purge})
}

func (a *API) restoreTenant(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Tenant not found")
		return
	}
	if err := a.Store.UpdateTenant(c.Request.Context(), id, bson.M{
		"status": models.StatusActive, "deleted_at": nil, "purge_after": nil,
	}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to restore tenant")
		return
	}
	a.Svc.Audit(c.Request.Context(), id, a.currentBundle(c).User, "tenant.restored", "tenant", id.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) purgeTenant(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Tenant not found")
		return
	}
	if id == a.currentTenantID(c) {
		fail(c, http.StatusBadRequest, "self_delete", "You cannot purge the tenant you are currently managing")
		return
	}
	actorTenant := a.currentTenantID(c)
	if err := a.Store.PurgeTenant(c.Request.Context(), id); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to purge tenant")
		return
	}
	a.Svc.Audit(c.Request.Context(), actorTenant, a.currentBundle(c).User, "platform.tenant.purged", "tenant", id.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"ok": true})
}
