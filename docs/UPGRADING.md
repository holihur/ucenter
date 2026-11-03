# Upgrade and migration guide

## Before upgrading

1. Read the release notes and back up MongoDB:
   `mongodump --uri "$MONGO_URI" --db ucenter --gzip --archive=...`
2. Back up `ENCRYPTION_KEY` in a separate secret store.
3. Record the current image/commit and `docker compose config` output.
4. Upgrade a staging copy and run the smoke suite before production.

## Supported versions

- Go 1.24+ (build with `-tags nomsgpack`)
- MongoDB 6/7
- Node 20+/pnpm 10+ for frontend builds
- Docker Compose v2

## Application upgrade

```bash
git fetch && git checkout <release-tag>
make test                       # local equivalent of CI
docker compose build ucenter
docker compose up -d ucenter
docker compose run --rm --entrypoint ./seed ucenter   # only for demo data
```

Migrations run at startup via `EnsureIndexes`; they are idempotent and additive. The
server does not drop collections automatically. Apply the reverse-proxy/TLS changes from
[CUSTOM-DOMAINS.md](CUSTOM-DOMAINS.md) before adding domains.

## Data compatibility notes

### Encrypted secrets

Signing PEMs, client-secret copies and tenant IdP/SCIM/webhook secrets are AES-256-GCM
encrypted. Legacy plaintext values are migrated on write/access. Keep the same
`ENCRYPTION_KEY` across restarts; changing it without re-encryption makes existing secrets
unreadable. Plan a key-rotation migration rather than swapping the variable.

### Subscriptions and metering

Tenants created before billing have no `subscription`; they are normalized to Free/active
at read time and in quota checks. No backfill is required. New indexes are created for
`usage_events` and `invoices`.

### Identity data

- Upstream identities live in `user_identities`; enabling a provider does not rewrite
  existing users.
- SAML uses `idp_states`/`IdPState` fields; old rows expire via TTL.
- WebAuthn adds `webauthn_credentials`; TOTP remains available.
- SCIM groups map to RBAC roles by name; review mappings after a role rename.
- Soft-deleted tenants remain restorable for 30 days; purge is irreversible.

### Domain uniqueness

Domain-to-tenant uniqueness is enforced by the API. If an older database contains
duplicate `domains`, resolve them before deploying:

```javascript
db.tenants.aggregate([
  {$unwind: "$domains"},
  {$group: {_id: "$domains", tenants: {$push: "$slug"}, count: {$sum: 1}}},
  {$match: {count: {$gt: 1}}}
])
```

Remove the duplicate from all but one tenant via the platform API.

## Environment changes

New optional variables (`WEBHOOK_*`, `BILLING_PROVIDER`, `STRIPE_*`,
`ALLOW_SELF_SIGNUP`, `WEBAUTHN_*`) are documented in the README and `.env.example`.
Defaults are backward compatible: webhooks disabled, mock billing, mock mail.

## Rollback

Keep the previous image and database backup. Rolling back code is safe for additive
collections/indexes; a database restored from before the upgrade is also safe. Rotating
`ENCRYPTION_KEY` or purging tenants is not reversible and requires the corresponding
backup/key.

## Verification after upgrade

```bash
curl -fsS http://localhost:8080/readyz
BASE_URL=http://localhost:8080 ./scripts/oidc-smoke.sh
BASE_URL=http://localhost:8080 ./scripts/billing-smoke.sh
BASE_URL=http://localhost:8080 ./scripts/tenant-smoke.sh
```

Then run the Playwright suite or the GitHub Actions workflow.
