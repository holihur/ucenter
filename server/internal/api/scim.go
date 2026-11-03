package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
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

const (
	scimUserSchema  = "urn:ietf:params:scim:schemas:core:2.0:User"
	scimGroupSchema = "urn:ietf:params:scim:schemas:core:2.0:Group"
	scimListSchema  = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	scimErrorSchema = "urn:ietf:params:scim:api:messages:2.0:Error"
	scimPatchSchema = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
)

type scimName struct {
	Formatted  string `json:"formatted,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

type scimEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

type scimGroupRef struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	Ref     string `json:"$ref,omitempty"`
}

type scimRole struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
}

type scimMeta struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Location     string `json:"location,omitempty"`
	Version      string `json:"version,omitempty"`
}

type scimUserResource struct {
	Schemas     []string       `json:"schemas"`
	ID          string         `json:"id"`
	ExternalID  string         `json:"externalId,omitempty"`
	UserName    string         `json:"userName"`
	Name        scimName       `json:"name,omitempty"`
	DisplayName string         `json:"displayName,omitempty"`
	Emails      []scimEmail    `json:"emails,omitempty"`
	Active      bool           `json:"active"`
	Groups      []scimGroupRef `json:"groups,omitempty"`
	Roles       []scimRole     `json:"roles,omitempty"`
	Meta        scimMeta       `json:"meta"`
}

type scimGroupResource struct {
	Schemas     []string       `json:"schemas"`
	ID          string         `json:"id"`
	ExternalID  string         `json:"externalId,omitempty"`
	DisplayName string         `json:"displayName"`
	Members     []scimGroupRef `json:"members,omitempty"`
	Meta        scimMeta       `json:"meta"`
}

type scimListResponse struct {
	Schemas      []string      `json:"schemas"`
	TotalResults int           `json:"totalResults"`
	StartIndex   int           `json:"startIndex"`
	ItemsPerPage int           `json:"itemsPerPage"`
	Resources    []interface{} `json:"Resources"`
}

func scimError(c *gin.Context, status int, detail string) {
	c.Header("Content-Type", "application/scim+json")
	c.AbortWithStatusJSON(status, gin.H{
		"schemas": []string{scimErrorSchema},
		"detail":  detail,
		"status":  strconv.Itoa(status),
	})
}

func scimJSON(c *gin.Context, status int, body interface{}) {
	c.Header("Content-Type", "application/scim+json")
	c.JSON(status, body)
}

func (a *API) scimAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenant, err := a.Store.FindTenantBySlug(ctx, strings.ToLower(c.Param("slug")))
		if err != nil || tenant.Status != models.StatusActive {
			scimError(c, http.StatusNotFound, "Tenant not found")
			return
		}
		if !tenant.Settings.SCIMEnabled || tenant.Settings.SCIMTokenHash == "" {
			scimError(c, http.StatusUnauthorized, "SCIM provisioning is not enabled for this tenant")
			return
		}
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			scimError(c, http.StatusUnauthorized, "Bearer token is required")
			return
		}
		token := strings.TrimSpace(auth[7:])
		if subtle.ConstantTimeCompare([]byte(security.HashToken(token)), []byte(tenant.Settings.SCIMTokenHash)) != 1 {
			scimError(c, http.StatusUnauthorized, "Invalid SCIM bearer token")
			return
		}
		c.Set(ctxTenant, tenant)
		c.Set(ctxTenantID, tenant.ID)
		c.Next()
	}
}

func (a *API) scimServiceProviderConfig(c *gin.Context) {
	scimJSON(c, http.StatusOK, gin.H{
		"schemas":          []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"documentationUri": a.Cfg.BaseURL + "/docs/SCIM.md",
		"patch":            gin.H{"supported": true},
		"bulk":             gin.H{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":           gin.H{"supported": true, "maxResults": 200},
		"changePassword":   gin.H{"supported": false},
		"sort":             gin.H{"supported": false},
		"etag":             gin.H{"supported": true},
		"authenticationSchemes": []gin.H{{
			"type": "oauthbearertoken", "name": "OAuth Bearer Token", "description": "Tenant SCIM bearer token",
		}},
	})
}

func (a *API) scimSchemas(c *gin.Context) {
	scimJSON(c, http.StatusOK, gin.H{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": 2, "startIndex": 1, "itemsPerPage": 2,
		"Resources": []gin.H{
			{"id": scimUserSchema, "name": "User", "description": "User Account"},
			{"id": scimGroupSchema, "name": "Group", "description": "Group"},
		},
	})
}

func (a *API) scimPagination(c *gin.Context) (start, count int) {
	start, _ = strconv.Atoi(c.DefaultQuery("startIndex", "1"))
	count, _ = strconv.Atoi(c.DefaultQuery("count", "100"))
	if start < 1 {
		start = 1
	}
	if count < 0 {
		count = 0
	}
	if count > 200 {
		count = 200
	}
	return
}

type scimFilter struct {
	Attr  string
	Value string
}

func parseSCIMFilter(raw string) (*scimFilter, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Fields(raw)
	if len(parts) < 3 {
		return nil, fmt.Errorf("unsupported filter")
	}
	if !strings.EqualFold(parts[1], "eq") {
		return nil, fmt.Errorf("only the eq operator is supported")
	}
	value := strings.TrimSpace(strings.Join(parts[2:], " "))
	value = strings.Trim(value, `"`)
	return &scimFilter{Attr: parts[0], Value: value}, nil
}

