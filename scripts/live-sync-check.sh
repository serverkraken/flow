#!/usr/bin/env bash
# Asserts a start fired over REST shows up on the SSE stream (the M1a Done-gate,
# server side). Requires a running flow-server + DB and a real token.
set -euo pipefail
BASE="${BASE:-https://localhost:8080}"
: "${TOKEN:?set TOKEN to an allowlisted Authentik access token}"
auth=(-H "Authorization: Bearer $TOKEN")
# Dev flow-server uses a self-signed cert; FLOW_INSECURE_TLS=1 (dev only) adds -k.
insecure=(); [ "${FLOW_INSECURE_TLS:-}" = "1" ] && insecure=(-k)

tmp=$(mktemp)
curl "${insecure[@]}" -N "${auth[@]}" "$BASE/api/v1/events" >"$tmp" 2>/dev/null &
spid=$!
trap 'kill "$spid" 2>/dev/null || true; rm -f "$tmp"' EXIT
sleep 1

curl "${insecure[@]}" -fsS -X POST "${auth[@]}" -H 'Content-Type: application/json' -d '{}' \
  "$BASE/api/v1/sessions" >/dev/null

for _ in $(seq 1 20); do
  if grep -q 'event: session.started' "$tmp"; then
    echo "OK: session.started observed on the SSE stream"
    echo "(cleanup: stop the running session via the WebUI or 'flow worktime')"
    exit 0
  fi
  sleep 0.25
done
echo "FAIL: no session.started event within 5s"
exit 1
