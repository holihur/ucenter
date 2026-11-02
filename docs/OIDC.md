# OIDC reference

UCenter implements the OpenID Connect authorization code flow with PKCE on top of
OAuth 2.0. Every tenant is an independent issuer, so the same deployment can serve
multiple brands or customers with isolated keys, clients and users.

## Discovery

```
GET {BASE_URL}/t/{tenant}/.well-known/openid-configuration
GET {BASE_URL}/t/{tenant}/oidc/jwks
```

Example issuer: `https://id.example.com/t/acme`.

The discovery document advertises:

| Field | Value |
| --- | --- |
| `issuer` | `{BASE_URL}/t/{tenant}` |
| `authorization_endpoint` | `{issuer}/oidc/authorize` |
| `token_endpoint` | `{issuer}/oidc/token` |
| `userinfo_endpoint` | `{issuer}/oidc/userinfo` |
| `jwks_uri` | `{issuer}/oidc/jwks` |
| `end_session_endpoint` | `{issuer}/oidc/end_session` |
| `revocation_endpoint` | `{issuer}/oidc/revoke` |
| `introspection_endpoint` | `{issuer}/oidc/introspect` |
| `response_types_supported` | `code` |
| `grant_types_supported` | `authorization_code`, `refresh_token`, `client_credentials` |
| `token_endpoint_auth_methods_supported` | `client_secret_basic`, `client_secret_post`, `none` |
| `code_challenge_methods_supported` | `S256`, `plain` |
| `id_token_signing_alg_values_supported` | `RS256` |

## Scopes

| Scope | Grants |
| --- | --- |
| `openid` | Required. Issues an ID token. |
| `profile` | `name`, `preferred_username`, `picture` |
| `email` | `email`, `email_verified` |
| `roles` | Tenant role keys in the ID/access token and userinfo |
| `offline_access` | Issues a rotating refresh token |

Scopes must be enabled on the client. Unknown scopes are rejected at the authorization
endpoint with `invalid_scope`.

## Authorization request

```
GET /t/acme/oidc/authorize
  ?client_id=app_xxxxxxxx
  &redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback
  &response_type=code
  &scope=openid%20profile%20email%20offline_access%20roles
  &state=<opaque>
  &nonce=<opaque>
  &code_challenge=<S256>
  &code_challenge_method=S256
```

Optional parameters:

| Parameter | Behaviour |
| --- | --- |
| `prompt=login` | Forces re-authentication even if a session exists |
| `prompt=none` | Never shows UI; returns `login_required` or `consent_required` |
| `login_hint` | Pre-fills the email field on the login page |

If the user is not signed in, the browser is redirected to the SPA login page. After
login, the SPA calls `POST /api/auth/request/{id}/complete`. The server either redirects
back to the client with a code or asks for consent. Trusted clients and previously
approved scopes skip the consent screen.

Redirect URIs must match an entry registered on the client exactly. `http` is only
allowed for `localhost`, `127.0.0.1` and `::1`; use HTTPS everywhere else.

## Token exchange

```bash
curl -u "$CLIENT_ID:$CLIENT_SECRET" \
  -X POST "$ISSUER/oidc/token" \
  --data-urlencode "grant_type=authorization_code" \
  --data-urlencode "code=$CODE" \
  --data-urlencode "redirect_uri=$REDIRECT_URI" \
  --data-urlencode "code_verifier=$VERIFIER"
```

Response:

```json
{
  "access_token": "eyJ...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "id_token": "eyJ...",
  "refresh_token": "1P...",
  "scope": "openid profile email offline_access roles"
}
```

Authorization codes are single-use and expire after five minutes. The `redirect_uri`
and PKCE verifier must match the authorization request.

### Refresh

```bash
curl -u "$CLIENT_ID:$CLIENT_SECRET" \
  -X POST "$ISSUER/oidc/token" \
  --data-urlencode "grant_type=refresh_token" \
  --data-urlencode "refresh_token=$REFRESH_TOKEN"
```

