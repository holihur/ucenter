package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/store"
)

var (
	slugRE  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$`)
	keyRE   = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)
	scopeRE = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,64}$`)
)

func adminUserView(u *models.User, tenantID primitive.ObjectID) gin.H {
	roles := []string{}
	if m := u.Membership(tenantID); m != nil {
		roles = m.Roles
	}
	return gin.H{
		"id":            u.ID,
		"email":         u.Email,
		"emailVerified": u.EmailVerified,
		"username":      u.Username,
		"name":          u.Name,
		"picture":       u.Picture,
		"status":        u.Status,
		"platformAdmin": u.PlatformAdmin,
		"mfaEnabled":    u.MFA.Enabled,
		"roles":         roles,
		"lastLoginAt":   u.LastLoginAt,
		"createdAt":     u.CreatedAt,
		"updatedAt":     u.UpdatedAt,
	}
}

func clientView(c *models.Client) gin.H {
	return gin.H{
		"id":                     c.ID,
		"clientId":               c.ClientID,
		"name":                   c.Name,
		"description":            c.Description,
		"type":                   c.Type,
		"redirectUris":           c.RedirectURIs,
		"postLogoutRedirectUris": c.PostLogoutRedirectURIs,
		"allowedScopes":          c.AllowedScopes,
		"grantTypes":             c.GrantTypes,
		"requirePkce":            c.RequirePKCE,
		"trusted":                c.Trusted,
		"accessTokenTtl":         c.AccessTokenTTL,
		"idTokenTtl":             c.IDTokenTTL,
		"refreshTokenTtl":        c.RefreshTokenTTL,
		"enabled":                c.Enabled,
		"logoUrl":                c.LogoURL,
		"secretSet":              c.ClientSecretHash != "",
		"createdAt":              c.CreatedAt,
		"updatedAt":              c.UpdatedAt,
	}
}

func pagination(c *gin.Context) (int64, int64) {
	page, _ := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 64)
	size, _ := strconv.ParseInt(c.DefaultQuery("pageSize", "20"), 10, 64)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func (a *API) listUsers(c *gin.Context) {
	page, size := pagination(c)
	users, total, err := a.Store.ListUsers(c.Request.Context(), store.UserFilter{
		TenantID: a.currentTenantID(c),
		Search:   strings.TrimSpace(c.Query("q")),
		Status:   c.Query("status"),
		Role:     c.Query("role"),
		Page:     page,
		PageSize: size,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load users")
		return
	}
	items := make([]gin.H, 0, len(users))
	for i := range users {
		items = append(items, adminUserView(&users[i], a.currentTenantID(c)))
	}
	ok(c, gin.H{"items": items, "total": total, "page": page, "pageSize": size})
}

type createUserRequest struct {
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	Password string   `json:"password"`
}

func (a *API) createUser(c *gin.Context) {
	var body createUserRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	ctx := c.Request.Context()
	tenantID := a.currentTenantID(c)
	email := security.NormalizeEmail(body.Email)
	if !validEmail(email) {
		fail(c, http.StatusBadRequest, "invalid_email", "Enter a valid email address")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		fail(c, http.StatusBadRequest, "invalid_name", "Name is required")
		return
	}
	if err := a.checkUserQuota(c); err != nil {
		fail(c, http.StatusForbidden, "quota_exceeded", err.Error())
		return
	}
	roles := normalizeRoles(body.Roles)
	if err := a.validateRoleAssignment(c, roles); err != nil {
		fail(c, http.StatusForbidden, "invalid_roles", err.Error())
		return
	}
	tempPassword := ""
	if body.Password != "" {
		if err := validatePassword(a.currentTenant(c), body.Password); err != nil {
			fail(c, http.StatusBadRequest, "weak_password", err.Error())
			return
		}
	}
	if existing, err := a.Store.FindUserByEmail(ctx, email); err == nil {
		if existing.Membership(tenantID) != nil {
			fail(c, http.StatusConflict, "user_exists", "This user is already a member of the organization")
			return
		}
		if err := a.Store.SetMembership(ctx, existing.ID, tenantID, roles); err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "Unable to add user to organization")
			return
		}
		a.sendInviteEmail(ctx, a.currentTenant(c), existing)
		a.audit(c, "user.membership.added", "user", existing.ID.Hex(), map[string]interface{}{"roles": roles})
		ok(c, gin.H{"user": adminUserView(existing, tenantID), "tempPassword": "", "inviteSent": true})
		return
	}
	password := body.Password
	if password == "" {
		tempPassword = generateTempPassword()
		password = tempPassword
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create user")
		return
	}
	user := &models.User{
		Email:         email,
		EmailVerified: true,
		Username:      strings.TrimSpace(body.Username),
		Name:          strings.TrimSpace(body.Name),
		PasswordHash:  hash,
		Status:        models.StatusActive,
		Memberships: []models.Membership{{
			TenantID: tenantID,
			Roles:    roles,
			JoinedAt: time.Now(),
		}},
	}
	if user.Username == "" {
		user.Username = strings.Split(email, "@")[0]
	}
	if err := a.Store.CreateUser(ctx, user); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			fail(c, http.StatusConflict, "user_exists", "A user with this email already exists")
			return
		}
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create user")
		return
	}
	a.sendInviteEmail(ctx, a.currentTenant(c), user)
	a.audit(c, "user.created", "user", user.ID.Hex(), map[string]interface{}{"email": email, "roles": roles})
	c.JSON(http.StatusCreated, gin.H{"user": adminUserView(user, tenantID), "tempPassword": tempPassword, "inviteSent": true})
}