func (a *API) scimRoles(ctx *gin.Context) (map[string]models.Role, error) {
	roles, err := a.Store.ListRoles(ctx.Request.Context(), a.currentTenantID(ctx))
	if err != nil {
		return nil, err
	}
	out := make(map[string]models.Role, len(roles))
	for _, role := range roles {
		out[role.Key] = role
	}
	return out, nil
}

func (a *API) scimUserView(c *gin.Context, user *models.User, roles map[string]models.Role) scimUserResource {
	membership := user.Membership(a.currentTenantID(c))
	groups := []scimGroupRef{}
	roleList := []scimRole{}
	if membership != nil {
		for _, key := range membership.Roles {
			if role, ok := roles[key]; ok {
				groups = append(groups, scimGroupRef{Value: role.ID.Hex(), Display: role.Name, Ref: a.scimLocation(c, "Groups", role.ID.Hex())})
				roleList = append(roleList, scimRole{Value: role.Key, Display: role.Name})
			}
		}
	}
	given, family := splitName(user.Name)
	active := user.Status == models.StatusActive
	return scimUserResource{
		Schemas: []string{scimUserSchema}, ID: user.ID.Hex(), ExternalID: user.ExternalID,
		UserName: user.Email, Name: scimName{Formatted: user.Name, GivenName: given, FamilyName: family},
		DisplayName: user.Name, Emails: []scimEmail{{Value: user.Email, Type: "work", Primary: true}},
		Active: active, Groups: groups, Roles: roleList,
		Meta: scimMeta{
			ResourceType: "User", Created: user.CreatedAt.Format(time.RFC3339), LastModified: user.UpdatedAt.Format(time.RFC3339),
			Location: a.scimLocation(c, "Users", user.ID.Hex()), Version: fmt.Sprintf(`W/"%d"`, user.UpdatedAt.Unix()),
		},
	}
}

func (a *API) scimGroupView(c *gin.Context, role *models.Role) scimGroupResource {
	users, _ := a.Store.ListUsersWithRole(c.Request.Context(), a.currentTenantID(c), role.Key)
	members := make([]scimGroupRef, 0, len(users))
	for _, user := range users {
		members = append(members, scimGroupRef{Value: user.ID.Hex(), Display: user.Email, Ref: a.scimLocation(c, "Users", user.ID.Hex())})
	}
	return scimGroupResource{
		Schemas: []string{scimGroupSchema}, ID: role.ID.Hex(), ExternalID: role.Key, DisplayName: role.Name, Members: members,
		Meta: scimMeta{
			ResourceType: "Group", Created: role.CreatedAt.Format(time.RFC3339), LastModified: role.UpdatedAt.Format(time.RFC3339),
			Location: a.scimLocation(c, "Groups", role.ID.Hex()), Version: fmt.Sprintf(`W/"%d"`, role.UpdatedAt.Unix()),
		},
	}
}

