#!/usr/bin/env bash
# smoke-m7-webui-write.sh — boot-time smoke for the Plan E · M7 WebUI write
# surface and Plan E · Task 14 SSE endpoint.
#
# Confirms the M6 read-only probes still pass and then exercises the M7
# mutations (project create / rename / archive) plus the SSE stream when a
# real OIDC token + cookie session can be minted. Mutation probes are
# SKIPPED when the OIDC token-exchange env vars are not exported — the
# minimum bar is "M6 read paths still work + binary boots". The full
# cookie round trip lives in scripts/run-flow-server.sh + the Plan E ·
# Task 16 manual E2E.
#
# Prerequisites:
#   - go toolchain on PATH (the script builds bin/flow-server if missing)
#   - FLOW_* env vars exported (or use scripts/run-flow-server.sh which
#     sets sensible local-dev defaults against deploy/podman dex)
#   - An OIDC issuer reachable at FLOW_OIDC_ISSUER
#
# Optional (mutation probes only):
#   - FLOW_SMOKE_OIDC_ID_TOKEN  — a valid ID-Token for the configured
#     issuer/audience that the server will verify before issuing a session
#     cookie. Mint it once via dex's resource-owner password grant
#     against deploy/podman/dex-config.yaml (alice@example.com / password)
#     and export it.
#
# Usage:
#   chmod +x scripts/smoke-m7-webui-write.sh
#   ./scripts/smoke-m7-webui-write.sh
#
# Exits non-zero on the first failing assertion.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

info() { printf '\n[smoke-m7] %s\n' "$*"; }
ok()   { printf '  OK   %s\n' "$*"; }
skip() { printf '  SKIP %s\n' "$*"; }
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
export FLOW_SERVER_DB="${FLOW_SERVER_DB:-/tmp/flow-smoke-m7/server.db}"
mkdir -p "$(dirname "${FLOW_SERVER_DB}")"

info "start flow-server (background)"
./bin/flow-server &
SERVER_PID=$!
trap 'kill "${SERVER_PID}" 2>/dev/null || true' EXIT

BASE="${FLOW_SERVER_BASE_URL:-http://localhost:8080}"

# Wait up to 10s for server to come up. OIDC discovery can stall on cold
# DNS — fail loudly with a hint rather than hammering a dead port.
for i in $(seq 1 10); do
    if curl -sf "${BASE}/healthz" -o /dev/null 2>/dev/null; then
        break
    fi
    sleep 1
    if [ "$i" -eq 10 ]; then
        echo "FAIL: flow-server did not respond on ${BASE}/healthz within 10s"
        echo "      (check FLOW_OIDC_ISSUER reachability and FLOW_* env)"
        exit 1
    fi
done

echo "[1] healthz reachable"

# ---- Part A: M6 read-only paths still work (regression guard) ---------------

info "[A1] /healthz responds"
curl -fsSL "${BASE}/healthz" -o /tmp/smoke-m7-healthz.txt
grep -qi "ok" /tmp/smoke-m7-healthz.txt \
  || fail "/healthz body missing 'ok': $(cat /tmp/smoke-m7-healthz.txt)"
ok "/healthz returned ok"

info "[A2] /auth/landing renders"
curl -fsSL "${BASE}/auth/landing" -o /tmp/smoke-m7-landing.html
grep -qi "anmelden\\|login\\|sign in" /tmp/smoke-m7-landing.html \
  || fail "/auth/landing body looks empty"
ok "landing rendered $(wc -c </tmp/smoke-m7-landing.html) bytes"

info "[A3] static assets reachable"
for asset in styles.css apexcharts.min.js htmx.min.js htmx-sse.min.js alpine.min.js; do
  curl -fsS "${BASE}/static/${asset}" -o /tmp/smoke-m7-asset.bin
  [[ -s /tmp/smoke-m7-asset.bin ]] || fail "/static/${asset} is empty"
done
ok "static bundle served"

info "[A4] anonymous cookie-authed routes redirect"
for path in / /worktime /notes /repos /projects /settings; do
  status="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}${path}")"
  [[ "${status}" == "302" ]] \
    || fail "expected 302 for anonymous ${path}, got ${status}"
done
ok "all M6 cookie-authed routes redirect when unauthenticated"

info "[A5] anonymous SSE endpoint redirects to landing"
status="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}/api/v1/events?stream=ui")"
[[ "${status}" == "302" || "${status}" == "401" ]] \
  || fail "expected 302/401 for anonymous /api/v1/events, got ${status}"
ok "/api/v1/events refuses unauthenticated requests"

# ---- Part B: M7 mutation probes (require an OIDC ID-Token) ------------------
#
# The server's BrowserAuthMiddleware mints a session cookie from a valid
# ID-Token via the cookie-exchange callback. Without a real token the
# write surface stays gated behind /auth/landing — we'd be testing the
# redirect, not the handlers. SKIP gracefully when the env var is unset
# so this script stays useful in CI without dex.

