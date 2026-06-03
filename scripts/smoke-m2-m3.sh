#!/usr/bin/env bash
# smoke-m2-m3.sh — multi-device E2E smoke for flow Phase 1 M2+M3 (sessions-sync).
#
# Prerequisites:
#   - podman + podman-compose available on PATH
#   - openssl available on PATH
#   - dex stack configured under deploy/podman/ (see make dex-up)
#   - A dex static user exists: alice@example.com / password
#     (see deploy/podman/dex-config.yaml; alice's sub is ChBhbGljZS1zdGF0aWMtdWlkEgVsb2NhbA)
#
# Usage:
#   chmod +x scripts/smoke-m2-m3.sh
#   ./scripts/smoke-m2-m3.sh
#
# The script does NOT have to be run during development — dex/podman may not be
# available in all environments. It is the human-executable companion to the
# automated integration tests. Run it when:
#   - You want to verify end-to-end multi-device sync with real OIDC tokens.
#   - You are doing a release acceptance check.
#   - You suspect a regression in sync logic that unit tests do not catch.
#
# Verb adjustments from plan template:
#   - FLOW_BASE_URL → FLOW_SERVER_BASE_URL  (actual env var name in config.go)
#   - XDG_DATA_HOME per client → FLOW_CACHE_DB per client
#     cmd/flow/main.go supports $FLOW_CACHE_DB directly (the XDG_DATA_HOME
#     path was the indirect fallback; FLOW_CACHE_DB is simpler for test isolation
#     and avoids side-effects on the kompendium index path).
#   - Phase 6 "worktime status" replaced with "worktime today --check" is not
#     available. Instead we use:
#       flow sync pull (TUI-internal; not a CLI verb)
#     so Phase 6 active-session check is done by grepping `flow worktime today`
#     output — the today-TUI prints the active session header even when piped.
#     NOTE: `flow worktime today` opens the TUI; when stdout is not a tty it
#     falls back to printing a plain summary. If the fallback is not implemented,
#     Phase 6 may time out — see Troubleshooting at the bottom of the runbook.
#
# Pull interval: 30 s (Worker.PullInterval default). sleeps are 35 s to allow
# one full pull cycle to complete.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

# ---- helpers ----------------------------------------------------------------

info()  { printf '\n[smoke] %s\n' "$*"; }
ok()    { printf '  OK  %s\n' "$*"; }
fail()  { printf '  FAIL %s\n' "$*" >&2; exit 1; }

# ---- Phase 1: clean state ---------------------------------------------------

info "Phase 1: clean state"
rm -rf /tmp/flow-smoke
mkdir -p /tmp/flow-smoke/{a,b}
ok "Temp dirs created: /tmp/flow-smoke/{a,b}"

# ---- Phase 2: build ---------------------------------------------------------

info "Phase 2: build"
make build-server
go build -o bin/flow ./cmd/flow
ok "bin/flow-server and bin/flow built"

# ---- Phase 3: dex up --------------------------------------------------------

info "Phase 3: dex up"
make dex-up
sleep 2
ok "dex container started on localhost:5556"

# ---- Phase 4: flow-server up (background) -----------------------------------

info "Phase 4: flow-server up (background)"

FLOW_SERVER_DB=/tmp/flow-smoke/server.db \
FLOW_OIDC_ISSUER=http://localhost:5556 \
FLOW_OIDC_CLIENT_ID=flow-server \
FLOW_OIDC_CLIENT_SECRET=flow-server-secret \
FLOW_COOKIE_HASH_KEY="$(openssl rand -hex 32)" \
FLOW_COOKIE_BLOCK_KEY="$(openssl rand -hex 16)" \
FLOW_ALLOWED_SUBS=ChBhbGljZS1zdGF0aWMtdWlkEgVsb2NhbA \
FLOW_SERVER_BASE_URL=http://localhost:8080 \
./bin/flow-server &

SERVER_PID=$!
trap 'kill "${SERVER_PID}" 2>/dev/null || true; make dex-down' EXIT

sleep 2
ok "flow-server started (pid=${SERVER_PID})"

# Verify server is reachable before proceeding.
if ! curl -sf http://localhost:8080/healthz >/dev/null 2>&1; then
  fail "flow-server not reachable on :8080 — check logs above"
fi
ok "Healthcheck passed"

# ---- Phase 5: client A — login + project create + worktime start ------------

info "Phase 5: client A — login + project create + worktime start"

FLOW_CACHE_DB=/tmp/flow-smoke/a/cache.db ./bin/flow \
  login --server=http://localhost:8080 --client-id=flow-cli