func (a *API) getUser(c *gin.Context) {
	user, err := a.findTenantUser(c)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "User not found")
		return
	}
	ok(c, gin.H{"user": adminUserView(user, a.currentTenantID(c))})
}

func (a *API) findTenantUser(c *gin.Context) (*models.User, error) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		return nil, store.ErrNotFound
	}
	user, err := a.Store.FindUserByID(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	if user.Membership(a.currentTenantID(c)) == nil {
		return nil, store.ErrNotFound
	}
	return user, nil
}

type updateUserRequest struct {
	Name          *string   `json:"name"`
	Username      *string   `json:"username"`
	Picture       *string   `json:"picture"`
	Status        *string   `json:"status"`
	Roles         *[]string `json:"roles"`
	EmailVerified *bool     `json:"emailVerified"`
	PlatformAdmin *bool     `json:"platformAdmin"`
}

func (a *API) updateUser(c *gin.Context) {
	target, err := a.findTenantUser(c)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "User not found")
		return
	}
	b := a.currentBundle(c)
	tenantID := a.currentTenantID(c)
	var body updateUserRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	isOwner := b.User.PlatformAdmin || b.User.HasRole(tenantID, "owner")
	targetIsOwner := target.HasRole(tenantID, "owner")
	if targetIsOwner && !isOwner {
		fail(c, http.StatusForbidden, "forbidden", "Only an owner can modify another owner")
		return
	}
	if body.PlatformAdmin != nil && !b.User.PlatformAdmin {
		fail(c, http.StatusForbidden, "forbidden", "Only platform administrators can change platform access")
		return
	}
	if target.ID == b.User.ID && body.Status != nil && *body.Status != models.StatusActive {
		fail(c, http.StatusBadRequest, "self_disable", "You cannot disable your own account")
		return
	}

	update := bson.M{}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" || len(name) > 120 {
			fail(c, http.StatusBadRequest, "invalid_name", "Name must be between 1 and 120 characters")
			return
		}
		update["name"] = name
	}
	if body.Username != nil {
		update["username"] = strings.TrimSpace(*body.Username)
	}
	if body.Picture != nil {
		update["picture"] = strings.TrimSpace(*body.Picture)
	}
	if body.EmailVerified != nil {
		update["email_verified"] = *body.EmailVerified
	}
	if body.PlatformAdmin != nil {
		update["platform_admin"] = *body.PlatformAdmin
	}
	if body.Status != nil {
		if *body.Status != models.StatusActive && *body.Status != models.StatusDisabled && *body.Status != models.StatusInvited {
			fail(c, http.StatusBadRequest, "invalid_status", "Invalid user status")
			return
		}
		update["status"] = *body.Status
	}

	if body.Roles != nil {
		roles := normalizeRoles(*body.Roles)
		if err := a.validateRoleAssignment(c, roles); err != nil {
			fail(c, http.StatusForbidden, "invalid_roles", err.Error())
			return
		}
		if targetIsOwner && !contains(roles, "owner") {
			count, _ := a.Store.CountUsersWithRole(c.Request.Context(), tenantID, "owner")
			if count <= 1 {
				fail(c, http.StatusBadRequest, "last_owner", "The organization must keep at least one owner")
				return
			}
		}
		if err := a.Store.SetMembership(c.Request.Context(), target.ID, tenantID, roles); err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "Unable to update roles")
			return
		}
		update["roles"] = roles
	}
	if len(update) > 0 {
		rolesUpdate := update["roles"]
		delete(update, "roles")
		if len(update) > 0 {
			if err := a.Store.UpdateUser(c.Request.Context(), target.ID, update); err != nil {
				fail(c, http.StatusInternalServerError, "internal_error", "Unable to update user")
				return
			}
		}
		if rolesUpdate != nil {
			update["roles"] = rolesUpdate
		}
	}
	if (body.Status != nil && *body.Status == models.StatusDisabled) || body.Roles != nil {
		_ = a.Store.RevokeAllUserSessions(c.Request.Context(), tenantID, target.ID)
		_ = a.Store.RevokeUserRefreshTokens(c.Request.Context(), tenantID, target.ID)
		_ = a.Store.BumpTokenVersion(c.Request.Context(), target.ID)
	}
	updated, _ := a.Store.FindUserByID(c.Request.Context(), target.ID)
	a.audit(c, "user.updated", "user", target.ID.Hex(), map[string]interface{}{"fields": keysOf(update)})
	ok(c, gin.H{"user": adminUserView(updated, tenantID)})
}

