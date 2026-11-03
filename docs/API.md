# API guide

UCenter exposes four API surfaces:

| Surface | Base path | Authentication |
| --- | --- | --- |
| Admin/console | `/api/admin` | Session cookie **or** `Authorization: Bearer ucm_...` |
| Account | `/api/account`, `/api/auth` | Session cookie |
| OIDC provider | `/t/{tenant}` | OAuth 2.0 / OIDC grants |
| SCIM 2.0 | `/t/{tenant}/scim/v2` | Tenant SCIM bearer token (`ucs_...`) |
| MCP | `/mcp` | MCP/API bearer token (`ucm_...`) |

The authoritative schema is [openapi.yaml](openapi.yaml) (OpenAPI 3.1). Generated clients:
TypeScript in `sdk/typescript`, Go in `sdk/go`, plus `examples/oidc-vite`.

## Conventions

- JSON requests/responses unless noted. Errors use
  `{"error":{"code":"...","message":"..."}}`.
- List endpoints use `page` and `pageSize` (maximum 200) and return
  `{items,total,page,pageSize}` unless documented otherwise.
- Times are RFC 3339 UTC strings. IDs are MongoDB ObjectID hex strings.
- `X-Tenant-ID` switches the active tenant for a platform administrator; it is ignored for
  tenant-scoped API tokens.
- Admin write routes require the permission listed in [AUTHZ.md](AUTHZ.md). API-token
  scopes are intersected with the owner's current permissions.

## Admin examples

```bash
# Password login (stores the uc_session cookie)
curl -c jar -H 'Content-Type: application/json' \
  -d '{"tenant":"acme","email":"owner@acme.test","password":"Password123!"}' \
  http://localhost:8080/api/auth/login

# Read-only API token
TOKEN=$(curl -b jar -H 'Content-Type: application/json' \
  -d '{"name":"readonly","scopes":["users:read"]}' \
  http://localhost:8080/api/admin/api-tokens | jq -r .token)

curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/admin/users
```

## OIDC

Discovery: `GET /t/{tenant}/.well-known/openid-configuration`
Authorization: `GET /t/{tenant}/oidc/authorize` (PKCE required for public clients)
Token: `POST /t/{tenant}/oidc/token`
UserInfo: `GET /t/{tenant}/oidc/userinfo`
JWKS: `GET /t/{tenant}/oidc/jwks`
Revocation/introspection/end-session are documented in [OIDC.md](OIDC.md).

## Provisioning and events

- SCIM endpoints and filtering: [SCIM.md](SCIM.md)
- Upstream federation: [UPSTREAM-IDP.md](UPSTREAM-IDP.md)
- Webhook signatures and retries: [WEBHOOKS.md](WEBHOOKS.md)
- Billing/usage: [BILLING.md](BILLING.md)

## SDK snippets

```ts
const client = new UCenterClient({ baseURL, token: process.env.UCENTER_TOKEN });
const users = await client.listUsers({ pageSize: 50 });
```

```go
c := ucenter.New(ucenter.Options{BaseURL: baseURL, Token: token})
users, err := c.ListUsers(ctx, url.Values{"pageSize": {"50"}})
```

See the SDK READMEs for the full method list.
