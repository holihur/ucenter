# Acceptance report

**Project:** UCenter — multi-tenant SSO & identity center
**Acceptance commit:** `58b73ba` (master)
**Accepted:** 2026-11-03
**CI run:** [41577613320](https://github.com/holihur/ucenter/actions/runs/41577613320) — **passed**
**Playwright:** 12/12 passed

## Scope delivered

| Phase | Delivered |
| --- | --- |
| P0 production readiness | Full 10-language i18n + Arabic RTL, SMTP/mock email flows, encrypted secrets, signing-key rotation/JWKS overlap, access-token revocation, health/readiness/Prometheus/structured logs, CI + Playwright, Redis rate limiting |
| P1 enterprise identity | SAML 2.0 SP, upstream OIDC/social/WeChat/Alipay/WeCom/DingTalk/Feishu/QQ/Weibo/Douyin/Apple/Google/Microsoft/GitHub/custom IdPs, SCIM 2.0, WebAuthn/passkeys, tenant lifecycle (signup/domain/export/quota/soft delete/restore/purge) |
| P2 platform | OpenAPI 3.1, TypeScript + Go SDKs, examples, HMAC webhooks with retries/dead letters, API-token scopes + delegated-admin boundaries, custom domains/white-label, usage metering/plans/quotas/mock + Stripe test billing |

## Verification snapshot

All checks below ran in GitHub Actions on a Mongo 7 service with `-tags nomsgpack`,
`MAIL_DRIVER=mock`, `BILLING_PROVIDER=mock` and local mock IdP/SAML/billing providers.

| Check | Result |
| --- | --- |
| `go test -tags nomsgpack ./...` | ✅ |
| `go vet -tags nomsgpack ./...` | ✅ |
| `pnpm typecheck` | ✅ |
| `pnpm build` | ✅ |
| `docker compose config` | ✅ |
| OpenAPI validation | ✅ |
| Go SDK tests | ✅ |
| TypeScript SDK build + live smoke | ✅ |
| Vite OIDC example build | ✅ |
| Seed + server boot + `/healthz`/`/readyz` | ✅ |
| OIDC authorization-code + PKCE smoke | ✅ |
| MCP initialize/tools/scope/revoke smoke | ✅ |
| Email invite/verify/reset/MFA-recovery smoke | ✅ |
| Billing metering/plan/invoice/quota smoke | ✅ |
| API scope/delegated-admin smoke | ✅ |
| Webhook retry/HMAC/SSRF smoke | ✅ |
| Tenant lifecycle/domain-conflict smoke | ✅ |
| SCIM smoke | ✅ |
| Mock SAML IdP login + Playwright | ✅ |
| Playwright full UI suite | ✅ 12/12 |

### Playwright coverage

1. Login, dark theme, Chinese preference persistence
2. All ten languages switch locale and text direction
3. Tenant auth-template differentiation
4. Login template gallery
5. Mock upstream IdP provisioning/linking
6. Mock SAML login/JIT
7. Hand-drawn auth template
8. Custom-domain resolution + white-label branding
9. Passkey enrollment, password+passkey login, removal
10. Arabic RTL
11. Billing plans/usage page
12. MCP access + mail outbox

## Success-criteria matrix

| Criterion | Evidence |
| --- | --- |
| 10-language i18n, RTL, no hardcoded text | All 10 locale files have the same 735 keys; Playwright checks every locale + Arabic RTL; source scan has no missing `t()` keys |
| Email invite/verify/reset/MFA recovery with mock | `scripts/email-smoke.sh`, mock outbox UI, CI pass |
| Token revocation, key rotation/JWKS overlap, encrypted secrets | Unit tests for cipher/rotation/token version; AES-GCM storage; `/admin/signing-keys/rotate`; userinfo/introspection revocation |
| CI/E2E/health/metrics/logs/rate limiting | Workflow above; `/healthz`, `/readyz`, Prometheus `/metrics`; JSON redacted logs; miniredis unit tests + Redis fallback |
| SAML/OIDC/social/SCIM/WebAuthn/tenant lifecycle | Go tests, local mock IdP/SAML, SCIM/tenant smokes, Playwright |
| OpenAPI/SDK/examples/webhooks/authz/custom domains/billing | OpenAPI 3.1, both SDK builds/smokes, docs, webhook/authz/billing/domain tests |
| All tests pass, compose valid, no unfinished markers | CI green; source/docs scan is clean; no unfinished code markers in shipped code |
| Acceptance report | This document |

## External integrations and credentials

No real third-party credentials are committed or required.

| Integration | Local/test implementation | Production switch |
| --- | --- | --- |
| Email | Mock outbox | `MAIL_DRIVER=smtp` + SMTP vars |
| SAML IdP | `cmd/mock-saml-idp` | Tenant SAML metadata |
| Social/WeChat/Alipay/etc. | Mock + configured adapters | Tenant provider credentials (AES-GCM encrypted) |
| Billing | Mock checkout/invoices | `BILLING_PROVIDER=stripe` + test/live key |
| Custom domains/TLS | Reverse-proxy docs | Operator DNS + certificates |

## Security posture

- Argon2id passwords, TOTP + hashed recovery codes, WebAuthn, exact redirect matching,
  PKCE for public clients, rotating refresh tokens, hashed one-time tokens.
- Per-tenant RSA keys; multi-key JWKS overlap; encrypted signing PEMs and client secrets.
- Tenant isolation, delegated-admin ceilings, token-scope intersection, SSRF-safe webhooks,
  exact-host domain resolution, quota enforcement and audit trails.
- CSP/security headers, trusted-proxy handling, rate limits with Redis fallback.

## Known limitations

See [README Known limitations](../README.md#known-limitations): external credentials are
adapter/mock-only, offline JWT resource servers cannot observe immediate revocation,
Stripe production reconciliation should use webhooks, custom-domain TLS is operator-managed,
and SCIM/SAML intentionally target the common enterprise profile rather than every optional
extension.

## Reproduction

```bash
git checkout 58b73ba
make test
docker compose config
BASE_URL=http://localhost:8080 ./scripts/oidc-smoke.sh
BASE_URL=http://localhost:8080 ./scripts/mcp-smoke.sh
BASE_URL=http://localhost:8080 ./scripts/email-smoke.sh
BASE_URL=http://localhost:8080 ./scripts/billing-smoke.sh
BASE_URL=http://localhost:8080 ./scripts/authz-smoke.sh
BASE_URL=http://localhost:8080 ./scripts/webhook-smoke.sh
BASE_URL=http://localhost:8080 ./scripts/tenant-smoke.sh
BASE_URL=http://localhost:8080 ./scripts/scim-smoke.sh
```

Then run the GitHub Actions workflow; it provisions MongoDB, seeds data, starts the server
and executes every check in the table above.

## Sign-off

The ordered P0 → P1 → P2 backlog and release acceptance are complete. No requirement was
deferred as an unfinished code marker; external-provider work is delivered as documented
adapters plus local mocks, as allowed by the project boundary.
