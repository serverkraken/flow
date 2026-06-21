#!/usr/bin/env bash
# Smoke: a token minted for the public `flow-cli` client (aud=flow-cli) is
# accepted by flow-server's multi-audience verifier at /api/v1/me.
# Requires: make dev-up + flow-server running (make dev-run) in another shell.
set -euo pipefail
ISSUER="${ISSUER:-http://localhost:5556/dex}"
SERVER="${FLOW_SERVER_URL:-https://localhost:8080}"
# Dev flow-server uses a self-signed cert; FLOW_INSECURE_TLS=1 (dev only) adds -k.
insecure=(); [ "${FLOW_INSECURE_TLS:-}" = "1" ] && insecure=(-k)

# Password grant with the flow-cli client id (public client, no secret).
resp=$(curl -fsS -d client_id=flow-cli -d grant_type=password \
  -d "username=msoent@dev.local" -d "password=password" \
  --data-urlencode "scope=openid profile email offline_access" \
  "$ISSUER/token")
at=$(printf '%s' "$resp" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
[ -n "$at" ] || { echo "smoke-m1b: no access_token: $resp" >&2; exit 1; }

code=$(curl "${insecure[@]}" -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $at" "$SERVER/api/v1/me")
[ "$code" = "200" ] || { echo "smoke-m1b: /api/v1/me returned $code (want 200)" >&2; exit 1; }
echo "smoke-m1b: OK — flow-cli audience accepted at /api/v1/me"
