#!/usr/bin/env bash
# Tenant lifecycle smoke test: self-signup, domain resolution, export, quota,
# soft delete/restore/purge.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
STAMP="$(date +%s)"
SLUG="lifecycle-$STAMP"
DOMAIN="lifecycle-$STAMP.test"
OWNER="owner-$STAMP@example.com"
JAR="$(mktemp)"
PLATJAR="$(mktemp)"
trap 'rm -f "$JAR" "$PLATJAR"' EXIT

for bin in curl jq; do command -v "$bin" >/dev/null || { echo "missing $bin" >&2; exit 1; }; done
json() { curl -sS -H 'Content-Type: application/json' "$@"; }

echo "→ self-service signup"
SIGNUP="$(json -c "$JAR" -X POST -d "$(jq -nc --arg s "$SLUG" --arg n "Lifecycle Org" --arg e "$OWNER" '{slug:$s,name:$n,ownerName:"Lifecycle Owner",ownerEmail:$e,password:"Password123!"}')" "$BASE_URL/api/public/signup")"
TENANT_ID="$(jq -r '.tenant.id' <<<"$SIGNUP")"
[[ "$TENANT_ID" != "null" ]] || { echo "signup failed: $SIGNUP" >&2; exit 1; }
echo "  tenant=$TENANT_ID"

echo "→ host resolution"
json -b "$JAR" -X PATCH -d "$(jq -nc --arg d "$DOMAIN" '{domains:[$d]}')" "$BASE_URL/api/admin/settings" >/dev/null
RESOLVED="$(json -G --data-urlencode "domain=$DOMAIN" "$BASE_URL/api/public/tenant-by-host")"
jq -e --arg s "$SLUG" '.slug == $s' <<<"$RESOLVED" >/dev/null

echo "→ export"
EXPORT="$(json -b "$JAR" "$BASE_URL/api/admin/export")"
jq -e --arg s "$SLUG" '.tenant.slug == $s and (.users | length) >= 1' <<<"$EXPORT" >/dev/null

echo "→ quota"
TENANT_SETTINGS="$(json -b "$JAR" "$BASE_URL/api/admin/settings")"
SETTINGS="$(jq -c '.tenant.settings | .quotaUsers=1' <<<"$TENANT_SETTINGS")"
json -b "$JAR" -X PATCH -H 'Content-Type: application/json' -d "$(jq -nc --argjson s "$SETTINGS" '{settings:$s}')" "$BASE_URL/api/admin/settings" >/dev/null
STATUS="$(json -b "$JAR" -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{"email":"quota-blocked@example.com","name":"Blocked","roles":["member"]}' "$BASE_URL/api/admin/users")"
[[ "$STATUS" == "403" ]] || { echo "quota was not enforced: $STATUS" >&2; exit 1; }
json -b "$JAR" -X PATCH -H 'Content-Type: application/json' -d "$(jq -nc --argjson s "$(jq -c '.tenant.settings | .quotaUsers=0' <<<"$TENANT_SETTINGS")" '{settings:$s}')" "$BASE_URL/api/admin/settings" >/dev/null

echo "→ domain conflict validation"
json -c "$PLATJAR" -X POST -H 'Content-Type: application/json' -d '{"tenant":"acme","email":"admin@ucenter.local","password":"Admin12345!"}' "$BASE_URL/api/auth/login" >/dev/null
GLOBEX_ID="$(json -b "$PLATJAR" "$BASE_URL/api/admin/tenants" | jq -r '.items[] | select(.slug=="globex") | .id')"
STATUS="$(json -b "$PLATJAR" -o /dev/null -w '%{http_code}' -X PATCH -d "$(jq -nc --arg d "$DOMAIN" '{domains:[$d]}')" "$BASE_URL/api/admin/tenants/$GLOBEX_ID")"
[[ "$STATUS" == "400" ]] || { echo "duplicate domain was accepted: $STATUS" >&2; exit 1; }
echo "  duplicate domain rejected"

echo "→ platform login + soft delete"
json -b "$PLATJAR" -X DELETE "$BASE_URL/api/admin/tenants/$TENANT_ID" | jq -e '.ok == true' >/dev/null
LIST="$(json -b "$PLATJAR" "$BASE_URL/api/admin/tenants")"
jq -e --arg id "$TENANT_ID" 'any(.items[]; .id == $id and .status == "disabled" and .deletedAt != null)' <<<"$LIST" >/dev/null

echo "→ restore"
json -b "$PLATJAR" -X POST "$BASE_URL/api/admin/tenants/$TENANT_ID/restore" | jq -e '.ok == true' >/dev/null
RESOLVED="$(json -G --data-urlencode "domain=$DOMAIN" "$BASE_URL/api/public/tenant-by-host")"
jq -e --arg s "$SLUG" '.slug == $s' <<<"$RESOLVED" >/dev/null

echo "→ purge"
json -b "$PLATJAR" -X POST "$BASE_URL/api/admin/tenants/$TENANT_ID/purge" | jq -e '.ok == true' >/dev/null
STATUS="$(json -o /dev/null -w '%{http_code}' "$BASE_URL/api/public/tenants/$SLUG")"
[[ "$STATUS" == "404" ]] || { echo "purged tenant still resolves: $STATUS" >&2; exit 1; }
echo "✓ tenant lifecycle smoke test passed"
