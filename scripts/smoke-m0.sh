#!/usr/bin/env bash
# Boots flow-server against the compose DB with a FAKE issuer is NOT possible
# (real OIDC needed). This smoke runs the routes that don't need a real token
# plus prints manual steps for the authed ones. Run `make db-up` first.
set -euo pipefail
BASE="${BASE:-http://localhost:8080}"

echo "== /healthz =="
curl -fsS "$BASE/healthz" && echo

echo "== /api/v1/me without token (expect 401) =="
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/me")
[ "$code" = "401" ] && echo "OK 401" || { echo "FAIL: $code"; exit 1; }

cat <<'EOF'
== authed routes (manual, needs a real Authentik token) ==
  TOKEN=<paste access token for an allowlisted sub>
  curl -fsS -H "Authorization: Bearer $TOKEN" $BASE/api/v1/me ; echo
  # in one shell: stream events
  curl -N  -H "Authorization: Bearer $TOKEN" $BASE/api/v1/events &
  # in another: fire a ping (FLOW_DEV=1)
  curl -fsS -X POST -H "Authorization: Bearer $TOKEN" $BASE/api/v1/debug/ping
  # the streaming shell should print:  event: ping
EOF