if [[ -z "${FLOW_SMOKE_OIDC_ID_TOKEN:-}" ]]; then
  skip "FLOW_SMOKE_OIDC_ID_TOKEN unset — M7 mutation probes skipped"
  skip "  (run \`make dex-up\` and mint a token to exercise this branch)"
  echo ""
  echo "== M7 anonymous smoke PASSED (mutations skipped) =="
  exit 0
fi

# Token-exchange endpoint mirrors the OIDC client wiring in
# httpserver/auth_browser.go: POST the bearer token to the callback path
# and read the Set-Cookie header.
COOKIE_JAR="$(mktemp)"
trap 'rm -f "${COOKIE_JAR}"; kill "${SERVER_PID}" 2>/dev/null || true' EXIT

info "[B0] exchange ID-Token for session cookie"
status="$(curl -s -o /tmp/smoke-m7-callback.html -w '%{http_code}' \
  -c "${COOKIE_JAR}" \
  -H "Authorization: Bearer ${FLOW_SMOKE_OIDC_ID_TOKEN}" \
  "${BASE}/auth/callback")"
if [[ "${status}" != "200" && "${status}" != "302" ]]; then
  fail "/auth/callback rejected token: status=${status}; body=$(head -c 400 /tmp/smoke-m7-callback.html)"
fi
grep -q "flow_session" "${COOKIE_JAR}" \
  || fail "no flow_session cookie set after token exchange"
ok "session cookie minted"

# Curl wrapper that carries the session cookie.
authcurl() { curl -fsS -b "${COOKIE_JAR}" "$@"; }

info "[B1] POST /projects creates a project + returns row partial"
PROJECT_NAME="Smoke M7 $(date +%s)"
authcurl -X POST \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "name=${PROJECT_NAME}" \
  -o /tmp/smoke-m7-create.html \
  "${BASE}/projects"
grep -q 'data-testid="projects-row"' /tmp/smoke-m7-create.html \
  || fail "POST /projects body missing projects-row partial"
grep -q "${PROJECT_NAME}" /tmp/smoke-m7-create.html \
  || fail "POST /projects body missing project name"
ok "POST /projects → row rendered"

# Extract the new project ID from the row's data-id attribute. The
# templ-generated row uses data-id="<uuid>" — same shape as repos_vm.
PROJECT_ID="$(sed -n 's/.*data-id="\([0-9a-f-]\{36\}\)".*/\1/p' /tmp/smoke-m7-create.html | head -1)"
[[ -n "${PROJECT_ID}" ]] || fail "could not extract project ID from create response"
ok "created project id=${PROJECT_ID}"

info "[B2] PUT /projects/{id} renames the project"
authcurl -X PUT \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "name=${PROJECT_NAME} (renamed)" \
  --data-urlencode "version=1" \
  -o /tmp/smoke-m7-rename.html \
  "${BASE}/projects/${PROJECT_ID}"
grep -q 'data-testid="projects-row"' /tmp/smoke-m7-rename.html \
  || fail "PUT /projects/{id} body missing projects-row partial"
grep -q "${PROJECT_NAME} (renamed)" /tmp/smoke-m7-rename.html \
  || fail "PUT /projects/{id} body missing new name"
ok "PUT /projects/{id} → renamed row"

info "[B3] POST /projects/{id}/archive returns archived row"
authcurl -X POST -o /tmp/smoke-m7-archive.html \
  "${BASE}/projects/${PROJECT_ID}/archive"
grep -q 'data-testid="projects-row"' /tmp/smoke-m7-archive.html \
  || fail "POST archive body missing projects-row partial"
# Archived rows are rendered with the archived eyebrow / strikethrough —
# the marker is the data-archived attribute set on the row.
grep -q 'data-archived="true"' /tmp/smoke-m7-archive.html \
  || fail "POST archive did not flag row as archived"
ok "POST archive → archived row"

info "[B4] GET /api/v1/events streams text/event-stream"
# Curl with --max-time keeps the long-lived stream from hanging the
# script. The handler always emits the ": connected" comment immediately,
# so a 1s window is enough to assert headers + first bytes.
http_code_and_ct="$(curl -s -b "${COOKIE_JAR}" \
  --max-time 1 \
  -o /tmp/smoke-m7-sse.txt \
  -w '%{http_code} %{content_type}' \
  "${BASE}/api/v1/events?stream=ui" || true)"
read -r sse_code sse_ct <<<"${http_code_and_ct}"
[[ "${sse_code}" == "200" ]] \
  || fail "GET /api/v1/events status=${sse_code}, want 200"
[[ "${sse_ct}" == text/event-stream* ]] \
  || fail "GET /api/v1/events content-type=${sse_ct}, want text/event-stream*"
grep -q ": connected" /tmp/smoke-m7-sse.txt \
  || fail "SSE stream did not emit initial ': connected' comment"
ok "GET /api/v1/events → text/event-stream + initial ping"

echo ""
echo "== M7 write-surface smoke PASSED =="