func (a *API) deleteUser(c *gin.Context) {
	target, err := a.findTenantUser(c)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "User not found")
		return
	}
	b := a.currentBundle(c)
	tenantID := a.currentTenantID(c)
	if target.ID == b.User.ID {
		fail(c, http.StatusBadRequest, "self_delete", "You cannot remove yourself from the organization")
		return
	}
	if target.HasRole(tenantID, "owner") && !b.User.PlatformAdmin && !b.User.HasRole(tenantID, "owner") {
		fail(c, http.StatusForbidden, "forbidden", "Only an owner can remove another owner")
		return
	}
	if target.HasRole(tenantID, "owner") {
		count, _ := a.Store.CountUsersWithRole(c.Request.Context(), tenantID, "owner")
		if count <= 1 {
			fail(c, http.StatusBadRequest, "last_owner", "The organization must keep at least one owner")
			return
		}
	}
	ctx := c.Request.Context()
	_ = a.Store.RevokeAllUserSessions(ctx, tenantID, target.ID)
	_ = a.Store.RevokeUserRefreshTokens(ctx, tenantID, target.ID)
	_ = a.Store.DeleteUserIdentities(ctx, tenantID, target.ID)
	remaining := len(target.Memberships)
	if target.PlatformAdmin {
		remaining = 2
	}
	if remaining <= 1 {
		if err := a.Store.DeleteUser(ctx, target.ID); err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "Unable to delete user")
			return
		}
	} else if err := a.Store.RemoveMembership(ctx, target.ID, tenantID); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to remove user")
		return
	}
	a.audit(c, "user.deleted", "user", target.ID.Hex(), map[string]interface{}{"email": target.Email})
	ok(c, gin.H{"ok": true})
}

func (a *API) resetUserPassword(c *gin.Context) {
	target, err := a.findTenantUser(c)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "User not found")
		return
	}
	b := a.currentBundle(c)
	if target.HasRole(a.currentTenantID(c), "owner") && !b.User.PlatformAdmin && !b.User.HasRole(a.currentTenantID(c), "owner") {
		fail(c, http.StatusForbidden, "forbidden", "Only an owner can reset another owner's password")
		return
	}
	temp := generateTempPassword()
	hash, err := security.HashPassword(temp)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to reset password")
		return
	}
	ctx := c.Request.Context()
	if err := a.Store.UpdateUser(ctx, target.ID, bson.M{"password_hash": hash}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to reset password")
		return
	}
	_ = a.Store.RevokeAllUserSessions(ctx, a.currentTenantID(c), target.ID)
	_ = a.Store.RevokeUserRefreshTokens(ctx, a.currentTenantID(c), target.ID)
	_ = a.Store.BumpTokenVersion(ctx, target.ID)
	a.audit(c, "user.password.reset", "user", target.ID.Hex(), nil)
	ok(c, gin.H{"tempPassword": temp})
}

func (a *API) revokeUserSessions(c *gin.Context) {
	target, err := a.findTenantUser(c)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "User not found")
		return
	}
	ctx := c.Request.Context()
	_ = a.Store.RevokeAllUserSessions(ctx, a.currentTenantID(c), target.ID)
	_ = a.Store.RevokeUserRefreshTokens(ctx, a.currentTenantID(c), target.ID)
	_ = a.Store.BumpTokenVersion(ctx, target.ID)
	a.audit(c, "sessions.revoked", "user", target.ID.Hex(), nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) listRoles(c *gin.Context) {
	roles, err := a.Store.ListRoles(c.Request.Context(), a.currentTenantID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load roles")
		return
	}
	ok(c, gin.H{"items": roles})
}

type roleRequest struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (a *API) createRole(c *gin.Context) {
	var body roleRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	body.Key = strings.ToLower(strings.TrimSpace(body.Key))
	body.Name = strings.TrimSpace(body.Name)
	if !keyRE.MatchString(body.Key) {
		fail(c, http.StatusBadRequest, "invalid_key", "Role key must be 3-32 lowercase characters (letters, numbers, - or _)")
		return
	}
	if body.Name == "" {
		fail(c, http.StatusBadRequest, "invalid_name", "Role name is required")
		return
	}
	perms, err := a.validatePermissions(c, body.Permissions)
	if err != nil {
		fail(c, http.StatusForbidden, "invalid_permissions", err.Error())
		return
	}
	role := &models.Role{
		TenantID:    a.currentTenantID(c),
		Key:         body.Key,
		Name:        body.Name,
		Description: strings.TrimSpace(body.Description),
		Permissions: perms,
	}
	if err := a.Store.CreateRole(c.Request.Context(), role); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			fail(c, http.StatusConflict, "role_exists", "A role with this key already exists")
			return
		}
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create role")
		return
	}
	a.audit(c, "role.created", "role", role.ID.Hex(), map[string]interface{}{"key": role.Key})
	c.JSON(http.StatusCreated, gin.H{"role": role})
}

func (a *API) updateRole(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Role not found")
		return
	}
	role, err := a.Store.FindRoleByID(c.Request.Context(), a.currentTenantID(c), id)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "Role not found")
		return
	}
	var body roleRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	update := bson.M{}
	if name := strings.TrimSpace(body.Name); name != "" {
		update["name"] = name
	}
	update["description"] = strings.TrimSpace(body.Description)
	if !role.System {
		perms, err := a.validatePermissions(c, body.Permissions)
		if err != nil {
			fail(c, http.StatusForbidden, "invalid_permissions", err.Error())
			return
		}
		update["permissions"] = perms
	}
	if err := a.Store.UpdateRole(c.Request.Context(), a.currentTenantID(c), id, update); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to update role")
		return
	}
	a.audit(c, "role.updated", "role", id.Hex(), map[string]interface{}{"key": role.Key})
	updated, _ := a.Store.FindRoleByID(c.Request.Context(), a.currentTenantID(c), id)
	ok(c, gin.H{"role": updated})
}

