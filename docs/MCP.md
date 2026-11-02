# MCP access

UCenter exposes a **Model Context Protocol** server so AI clients such as Claude
Desktop, Cursor, VS Code extensions and custom agents can inspect and operate a tenant
directly.

## Endpoint

```
POST /mcp
```

- Transport: **Streamable HTTP** (stateless, JSON responses)
- Auth: `Authorization: Bearer ucm_...` (or `X-API-Key: ucm_...`)
- Scope: the API token is bound to one tenant and a subset of the owner's permissions
- Every tool call is written to the tenant audit log

## Create a token

1. Sign in to the admin console.
2. Open **MCP access**.
3. Click **Create token**, choose scopes and expiry (1–365 days).
4. Copy the `ucm_...` value immediately; it is shown only once.

Tokens are stored as SHA-256 hashes and can be revoked at any time. The effective
scopes are intersected with the owner's current permissions on every request, so
removing a role or disabling the user immediately limits the token.

## Client configuration

Claude Desktop (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "ucenter": {
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer ucm_your_token"
      }
    }
  }
}
```

Cursor (`.cursor/mcp.json`) uses the same shape. Restart the client after changing the
configuration.

## Tools

Read-only tools are registered only when the token has the matching scope.

| Tool | Scope | Description |
| --- | --- | --- |
| `whoami` | — | Current user, tenant, roles, scopes, issuer |
| `get_tenant` | `tenant:read` | Tenant configuration and branding |
| `get_discovery` | — | OIDC discovery document |
| `list_users` | `users:read` | Search/filter/paginate users |
| `get_user` | `users:read` | Get a user by ID or email |
| `create_user` | `users:write` | Create a user / add an existing identity to the tenant |
| `update_user` | `users:write` | Update name, username, status or roles |
| `list_roles` | `roles:read` | List roles and permissions |
| `create_role` | `roles:write` | Create a custom role |
| `list_clients` | `clients:read` | List OIDC clients |
| `create_client` | `clients:write` | Register an OIDC client; returns a one-time secret |
| `rotate_client_secret` | `clients:write` | Rotate a confidential client secret |
| `list_sessions` | `sessions:read` | List active sessions |
| `revoke_session` | `sessions:write` | Revoke one session (`confirm=true`) |
| `revoke_user_sessions` | `sessions:write` | Revoke all sessions for a user (`confirm=true`) |
| `list_audit` | `audit:read` | Search/paginate audit events |
| `list_tenants` | `platform:manage` | List all tenants (platform administrators only) |

## Protocol example

Initialize:

```bash
curl -s http://localhost:8080/mcp \
  -H "Authorization: Bearer $UCENTER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}'
```

List tools:

```bash
curl -s http://localhost:8080/mcp \
  -H "Authorization: Bearer $UCENTER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

Call a tool:

```bash
curl -s http://localhost:8080/mcp \
  -H "Authorization: Bearer $UCENTER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_users","arguments":{"pageSize":5}}}'
```

## Security notes

- Use HTTPS in production; bearer tokens are credentials.
- Create separate tokens per client and the minimum scopes needed.
- Destructive tools require `confirm=true` and are audited.
- Token revocation is immediate.
- MCP clients should not be given `platform:manage` unless they need cross-tenant access.
- The endpoint is stateless; do not rely on server-side MCP sessions.

## Limitations

- MCP tools cover administration; end-user self-service tools are not exposed.
- There is no MCP resource/prompt catalogue yet; tools are the supported interface.
- Rate limiting currently follows the HTTP server defaults; add a dedicated MCP rate limit
  before exposing it to untrusted clients.
