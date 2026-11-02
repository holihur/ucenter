package models

type Permission struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

var AllPermissions = []Permission{
	{Key: "users:read", Label: "View users", Description: "List and inspect users in the tenant", Group: "Users"},
	{Key: "users:write", Label: "Manage users", Description: "Create, update, disable and delete users", Group: "Users"},
	{Key: "roles:read", Label: "View roles", Description: "List roles and their permissions", Group: "Roles"},
	{Key: "roles:write", Label: "Manage roles", Description: "Create, update and delete roles", Group: "Roles"},
	{Key: "clients:read", Label: "View applications", Description: "List OIDC clients and configuration", Group: "Applications"},
	{Key: "clients:write", Label: "Manage applications", Description: "Create, update, rotate secrets and delete OIDC clients", Group: "Applications"},
	{Key: "sessions:read", Label: "View sessions", Description: "List active sessions", Group: "Security"},
	{Key: "sessions:write", Label: "Revoke sessions", Description: "Revoke active user sessions", Group: "Security"},
	{Key: "audit:read", Label: "View audit log", Description: "Read tenant audit events", Group: "Security"},
	{Key: "tenant:read", Label: "View tenant settings", Description: "Read tenant configuration and branding", Group: "Tenant"},
	{Key: "tenant:write", Label: "Manage tenant settings", Description: "Update tenant configuration, branding and security policy", Group: "Tenant"},
	{Key: "platform:manage", Label: "Manage platform", Description: "Create and manage tenants across the platform", Group: "Platform"},
}

var PermissionSet = func() map[string]bool {
	m := make(map[string]bool, len(AllPermissions))
	for _, p := range AllPermissions {
		m[p.Key] = true
	}
	return m
}()

var TenantPermissions = func() []string {
	out := make([]string, 0, len(AllPermissions)-1)
	for _, p := range AllPermissions {
		if p.Key != "platform:manage" {
			out = append(out, p.Key)
		}
	}
	return out
}()

var SystemRoles = []Role{
	{Key: "owner", Name: "Owner", Description: "Full control of the tenant, including security settings", Permissions: TenantPermissions, System: true},
	{Key: "admin", Name: "Administrator", Description: "Manage users, roles, applications and security", Permissions: []string{
		"users:read", "users:write", "roles:read", "roles:write",
		"clients:read", "clients:write", "sessions:read", "sessions:write",
		"audit:read", "tenant:read",
	}, System: true},
	{Key: "member", Name: "Member", Description: "Standard tenant member with access to their own account", Permissions: []string{}, System: true},
}