func (a *API) deleteRole(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Role not found")
		return
	}
	ctx := c.Request.Context()
	role, err := a.Store.FindRoleByID(ctx, a.currentTenantID(c), id)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "Role not found")
		return
	}
	if role.System {
		fail(c, http.StatusBadRequest, "system_role", "System roles cannot be deleted")
		return
	}
	count, _ := a.Store.CountUsersWithRole(ctx, a.currentTenantID(c), role.Key)
	if count > 0 {
		fail(c, http.StatusBadRequest, "role_in_use", fmt.Sprintf("%d user(s) still have this role", count))
		return
	}
	if err := a.Store.DeleteRole(ctx, a.currentTenantID(c), id); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to delete role")
		return
	}
	a.audit(c, "role.deleted", "role", id.Hex(), map[string]interface{}{"key": role.Key})
	ok(c, gin.H{"ok": true})
}

func (a *API) validatePermissions(c *gin.Context, perms []string) ([]string, error) {
	allowed := map[string]bool{}
	for _, p := range a.currentPermissions(c) {
		allowed[p] = true
	}
	out := []string{}
	seen := map[string]bool{}
	for _, p := range perms {
		if p == "platform:manage" || !models.PermissionSet[p] {
			return nil, fmt.Errorf("unknown permission: %s", p)
		}
		if !allowed[p] {
			return nil, fmt.Errorf("you cannot grant a permission you do not have: %s", p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

func (a *API) validateRoleAssignment(c *gin.Context, roles []string) error {
	b := a.currentBundle(c)
	tenantID := a.currentTenantID(c)
	docs, err := a.Store.RolesByKeys(c.Request.Context(), tenantID, roles)
	if err != nil {
		return errors.New("unable to validate roles")
	}
	if len(docs) != len(roles) {
		return errors.New("one or more roles do not exist")
	}
	isOwner := b.User.PlatformAdmin || b.User.HasRole(tenantID, "owner")
	actorPerms := map[string]bool{}
	for _, p := range a.currentPermissions(c) {
		actorPerms[p] = true
	}
	for _, r := range docs {
		if r.Key == "owner" && !isOwner {
			return errors.New("only an owner can grant the owner role")
		}
		if !isOwner {
			for _, p := range r.Permissions {
				if !actorPerms[p] {
					return fmt.Errorf("you cannot grant the %s role because it includes permissions you do not have", r.Name)
				}
			}
		}
	}
	return nil
}

func (a *API) listClients(c *gin.Context) {
	clients, err := a.Store.ListClients(c.Request.Context(), a.currentTenantID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load applications")
		return
	}
	items := make([]gin.H, 0, len(clients))
	for i := range clients {
		items = append(items, clientView(&clients[i]))
	}
	ok(c, gin.H{"items": items})
}

type clientRequest struct {
	Name                   string   `json:"name"`
	Description            string   `json:"description"`
	Type                   string   `json:"type"`
	RedirectURIs           []string `json:"redirectUris"`
	PostLogoutRedirectURIs []string `json:"postLogoutRedirectUris"`
	AllowedScopes          []string `json:"allowedScopes"`
	GrantTypes             []string `json:"grantTypes"`
	RequirePKCE            *bool    `json:"requirePkce"`
	Trusted                bool     `json:"trusted"`
	AccessTokenTTL         int      `json:"accessTokenTtl"`
	IDTokenTTL             int      `json:"idTokenTtl"`
	RefreshTokenTTL        int      `json:"refreshTokenTtl"`
	Enabled                *bool    `json:"enabled"`
	LogoURL                string   `json:"logoUrl"`
}

func (a *API) createClient(c *gin.Context) {
	if err := a.checkClientQuota(c); err != nil {
		fail(c, http.StatusForbidden, "quota_exceeded", err.Error())
		return
	}
	var body clientRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	client, secret, err := a.buildClient(c, &body, nil)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_client", err.Error())
		return
	}
	if err := a.Store.CreateClient(c.Request.Context(), client); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create application")
		return
	}
	a.audit(c, "client.created", "client", client.ClientID, map[string]interface{}{"name": client.Name})
	c.JSON(http.StatusCreated, gin.H{"client": clientView(client), "clientSecret": secret})
}

func (a *API) updateClient(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Application not found")
		return
	}
	existing, err := a.Store.FindClientByID(c.Request.Context(), a.currentTenantID(c), id)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "Application not found")
		return
	}
	var body clientRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	client, secret, err := a.buildClient(c, &body, existing)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_client", err.Error())
		return
	}
	update := bson.M{
		"name":                      client.Name,
		"description":               client.Description,
		"type":                      client.Type,
		"redirect_uris":             client.RedirectURIs,
		"post_logout_redirect_uris": client.PostLogoutRedirectURIs,
		"allowed_scopes":            client.AllowedScopes,
		"grant_types":               client.GrantTypes,
		"require_pkce":              client.RequirePKCE,
		"trusted":                   client.Trusted,
		"access_token_ttl":          client.AccessTokenTTL,
		"id_token_ttl":              client.IDTokenTTL,
		"refresh_token_ttl":         client.RefreshTokenTTL,
		"enabled":                   client.Enabled,
		"logo_url":                  client.LogoURL,
	}
	if secret != "" {
		update["client_secret_hash"] = security.HashClientSecret(secret)
		if encrypted, encErr := a.Cipher.Encrypt(secret); encErr == nil {
			update["client_secret_enc"] = encrypted
		}
	}
	if err := a.Store.UpdateClient(c.Request.Context(), a.currentTenantID(c), id, update); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to update application")
		return
	}
	a.audit(c, "client.updated", "client", existing.ClientID, map[string]interface{}{"name": client.Name})
	updated, _ := a.Store.FindClientByID(c.Request.Context(), a.currentTenantID(c), id)
	ok(c, gin.H{"client": clientView(updated), "clientSecret": secret})
}

