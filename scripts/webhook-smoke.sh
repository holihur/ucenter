#!/usr/bin/env bash
# Webhook contract smoke test: create endpoint, trigger user.created, verify retry,
# HMAC signature, delivery log, SSRF protection.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
STAMP="$(date +%s)"
EMAIL="webhook-$STAMP@acme.test"
JAR="$(mktemp)"
trap 'rm -f "$JAR"' EXIT
LOG="${WEBHOOK_RECEIVER_LOG:-/tmp/webhook-deliveries.jsonl}"

for bin in curl jq openssl; do command -v "$bin" >/dev/null || { echo "missing $bin" >&2; exit 1; }; done
json() { curl -sS -H 'Content-Type: application/json' "$@"; }

echo "→ owner login"
json -c "$JAR" -X POST -d '{"tenant":"acme","email":"owner@acme.test","password":"Password123!"}' "$BASE_URL/api/auth/login" >/dev/null

echo "→ create webhook"
CREATED="$(json -b "$JAR" -X POST -d "$(jq -nc '{name:"CI receiver",url:"http://127.0.0.1:9092/hook",events:["user.created"],enabled:true}')" "$BASE_URL/api/admin/webhooks")"
HOOK_ID="$(jq -r '.webhook.id' <<<"$CREATED")"
SECRET="$(jq -r '.secret' <<<"$CREATED")"
[[ "$SECRET" == whsec_* ]] || { echo "missing webhook secret" >&2; exit 1; }

echo "→ SSRF protection"
STATUS="$(json -b "$JAR" -o /dev/null -w '%{http_code}' -X POST -d '{"name":"metadata","url":"http://169.254.169.254/latest/meta-data","events":["user.created"]}' "$BASE_URL/api/admin/webhooks")"
[[ "$STATUS" == "400" ]] || { echo "SSRF URL was accepted: $STATUS" >&2; exit 1; }

echo "→ trigger user.created"
USER_ID="$(json -b "$JAR" -X POST -d "$(jq -nc --arg e "$EMAIL" '{email:$e,name:"Webhook User",roles:["member"]}')" "$BASE_URL/api/admin/users" | jq -r '.user.id')"

echo "→ wait for retry + delivery"
DELIVERED=""
for _ in $(seq 1 40); do
  LIST="$(json -b "$JAR" "$BASE_URL/api/admin/webhook-deliveries?endpointId=$HOOK_ID&pageSize=10")"
  DELIVERED="$(jq -r --arg id "$HOOK_ID" '.items[] | select(.endpointId == $id and .event == "user.created" and .status == "delivered") | .id' <<<"$LIST" | head -1)"
  [[ -n "$DELIVERED" ]] && break
  sleep 1
done
[[ -n "$DELIVERED" ]] || { echo "webhook was not delivered" >&2; exit 1; }
ATTEMPTS="$(jq -r --arg id "$DELIVERED" '.items[] | select(.id == $id) | .attempts' <<<"$LIST")"
[[ "$ATTEMPTS" -ge 2 ]] || { echo "retry was not exercised: $ATTEMPTS" >&2; exit 1; }
echo "  delivered after $ATTEMPTS attempts"

echo "→ verify HMAC signature"
LINE="$(tail -n 1 "$LOG")"
TIMESTAMP="$(jq -r .timestamp <<<"$LINE")"
BODY="$(jq -r .body <<<"$LINE")"
SIGNATURE="$(jq -r .signature <<<"$LINE" | sed 's/^sha256=//')"
EXPECTED="$(printf '%s.%s' "$TIMESTAMP" "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex | awk '{print $2}')"
[[ "$SIGNATURE" == "$EXPECTED" ]] || { echo "signature mismatch" >&2; exit 1; }
jq -e --arg id "$DELIVERED" '.delivery == $id' <<<"$LINE" >/dev/null
echo "  signature valid"

echo "→ cleanup"
json -b "$JAR" -X DELETE "$BASE_URL/api/admin/webhooks/$HOOK_ID" >/dev/null
json -b "$JAR" -X DELETE "$BASE_URL/api/admin/users/$USER_ID" >/dev/null
echo "✓ webhook smoke test passed"