Refresh tokens rotate: the old token is revoked and a new one is returned. Reusing the
old token returns `invalid_grant`. Refresh fails if the underlying session was revoked or
the user was removed/disabled.

### Client credentials

Confidential clients with the `client_credentials` grant can request a machine token:

```bash
curl -u "$CLIENT_ID:$CLIENT_SECRET" \
  -X POST "$ISSUER/oidc/token" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "scope=openid"
```

The subject is `client:{client_id}`; no ID token or refresh token is issued.

## Userinfo

```bash
curl -H "Authorization: Bearer $ACCESS_TOKEN" "$ISSUER/oidc/userinfo"
```

Response fields depend on the granted scopes:

```json
{
  "sub": "6ae819d8fdc5ac91ca2fea8b",
  "tenant_id": "6ae819d7fdc5ac91ca2fea7b",
  "updated_at": 1793595864,
  "name": "Olivia Owner",
  "preferred_username": "owner",
  "email": "owner@acme.test",
  "email_verified": true,
  "roles": ["owner"]
}
```

## ID token claims

| Claim | Description |
| --- | --- |
| `iss` | Tenant issuer URL |
| `sub` | Stable user ID (MongoDB ObjectID hex) |
| `aud` | Client ID |
| `exp`, `iat`, `nbf` | Standard timestamps |
| `jti` | Unique token ID |
| `auth_time` | Unix time of the authentication event |
| `nonce` | Echo of the authorization request nonce |
| `amr` | Authentication methods (`pwd`, optionally `otp`) |
| `sid` | Session identifier (hash) |
| `tenant_id` | Tenant ObjectID hex |
| `email`, `email_verified` | Only with `email` scope |
| `name`, `preferred_username`, `picture` | Only with `profile` scope |
| `roles` | Only with `roles` scope |

Access tokens are JWTs signed with the same tenant key and include `token_type: "access"`,
`client_id`, `scope`, `sid`, `tenant_id` and the granted claims. They can be validated
locally against the tenant JWKS.

## Revocation and introspection

```bash
# Revoke a refresh token
curl -u "$CLIENT_ID:$CLIENT_SECRET" -X POST "$ISSUER/oidc/revoke" \
  --data-urlencode "token=$REFRESH_TOKEN"

# Introspect an access or refresh token
curl -u "$CLIENT_ID:$CLIENT_SECRET" -X POST "$ISSUER/oidc/introspect" \
  --data-urlencode "token=$ACCESS_TOKEN"
```

Revocation always returns `200` for valid client credentials. Access tokens are
stateless; they remain valid until expiry. Revoking a refresh token prevents further
refreshes.

## RP-initiated logout

```
GET /t/acme/oidc/end_session
  ?id_token_hint=<id_token>
  &post_logout_redirect_uri=https%3A%2F%2Fapp.example.com
  &state=<opaque>
```

The session cookie is cleared and the user's refresh tokens are revoked.
`post_logout_redirect_uri` must be registered on the client identified by `client_id` or
the `id_token_hint` audience. Without a registered post-logout URI the endpoint returns
`{"ok":true}`.

## Error responses

Token endpoint errors follow RFC 6749:

```json
{
  "error": "invalid_grant",
  "error_description": "PKCE verification failed"
}
```

Common codes: `invalid_request`, `invalid_client`, `invalid_grant`, `invalid_scope`,
`unsupported_grant_type`, `login_required`, `consent_required`, `access_denied`.

## Testing checklist

1. Discovery returns the expected issuer and endpoints.
2. JWKS contains an RSA key with the `kid` used by the ID token.
3. Authorize with PKCE reaches the login page and returns a code.
4. Token exchange returns access, ID and refresh tokens.
5. Userinfo returns `sub` and scope-appropriate claims.
6. Refresh rotates the token and rejects replay.
7. Logout clears the session and subsequent userinfo/refresh attempts fail.
8. A client from another tenant cannot use this tenant's issuer or codes.

Run the included script for an automated version:

```bash
BASE_URL=http://localhost:8080 ./scripts/oidc-smoke.sh
```
