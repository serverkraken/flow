#!/usr/bin/env bash
# scripts/smoke-r1-routes.sh — R1 Route-Smoke: beweist pro Route, dass sie
# GEMOUNTET ist (Status-Codes, kein Login nötig: 401/302 sind Beweise).
# Voraussetzungen: podman, openssl, gebautes Repo. Startet PG-Container +
# dex (compose) + flow-server, räumt via trap auf.
set -euo pipefail
cd "$(dirname "$0")/.."

PG_CTR="flow-r1-smoke-pg"
SRV_PID=""
cleanup() {
  [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null || true
  podman rm -f "$PG_CTR" >/dev/null 2>&1 || true
  (cd deploy/podman && podman-compose down dex >/dev/null 2>&1) || true
}
trap cleanup EXIT

podman run -d --rm --name "$PG_CTR" -p 15432:5432 \
  -e POSTGRES_DB=flow -e POSTGRES_USER=flow -e POSTGRES_PASSWORD=flow \
  postgres:16-alpine >/dev/null
(cd deploy/podman && podman-compose up -d dex >/dev/null)

for i in $(seq 1 30); do
  podman exec "$PG_CTR" pg_isready -U flow -d flow >/dev/null 2>&1 && break
  sleep 1
done

make build-server >/dev/null

FLOW_PG_DSN="postgres://flow:flow@localhost:15432/flow?sslmode=disable" \
FLOW_OIDC_ISSUER="http://localhost:5556" \
FLOW_OIDC_CLIENT_ID="flow-server-dev" \
FLOW_OIDC_CLIENT_SECRET="dev-secret" \
FLOW_ALLOWED_SUBS="Cghsb2NhbGRldhIFbG9jYWw" \
FLOW_COOKIE_HASH_KEY="$(openssl rand -hex 32)" \
FLOW_COOKIE_BLOCK_KEY="$(openssl rand -hex 32)" \
./bin/flow-server &
SRV_PID=$!
sleep 2

BASE="http://localhost:8080"
fail=0
check() { # check METHOD PATH EXPECTED [HEADER]
  local method=$1 path=$2 want=$3 hdr=${4:-}
  local got
  if [ -n "$hdr" ]; then
    got=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" -H "$hdr" "$BASE$path")
  else
    got=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" "$BASE$path")
  fi
  if [ "$got" = "$want" ]; then
    echo "OK   $method $path → $got"
  else
    echo "FAIL $method $path → $got (want $want)"
    fail=1
  fi
}

echo "— public —"
check GET /healthz 200
check GET /readyz 200
check GET /metrics 200
check GET /api/v1/meta 200
check GET /api/v1/oidc/config 200
check GET /auth/landing 200
check GET /login 302

echo "— Bearer-API: 401 beweist gemountet, 404 wäre ein Wiring-Loch —"
AUTH="Authorization: Bearer kaputt"
check GET  "/api/v1/worktime/sessions?from=2026-01-01&to=2026-01-02" 401 "$AUTH"
check POST /api/v1/worktime/sessions 401 "$AUTH"
check POST /api/v1/worktime/sessions:bulk 401 "$AUTH"
check PUT  /api/v1/worktime/sessions/x 401 "$AUTH"
check DELETE /api/v1/worktime/sessions/x 401 "$AUTH"
check GET  /api/v1/worktime/active 401 "$AUTH"
check POST /api/v1/worktime/active/start 401 "$AUTH"
check POST /api/v1/worktime/active/stop 401 "$AUTH"
check POST /api/v1/worktime/active/pause 401 "$AUTH"
check POST /api/v1/worktime/active/resume 401 "$AUTH"
check GET  /api/v1/projects 401 "$AUTH"
check POST /api/v1/projects 401 "$AUTH"
check PUT  /api/v1/projects/x 401 "$AUTH"
check GET  /api/v1/documents 401 "$AUTH"
check GET  /api/v1/documents/foo.md 401 "$AUTH"
check PUT  /api/v1/documents/foo.md 401 "$AUTH"
check DELETE /api/v1/documents/foo.md 401 "$AUTH"
check GET  /api/v1/repos/key/note 401 "$AUTH"
check PUT  /api/v1/repos/key/note 401 "$AUTH"
check GET  "/api/v1/day-offs?year=2026" 401 "$AUTH"
check PUT  /api/v1/day-offs/2026-01-01 401 "$AUTH"
check DELETE /api/v1/day-offs/2026-01-01 401 "$AUTH"
check GET  /api/v1/settings 401 "$AUTH"
check PUT  /api/v1/settings 401 "$AUTH"
check GET  /api/v1/me-bearer 401 "$AUTH"
check GET  /api/v1/events 401 "$AUTH"

echo "— alte Sync-Routen: Bearer-Middleware feuert vor chi-404 → 401 ist kein Wiring-Loch —"
# Die alten Sync-Routen sind nicht registriert; weil die Bearer-Middleware
# aber den gesamten /api/v1-Prefix umhüllt, lehnt sie den kaputten Token ab
# (401) bevor chi überhaupt prüft, ob ein Handler existiert.  401 ≠ 404, also
# kein Wiring-Loch — die Handler sind schlicht nicht da.
check GET /api/v1/sessions 401 "$AUTH"
check GET /api/v1/active 401 "$AUTH"
check GET /api/v1/repos 401 "$AUTH"
check GET /api/v1/repo-notes 401 "$AUTH"

echo "— WebUI (ohne Cookie → 302 auf /auth/landing) —"
for p in / /worktime /notes /repos /projects /settings; do
  check GET "$p" 302
done
check GET /api/v1/events 401 "Accept: text/event-stream"

exit $fail
