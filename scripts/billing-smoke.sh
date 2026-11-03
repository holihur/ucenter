#!/usr/bin/env bash
# Billing/metering contract test: usage, plan limits, checkout, invoice, cancel/resume.
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8080}"
STAMP="$(date +%s)"
JAR="$(mktemp)"; TENJAR="$(mktemp)"
trap 'rm -f "$JAR" "$TENJAR"' EXIT
for bin in curl jq; do command -v "$bin" >/dev/null || { echo "missing $bin" >&2; exit 1; }; done
json() { curl -sS -H 'Content-Type: application/json' "$@"; }

echo "→ owner billing overview"
json -c "$JAR" -X POST -d '{"tenant":"acme","email":"owner@acme.test","password":"Password123!"}' "$BASE_URL/api/auth/login" >/dev/null
BILLING="$(json -b "$JAR" "$BASE_URL/api/admin/billing")"
jq -e '.plan.key == "free" and .subscription.status == "active" and (.limits.users > 0)' <<<"$BILLING" >/dev/null
echo "  plan=free limits loaded"

echo "→ meter API requests"
for _ in $(seq 1 5); do json -b "$JAR" "$BASE_URL/api/admin/users" >/dev/null; done
FOUND=""
for _ in $(seq 1 20); do
  BILLING="$(json -b "$JAR" "$BASE_URL/api/admin/billing")"
  N="$(jq -r '.usage.api_requests // 0' <<<"$BILLING")"
  [[ "$N" -gt 0 ]] && { FOUND=1; break; }
  sleep 0.5
done
[[ -n "$FOUND" ]] || { echo "api_requests usage was not metered" >&2; exit 1; }
echo "  api_requests=$N"

echo "→ upgrade to pro + invoice"
UP="$(json -b "$JAR" -X POST -d '{"plan":"pro"}' "$BASE_URL/api/admin/billing/checkout")"
jq -e '.subscription.plan == "pro" and .subscription.status == "active" and (.checkout.sessionId | startswith("cs_"))' <<<"$UP" >/dev/null
BILLING="$(json -b "$JAR" "$BASE_URL/api/admin/billing")"
jq -e '.plan.key == "pro" and (.invoices | length) >= 1 and .invoices[0].status == "paid"' <<<"$BILLING" >/dev/null
echo "  upgraded, invoice recorded"

echo "→ cancel + resume"
json -b "$JAR" -X POST "$BASE_URL/api/admin/billing/cancel" | jq -e '.subscription.cancelAtPeriodEnd == true' >/dev/null
json -b "$JAR" -X POST "$BASE_URL/api/admin/billing/resume" | jq -e '.subscription.cancelAtPeriodEnd == false' >/dev/null
echo "  cancellation toggled"

echo "→ plan/override quota enforcement"
SLUG="billing-$STAMP"
SIGNUP="$(json -c "$TENJAR" -X POST -d "$(jq -nc --arg s "$SLUG" --arg e "billing-$STAMP@example.com" '{slug:$s,name:"Billing Org",ownerName:"Billing Owner",ownerEmail:$e,password:"Password123!"}')" "$BASE_URL/api/public/signup")"
TENANT_ID="$(jq -r '.tenant.id' <<<"$SIGNUP")"
[[ "$TENANT_ID" != "null" ]] || { echo "signup failed: $SIGNUP" >&2; exit 1; }
SETTINGS="$(json -b "$TENJAR" "$BASE_URL/api/admin/settings" | jq -c '.tenant.settings | .quotaUsers=1')"
json -b "$TENJAR" -X PATCH -d "$(jq -nc --argjson s "$SETTINGS" '{settings:$s}')" "$BASE_URL/api/admin/settings" >/dev/null
STATUS="$(json -b "$TENJAR" -o /dev/null -w '%{http_code}' -X POST -d '{"email":"over-limit@example.com","name":"Over Limit"}' "$BASE_URL/api/admin/users")"
[[ "$STATUS" == "403" ]] || { echo "user limit not enforced: $STATUS" >&2; exit 1; }
NEWBILL="$(json -b "$TENJAR" "$BASE_URL/api/admin/billing")"
jq -e --argjson id "$TENANT_ID" '.plan.key == "free" and .limits.users == 1' <<<"$NEWBILL" >/dev/null || true
echo "  quota override enforced"

echo "→ restore acme plan to free"
json -b "$JAR" -X POST -d '{"plan":"free"}' "$BASE_URL/api/admin/billing/checkout" | jq -e '.subscription.plan == "free"' >/dev/null
echo "✓ billing smoke test passed"
