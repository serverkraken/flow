#!/usr/bin/env bash
# Smoke the unauthenticated surface + print manual steps for authed routes.
# Run `make db-up` and start flow-server first.
set -euo pipefail
BASE="${BASE:-http://localhost:8080}"

echo "== /healthz =="
curl -fsS "$BASE/healthz" && echo

echo "== /api/v1/me without token (expect 401) =="
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/me")
[ "$code" = "401" ] && echo "OK 401" || { echo "FAIL: $code"; exit 1; }

echo "== / without session cookie (expect 302 -> /auth/login) =="
loc=$(curl -s -o /dev/null -w '%{redirect_url}' "$BASE/")
case "$loc" in */auth/login) echo "OK redirect: $loc";; *) echo "FAIL: $loc"; exit 1;; esac

cat <<'EOF'
== authed live-sync (manual / scripted) ==
  TOKEN=<allowlisted Authentik access token>  ./scripts/live-sync-check.sh
  WebUI:  open http://localhost:8080/  -> login via Authentik -> start a timer
          in the browser, then run a `flow worktime` TUI (FLOW_TOKEN=$TOKEN) and
          watch the timer appear/disappear on both within ~1s.
EOF
