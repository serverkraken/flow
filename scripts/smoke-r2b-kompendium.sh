#!/usr/bin/env bash
# scripts/smoke-r2b-kompendium.sh — R2b Smoke: Kompendium-Client prüft auf
# Not-Configured / LoggedOut-Reaktion (Server-Dogfood-Pfade: Task 9 / manuell).
# Voraussetzungen: gebautes Repo (make build).
set -euo pipefail
cd "$(dirname "$0")/.."

PASS=0; FAIL=0
check() {
  local desc="$1" expect_exit="$2" expect_out="$3"
  shift 3
  local actual_out actual_exit
  actual_out=$("$@" 2>&1) && actual_exit=0 || actual_exit=$?
  if [ "$actual_exit" -ne "$expect_exit" ]; then
    echo "FAIL $desc: exit $actual_exit (want $expect_exit)" >&2; FAIL=$((FAIL+1)); return
  fi
  if ! echo "$actual_out" | grep -qiE "$expect_out"; then
    echo "FAIL $desc: output missing '$expect_out'" >&2
    echo "  got: $actual_out" >&2; FAIL=$((FAIL+1)); return
  fi
  echo "ok   $desc"; PASS=$((PASS+1))
}

make build >/dev/null 2>&1

# Temp dir for docs import tests
TMPDIR_TEST=$(mktemp -d)
trap 'rm -rf "$TMPDIR_TEST"' EXIT
echo "# test" > "$TMPDIR_TEST/test.md"

# Test 1: No FLOW_SERVER_URL — kompendium ls aborts with clear message
check "no-server-url: kompendium ls" 1 "konfiguriert|FLOW_SERVER_URL" \
  env -u FLOW_SERVER_URL ./bin/flow kompendium ls

# Test 2: No FLOW_SERVER_URL — docs import aborts with clear message
check "no-server-url: docs import" 1 "FLOW_SERVER_URL nicht gesetzt|konfiguriert" \
  env -u FLOW_SERVER_URL ./bin/flow docs import "$TMPDIR_TEST"

# Test 3: Server URL set but no login — kompendium ls aborts with login hint
check "no-login: kompendium ls" 1 "nicht angemeldet|login|einloggen" \
  env FLOW_SERVER_URL=http://localhost:18080 ./bin/flow kompendium ls

# Test 4: Server URL set but no login — kompendium search aborts with login hint
check "no-login: kompendium search" 1 "nicht angemeldet|login|einloggen" \
  env FLOW_SERVER_URL=http://localhost:18080 ./bin/flow kompendium search some-query

# Test 5: Server URL set but no login — docs import aborts with login hint
check "no-login: docs import" 1 "nicht angemeldet|login|einloggen" \
  env FLOW_SERVER_URL=http://localhost:18080 ./bin/flow docs import "$TMPDIR_TEST"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
