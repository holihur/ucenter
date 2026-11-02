#!/usr/bin/env bash
# End-to-end OIDC authorization-code + PKCE smoke test.
#
# Usage:
#   BASE_URL=http://localhost:8080 \
#   TENANT=acme EMAIL=owner@acme.test PASSWORD='Password123!' \
#   CLIENT_ID=app_demo_acme CLIENT_SECRET=demo-secret-acme \
#   ./scripts/oidc-smoke.sh
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TENANT="${TENANT:-acme}"
EMAIL="${EMAIL:-owner@acme.test}"
PASSWORD="${PASSWORD:-Password123!}"
CLIENT_ID="${CLIENT_ID:-app_demo_acme}"
CLIENT_SECRET="${CLIENT_SECRET:-demo-secret-acme}"
REDIRECT_URI="${REDIRECT_URI:-http://localhost:3000/callback}"
SCOPE="${SCOPE:-openid profile email offline_access roles}"

for bin in curl jq openssl; do
  command -v "$bin" >/dev/null || { echo "missing dependency: $bin" >&2; exit 1; }
done

JAR="$(mktemp)"
trap 'rm -f "$JAR"' EXIT

VERIFIER="$(openssl rand -base64 48 | tr -d '=+/' | cut -c1-64)"
CHALLENGE="$(printf '%s' "$VERIFIER" | openssl dgst -sha256 -binary | openssl base64 -A | tr '+/' '-_' | tr -d '=')"

echo "→ discovery"
curl -fsS "$BASE_URL/t/$TENANT/.well-known/openid-configuration" | jq -e '.issuer, .authorization_endpoint, .jwks_uri' >/dev/null
curl -fsS "$BASE_URL/t/$TENANT/oidc/jwks" | jq -e '.keys[0].kid' >/dev/null

echo "→ authorize"
LOCATION="$(curl -sS -o /dev/null -w '%{redirect_url}' -G "$BASE_URL/t/$TENANT/oidc/authorize" \
  --data-urlencode "client_id=$CLIENT_ID" \
  --data-urlencode "redirect_uri=$REDIRECT_URI" \
  --data-urlencode "response_type=code" \
  --data-urlencode "scope=$SCOPE" \
  --data-urlencode "state=smoke-state" \
  --data-urlencode "nonce=smoke-nonce" \
  --data-urlencode "code_challenge=$CHALLENGE" \
  --data-urlencode "code_challenge_method=S256")"
REQUEST_ID="$(printf '%s' "$LOCATION" | sed -n 's/.*[?&]request=\([^&]*\).*/\1/p')"
[[ -n "$REQUEST_ID" ]] || { echo "no authorization request id in redirect: $LOCATION" >&2; exit 1; }
echo "  request=$REQUEST_ID"

echo "→ login"
LOGIN_PAYLOAD="$(jq -nc --arg tenant "$TENANT" --arg email "$EMAIL" --arg password "$PASSWORD" --arg requestId "$REQUEST_ID" \
  '{tenant:$tenant,email:$email,password:$password,requestId:$requestId}')"
LOGIN="$(curl -fsS -c "$JAR" -H 'Content-Type: application/json' -d "$LOGIN_PAYLOAD" "$BASE_URL/api/auth/login")"
if [[ "$(jq -r '.mfaRequired // false' <<<"$LOGIN")" == "true" ]]; then
  echo "MFA is enabled for $EMAIL; this script does not handle TOTP prompts." >&2
  exit 1
fi
echo "  signed in as $(jq -r '.user.email' <<<"$LOGIN")"

echo "→ complete authorization"
COMPLETE="$(curl -fsS -b "$JAR" -X POST "$BASE_URL/api/auth/request/$REQUEST_ID/complete")"
if [[ "$(jq -r '.next' <<<"$COMPLETE")" == "consent" ]]; then
  echo "  consent required; approving"
  COMPLETE="$(curl -fsS -b "$JAR" -H 'Content-Type: application/json' -d '{"approve":true}' \
    "$BASE_URL/api/auth/request/$REQUEST_ID/consent")"
fi
REDIRECT="$(jq -r '.redirectUrl' <<<"$COMPLETE")"
CODE="$(printf '%s' "$REDIRECT" | sed -n 's/.*[?&]code=\([^&]*\).*/\1/p')"
STATE="$(printf '%s' "$REDIRECT" | sed -n 's/.*[?&]state=\([^&]*\).*/\1/p')"
[[ -n "$CODE" ]] || { echo "no code in redirect: $REDIRECT" >&2; exit 1; }
[[ "$STATE" == "smoke-state" ]] || { echo "state mismatch: $STATE" >&2; exit 1; }
echo "  received authorization code"

echo "→ token exchange"
TOKENS="$(curl -fsS -u "$CLIENT_ID:$CLIENT_SECRET" -X POST "$BASE_URL/t/$TENANT/oidc/token" \
  --data-urlencode "grant_type=authorization_code" \
  --data-urlencode "code=$CODE" \
  --data-urlencode "redirect_uri=$REDIRECT_URI" \
  --data-urlencode "code_verifier=$VERIFIER")"
ACCESS_TOKEN="$(jq -r '.access_token' <<<"$TOKENS")"
REFRESH_TOKEN="$(jq -r '.refresh_token // empty' <<<"$TOKENS")"
jq -e '.token_type == "Bearer" and .expires_in > 0 and (.id_token | length > 0)' <<<"$TOKENS" >/dev/null
echo "  access + id token issued (refresh: $([[ -n "$REFRESH_TOKEN" ]] && echo yes || echo no))"

echo "→ userinfo"
USERINFO="$(curl -fsS -H "Authorization: Bearer $ACCESS_TOKEN" "$BASE_URL/t/$TENANT/oidc/userinfo")"
jq -e --arg email "$EMAIL" '.sub and .email == $email' <<<"$USERINFO" >/dev/null
echo "  userinfo: $(jq -c '{sub,email,name,roles}' <<<"$USERINFO")"

echo "→ introspection"
INTROSPECT="$(curl -fsS -u "$CLIENT_ID:$CLIENT_SECRET" -X POST "$BASE_URL/t/$TENANT/oidc/introspect" \
  --data-urlencode "token=$ACCESS_TOKEN")"
jq -e '.active == true' <<<"$INTROSPECT" >/dev/null
echo "  token active"

if [[ -n "$REFRESH_TOKEN" ]]; then
  echo "→ refresh rotation"
  REFRESHED="$(curl -fsS -u "$CLIENT_ID:$CLIENT_SECRET" -X POST "$BASE_URL/t/$TENANT/oidc/token" \
    --data-urlencode "grant_type=refresh_token" \
    --data-urlencode "refresh_token=$REFRESH_TOKEN")"
  jq -e '.access_token and .refresh_token' <<<"$REFRESHED" >/dev/null
  echo "  new access + refresh token issued"

  echo "→ reuse of rotated refresh token must fail"
  STATUS="$(curl -sS -o /dev/null -w '%{http_code}' -u "$CLIENT_ID:$CLIENT_SECRET" -X POST "$BASE_URL/t/$TENANT/oidc/token" \
    --data-urlencode "grant_type=refresh_token" \
    --data-urlencode "refresh_token=$REFRESH_TOKEN")"
  [[ "$STATUS" == "400" ]] || { echo "expected 400 for reused refresh token, got $STATUS" >&2; exit 1; }
  echo "  replay rejected"
fi

echo "→ logout"
curl -fsS -b "$JAR" -X POST "$BASE_URL/api/auth/logout" >/dev/null
echo "✓ OIDC smoke test passed"
