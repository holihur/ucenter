#!/usr/bin/env bash
# RBAC/API-scope/delegated-admin contract test.
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8080}"
STAMP="$(date +%s)"
JAR="$(mktemp)"; ADMINJAR="$(mktemp)"
trap 'rm -f "$JAR" "$ADMINJAR"' EXIT
for bin in curl jq; do command -v "$bin" >/dev/null || { echo "missing $bin" >&2; exit 1; }; done
json() { curl -sS -H 'Content-Type: application/json' "$@"; }

echo "→ owner API token with users:read"
json -c "$JAR" -X POST -d '{"tenant":"acme","email":"owner@acme.test","password":"Password123!"}' "$BASE_URL/api/auth/login" >/dev/null
TOKEN="$(json -b "$JAR" -X POST -d '{"name":"authz-readonly","scopes":["users:read"],"expiresInDays":1}' "$BASE_URL/api/admin/api-tokens" | jq -r '.token')"
[[ "$TOKEN" == ucm_* ]] || { echo "token missing" >&2; exit 1; }
AUTH="Authorization: Bearer $TOKEN"
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "$AUTH" "$BASE_URL/api/admin/users")" == "200" ]] || { echo "scope read failed" >&2; exit 1; }
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "$AUTH" -H 'Content-Type: application/json' -d '{"email":"nope@acme.test","name":"Nope"}' "$BASE_URL/api/admin/users")" == "403" ]] || { echo "scope write not blocked" >&2; exit 1; }
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "$AUTH" "$BASE_URL/api/admin/clients")" == "403" ]] || { echo "client scope not blocked" >&2; exit 1; }
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "$AUTH" "$BASE_URL/api/auth/me")" == "401" ]] || { echo "API token unexpectedly reached session-only endpoint" >&2; exit 1; }
echo "  read allowed, write/other-resource blocked, session endpoint isolated"

echo "→ delegated admin boundaries"
json -c "$ADMINJAR" -X POST -d '{"tenant":"acme","email":"admin@acme.test","password":"Password123!"}' "$BASE_URL/api/auth/login" >/dev/null
OWNER_ROLE_ID="$(json -b "$ADMINJAR" "$BASE_URL/api/admin/roles" | jq -r '.items[] | select(.key=="owner") | .id')"
ADMIN_ROLE_ID="$(json -b "$ADMINJAR" "$BASE_URL/api/admin/roles" | jq -r '.items[] | select(.key=="admin") | .id')"
STATUS="$(json -b "$ADMINJAR" -o /dev/null -w '%{http_code}' -X POST -d "$(jq -nc --arg e "delegated-$STAMP@acme.test" --arg id "$OWNER_ROLE_ID" '{email:$e,name:"Delegated",roles:["owner"]}')" "$BASE_URL/api/admin/users")"
[[ "$STATUS" == "403" ]] || { echo "non-owner granted owner: $STATUS" >&2; exit 1; }
STATUS="$(json -b "$ADMINJAR" -o /dev/null -w '%{http_code}' -X POST -d "$(jq -nc --arg e "delegated-$STAMP@acme.test" --arg id "$ADMIN_ROLE_ID" '{email:$e,name:"Delegated",roles:["admin"]}')" "$BASE_URL/api/admin/users")"
[[ "$STATUS" == "201" ]] || { echo "delegated admin could not grant admin role: $STATUS" >&2; exit 1; }
USER_ID="$(json -b "$ADMINJAR" "$BASE_URL/api/admin/users?q=delegated-$STAMP@acme.test" | jq -r '.items[0].id')"
[[ "$(json -b "$ADMINJAR" -o /dev/null -w '%{http_code}' -X PATCH -d '{"roles":["owner"]}' "$BASE_URL/api/admin/users/$USER_ID")" == "403" ]] || { echo "delegated admin escalated to owner" >&2; exit 1; }
json -b "$ADMINJAR" -X DELETE "$BASE_URL/api/admin/users/$USER_ID" >/dev/null
echo "  owner escalation blocked, admin delegation allowed"

echo "→ revoke token"
TOKEN_ID="$(json -b "$JAR" "$BASE_URL/api/admin/api-tokens" | jq -r '.items[0].id')"
json -b "$JAR" -X DELETE "$BASE_URL/api/admin/api-tokens/$TOKEN_ID" >/dev/null
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "$AUTH" "$BASE_URL/api/admin/users")" == "401" ]] || { echo "revoked token still works" >&2; exit 1; }
echo "✓ authorization smoke test passed"
