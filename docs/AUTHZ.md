# Authorization model

UCenter combines tenant RBAC, API-token scopes and delegated administration.

## Permissions

Permissions are strings such as `users:read`, `users:write`, `clients:write`,
`audit:read`, `tenant:write` and `platform:manage`. Roles are tenant-scoped sets of
permissions. The seeded roles are `owner`, `admin` and `member`; custom roles can be
created in **Admin → Roles**.

Every admin route is guarded by a permission:

| Route family | Permission |
| --- | --- |
| Users | `users:read` / `users:write` |
| Roles | `roles:read` / `roles:write` |
| Applications | `clients:read` / `clients:write` |
| Sessions | `sessions:read` / `sessions:write` |
| Audit | `audit:read` |
| Tenant settings/export | `tenant:read` / `tenant:write` |
| Tenants | `platform:manage` |

## API-token scopes

API tokens (`ucm_...`) are tenant-bound, hashed at rest and carry a subset of the
owner's permissions. They can authenticate admin API calls:

```http
Authorization: Bearer ucm_...
```

The effective scopes are **token scopes ∩ owner's current permissions**, recalculated on
every request. Removing a role or disabling the user immediately narrows the token.
Tokens cannot be used on session-only endpoints such as `/api/auth/me`, and they cannot be
used to switch tenants with `X-Tenant-ID`.

Create tokens in **Admin → MCP access** (the same tokens work for the MCP endpoint and the
admin API) or via `POST /api/admin/api-tokens`.

## Delegated administration

- A non-owner can only grant roles whose permissions are a subset of their own.
- Only an owner or platform admin can grant the `owner` role.
- Only an owner can modify another owner or reset their password.
- The last owner cannot be removed from the tenant.
- Tenant admins cannot use `platform:manage`; only platform administrators can manage
  tenants.

These boundaries are enforced server-side and covered by
`scripts/authz-smoke.sh`.

## Verification

```bash
BASE_URL=http://localhost:8080 ./scripts/authz-smoke.sh
```

The script verifies read-only token access, blocked writes and other resources, session
endpoint isolation, delegated admin role ceilings, owner-escalation rejection and token
revocation.
