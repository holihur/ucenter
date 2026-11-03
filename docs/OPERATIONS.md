# Operations manual

## Topology

UCenter is one Go process serving the SPA, API, OIDC provider, MCP endpoint, webhook
worker and billing worker. MongoDB is required. Redis is optional for shared rate limits.
Terminate TLS at a reverse proxy (nginx, Caddy, an ingress controller) and forward `Host`
and `X-Forwarded-*`.

```text
client → TLS reverse proxy → ucenter:8080 → MongoDB
                                  ├─ Redis (optional, rate limits)
                                  └─ SMTP (optional, real email)
```

Set `TRUSTED_PROXIES` to the proxy addresses so audit IPs and rate limits are correct.
Set `COOKIE_SECURE=true` and `BASE_URL=https://sso.example.com` in production.

## Required production settings

| Variable | Guidance |
| --- | --- |
| `BASE_URL` | Stable public HTTPS origin; changing it changes OIDC issuers |
| `ENCRYPTION_KEY` | `openssl rand -base64 32`; back it up separately from MongoDB |
| `COOKIE_SECURE` | `true` behind HTTPS |
| `TRUSTED_PROXIES` | Proxy CIDRs/IPs |
| `ALLOW_SELF_SIGNUP` | `false` unless public tenant signup is desired |
| `MAIL_DRIVER` | `smtp` with `SMTP_*`; default `mock` only for local testing |
| `BILLING_PROVIDER` | `mock` unless Stripe test mode is configured |
| `WEBHOOK_ALLOW_PRIVATE` | Keep `false` in production (SSRF protection) |

Never commit `.env`, Stripe keys, SMTP passwords or tenant client secrets.

## Health, metrics and logs

- `GET /healthz` — liveness, no dependencies.
- `GET /readyz` — readiness (MongoDB ping).
- `GET /metrics` — Prometheus; protect with `METRICS_TOKEN` if exposed.
- Logs are structured JSON when `LOG_FORMAT=json`; sensitive fields are redacted.
- Prometheus compose profile: `docker compose --profile observability up -d`.

Alert on readiness failures, 5xx rate, login failures, webhook dead letters, MongoDB disk
and backup age.

## Backup and restore

Back up MongoDB (all tenant data, encrypted signing keys and secrets live there) and keep
`ENCRYPTION_KEY` in a separate secret store. Without the key, encrypted values cannot be
read.

```bash
# Logical backup
mongodump --uri "$MONGO_URI" --db ucenter --gzip --archive=/backup/ucenter-$(date +%F).gz

# Restore into an empty database
mongorestore --uri "$MONGO_URI" --gzip --archive=/backup/ucenter-2026-01-01.gz

# Filesystem-consistent option: mongodump inside a replica set or use Atlas snapshots.
```

Test restores quarterly. After restoring, start with the same `ENCRYPTION_KEY`; verify
`/readyz`, sign in, and confirm JWKS/OIDC discovery.

## Key rotation

`POST /api/admin/signing-keys/rotate` creates a new primary RSA key while previous keys
remain in JWKS for overlap. Rotate on a schedule or after suspected compromise. Client
secrets are encrypted with `ENCRYPTION_KEY`; rotating that key requires a planned
re-encryption migration, not an in-place env change.

## Email

`MAIL_DRIVER=mock` stores messages in the admin outbox for local verification.
`MAIL_DRIVER=smtp` uses `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM`,
`SMTP_FROM_NAME`, `SMTP_TLS`. Use STARTTLS on port 587 or implicit TLS on 465. Verify SPF,
DKIM and DMARC at the mail provider.

## Webhooks and billing workers

The API process also runs retry and usage workers. For multiple replicas, webhook
deliveries use leases so one worker sends each delivery; usage events are append-only.
Monitor dead-letter deliveries and webhook receiver latency. Billing mock is synchronous;
Stripe test mode uses the configured HTTP adapter.

## Tenant lifecycle

- Soft delete sets `deleted_at` and disables resolution; restore within 30 days.
- `cmd/purge-tenants` permanently removes expired tenants and their data. Run it from cron
  after reviewing the dry-run/log output.
- Quotas combine plan limits and tenant overrides. Platform purge is irreversible.

## Scaling and availability

- Run at least two API replicas behind the proxy with a shared MongoDB replica set.
- Configure Redis for shared rate limits; the server falls back to memory on Redis outage.
- Sticky sessions are not required: session cookies are opaque and session records live
  in MongoDB. WebAuthn challenge state is short-lived in MongoDB.
- Use MongoDB backups/snapshots and a replica set for production availability.
- The webhook/usage writers are asynchronous; size MongoDB write capacity accordingly.

## Incident checklist

1. Check `/readyz`, `/metrics` and structured logs.
2. Verify DNS/TLS/proxy and `TRUSTED_PROXIES`.
3. Check MongoDB connectivity/disk and Redis (if configured).
4. For suspected key compromise: rotate signing keys, revoke sessions/API tokens, rotate
   `ENCRYPTION_KEY` with a migration, then invalidate refresh tokens.
5. Restore from the latest verified backup only after preserving forensic logs.
