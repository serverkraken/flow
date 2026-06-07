#!/usr/bin/env bash
# litestream-restore-drill.sh — proves the Litestream backup is actually usable.
#
# What it does
#   1. Sanity-checks prerequisites: docker/podman compose stack from
#      deploy/podman/ is running, sqlite3 is on PATH, .env has the
#      LITESTREAM_* keys.
#   2. Reads the per-table row counts from the LIVE server.db inside the
#      flow-data volume.
#   3. Spawns a one-shot litestream container that restores the replica from
#      MinIO into a temp directory.
#   4. Compares the row counts between live and restored DB. Restored must
#      be within `TOLERANCE` rows below live for every tracked table
#      (default 5 — covers in-flight rows that haven't synced yet).
#   5. Prints PASS or FAIL and exits accordingly.
#
# What it does NOT do
#   - Touch the live server.db. The drill is read-only on prod state.
#   - Run as part of `make ci`. This is a manual-trigger check.
#
# Usage
#   make drill-restore                 # convenience wrapper
#   ./scripts/litestream-restore-drill.sh
#
# Override knobs (env)
#   COMPOSE_DIR=deploy/podman          # where docker-compose.yml lives
#   COMPOSE=podman-compose             # or `docker compose`
#   TOLERANCE=5                        # max rows live - restored per table
#
# Exit codes
#   0  drill PASS
#   1  prerequisite missing / restore failed
#   2  drill FAIL (row counts diverge beyond tolerance)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

COMPOSE_DIR="${COMPOSE_DIR:-deploy/podman}"
COMPOSE="${COMPOSE:-podman-compose}"
TOLERANCE="${TOLERANCE:-5}"

# Tables checked. Order matters only for output readability.
TABLES=(
  users
  projects
  sessions
  active_sessions
  repos
  repo_notes
)

# ---- helpers ----------------------------------------------------------------

info() { printf '\n[drill] %s\n' "$*"; }
ok()   { printf '  OK    %s\n' "$*"; }
warn() { printf '  WARN  %s\n' "$*" >&2; }
die()  { printf '  FAIL  %s\n' "$*" >&2; exit 1; }

TMP_DIR=""
cleanup() {
  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi
  # Best-effort: remove the helper container if it lingered.
  ${COMPOSE} -f "${COMPOSE_DIR}/docker-compose.yml" rm -fsv litestream-restore >/dev/null 2>&1 || true
}
trap cleanup EXIT

# ---- 1. prerequisites -------------------------------------------------------

info "Phase 1: prerequisites"

command -v sqlite3 >/dev/null 2>&1 || die "sqlite3 not on PATH"
ok "sqlite3 present: $(sqlite3 --version | awk '{print $1}')"

command -v "${COMPOSE%% *}" >/dev/null 2>&1 || die "${COMPOSE} not on PATH (override with COMPOSE=...)"
ok "${COMPOSE} present"

if [[ ! -f "${COMPOSE_DIR}/.env" ]]; then
  die ".env missing under ${COMPOSE_DIR}/ — copy .env.example first"
fi

# Pull LITESTREAM_* out of .env for the restore container.
# shellcheck disable=SC1091
set -a
source "${COMPOSE_DIR}/.env"
set +a

: "${LITESTREAM_ACCESS_KEY_ID:?LITESTREAM_ACCESS_KEY_ID not set in ${COMPOSE_DIR}/.env}"
: "${LITESTREAM_SECRET_ACCESS_KEY:?LITESTREAM_SECRET_ACCESS_KEY not set in ${COMPOSE_DIR}/.env}"
ok "LITESTREAM_* keys present"

# Stack must be running so we can talk to MinIO + read the live DB.
if ! ${COMPOSE} -f "${COMPOSE_DIR}/docker-compose.yml" ps --services --filter status=running 2>/dev/null | grep -q '^flow-server$'; then
  die "flow-server container is not running — start the stack with: cd ${COMPOSE_DIR} && ${COMPOSE} up -d"
fi
ok "compose stack is up"