func (a *API) buildClient(c *gin.Context, body *clientRequest, existing *models.Client) (*models.Client, string, error) {
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return nil, "", errors.New("application name is required")
	}
	typ := body.Type
	if typ == "" {
		typ = models.ClientTypeConfidential
	}
	if typ != models.ClientTypeConfidential && typ != models.ClientTypePublic {
		return nil, "", errors.New("type must be confidential or public")
	}
	grants := body.GrantTypes
	if len(grants) == 0 {
		grants = []string{"authorization_code", "refresh_token"}
	}
	for _, g := range grants {
		if g != "authorization_code" && g != "refresh_token" && g != "client_credentials" {
			return nil, "", fmt.Errorf("unsupported grant type: %s", g)
		}
	}
	if contains(grants, "client_credentials") && typ == models.ClientTypePublic {
		return nil, "", errors.New("client_credentials requires a confidential client")
	}
	if contains(grants, "authorization_code") && len(body.RedirectURIs) == 0 {
		return nil, "", errors.New("at least one redirect URI is required for the authorization code flow")
	}
	for _, u := range body.RedirectURIs {
		if err := validateRedirectURI(u); err != nil {
			return nil, "", err
		}
	}
	for _, u := range body.PostLogoutRedirectURIs {
		if err := validateRedirectURI(u); err != nil {
			return nil, "", fmt.Errorf("post-logout URI: %w", err)
		}
	}
	scopes := body.AllowedScopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	hasOpenID := false
	for _, s := range scopes {
		if !scopeRE.MatchString(s) {
			return nil, "", fmt.Errorf("invalid scope: %s", s)
		}
		if s == "openid" {
			hasOpenID = true
		}
	}
	if !hasOpenID {
		return nil, "", errors.New("the openid scope is required")
	}
	requirePKCE := typ == models.ClientTypePublic
	if body.RequirePKCE != nil {
		requirePKCE = *body.RequirePKCE || typ == models.ClientTypePublic
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	accessTTL := clamp(body.AccessTokenTTL, 3600, 60, 86400)
	idTTL := clamp(body.IDTokenTTL, accessTTL, 60, 86400)
	refreshTTL := clamp(body.RefreshTokenTTL, 2592000, 300, 7776000)
	logo := strings.TrimSpace(body.LogoURL)
	if logo != "" && !strings.HasPrefix(logo, "https://") && !strings.HasPrefix(logo, "http://") {
		return nil, "", errors.New("logo URL must be an http(s) URL")
	}

	client := &models.Client{
		TenantID:               a.currentTenantID(c),
		Name:                   name,
		Description:            strings.TrimSpace(body.Description),
		Type:                   typ,
		RedirectURIs:           body.RedirectURIs,
		PostLogoutRedirectURIs: body.PostLogoutRedirectURIs,
		AllowedScopes:          scopes,
		GrantTypes:             grants,
		RequirePKCE:            requirePKCE,
		Trusted:                body.Trusted,
		AccessTokenTTL:         accessTTL,
		IDTokenTTL:             idTTL,
		RefreshTokenTTL:        refreshTTL,
		Enabled:                enabled,
		LogoURL:                logo,
	}
	secret := ""
	if existing != nil {
		client.ClientID = existing.ClientID
		client.ClientSecretHash = existing.ClientSecretHash
		client.ID = existing.ID
		if typ == models.ClientTypeConfidential && existing.Type == models.ClientTypePublic {
			secret, _ = security.GenerateClientSecret()
			client.ClientSecretHash = security.HashClientSecret(secret)
			encrypted, encErr := a.Cipher.Encrypt(secret)
			if encErr != nil {
				return nil, "", encErr
			}
			client.ClientSecretEnc = encrypted
		}
	} else {
		suffix, err := security.RandomHex(12)
		if err != nil {
			return nil, "", err
		}
		client.ClientID = "app_" + suffix
		if typ == models.ClientTypeConfidential {
			secret, err = security.GenerateClientSecret()
			if err != nil {
				return nil, "", err
			}
			client.ClientSecretHash = security.HashClientSecret(secret)
			encrypted, encErr := a.Cipher.Encrypt(secret)
			if encErr != nil {
				return nil, "", encErr
			}
			client.ClientSecretEnc = encrypted
		}
	}
	return client, secret, nil
}