FLOW_CACHE_DB=/tmp/flow-smoke/a/cache.db ./bin/flow \
  projects create "Smoke A"

FLOW_CACHE_DB=/tmp/flow-smoke/a/cache.db ./bin/flow \
  worktime start --project=smoke-a

ok "Client A: logged in, created project 'Smoke A', started session"

info "Waiting 35 s for sync tick ..."
sleep 35

# ---- Phase 6: client B — login + projects list should show 'Smoke A' --------

info "Phase 6: client B — login + projects list + active session check"

FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db ./bin/flow \
  login --server=http://localhost:8080 --client-id=flow-cli

FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db ./bin/flow \
  projects list \
  | grep -q "Smoke A" \
  && ok "Project synced to client B" \
  || fail "Project 'Smoke A' missing on client B after sync"

# worktime today prints the active session in non-TUI (pipe) mode.
# If this grep fails the binary may not have a plain-text fallback yet;
# see Troubleshooting in docs/runbook/m2-m3-smoke-test.md.
FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db ./bin/flow \
  worktime today 2>/dev/null \
  | grep -qi "Smoke A" \
  && ok "Active session visible on client B" \
  || fail "Active session not synced to client B (worktime today output did not mention 'Smoke A')"

# ---- Phase 7: client B tries to start same project (expects conflict) -------

info "Phase 7: client B tries to start same project (expects conflict)"

FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db ./bin/flow \
  worktime start --project=smoke-a 2>&1 \
  | grep -qiE "konflikt|conflict|already|läuft|running" \
  && ok "Server-side race / conflict detected as expected" \
  || fail "Race NOT detected — expected an error starting a session that is already active on A"

# ---- Phase 8: client A stop, client B should see Session row ----------------

info "Phase 8: client A stop"

FLOW_CACHE_DB=/tmp/flow-smoke/a/cache.db ./bin/flow \
  worktime stop --project=smoke-a

ok "Client A: session stopped"

info "Waiting 35 s for sync tick ..."
sleep 35

FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db ./bin/flow \
  worktime today 2>/dev/null \
  | grep -qi "Smoke A" \
  && ok "Stopped session synced to client B (visible in today's list)" \
  || fail "Stopped session missing on client B after sync"

# ---- Phase 9: TSV migration smoke -------------------------------------------

info "Phase 9: TSV migration smoke (standalone; no server required)"

TSV_COPY=/tmp/flow-smoke/worktime.log.copy

if [[ -f "${HOME}/.tmux/worktime.log" ]]; then
  cp "${HOME}/.tmux/worktime.log" "${TSV_COPY}"

  # First run: import rows.
  FLOW_CACHE_DB=/tmp/flow-smoke/migrate.db ./bin/flow \
    worktime migrate-from-tsv --tsv="${TSV_COPY}" \
    | tee /tmp/flow-smoke/migrate-run1.txt

  ROW_COUNT="$(sqlite3 /tmp/flow-smoke/migrate.db \
    'SELECT COUNT(*) FROM worktime_sessions;' 2>/dev/null || echo '0')"
  ok "TSV import run 1: ${ROW_COUNT} sessions in DB"

  # Restore copy (migrate-from-tsv renames the source file).
  if [[ -f "${TSV_COPY}.migrated-"* ]] 2>/dev/null; then
    RENAMED_TSV="$(ls "${TSV_COPY}".migrated-* | head -1)"
    cp "${RENAMED_TSV}" "${TSV_COPY}"
  fi

  # Idempotency check: re-import from the same data; row count must not grow.
  FLOW_CACHE_DB=/tmp/flow-smoke/migrate.db ./bin/flow \
    worktime migrate-from-tsv --tsv="${TSV_COPY}" \
    | tee /tmp/flow-smoke/migrate-run2.txt

  ROW_COUNT2="$(sqlite3 /tmp/flow-smoke/migrate.db \
    'SELECT COUNT(*) FROM worktime_sessions;' 2>/dev/null || echo '0')"

  if [[ "${ROW_COUNT}" == "${ROW_COUNT2}" ]]; then
    ok "TSV migration is idempotent (${ROW_COUNT} rows both runs)"
  else
    fail "Idempotency violation: run 1 = ${ROW_COUNT} rows, run 2 = ${ROW_COUNT2} rows"
  fi
else
  info "  ~/.tmux/worktime.log not found — skipping TSV migration smoke"
fi

# ---- Phase 10: cleanup ------------------------------------------------------

info "Phase 10: cleanup"
# EXIT trap handles: kill SERVER_PID + make dex-down + rm not needed (leave for inspection)
ok "Trap will stop server + dex on exit"

echo ""
echo "== ALL SMOKE PASSED =="
