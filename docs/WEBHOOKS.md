# Webhooks

UCenter emits signed tenant events to administrator-configured HTTPS endpoints. The
dispatcher is included in the API process, retries failures with exponential backoff and
keeps every delivery in the audit-facing delivery log.

## Endpoint management

```text
GET    /api/admin/webhooks
POST   /api/admin/webhooks
PATCH  /api/admin/webhooks/{id}
DELETE /api/admin/webhooks/{id}
POST   /api/admin/webhooks/{id}/test
POST   /api/admin/webhooks/{id}/rotate-secret
GET    /api/admin/webhook-deliveries
```

Creating or rotating a webhook returns a `whsec_...` signing secret **once**. Only the
encrypted secret is stored.

## Events

See `models.WebhookEvents`; examples:

- `auth.login.success` / `auth.login.failed`
- `user.created` / `user.registered` / `user.updated` / `user.deleted`
- `user.password.reset`, `user.mfa.enabled`, `user.mfa.disabled`
- `oidc.authorize.approved`, `oidc.token.issued`
- `client.*`, `role.*`, `tenant.settings.updated`, `idp.updated`
- `scim.user.updated`

An endpoint with no selected events receives all events.

## Delivery format

Headers:

```text
X-UCenter-Event: user.created
X-UCenter-Delivery: <delivery id>
X-UCenter-Timestamp: <unix seconds>
X-UCenter-Signature: sha256=<hex hmac>
Content-Type: application/json
```

Signature:

```text
HMAC_SHA256(secret, timestamp + "." + raw_request_body)
```

Body:

```json
{
  "id": "event id",
  "type": "user.created",
  "tenant_id": "…",
  "created_at": "2026-01-01T00:00:00Z",
  "data": { "action": "user.created", "actor_email": "...", "...": "..." }
}
```

Verify signatures with a constant-time comparison and reject stale timestamps.

## Retry policy

Failures transition to `retrying` with exponential backoff
(`base * 2^(attempts-1)`), up to `WEBHOOK_MAX_ATTEMPTS` (default 5); then `dead`.
Deliveries are claimed with a lease so concurrent workers do not send the same delivery
simultaneously. The delivery log exposes attempts, response code, error and payload.

## SSRF protection

- Production requires HTTPS and globally routable IPs.
- Private, loopback, link-local, multicast and unspecified targets are rejected.
- Link-local/metadata addresses (including `169.254.169.254`) are always rejected.
- Redirects are disabled and the URL is revalidated before every delivery.
- `WEBHOOK_ALLOW_PRIVATE=true` allows loopback/private targets for local receivers. Use
  only in development/CI.

## Local verification

```bash
python3 scripts/webhook-receiver.py &
BASE_URL=http://localhost:8080 WEBHOOK_ALLOW_PRIVATE=true ./scripts/webhook-smoke.sh
```

The receiver deliberately fails the first request, so the smoke test proves retry,
event filtering, HMAC signature and delivery logging.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `WEBHOOK_WORKER_SECONDS` | `2` | Delivery polling interval |
| `WEBHOOK_RETRY_BASE_SECONDS` | `30` | Initial retry delay |
| `WEBHOOK_MAX_ATTEMPTS` | `5` | Maximum attempts before dead letter |
| `WEBHOOK_ALLOW_PRIVATE` | `false` | Allow local/private receivers (development only) |