func (a *API) rotateClientSecret(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Application not found")
		return
	}
	client, err := a.Store.FindClientByID(c.Request.Context(), a.currentTenantID(c), id)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "Application not found")
		return
	}
	if client.Type != models.ClientTypeConfidential {
		fail(c, http.StatusBadRequest, "public_client", "Public clients do not use a client secret")
		return
	}
	secret, err := security.GenerateClientSecret()
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to rotate secret")
		return
	}
	encrypted, encErr := a.Cipher.Encrypt(secret)
	if encErr != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to rotate secret")
		return
	}
	if err := a.Store.UpdateClient(c.Request.Context(), a.currentTenantID(c), id, bson.M{
		"client_secret_hash": security.HashClientSecret(secret),
		"client_secret_enc":  encrypted,
	}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to rotate secret")
		return
	}
	a.audit(c, "client.secret.rotated", "client", client.ClientID, nil)
	ok(c, gin.H{"clientSecret": secret})
}

func (a *API) deleteClient(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Application not found")
		return
	}
	client, err := a.Store.FindClientByID(c.Request.Context(), a.currentTenantID(c), id)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "Application not found")
		return
	}
	ctx := c.Request.Context()
	if err := a.Store.DeleteClient(ctx, a.currentTenantID(c), id); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to delete application")
		return
	}
	_ = a.Store.RevokeClientRefreshTokens(ctx, a.currentTenantID(c), client.ClientID)
	a.audit(c, "client.deleted", "client", client.ClientID, map[string]interface{}{"name": client.Name})
	ok(c, gin.H{"ok": true})
}

func (a *API) listSessions(c *gin.Context) {
	sessions, err := a.Store.ListSessions(c.Request.Context(), a.currentTenantID(c), 200)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load sessions")
		return
	}
	ids := make([]primitive.ObjectID, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.UserID)
	}
	users, _ := a.Store.FindUsersByIDs(c.Request.Context(), ids)
	items := make([]gin.H, 0, len(sessions))
	for _, s := range sessions {
		u := users[s.UserID]
		items = append(items, gin.H{
			"id":        s.ID,
			"userId":    s.UserID,
			"email":     u.Email,
			"name":      u.Name,
			"ip":        s.IP,
			"userAgent": s.UserAgent,
			"mfa":       s.MFA,
			"createdAt": s.CreatedAt,
			"expiresAt": s.ExpiresAt,
			"authTime":  s.AuthTime,
		})
	}
	ok(c, gin.H{"items": items})
}

func (a *API) revokeSession(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	sess, err := a.Store.FindSession(ctx, id)
	if err != nil || sess.TenantID != a.currentTenantID(c) {
		fail(c, http.StatusNotFound, "not_found", "Session not found")
		return
	}
	if err := a.Store.RevokeSession(ctx, id); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to revoke session")
		return
	}
	a.audit(c, "session.revoked", "session", id, map[string]interface{}{"userId": sess.UserID.Hex()})
	ok(c, gin.H{"ok": true})
}

func (a *API) listAudit(c *gin.Context) {
	page, size := pagination(c)
	logs, total, err := a.Store.ListAudit(c.Request.Context(), store.AuditFilter{
		TenantID: a.currentTenantID(c),
		Action:   strings.TrimSpace(c.Query("action")),
		Search:   strings.TrimSpace(c.Query("q")),
		Page:     page,
		PageSize: size,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load audit log")
		return
	}
	ok(c, gin.H{"items": logs, "total": total, "page": page, "pageSize": size})
}

func (a *API) getSettings(c *gin.Context) {
	ok(c, gin.H{"tenant": a.currentTenant(c)})
}

type settingsRequest struct {
	Name     *string                `json:"name"`
	Branding *models.Branding       `json:"branding"`
	Settings *models.TenantSettings `json:"settings"`
	Domains  *[]string              `json:"domains"`
}

func (a *API) updateSettings(c *gin.Context) {
	var body settingsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	tenant := a.currentTenant(c)
	update := bson.M{}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" || len(name) > 120 {
			fail(c, http.StatusBadRequest, "invalid_name", "Name must be between 1 and 120 characters")
			return
		}
		update["name"] = name
	}
	if body.Branding != nil {
		b := *body.Branding
		if b.LogoURL != "" && !strings.HasPrefix(b.LogoURL, "https://") && !strings.HasPrefix(b.LogoURL, "http://") {
			fail(c, http.StatusBadRequest, "invalid_logo", "Logo must be an http(s) URL")
			return
		}
		if b.PrimaryColor != "" && !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(b.PrimaryColor) {
			fail(c, http.StatusBadRequest, "invalid_color", "Primary color must be a hex color like #4f46e5")
			return
		}
		if !models.ValidLoginTemplate(b.LoginTemplate) {
			fail(c, http.StatusBadRequest, "invalid_login_template", "Unknown login page template")
			return
		}
		update["branding"] = b
	}
	if body.Domains != nil {
		domains := []string{}
		for _, d := range *body.Domains {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" {
				continue
			}
			if !regexp.MustCompile(`^[a-z0-9.-]+\.[a-z]{2,}$`).MatchString(d) {
				fail(c, http.StatusBadRequest, "invalid_domain", "Invalid domain: "+d)
				return
			}
			domains = append(domains, d)
		}
		update["domains"] = domains
	}
	if body.Settings != nil {
		s := *body.Settings
		// SCIM stays managed by its dedicated endpoints.
		s.SCIMEnabled = tenant.Settings.SCIMEnabled
		s.SCIMTokenHash = tenant.Settings.SCIMTokenHash
		s.SCIMTokenPrefix = tenant.Settings.SCIMTokenPrefix
		if s.PasswordMinLength < 8 {
			s.PasswordMinLength = 8
		}
		if s.PasswordMinLength > 128 {
			s.PasswordMinLength = 128
		}
		if s.SessionTTLHours < 1 {
			s.SessionTTLHours = 1
		}
		if s.SessionTTLHours > 720 {
			s.SessionTTLHours = 720
		}
		if s.QuotaUsers < 0 {
			s.QuotaUsers = 0
		}
		if s.QuotaClients < 0 {
			s.QuotaClients = 0
		}
		domains := []string{}
		for _, d := range s.AllowedEmailDomains {
			d = strings.ToLower(strings.TrimSpace(d))
			if d != "" {
				domains = append(domains, d)
			}
		}
		s.AllowedEmailDomains = domains
		update["settings"] = s
	}
	if len(update) == 0 {
		ok(c, gin.H{"tenant": tenant})
		return
	}
	if err := a.Store.UpdateTenant(c.Request.Context(), tenant.ID, update); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to update settings")
		return
	}
	a.audit(c, "tenant.settings.updated", "tenant", tenant.ID.Hex(), map[string]interface{}{"fields": keysOf(update)})
	updated, _ := a.Store.FindTenantByID(c.Request.Context(), tenant.ID)
	ok(c, gin.H{"tenant": updated})
}

