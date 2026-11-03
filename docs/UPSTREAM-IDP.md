# Upstream identity providers

UCenter can act as the relying party for external identity providers while continuing to
issue its own OIDC tokens to downstream applications. Providers are configured per tenant
and only enabled providers appear on that tenant's login page.

## Routes

```text
GET  /api/public/tenants/{slug}/identity-providers
GET  /api/admin/identity-providers
PUT  /api/admin/identity-providers/{type}
DELETE /api/admin/identity-providers/{type}
GET  /t/{tenant}/idp/{type}/start
GET  /t/{tenant}/idp/{type}/callback
GET  /api/me/identities
DELETE /api/me/identities/{id}
```

- `start` accepts `return_to` (internal path), `request` (an in-flight OIDC authorization
  request) and `bind=1` (link to the signed-in user instead of logging in).
- `callback` consumes a one-time, hashed state value (10-minute TTL), exchanges the code,
  links or provisions the user, creates a UCenter session and redirects to the original
  OIDC request/consent flow.
- Admin endpoints require `tenant:read` / `tenant:write`. Secrets are encrypted with
  `ENCRYPTION_KEY` and returned masked.

## Built-in providers

| Type | Mode | Notes |
| --- | --- | --- |
| `wechat_open` | QR | WeChat Open Platform `snsapi_login`; openid + unionid |
| `wechat_mp` | redirect | WeChat Official Account H5; `snsapi_userinfo` |
| `wecom` | redirect | CorpID + agent + secret; userid/email/mobile |
| `alipay` | redirect | RSA2-signed `alipay.system.oauth.token` + user info |
| `dingtalk` | redirect | Generic OAuth2 preset, configurable endpoints |
| `feishu` | redirect | Generic OAuth2 preset |
| `qq` | redirect | Generic OAuth2 preset |
| `weibo` | redirect | Generic OAuth2 preset; `{uid}` substitution |
| `douyin` | redirect | Generic OAuth2 preset, JSON token response |
| `google` / `microsoft` | redirect | OIDC userinfo |
| `github` | redirect | OAuth2 userinfo |
| `apple` | redirect | OIDC; configure the Apple client secret JWT |
| `custom_oauth2` | redirect | Any OAuth2 provider |
| `custom_oidc` | redirect | Any OIDC provider |
| `mock` | redirect | Local test provider |

Generic providers accept `authorizeUrl`, `tokenUrl`, `userinfoUrl`, `clientId`,
`clientSecret`, `scopes` and field mappings (`userIdField`, `emailField`, `nameField`,
`avatarField`). Token transport options (`tokenMethod`, `tokenFormat`, `authStyle`,
`tokenField`, `clientIdParam`, `clientSecretParam`) are stored in the provider config and
can be adjusted for providers with non-standard token APIs.

## Account linking and provisioning

- Existing `(tenant, provider, subject)` or `(tenant, provider, union_id)` links log the
  user in directly.
- If the provider returns an email that matches an existing tenant member, linking only
  happens when **Trust provider email** is enabled. Otherwise the user must sign in with
  their password and link the provider from **Account → Linked sign-in methods**.
- With **Auto create users** enabled, a new member is provisioned with the configured
  default roles. WeChat/Alipay identities without an email receive a non-routable
  `@<tenant>.idp.local` address and must bind a real email/phone later.
- Users with TOTP MFA enabled cannot bypass MFA through a social provider; they must use
  password + TOTP.
- Unlinking is available from the account page. Ensure another sign-in method exists first.

## Admin configuration

Open **Admin → Identity providers**, choose a provider, fill the generated callback URL
into the provider console, and save:

```text
https://id.example.com/t/acme/idp/wechat_open/callback
https://id.example.com/t/acme/idp/alipay/callback
```

Enable only the providers the tenant wants. The login page fetches the public provider list
and shows nothing when no provider is configured/enabled.

## Security notes

- One-time `state` is stored as a SHA-256 hash and consumed atomically.
- Provider secrets and Alipay private keys are AES-256-GCM encrypted at rest.
- Auto-linking by email is opt-in and only for providers whose email is trusted.
- All provider logins, link/unlink events and provisioning are audited.
- Real provider credentials are not committed. Use the `mock` provider or local test
  credentials for development and CI.

## Testing

The Playwright suite configures the `mock` provider, performs an upstream login, verifies
JIT provisioning and the linked-account UI, then cleans up. Provider URL construction and
Alipay RSA2 signing are covered by Go unit tests.
