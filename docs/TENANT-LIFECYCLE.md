# Tenant lifecycle

UCenter supports self-service tenant creation, hostname discovery, data export,
retention-based soft deletion and hard purge, plus per-tenant user/application quotas.

## Self-service signup

`POST /api/public/signup` is public when `ALLOW_SELF_SIGNUP=true` (default). It creates:

- a tenant with default settings and system roles
- an owner account and membership
- a session cookie

```json
{
  "slug": "acme",
  "name": "Acme Corporation",
  "ownerName": "Ada Lovelace",
  "ownerEmail": "ada@acme.com",
  "password": "a-strong-password"
}
```

`/signup` in the SPA provides the same flow. Slugs are permanent and used in the OIDC
issuer (`/t/{slug}`). Signup is rate-limited.

## Hostname discovery

Set tenant domains in **Settings → General** (one per line), then resolve a hostname:

```text
GET /api/public/tenant-by-host?domain=login.acme.com
```

The SPA login page can use `window.location.hostname`; the response contains public
branding and sign-in settings only. Point custom domains at the UCenter reverse proxy and
configure TLS there — UCenter itself does not terminate TLS.

## Export

```text
GET /api/admin/export
```

Requires `tenant:write`. Returns a JSON document with tenant configuration, sanitized
users, roles and clients (client secrets and password hashes are never exported).

## Soft delete and retention

Platform administrators can:

```text
DELETE /api/admin/tenants/{id}          # disable + schedule purge
POST   /api/admin/tenants/{id}/restore  # reactivate
POST   /api/admin/tenants/{id}/purge    # permanent, immediate
```

Soft delete sets `deleted_at` and `purge_after = now + 30 days`. Disabled/deleted tenants
cannot be used for login or OIDC. `cmd/purge-tenants` purges expired tenants in batch; the
admin purge endpoint performs an immediate hard purge (all tenant-scoped users, roles,
clients, sessions, keys, identities and audit events).

## Quotas

Tenant settings include `quotaUsers` and `quotaClients` (0 = unlimited). Quotas are
enforced for:

- admin invites and user creation
- self-registration
- SCIM user provisioning
- upstream IdP JIT provisioning
- MCP user creation
- application creation (admin and MCP)

Quota violations return `403 quota_exceeded`.

## Local verification

```bash
BASE_URL=http://localhost:8080 ./scripts/tenant-smoke.sh
```

The script covers signup, hostname resolution, export, quota enforcement, soft delete,
restore and purge.
