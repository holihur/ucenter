#!/usr/bin/env bash
# SCIM 2.0 contract smoke test: token auth, Users/Groups CRUD, filter, pagination,
# idempotent PUT, deactivate, tenant isolation and error paths.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TENANT="${TENANT:-acme}"
EMAIL="${EMAIL:-owner@acme.test}"
PASSWORD="${PASSWORD:-Password123!}"
STAMP="$(date +%s)"
USER_EMAIL="scim-$STAMP@acme.test"
EXTERNAL_ID="scim-$STAMP"
GROUP_NAME="SCIM Support $STAMP"
GROUP_KEY="scim-support-$STAMP"

for bin in curl jq; do command -v "$bin" >/dev/null || { echo "missing $bin" >&2; exit 1; }; done
JAR="$(mktemp)"
trap 'rm -f "$JAR"' EXIT

echo "→ owner login"
curl -fsS -c "$JAR" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$TENANT" --arg e "$EMAIL" --arg p "$PASSWORD" '{tenant:$t,email:$e,password:$p}')" \
  "$BASE_URL/api/auth/login" >/dev/null

echo "→ create SCIM token"
TOKEN="$(curl -fsS -b "$JAR" -X POST "$BASE_URL/api/admin/scim/token" | jq -r '.token')"
[[ "$TOKEN" == ucs_* ]] || { echo "invalid SCIM token" >&2; exit 1; }
SCIM="$BASE_URL/t/$TENANT/scim/v2"
AUTH="Authorization: Bearer $TOKEN"
CT="Content-Type: application/scim+json"

scim() { curl -sS -H "$AUTH" -H "$CT" "$@"; }
status() { curl -sS -o /dev/null -w '%{http_code}' -H "$AUTH" -H "$CT" "$@"; }

echo "→ ServiceProviderConfig"
scim "$SCIM/ServiceProviderConfig" | jq -e '.patch.supported == true' >/dev/null

echo "→ create user"
USER_BODY="$(jq -nc --arg e "$USER_EMAIL" --arg x "$EXTERNAL_ID" \
  '{schemas:["urn:ietf:params:scim:schemas:core:2.0:User"],externalId:$x,userName:$e,name:{givenName:"Scim",familyName:"User"},displayName:"Scim User",active:true,roles:[{value:"member"}]}')"
CREATED="$(scim -X POST -d "$USER_BODY" "$SCIM/Users")"
USER_ID="$(jq -r '.id' <<<"$CREATED")"
jq -e --arg e "$USER_EMAIL" '.userName == $e and .active == true' <<<"$CREATED" >/dev/null
echo "  user=$USER_ID"

echo "→ filter and pagination"
FILTERED="$(scim -G --data-urlencode "filter=userName eq \"$USER_EMAIL\"" --data-urlencode "startIndex=1" --data-urlencode "count=10" "$SCIM/Users")"
jq -e --arg e "$USER_EMAIL" '.totalResults >= 1 and any(.Resources[]; .userName == $e)' <<<"$FILTERED" >/dev/null

echo "→ idempotent PUT"
scim -X PUT -d "$USER_BODY" "$SCIM/Users/$USER_ID" | jq -e --arg id "$USER_ID" '.id == $id' >/dev/null
scim -X PUT -d "$USER_BODY" "$SCIM/Users/$USER_ID" | jq -e --arg id "$USER_ID" '.id == $id' >/dev/null

echo "→ PATCH deactivate"
PATCHED="$(scim -X PATCH -d '{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","path":"active","value":false}]}' "$SCIM/Users/$USER_ID")"
jq -e '.active == false' <<<"$PATCHED" >/dev/null

echo "→ duplicate create is 409"
[[ "$(scim -o /dev/null -w '%{http_code}' -X POST -d "$USER_BODY" "$SCIM/Users")" == "409" ]] || { echo "expected duplicate 409" >&2; exit 1; }

echo "→ group CRUD and membership mapping"
GROUP_BODY="$(jq -nc --arg n "$GROUP_NAME" --arg x "$GROUP_KEY" --arg u "$USER_ID" \
  '{schemas:["urn:ietf:params:scim:schemas:core:2.0:Group"],displayName:$n,externalId:$x,members:[{value:$u}]}')"
GROUP="$(scim -X POST -d "$GROUP_BODY" "$SCIM/Groups")"
GROUP_ID="$(jq -r '.id' <<<"$GROUP")"
jq -e --arg id "$USER_ID" 'any(.members[]; .value == $id)' <<<"$GROUP" >/dev/null
scim -G --data-urlencode "filter=externalId eq \"$GROUP_KEY\"" "$SCIM/Groups" | jq -e --arg id "$GROUP_ID" 'any(.Resources[]; .id == $id)' >/dev/null
scim "$SCIM/Users/$USER_ID" | jq -e --arg key "$GROUP_KEY" 'any(.roles[]; .value == $key)' >/dev/null
echo "  group=$GROUP_ID"

echo "→ tenant isolation"
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H "$AUTH" "$BASE_URL/t/globex/scim/v2/Users")" == "401" ]] || { echo "cross-tenant SCIM token was accepted" >&2; exit 1; }
[[ "$(curl -sS -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer ucs_invalid' "$SCIM/Users")" == "401" ]] || { echo "invalid SCIM token was accepted" >&2; exit 1; }

echo "→ cleanup"
scim -X DELETE "$SCIM/Groups/$GROUP_ID" >/dev/null
scim -X DELETE "$SCIM/Users/$USER_ID" >/dev/null
echo "✓ SCIM smoke test passed"
