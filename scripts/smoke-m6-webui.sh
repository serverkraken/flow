#!/usr/bin/env bash
# smoke-m6-webui.sh — boot-time smoke for the Plan E · M6 read-only WebUI.
#
# Confirms that the route registration from Plan E · Task 10 wires up
# without crashing: the server binds, /healthz still works, the
# unauthenticated landing renders, and a static asset is served. The full
# OIDC-cookie round trip lives in the Plan E · Task 16 manual E2E.
#
# Prerequisites:
#   - go toolchain on PATH (the script builds bin/flow-server if missing)
#   - FLOW_* env vars exported (or use scripts/run-flow-server.sh which
#     sets sensible local-dev defaults against deploy/podman dex)
#   - An OIDC issuer reachable at FLOW_OIDC_ISSUER (dex/Authentik); the
#     handler initialises but never needs a real token exchange in this
#     smoke
#
# Usage:
#   chmod +x scripts/smoke-m6-webui.sh
#   ./scripts/smoke-m6-webui.sh
#
# Exits non-zero on the first failing assertion.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

info() { printf '\n[smoke-m6] %s\n' "$*"; }
ok()   { printf '  OK  %s\n' "$*"; }
fail() { printf '  FAIL %s\n' "$*" >&2; exit 1; }

# ---- build ------------------------------------------------------------------

info "build flow-server"
go build -o bin/flow-server ./cmd/flow-server
ok "bin/flow-server built"

# ---- boot -------------------------------------------------------------------

# Use the convenience launcher when the operator hasn't already exported
# FLOW_* — it sources scripts/run-flow-server.sh defaults (dex on :5556,
# local cookie keys). When FLOW_OIDC_ISSUER is already set we trust the
# caller's environment.
if [[ -z "${FLOW_OIDC_ISSUER:-}" ]]; then
  info "FLOW_OIDC_ISSUER unset — sourcing scripts/run-flow-server.sh defaults"
  # shellcheck disable=SC1091
  source <(sed -n '/^export FLOW_/p' "${SCRIPT_DIR}/run-flow-server.sh")
  # Cookie keys: re-use the gitignored file the launcher manages.
  if [[ -f .flow-cookie-keys ]]; then
    # shellcheck disable=SC1091
    source .flow-cookie-keys
    export FLOW_COOKIE_HASH_KEY FLOW_COOKIE_BLOCK_KEY
  else
    export FLOW_COOKIE_HASH_KEY="$(openssl rand -hex 32)"
    export FLOW_COOKIE_BLOCK_KEY="$(openssl rand -hex 16)"
  fi
fi

# Per-smoke server.db so we don't clobber a real instance.
export FLOW_SERVER_DB="${FLOW_SERVER_DB:-/tmp/flow-smoke-m6/server.db}"
mkdir -p "$(dirname "${FLOW_SERVER_DB}")"

info "start flow-server (background)"
./bin/flow-server &
SERVER_PID=$!
trap 'kill "${SERVER_PID}" 2>/dev/null || true' EXIT

# Give the server a moment to bind. A polling loop is overkill for a
# 5-route smoke; 2 s is comfortably more than the SQLite migration cost.
sleep 2

BASE="${FLOW_SERVER_BASE_URL:-http://localhost:8080}"

# ---- 1. healthz still works (bearer routes untouched) -----------------------

info "[1] /healthz responds"
curl -fsSL "${BASE}/healthz" -o /tmp/smoke-m6-healthz.txt
grep -qi "ok" /tmp/smoke-m6-healthz.txt \
  || fail "/healthz body missing 'ok': $(cat /tmp/smoke-m6-healthz.txt)"
ok "/healthz returned ok"

# ---- 2. anonymous root redirects to /auth/landing ---------------------------

info "[2] anonymous GET / redirects to /auth/landing"
status="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}/")"
location="$(curl -s -o /dev/null -w '%{redirect_url}' "${BASE}/")"
[[ "${status}" == "302" ]] \
  || fail "expected 302 from anonymous /, got ${status}"
[[ "${location}" == *"/auth/landing"* ]] \
  || fail "redirect target was '${location}', expected to contain /auth/landing"
ok "anonymous / → 302 ${location}"

# ---- 3. landing page renders without auth -----------------------------------

info "[3] /auth/landing renders the login template"
curl -fsSL "${BASE}/auth/landing" -o /tmp/smoke-m6-landing.html
grep -qi "anmelden\\|login\\|sign in" /tmp/smoke-m6-landing.html \
  || fail "/auth/landing body looks empty: $(head -c 400 /tmp/smoke-m6-landing.html)"
ok "landing rendered $(wc -c </tmp/smoke-m6-landing.html) bytes"

# ---- 4. static assets are reachable without a session cookie ----------------

info "[4] /static/styles.css reachable"
curl -fsS "${BASE}/static/styles.css" -o /tmp/smoke-m6-styles.css
[[ -s /tmp/smoke-m6-styles.css ]] \
  || fail "/static/styles.css is empty"
ok "/static/styles.css served $(wc -c </tmp/smoke-m6-styles.css) bytes"

info "[5] /static/apexcharts.min.js reachable"
curl -fsS "${BASE}/static/apexcharts.min.js" -o /tmp/smoke-m6-apex.js
[[ -s /tmp/smoke-m6-apex.js ]] \
  || fail "/static/apexcharts.min.js is empty"
ok "/static/apexcharts.min.js served $(wc -c </tmp/smoke-m6-apex.js) bytes"

# ---- 6. cookie-authed routes refuse without session -------------------------
#
# Each WebUI route under cookie middleware must 302→/auth/landing when
# the request carries no cookie. We don't try to mint a session — that
# requires a real OIDC token exchange; Plan E · Task 16 covers it.

for path in / /worktime /notes /repos /projects /settings; do
  info "[6] anonymous GET ${path} → /auth/landing"
  status="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}${path}")"
  [[ "${status}" == "302" ]] \
    || fail "expected 302 for anonymous ${path}, got ${status}"
done
ok "all cookie-authed routes redirect when unauthenticated"

echo ""
echo "== M6 anonymous smoke PASSED =="