func (a *API) listTenants(c *gin.Context) {
	tenants, err := a.Store.ListTenants(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load tenants")
		return
	}
	items := []gin.H{}
	for i := range tenants {
		t := &tenants[i]
		users, _ := a.Store.CountUsersInTenant(c.Request.Context(), t.ID, "")
		clients, _ := a.Store.CountClients(c.Request.Context(), t.ID)
		items = append(items, gin.H{
			"id":        t.ID,
			"slug":      t.Slug,
			"name":      t.Name,
			"status":    t.Status,
			"domains":   t.Domains,
			"branding":  t.Branding,
			"users":     users,
			"clients":   clients,
			"createdAt": t.CreatedAt, "deletedAt": t.DeletedAt, "purgeAfter": t.PurgeAfter,
		})
	}
	ok(c, gin.H{"items": items})
}

type createTenantRequest struct {
	Slug       string          `json:"slug"`
	Name       string          `json:"name"`
	OwnerEmail string          `json:"ownerEmail"`
	OwnerName  string          `json:"ownerName"`
	Branding   models.Branding `json:"branding"`
}

func (a *API) createTenant(c *gin.Context) {
	var body createTenantRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	body.Slug = strings.ToLower(strings.TrimSpace(body.Slug))
	body.Name = strings.TrimSpace(body.Name)
	if !slugRE.MatchString(body.Slug) {
		fail(c, http.StatusBadRequest, "invalid_slug", "Slug must be 3-32 lowercase letters, numbers or hyphens")
		return
	}
	if body.Name == "" {
		fail(c, http.StatusBadRequest, "invalid_name", "Tenant name is required")
		return
	}
	ctx := c.Request.Context()
	ownerEmail := security.NormalizeEmail(body.OwnerEmail)
	if ownerEmail != "" && !validEmail(ownerEmail) {
		fail(c, http.StatusBadRequest, "invalid_email", "Enter a valid owner email address")
		return
	}
	tenant := &models.Tenant{
		Slug:     body.Slug,
		Name:     body.Name,
		Status:   models.StatusActive,
		Branding: body.Branding,
		Settings: models.DefaultSettings(),
	}
	if err := a.Store.CreateTenant(ctx, tenant); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			fail(c, http.StatusConflict, "tenant_exists", "A tenant with this slug already exists")
			return
		}
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create tenant")
		return
	}
	if err := a.Svc.ProvisionTenant(ctx, tenant); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to provision tenant roles")
		return
	}
	tempPassword := ""
	if ownerEmail != "" {
		email := ownerEmail
		if existing, err := a.Store.FindUserByEmail(ctx, email); err == nil {
			if err := a.Store.SetMembership(ctx, existing.ID, tenant.ID, []string{"owner"}); err != nil {
				fail(c, http.StatusInternalServerError, "internal_error", "Unable to assign tenant owner")
				return
			}
		} else {
			tempPassword = generateTempPassword()
			hash, _ := security.HashPassword(tempPassword)
			name := strings.TrimSpace(body.OwnerName)
			if name == "" {
				name = strings.Split(email, "@")[0]
			}
			owner := &models.User{
				Email:         email,
				EmailVerified: true,
				Username:      strings.Split(email, "@")[0],
				Name:          name,
				PasswordHash:  hash,
				Status:        models.StatusActive,
				Memberships: []models.Membership{{
					TenantID: tenant.ID,
					Roles:    []string{"owner"},
					JoinedAt: time.Now(),
				}},
			}
			if err := a.Store.CreateUser(ctx, owner); err != nil {
				fail(c, http.StatusInternalServerError, "internal_error", "Unable to create tenant owner")
				return
			}
		}
	}
	a.Svc.Audit(ctx, tenant.ID, a.currentBundle(c).User, "tenant.created", "tenant", tenant.ID.Hex(), a.clientIP(c), c.Request.UserAgent(),
		map[string]interface{}{"slug": tenant.Slug})
	c.JSON(http.StatusCreated, gin.H{"tenant": tenant, "tempPassword": tempPassword})
}