# ---- 2. snapshot live row counts -------------------------------------------

info "Phase 2: live row counts (from flow-data volume)"

declare -A LIVE_COUNTS

# Read the live DB by exec'ing a sqlite3 inside the flow-server container would
# require a shell + sqlite3 in the distroless image (neither is there). Instead
# we mount the volume into a throw-away alpine container.
read_live_count() {
  local table="$1"
  ${COMPOSE} -f "${COMPOSE_DIR}/docker-compose.yml" run --rm --no-deps \
    -v "flow-data:/var/lib/flow:ro" \
    --entrypoint sh \
    minio-setup -c "
      apk add --no-cache sqlite >/dev/null 2>&1 || true
      sqlite3 /var/lib/flow/server.db 'SELECT COUNT(*) FROM ${table};' 2>/dev/null || echo NA
    " 2>/dev/null | tr -d '\r' | tail -1
}

for t in "${TABLES[@]}"; do
  count="$(read_live_count "${t}")"
  if [[ "${count}" == "NA" || -z "${count}" ]]; then
    warn "table ${t}: not present in live DB (skipping)"
    LIVE_COUNTS["${t}"]=""
    continue
  fi
  LIVE_COUNTS["${t}"]="${count}"
  printf '  live  %-18s %s\n' "${t}" "${count}"
done

# ---- 3. restore replica into temp dir --------------------------------------

info "Phase 3: restore replica from MinIO"

TMP_DIR="$(mktemp -d -t flow-restore.XXXXXX)"
ok "temp dir: ${TMP_DIR}"

# Run litestream in restore mode. The image already lives in the compose
# stack (litestream service) so no extra pull cost. We re-use the same
# litestream.yml config so source/replica paths match prod.
${COMPOSE} -f "${COMPOSE_DIR}/docker-compose.yml" run --rm \
  --no-deps \
  -v "${TMP_DIR}:/restore:Z" \
  --entrypoint litestream \
  litestream \
  restore -config /etc/litestream.yml -o /restore/server.db /var/lib/flow/server.db \
  || die "litestream restore failed — check MinIO connectivity + replica path"

if [[ ! -s "${TMP_DIR}/server.db" ]]; then
  die "restored DB at ${TMP_DIR}/server.db is empty or missing"
fi
ok "restored DB: $(stat -f '%z' "${TMP_DIR}/server.db" 2>/dev/null || stat -c '%s' "${TMP_DIR}/server.db") bytes"

# ---- 4. compare row counts --------------------------------------------------

info "Phase 4: compare row counts (tolerance = ${TOLERANCE} rows)"

FAIL=0

for t in "${TABLES[@]}"; do
  live="${LIVE_COUNTS[${t}]}"
  if [[ -z "${live}" ]]; then
    continue
  fi
  restored="$(sqlite3 "${TMP_DIR}/server.db" "SELECT COUNT(*) FROM ${t};" 2>/dev/null || echo NA)"
  if [[ "${restored}" == "NA" ]]; then
    printf '  FAIL  %-18s table missing in restored DB\n' "${t}" >&2
    FAIL=1
    continue
  fi
  diff=$(( live - restored ))
  if (( diff < 0 )); then
    diff=$(( -diff ))
  fi
  if (( diff <= TOLERANCE )); then
    printf '  PASS  %-18s live=%s restored=%s (delta=%s)\n' "${t}" "${live}" "${restored}" "${diff}"
  else
    printf '  FAIL  %-18s live=%s restored=%s (delta=%s > %s)\n' "${t}" "${live}" "${restored}" "${diff}" "${TOLERANCE}" >&2
    FAIL=1
  fi
done

# ---- 5. verdict -------------------------------------------------------------

info "Phase 5: verdict"

if (( FAIL == 0 )); then
  printf '\n== DRILL PASS ==\nLitestream replica is usable; row counts match within tolerance.\n'
  exit 0
fi

printf '\n== DRILL FAIL ==\nOne or more tables diverged beyond TOLERANCE=%s rows.\n' "${TOLERANCE}" >&2
exit 2
