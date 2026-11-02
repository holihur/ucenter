#!/usr/bin/env bash
# Email flows: invite → accept → login, register → verify, forgot → reset, MFA recovery.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TENANT="${TENANT:-acme}"
OWNER_EMAIL="${OWNER_EMAIL:-owner@acme.test}"
OWNER_PASSWORD="${OWNER_PASSWORD:-Password123!}"
STAMP="$(date +%s)"
INVITE_EMAIL="invite-$STAMP@acme.test"
VERIFY_EMAIL="verify-$STAMP@acme.test"

for bin in curl jq; do command -v "$bin" >/dev/null || { echo "missing $bin" >&2; exit 1; }; done
JAR="$(mktemp)"; VJAR="$(mktemp)"
trap 'rm -f "$JAR" "$VJAR"' EXIT

latest_mail() {
  curl -fsS -b "$JAR" "$BASE_URL/api/admin/mail-outbox?limit=10" | jq -r '.items[0].text'
}
extract_token() {
  grep -o 'http://[^ ]*' | head -1 | sed -n 's/.*token=\([^&]*\).*/\1/p'
}

echo "→ owner login"
curl -fsS -c "$JAR" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$TENANT" --arg e "$OWNER_EMAIL" --arg p "$OWNER_PASSWORD" '{tenant:$t,email:$e,password:$p}')" \
  "$BASE_URL/api/auth/login" >/dev/null

echo "→ invite + accept"
CREATE="$(curl -fsS -b "$JAR" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg e "$INVITE_EMAIL" '{email:$e,name:"Invite Smoke",roles:["member"]}')" \
  "$BASE_URL/api/admin/users")"
INVITE_ID="$(jq -r '.user.id' <<<"$CREATE")"
INVITE_TOKEN="$(latest_mail | extract_token)"
curl -fsS -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$INVITE_TOKEN" '{token:$t,name:"Invite Smoke",password:"Invited12345!"}')" \
  "$BASE_URL/api/auth/accept-invite" >/dev/null
curl -fsS -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$TENANT" --arg e "$INVITE_EMAIL" '{tenant:$t,email:$e,password:"Invited12345!"}')" \
  "$BASE_URL/api/auth/login" >/dev/null
echo "  accepted and signed in"

echo "→ password reset"
curl -fsS -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$TENANT" --arg e "$INVITE_EMAIL" '{tenant:$t,email:$e}')" \
  "$BASE_URL/api/auth/forgot-password" >/dev/null
RESET_TOKEN="$(latest_mail | extract_token)"
curl -fsS -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$RESET_TOKEN" '{token:$t,password:"Reset12345!"}')" \
  "$BASE_URL/api/auth/reset-password" >/dev/null
curl -fsS -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$TENANT" --arg e "$INVITE_EMAIL" '{tenant:$t,email:$e,password:"Reset12345!"}')" \
  "$BASE_URL/api/auth/login" >/dev/null
echo "  reset password accepted"

echo "→ register + verify email"
REG="$(curl -fsS -c "$VJAR" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$TENANT" --arg e "$VERIFY_EMAIL" '{tenant:$t,email:$e,password:"Verify12345!",name:"Verify Smoke"}')" \
  "$BASE_URL/api/auth/register")"
VERIFY_ID="$(jq -r '.user.id' <<<"$REG")"
VERIFY_TOKEN="$(latest_mail | extract_token)"
curl -fsS -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$VERIFY_TOKEN" '{token:$t}')" \
  "$BASE_URL/api/auth/verify-email" >/dev/null
VERIFIED="$(curl -fsS -b "$JAR" "$BASE_URL/api/admin/users?q=$VERIFY_EMAIL" | jq -r '.items[0].emailVerified')"
[[ "$VERIFIED" == "true" ]] || { echo "email not verified" >&2; exit 1; }
echo "  email verified"

if command -v mongosh >/dev/null; then
  echo "→ MFA recovery"
  MONGO_URI="${MONGO_URI:-mongodb://127.0.0.1:27017}"
  MONGO_DB="${MONGO_DB:-ucenter}"
  mongosh --quiet "$MONGO_URI/$MONGO_DB" --eval "db.users.updateOne({email:'$INVITE_EMAIL'},{\$set:{'mfa.enabled':true,'mfa.secret':'SMOKE'}})" >/dev/null
  curl -fsS -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg t "$TENANT" --arg e "$INVITE_EMAIL" '{tenant:$t,email:$e}')" \
    "$BASE_URL/api/auth/mfa-recovery" >/dev/null
  MFA_TOKEN="$(latest_mail | extract_token)"
  curl -fsS -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg t "$MFA_TOKEN" '{token:$t}')" \
    "$BASE_URL/api/auth/mfa-recovery/confirm" >/dev/null
  STATE="$(mongosh --quiet "$MONGO_URI/$MONGO_DB" --eval "db.users.findOne({email:'$INVITE_EMAIL'},{'mfa.enabled':1}).mfa.enabled")"
  [[ "$STATE" == "false" ]] || { echo "MFA was not disabled" >&2; exit 1; }
  echo "  MFA disabled via email recovery"
else
  echo "  (mongosh not found; skipping MFA recovery assertion)"
fi

echo "→ error paths"
STATUS="$(curl -sS -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$RESET_TOKEN" '{token:$t,password:"Another12345!"}')" \
  "$BASE_URL/api/auth/reset-password")"
[[ "$STATUS" == "400" ]] || { echo "reused reset token: expected 400, got $STATUS" >&2; exit 1; }
STATUS="$(curl -sS -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"token":"uct_invalid","password":"Another12345!"}' "$BASE_URL/api/auth/reset-password")"
[[ "$STATUS" == "400" ]] || { echo "invalid reset token: expected 400, got $STATUS" >&2; exit 1; }

echo "→ rate limiting (5 requests / 15 minutes)"
RL_EMAIL="ratelimit-$STAMP@acme.test"
for i in 1 2 3 4 5; do
  curl -sS -o /dev/null -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg t "$TENANT" --arg e "$RL_EMAIL" '{tenant:$t,email:$e}')" \
    "$BASE_URL/api/auth/forgot-password"
done
STATUS="$(curl -sS -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg t "$TENANT" --arg e "$RL_EMAIL" '{tenant:$t,email:$e}')" \
  "$BASE_URL/api/auth/forgot-password")"
[[ "$STATUS" == "429" ]] || { echo "rate limit: expected 429, got $STATUS" >&2; exit 1; }
echo "  single-use, invalid-token and rate-limit paths passed"

echo "→ cleanup"
for id in "$INVITE_ID" "$VERIFY_ID"; do
  curl -fsS -b "$JAR" -X DELETE "$BASE_URL/api/admin/users/$id" >/dev/null
done
echo "✓ email smoke test passed"