type updateTenantRequest struct {
	Name     *string          `json:"name"`
	Status   *string          `json:"status"`
	Domains  *[]string        `json:"domains"`
	Branding *models.Branding `json:"branding"`
}

func (a *API) updateTenant(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Tenant not found")
		return
	}
	var body updateTenantRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	update := bson.M{}
	if body.Name != nil && strings.TrimSpace(*body.Name) != "" {
		update["name"] = strings.TrimSpace(*body.Name)
	}
	if body.Status != nil {
		if *body.Status != models.StatusActive && *body.Status != models.StatusDisabled {
			fail(c, http.StatusBadRequest, "invalid_status", "Invalid tenant status")
			return
		}
		update["status"] = *body.Status
	}
	if body.Domains != nil {
		update["domains"] = *body.Domains
	}
	if body.Branding != nil {
		update["branding"] = *body.Branding
	}
	if len(update) == 0 {
		fail(c, http.StatusBadRequest, "empty_update", "Nothing to update")
		return
	}
	if err := a.Store.UpdateTenant(c.Request.Context(), id, update); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to update tenant")
		return
	}
	tenant, _ := a.Store.FindTenantByID(c.Request.Context(), id)
	a.Svc.Audit(c.Request.Context(), id, a.currentBundle(c).User, "tenant.updated", "tenant", id.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"tenant": tenant})
}

func (a *API) overview(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := a.currentTenantID(c)
	users, _ := a.Store.CountUsersInTenant(ctx, tenantID, "")
	activeUsers, _ := a.Store.CountUsersInTenant(ctx, tenantID, models.StatusActive)
	clients, _ := a.Store.CountClients(ctx, tenantID)
	roles, _ := a.Store.CountRoles(ctx, tenantID)
	sessions, _ := a.Store.CountActiveSessions(ctx, tenantID)
	events, _ := a.Store.CountRecentAudit(ctx, tenantID, time.Now().Add(-24*time.Hour))
	recentAudit, _, _ := a.Store.ListAudit(ctx, store.AuditFilter{TenantID: tenantID, Page: 1, PageSize: 8})
	recentUsers, _, _ := a.Store.ListUsers(ctx, store.UserFilter{TenantID: tenantID, Page: 1, PageSize: 5})
	userItems := make([]gin.H, 0, len(recentUsers))
	for i := range recentUsers {
		userItems = append(userItems, adminUserView(&recentUsers[i], tenantID))
	}
	ok(c, gin.H{
		"stats": gin.H{
			"users":       users,
			"activeUsers": activeUsers,
			"clients":     clients,
			"roles":       roles,
			"sessions":    sessions,
			"events24h":   events,
		},
		"recentAudit": recentAudit,
		"recentUsers": userItems,
	})
}

func normalizeRoles(roles []string) []string {
	if len(roles) == 0 {
		return []string{"member"}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, r := range roles {
		r = strings.ToLower(strings.TrimSpace(r))
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	if len(out) == 0 {
		return []string{"member"}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func keysOf(m bson.M) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && strings.EqualFold(addr.Address, email) && strings.Contains(email, ".")
}

func generateTempPassword() string {
	raw, err := security.RandomToken(12)
	if err != nil {
		return "ChangeMe-12345"
	}
	return "Tmp-" + raw
}

func validateRedirectURI(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("invalid redirect URI: %s", raw)
	}
	if u.Fragment != "" {
		return errors.New("redirect URIs must not contain a fragment")
	}
	switch u.Scheme {
	case "http", "https":
		if u.Host == "" {
			return fmt.Errorf("redirect URI must include a host: %s", raw)
		}
		if u.Scheme == "http" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
			return fmt.Errorf("http redirect URIs are only allowed for localhost: %s", raw)
		}
	case "javascript", "data", "file":
		return fmt.Errorf("unsupported redirect URI scheme: %s", u.Scheme)
	}
	return nil
}

func clamp(v, fallback, min, max int) int {
	if v == 0 {
		return fallback
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func (a *API) rotateSigningKey(c *gin.Context) {
	key, err := a.Svc.Tokens.Rotate(c.Request.Context(), a.currentTenantID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to rotate signing key")
		return
	}
	a.audit(c, "tenant.signing_key.rotated", "signing_key", key.Kid, nil)
	ok(c, gin.H{"kid": key.Kid})
}

func (a *API) resetUserMFA(c *gin.Context) {
	target, err := a.findTenantUser(c)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "User not found")
		return
	}
	ctx := c.Request.Context()
	tenantID := a.currentTenantID(c)
	if err := a.Store.UpdateUser(ctx, target.ID, bson.M{
		"mfa.enabled": false, "mfa.secret": "", "mfa.recovery_codes": []string{}, "mfa.enabled_at": nil,
	}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to reset MFA")
		return
	}
	_ = a.Store.DeleteAllWebAuthnCredentials(ctx, tenantID, target.ID)
	_ = a.Store.RevokeAllUserSessions(ctx, tenantID, target.ID)
	_ = a.Store.RevokeUserRefreshTokens(ctx, tenantID, target.ID)
	_ = a.Store.BumpTokenVersion(ctx, target.ID)
	a.audit(c, "user.mfa.reset", "user", target.ID.Hex(), nil)
	ok(c, gin.H{"ok": true})
}
