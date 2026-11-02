#!/usr/bin/env bash
# MCP end-to-end smoke test: login → create token → initialize → tools/list →
# tools/call → scope enforcement → revoke → 401.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TENANT="${TENANT:-acme}"
EMAIL="${EMAIL:-owner@acme.test}"
PASSWORD="${PASSWORD:-Password123!}"

for bin in curl jq; do
  command -v "$bin" >/dev/null || { echo "missing dependency: $bin" >&2; exit 1; }
done

JAR="$(mktemp)"
trap 'rm -f "$JAR"' EXIT

post_mcp() {
  local token="$1" body="$2"
  curl -sS -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    -d "$body" "$BASE_URL/mcp"
}

echo "→ password login"
curl -fsS -c "$JAR" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg tenant "$TENANT" --arg email "$EMAIL" --arg password "$PASSWORD" \
    '{tenant:$tenant,email:$email,password:$password}')" \
  "$BASE_URL/api/auth/login" >/dev/null

echo "→ create scoped token (users:read, tenant:read)"
CREATED="$(curl -fsS -b "$JAR" -H 'Content-Type: application/json' \
  -d '{"name":"MCP smoke","scopes":["users:read","tenant:read"],"expiresInDays":1}' \
  "$BASE_URL/api/admin/api-tokens")"
TOKEN="$(jq -r '.token' <<<"$CREATED")"
TOKEN_ID="$(jq -r '.item.id' <<<"$CREATED")"
[[ "$TOKEN" == ucm_* ]] || { echo "unexpected token: $TOKEN" >&2; exit 1; }

echo "→ initialize"
INIT="$(post_mcp "$TOKEN" '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}')"
jq -e '.result.serverInfo.name == "ucenter"' <<<"$INIT" >/dev/null

echo "→ tools/list scope enforcement"
TOOLS="$(post_mcp "$TOKEN" '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}')"
jq -e '[.result.tools[].name] | index("list_users") != null' <<<"$TOOLS" >/dev/null
if jq -e '[.result.tools[].name] | index("create_user") != null' <<<"$TOOLS" >/dev/null; then
  echo "read-only token unexpectedly exposed create_user" >&2
  exit 1
fi

echo "→ whoami"
WHO="$(post_mcp "$TOKEN" '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"whoami","arguments":{}}}')"
jq -e --arg email "$EMAIL" '.result.structuredContent.user.email == $email' <<<"$WHO" >/dev/null

echo "→ list_users"
USERS="$(post_mcp "$TOKEN" '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_users","arguments":{"pageSize":2}}}')"
jq -e '.result.structuredContent.total >= 1' <<<"$USERS" >/dev/null
echo "  returned $(jq -r '.result.structuredContent.total' <<<"$USERS") users"

echo "→ create_user must be unavailable"
DENIED="$(post_mcp "$TOKEN" '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"create_user","arguments":{"email":"nope@example.com","name":"Nope"}}}')"
jq -e '.error.code == -32602' <<<"$DENIED" >/dev/null

echo "→ revoke token"
curl -fsS -b "$JAR" -X DELETE "$BASE_URL/api/admin/api-tokens/$TOKEN_ID" >/dev/null

echo "→ revoked token is rejected"
STATUS="$(curl -sS -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":6,"method":"tools/list","params":{}}' \
  "$BASE_URL/mcp")"
[[ "$STATUS" == "401" ]] || { echo "expected 401 for revoked token, got $STATUS" >&2; exit 1; }

echo "✓ MCP smoke test passed"