func (a *API) scimLocation(c *gin.Context, resource, id string) string {
	return fmt.Sprintf("%s/t/%s/scim/v2/%s/%s", a.Cfg.BaseURL, c.Param("slug"), resource, id)
}

func splitName(name string) (string, string) {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func (a *API) scimListUsers(c *gin.Context) {
	start, count := a.scimPagination(c)
	filter, err := parseSCIMFilter(c.Query("filter"))
	if err != nil {
		scimError(c, http.StatusBadRequest, err.Error())
		return
	}
	roles, _ := a.scimRoles(c)
	page := int64((start-1)/max(count, 1) + 1)
	pageSize := int64(count)
	if pageSize == 0 {
		pageSize = 200
	}
	var users []models.User
	var total int64
	if filter != nil && strings.EqualFold(filter.Attr, "userName") {
		user, err := a.Store.FindUserByEmail(c.Request.Context(), security.NormalizeEmail(filter.Value))
		if err == nil && user.Membership(a.currentTenantID(c)) != nil {
			users, total = []models.User{*user}, 1
		}
	} else if filter != nil && strings.EqualFold(filter.Attr, "externalId") {
		user, err := a.Store.FindUserByExternalID(c.Request.Context(), a.currentTenantID(c), filter.Value)
		if err == nil {
			users, total = []models.User{*user}, 1
		}
	} else if filter != nil && strings.EqualFold(filter.Attr, "active") {
		status := models.StatusActive
		if !strings.EqualFold(filter.Value, "true") {
			status = models.StatusDisabled
		}
		users, total, _ = a.Store.ListUsers(c.Request.Context(), store.UserFilter{TenantID: a.currentTenantID(c), Status: status, Page: page, PageSize: pageSize})
	} else {
		users, total, _ = a.Store.ListUsers(c.Request.Context(), store.UserFilter{TenantID: a.currentTenantID(c), Page: page, PageSize: pageSize})
	}
	resources := make([]interface{}, 0, len(users))
	for i := range users {
		resources = append(resources, a.scimUserView(c, &users[i], roles))
	}
	scimJSON(c, http.StatusOK, scimListResponse{Schemas: []string{scimListSchema}, TotalResults: int(total), StartIndex: start, ItemsPerPage: len(resources), Resources: resources})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type scimUserInput struct {
	Schemas     []string       `json:"schemas"`
	ExternalID  string         `json:"externalId"`
	UserName    string         `json:"userName"`
	Name        scimName       `json:"name"`
	DisplayName string         `json:"displayName"`
	Emails      []scimEmail    `json:"emails"`
	Active      *bool          `json:"active"`
	Groups      []scimGroupRef `json:"groups"`
	Roles       []scimRole     `json:"roles"`
}

type scimGroupInput struct {
	Schemas     []string       `json:"schemas"`
	ExternalID  string         `json:"externalId"`
	DisplayName string         `json:"displayName"`
	Members     []scimGroupRef `json:"members"`
}

type scimPatchOperation struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

type scimPatchRequest struct {
	Schemas    []string             `json:"schemas"`
	Operations []scimPatchOperation `json:"Operations"`
}

func decodeSCIM(c *gin.Context, out interface{}) bool {
	if err := json.NewDecoder(c.Request.Body).Decode(out); err != nil {
		scimError(c, http.StatusBadRequest, "Invalid SCIM JSON body")
		return false
	}
	return true
}

func (a *API) scimRoleKeys(c *gin.Context, roles []scimRole, groups []scimGroupRef) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, r := range roles {
		key := strings.TrimSpace(r.Value)
		if key == "" {
			continue
		}
		if _, err := a.Store.FindRoleByKey(c.Request.Context(), a.currentTenantID(c), key); err != nil {
			return nil, fmt.Errorf("role %q not found", key)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	for _, group := range groups {
		id, err := primitive.ObjectIDFromHex(group.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid group id %q", group.Value)
		}
		role, err := a.Store.FindRoleByID(c.Request.Context(), a.currentTenantID(c), id)
		if err != nil {
			return nil, fmt.Errorf("group %q not found", group.Value)
		}
		if !seen[role.Key] {
			seen[role.Key] = true
			out = append(out, role.Key)
		}
	}
	if len(out) == 0 {
		return []string{"member"}, nil
	}
	return out, nil
}

func scimUserName(in scimUserInput) string {
	if in.Name.Formatted != "" {
		return in.Name.Formatted
	}
	if in.DisplayName != "" {
		return in.DisplayName
	}
	if in.Name.GivenName != "" || in.Name.FamilyName != "" {
		return strings.TrimSpace(in.Name.GivenName + " " + in.Name.FamilyName)
	}
	return strings.Split(in.UserName, "@")[0]
}

func (a *API) scimCreateUser(c *gin.Context) {
	var in scimUserInput
	if !decodeSCIM(c, &in) {
		return
	}
	email := security.NormalizeEmail(in.UserName)
	if !validEmail(email) {
		scimError(c, http.StatusBadRequest, "userName must be a valid email address")
		return
	}
	ctx := c.Request.Context()
	tenantID := a.currentTenantID(c)
	if err := a.checkUserQuota(c); err != nil {
		scimError(c, http.StatusForbidden, err.Error())
		return
	}
	if existing, err := a.Store.FindUserByEmail(ctx, email); err == nil && existing.Membership(tenantID) != nil {
		scimError(c, http.StatusConflict, "userName already exists")
		return
	}
	if in.ExternalID != "" {
		if _, err := a.Store.FindUserByExternalID(ctx, tenantID, in.ExternalID); err == nil {
			scimError(c, http.StatusConflict, "externalId already exists")
			return
		}
	}
	roles, err := a.scimRoleKeys(c, in.Roles, in.Groups)
	if err != nil {
		scimError(c, http.StatusBadRequest, err.Error())
		return
	}
	password, _ := security.RandomToken(32)
	hash, _ := security.HashPassword(password)
	status := models.StatusActive
	if in.Active != nil && !*in.Active {
		status = models.StatusDisabled
	}
	user := &models.User{
		Email: email, EmailVerified: true, ExternalID: in.ExternalID,
		Username: strings.Split(email, "@")[0], Name: scimUserName(in), PasswordHash: hash,
		Status: status, Memberships: []models.Membership{{TenantID: tenantID, Roles: roles, JoinedAt: time.Now()}},
	}
	if err := a.Store.CreateUser(ctx, user); err != nil {
		if err == store.ErrDuplicate {
			scimError(c, http.StatusConflict, "userName already exists")
			return
		}
		scimError(c, http.StatusInternalServerError, "Unable to create user")
		return
	}
	roleMap, _ := a.scimRoles(c)
	scimJSON(c, http.StatusCreated, a.scimUserView(c, user, roleMap))
}

func (a *API) scimFindUser(c *gin.Context) (*models.User, bool) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		scimError(c, http.StatusNotFound, "User not found")
		return nil, false
	}
	user, err := a.Store.FindUserByID(c.Request.Context(), id)
	if err != nil || user.Membership(a.currentTenantID(c)) == nil {
		scimError(c, http.StatusNotFound, "User not found")
		return nil, false
	}
	return user, true
}

func (a *API) scimGetUser(c *gin.Context) {
	user, ok := a.scimFindUser(c)
	if !ok {
		return
	}
	roles, _ := a.scimRoles(c)
	scimJSON(c, http.StatusOK, a.scimUserView(c, user, roles))
}

func (a *API) scimUpdateUser(c *gin.Context) {
	user, ok := a.scimFindUser(c)
	if !ok {
		return
	}
	var in scimUserInput
	if !decodeSCIM(c, &in) {
		return
	}
	a.applySCIMUser(c, user, in)
}

func (a *API) applySCIMUser(c *gin.Context, user *models.User, in scimUserInput) {
	ctx := c.Request.Context()
	tenantID := a.currentTenantID(c)
	email := user.Email
	if strings.TrimSpace(in.UserName) != "" {
		email = security.NormalizeEmail(in.UserName)
		if !validEmail(email) {
			scimError(c, http.StatusBadRequest, "userName must be a valid email address")
			return
		}
		if existing, err := a.Store.FindUserByEmail(ctx, email); err == nil && existing.ID != user.ID {
			scimError(c, http.StatusConflict, "userName already exists")
			return
		}
	}
	if in.ExternalID != "" && in.ExternalID != user.ExternalID {
		if existing, err := a.Store.FindUserByExternalID(ctx, tenantID, in.ExternalID); err == nil && existing.ID != user.ID {
			scimError(c, http.StatusConflict, "externalId already exists")
			return
		}
	}
	update := bson.M{"email": email, "external_id": in.ExternalID}
	name := scimUserName(in)
	if name != "" {
		update["name"] = name
	}
	if in.Active != nil {
		if *in.Active {
			update["status"] = models.StatusActive
		} else {
			update["status"] = models.StatusDisabled
		}
	}
	if in.Roles != nil || in.Groups != nil {
		roles, err := a.scimRoleKeys(c, in.Roles, in.Groups)
		if err != nil {
			scimError(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.Store.SetMembership(ctx, user.ID, tenantID, roles); err != nil {
			scimError(c, http.StatusInternalServerError, "Unable to update roles")
			return
		}
	}
	if err := a.Store.UpdateUser(ctx, user.ID, update); err != nil {
		scimError(c, http.StatusInternalServerError, "Unable to update user")
		return
	}
	if in.Active != nil && !*in.Active {
		_ = a.Store.RevokeAllUserSessions(ctx, tenantID, user.ID)
		_ = a.Store.RevokeUserRefreshTokens(ctx, tenantID, user.ID)
		_ = a.Store.BumpTokenVersion(ctx, user.ID)
	}
	updated, _ := a.Store.FindUserByID(ctx, user.ID)
	roles, _ := a.scimRoles(c)
	a.Svc.Audit(ctx, tenantID, nil, "scim.user.updated", "user", user.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	scimJSON(c, http.StatusOK, a.scimUserView(c, updated, roles))
}

func (a *API) scimPatchUser(c *gin.Context) {
	user, ok := a.scimFindUser(c)
	if !ok {
		return
	}
	var req scimPatchRequest
	if !decodeSCIM(c, &req) {
		return
	}
	given, family := splitName(user.Name)
	in := scimUserInput{
		UserName: user.Email, ExternalID: user.ExternalID,
		Name: scimName{Formatted: user.Name, GivenName: given, FamilyName: family}, Active: boolPtr(user.Status == models.StatusActive),
	}
	roles, _ := a.scimRoles(c)
	if m := user.Membership(a.currentTenantID(c)); m != nil {
		for _, key := range m.Roles {
			if role, ok := roles[key]; ok {
				in.Groups = append(in.Groups, scimGroupRef{Value: role.ID.Hex(), Display: role.Name})
			}
		}
	}
	for _, op := range req.Operations {
		path := strings.ToLower(strings.TrimSpace(op.Path))
		switch path {
		case "active":
			var v bool
			if json.Unmarshal(op.Value, &v) == nil {
				in.Active = &v
			}
		case "username":
			_ = json.Unmarshal(op.Value, &in.UserName)
		case "externalid":
			_ = json.Unmarshal(op.Value, &in.ExternalID)
		case "displayname":
			_ = json.Unmarshal(op.Value, &in.DisplayName)
		case "name.formatted":
			_ = json.Unmarshal(op.Value, &in.Name.Formatted)
		case "name.givenname":
			_ = json.Unmarshal(op.Value, &in.Name.GivenName)
		case "name.familyname":
			_ = json.Unmarshal(op.Value, &in.Name.FamilyName)
		case "roles":
			var roleRefs []scimRole
			_ = json.Unmarshal(op.Value, &roleRefs)
			switch strings.ToLower(op.Op) {
			case "add":
				in.Roles = append(in.Roles, roleRefs...)
			case "remove":
				remove := map[string]bool{}
				for _, r := range roleRefs {
					remove[r.Value] = true
				}
				kept := []scimRole{}
				for _, r := range in.Roles {
					if !remove[r.Value] {
						kept = append(kept, r)
					}
				}
				in.Roles = kept
			case "replace":
				in.Roles = roleRefs
			}
		case "groups":
			var refs []scimGroupRef
			_ = json.Unmarshal(op.Value, &refs)
			switch strings.ToLower(op.Op) {
			case "add":
				in.Groups = append(in.Groups, refs...)
			case "remove":
				remove := map[string]bool{}
				for _, r := range refs {
					remove[r.Value] = true
				}
				kept := []scimGroupRef{}
				for _, r := range in.Groups {
					if !remove[r.Value] {
						kept = append(kept, r)
					}
				}
				in.Groups = kept
			case "replace":
				in.Groups = refs
			}
		default:
			scimError(c, http.StatusBadRequest, "Unsupported PATCH path: "+op.Path)
			return
		}
	}
	a.applySCIMUser(c, user, in)
}

func boolPtr(v bool) *bool { return &v }

func (a *API) scimDeleteUser(c *gin.Context) {
	user, ok := a.scimFindUser(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	tenantID := a.currentTenantID(c)
	_ = a.Store.RevokeAllUserSessions(ctx, tenantID, user.ID)
	_ = a.Store.RevokeUserRefreshTokens(ctx, tenantID, user.ID)
	_ = a.Store.DeleteUserIdentities(ctx, tenantID, user.ID)
	if len(user.Memberships) <= 1 && !user.PlatformAdmin {
		_ = a.Store.DeleteUser(ctx, user.ID)
	} else {
		_ = a.Store.RemoveMembership(ctx, user.ID, tenantID)
	}
	a.Svc.Audit(ctx, tenantID, nil, "scim.user.deleted", "user", user.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	c.Status(http.StatusNoContent)
}

func (a *API) scimListGroups(c *gin.Context) {
	start, count := a.scimPagination(c)
	roles, err := a.Store.ListRoles(c.Request.Context(), a.currentTenantID(c))
	if err != nil {
		scimError(c, http.StatusInternalServerError, "Unable to list groups")
		return
	}
	filter, err := parseSCIMFilter(c.Query("filter"))
	if err != nil {
		scimError(c, http.StatusBadRequest, err.Error())
		return
	}
	filtered := make([]models.Role, 0, len(roles))
	for _, role := range roles {
		if filter != nil {
			switch strings.ToLower(filter.Attr) {
			case "displayname":
				if !strings.EqualFold(role.Name, filter.Value) {
					continue
				}
			case "externalid":
				if role.Key != filter.Value {
					continue
				}
			}
		}
		filtered = append(filtered, role)
	}
	total := len(filtered)
	from := start - 1
	if from > total {
		from = total
	}
	to := from + count
	if to > total {
		to = total
	}
	items := filtered[from:to]
	resources := make([]interface{}, 0, len(items))
	for i := range items {
		resources = append(resources, a.scimGroupView(c, &items[i]))
	}
	scimJSON(c, http.StatusOK, scimListResponse{Schemas: []string{scimListSchema}, TotalResults: total, StartIndex: start, ItemsPerPage: len(resources), Resources: resources})
}

func scimGroupKey(displayName, externalID string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(externalID))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(displayName))
		key = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(key, "-")
		key = strings.Trim(key, "-")
	}
	if !keyRE.MatchString(key) {
		return "", fmt.Errorf("externalId/displayName cannot be converted to a valid role key")
	}
	return key, nil
}

func (a *API) scimSetGroupMembers(c *gin.Context, role *models.Role, members []scimGroupRef) error {
	ctx := c.Request.Context()
	tenantID := a.currentTenantID(c)
	desired := map[primitive.ObjectID]bool{}
	for _, member := range members {
		id, err := primitive.ObjectIDFromHex(member.Value)
		if err != nil {
			return fmt.Errorf("invalid member id")
		}
		if _, err := a.Store.FindUserByID(ctx, id); err != nil {
			return fmt.Errorf("member not found")
		}
		desired[id] = true
	}
	current, _ := a.Store.ListUsersWithRole(ctx, tenantID, role.Key)
	currentSet := map[primitive.ObjectID]bool{}
	for _, user := range current {
		currentSet[user.ID] = true
	}
	if role.Key == "owner" && len(desired) == 0 {
		return fmt.Errorf("the organization must keep at least one owner")
	}
	for _, user := range current {
		if desired[user.ID] {
			continue
		}
		m := user.Membership(tenantID)
		if m == nil {
			continue
		}
		roles := []string{}
		for _, key := range m.Roles {
			if key != role.Key {
				roles = append(roles, key)
			}
		}
		_ = a.Store.SetMembership(ctx, user.ID, tenantID, normalizeRoles(roles))
	}
	for id := range desired {
		if currentSet[id] {
			continue
		}
		user, err := a.Store.FindUserByID(ctx, id)
		if err != nil || user.Membership(tenantID) == nil {
			continue
		}
		m := user.Membership(tenantID)
		roles := append([]string{}, m.Roles...)
		if !contains(roles, role.Key) {
			roles = append(roles, role.Key)
		}
		_ = a.Store.SetMembership(ctx, user.ID, tenantID, roles)
	}
	return nil
}

func (a *API) scimCreateGroup(c *gin.Context) {
	var in scimGroupInput
	if !decodeSCIM(c, &in) {
		return
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		scimError(c, http.StatusBadRequest, "displayName is required")
		return
	}
	key, err := scimGroupKey(in.DisplayName, in.ExternalID)
	if err != nil {
		scimError(c, http.StatusBadRequest, err.Error())
		return
	}
	ctx := c.Request.Context()
	tenantID := a.currentTenantID(c)
	if _, err := a.Store.FindRoleByKey(ctx, tenantID, key); err == nil {
		scimError(c, http.StatusConflict, "group already exists")
		return
	}
	role := &models.Role{TenantID: tenantID, Key: key, Name: strings.TrimSpace(in.DisplayName), Permissions: []string{}}
	if err := a.Store.CreateRole(ctx, role); err != nil {
		scimError(c, http.StatusConflict, "group already exists")
		return
	}
	if err := a.scimSetGroupMembers(c, role, in.Members); err != nil {
		_ = a.Store.DeleteRole(ctx, tenantID, role.ID)
		scimError(c, http.StatusBadRequest, err.Error())
		return
	}
	a.Svc.Audit(ctx, tenantID, nil, "scim.group.created", "role", role.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	scimJSON(c, http.StatusCreated, a.scimGroupView(c, role))
}

func (a *API) scimFindGroup(c *gin.Context) (*models.Role, bool) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		scimError(c, http.StatusNotFound, "Group not found")
		return nil, false
	}
	role, err := a.Store.FindRoleByID(c.Request.Context(), a.currentTenantID(c), id)
	if err != nil {
		scimError(c, http.StatusNotFound, "Group not found")
		return nil, false
	}
	return role, true
}

func (a *API) scimGetGroup(c *gin.Context) {
	role, ok := a.scimFindGroup(c)
	if !ok {
		return
	}
	scimJSON(c, http.StatusOK, a.scimGroupView(c, role))
}

func (a *API) scimUpdateGroup(c *gin.Context) {
	role, ok := a.scimFindGroup(c)
	if !ok {
		return
	}
	var in scimGroupInput
	if !decodeSCIM(c, &in) {
		return
	}
	if err := a.scimSetGroupMembers(c, role, in.Members); err != nil {
		scimError(c, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.DisplayName) != "" && role.System == false {
		_ = a.Store.UpdateRole(c.Request.Context(), a.currentTenantID(c), role.ID, bson.M{"name": strings.TrimSpace(in.DisplayName)})
		role.Name = strings.TrimSpace(in.DisplayName)
	}
	scimJSON(c, http.StatusOK, a.scimGroupView(c, role))
}

func (a *API) scimPatchGroup(c *gin.Context) {
	role, ok := a.scimFindGroup(c)
	if !ok {
		return
	}
	var req scimPatchRequest
	if !decodeSCIM(c, &req) {
		return
	}
	current := a.scimGroupView(c, role)
	for _, op := range req.Operations {
		switch strings.ToLower(strings.TrimSpace(op.Path)) {
		case "displayname":
			var name string
			if json.Unmarshal(op.Value, &name) == nil && role.System == false {
				_ = a.Store.UpdateRole(c.Request.Context(), a.currentTenantID(c), role.ID, bson.M{"name": name})
				role.Name = name
			}
		case "members":
			var refs []scimGroupRef
			_ = json.Unmarshal(op.Value, &refs)
			switch strings.ToLower(op.Op) {
			case "add":
				current.Members = append(current.Members, refs...)
			case "remove":
				remove := map[string]bool{}
				for _, r := range refs {
					remove[r.Value] = true
				}
				kept := []scimGroupRef{}
				for _, m := range current.Members {
					if !remove[m.Value] {
						kept = append(kept, m)
					}
				}
				current.Members = kept
			case "replace":
				current.Members = refs
			}
		default:
			scimError(c, http.StatusBadRequest, "Unsupported PATCH path")
			return
		}
	}
	if err := a.scimSetGroupMembers(c, role, current.Members); err != nil {
		scimError(c, http.StatusBadRequest, err.Error())
		return
	}
	scimJSON(c, http.StatusOK, a.scimGroupView(c, role))
}

func (a *API) scimDeleteGroup(c *gin.Context) {
	role, ok := a.scimFindGroup(c)
	if !ok {
		return
	}
	if role.System {
		scimError(c, http.StatusBadRequest, "System groups cannot be deleted")
		return
	}
	users, _ := a.Store.ListUsersWithRole(c.Request.Context(), a.currentTenantID(c), role.Key)
	for _, user := range users {
		m := user.Membership(a.currentTenantID(c))
		if m == nil {
			continue
		}
		roles := []string{}
		for _, key := range m.Roles {
			if key != role.Key {
				roles = append(roles, key)
			}
		}
		_ = a.Store.SetMembership(c.Request.Context(), user.ID, a.currentTenantID(c), normalizeRoles(roles))
	}
	_ = a.Store.DeleteRole(c.Request.Context(), a.currentTenantID(c), role.ID)
	a.Svc.Audit(c.Request.Context(), a.currentTenantID(c), nil, "scim.group.deleted", "role", role.ID.Hex(), a.clientIP(c), c.Request.UserAgent(), nil)
	c.Status(http.StatusNoContent)
}

func (a *API) getSCIMSettings(c *gin.Context) {
	tenant := a.currentTenant(c)
	ok(c, gin.H{
		"enabled": tenant.Settings.SCIMEnabled, "tokenPrefix": tenant.Settings.SCIMTokenPrefix,
		"endpoint":       a.Cfg.BaseURL + "/t/" + tenant.Slug + "/scim/v2",
		"usersEndpoint":  a.Cfg.BaseURL + "/t/" + tenant.Slug + "/scim/v2/Users",
		"groupsEndpoint": a.Cfg.BaseURL + "/t/" + tenant.Slug + "/scim/v2/Groups",
	})
}

func (a *API) updateSCIMSettings(c *gin.Context) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if err := a.Store.UpdateTenant(c.Request.Context(), a.currentTenantID(c), bson.M{"settings.scim_enabled": body.Enabled}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to update SCIM settings")
		return
	}
	a.audit(c, "scim.settings.updated", "tenant", a.currentTenantID(c).Hex(), map[string]interface{}{"enabled": body.Enabled})
	ok(c, gin.H{"ok": true})
}

func (a *API) rotateSCIMToken(c *gin.Context) {
	raw, err := security.RandomToken(32)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to generate SCIM token")
		return
	}
	raw = "ucs_" + raw
	prefix := raw[:12]
	if err := a.Store.UpdateTenant(c.Request.Context(), a.currentTenantID(c), bson.M{
		"settings.scim_enabled": true, "settings.scim_token_hash": security.HashToken(raw), "settings.scim_token_prefix": prefix,
	}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to save SCIM token")
		return
	}
	a.audit(c, "scim.token.rotated", "tenant", a.currentTenantID(c).Hex(), nil)
	c.JSON(http.StatusCreated, gin.H{"token": raw, "prefix": prefix})
}
