# Usage metering and billing

UCenter ships a plan catalog, tenant subscriptions, monthly usage metering, plan quotas
and a provider abstraction. The default provider is **mock**; a Stripe test-mode adapter
can be enabled with runtime credentials.

## Plans

| Plan | Monthly | Users | Applications | API requests/month |
| --- | ---: | ---: | ---: | ---: |
| Free | $0 | 50 | 10 | 100,000 |
| Pro | $29 | 1,000 | 100 | 2,000,000 |
| Enterprise | $99 | 10,000 | 1,000 | 20,000,000 |

Limits live in `server/internal/billing/billing.go`. Tenant `quotaUsers` /
`quotaClients` overrides can only lower a plan limit, never raise it. A quota of `0`
means "use the plan limit".

## Metering

Admin API requests are metered as `api_requests`; successful password logins are
`logins`; token issuance is `token_requests`. Events are stored in `usage_events` with a
tenant, metric, quantity and timestamp, and aggregated for the current UTC month.
Recording is asynchronous and never blocks the request path.

Additional metrics can be recorded with `recordUsage(tenantID, metric, quantity, meta)`.

## Subscription lifecycle

`GET /api/admin/billing` returns the normalized subscription, plan, limits, usage, counts
and invoices. Legacy tenants without a subscription are treated as Free/active and a
current monthly period is synthesized.

```text
POST /api/admin/billing/checkout   {"plan":"pro"}
POST /api/admin/billing/cancel
POST /api/admin/billing/resume
GET  /api/admin/billing/invoices
```

The mock provider completes checkout immediately, records a paid invoice for paid plans and
returns a local success URL. Cancellation sets `cancelAtPeriodEnd`; resume clears it.

## Stripe test adapter

Set:

```bash
BILLING_PROVIDER=stripe
STRIPE_SECRET_KEY=sk_test_...        # never commit
STRIPE_SUCCESS_URL=https://app.example/admin/billing?checkout=success
STRIPE_CANCEL_URL=https://app.example/admin/billing?checkout=cancel
```

The adapter creates a Stripe Checkout Session (subscription mode, inline monthly price)
and calls the Stripe cancel API. It is covered by an `httptest` unit test with a dummy key;
no real key is required or committed. For production reconciliation, confirm checkout via
Stripe webhooks before granting paid limits.

## Quota behavior

Creating users, clients, SCIM users and JIT IdP users all pass through the same quota
checks. When the effective limit is reached:

- `POST /api/admin/users` → `403 quota_exceeded`
- client creation → `403 quota_exceeded`
- SCIM / JIT provisioning → rejected with the same condition

`scripts/billing-smoke.sh` verifies metering, upgrade/invoice, cancel/resume and a
tenant quota override.

## Verification

```bash
BASE_URL=http://localhost:8080 ./scripts/billing-smoke.sh
```

No real payment credentials are used in CI.
