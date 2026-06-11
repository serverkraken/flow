#!/usr/bin/env bash
# scripts/smoke-r2a-client.sh — R2a Client-Smoke: beweist dass der flow-Client
# korrekt auf Not-Configured / LoggedOut / Server-Down reagiert.
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
  if ! echo "$actual_out" | grep -q "$expect_out"; then
    echo "FAIL $desc: output missing '$expect_out'" >&2
    echo "  got: $actual_out" >&2; FAIL=$((FAIL+1)); return
  fi
  echo "ok   $desc"; PASS=$((PASS+1))
}

make build >/dev/null 2>&1

# Test 1: No FLOW_SERVER_URL — write command aborts with clear message
check "no-server-url: worktime start" 1 "FLOW_SERVER_URL nicht gesetzt" \
  env -u FLOW_SERVER_URL ./bin/flow worktime start

# Test 2: No FLOW_SERVER_URL — read command also aborts with clear message
check "no-server-url: worktime export" 1 "FLOW_SERVER_URL nicht gesetzt" \
  env -u FLOW_SERVER_URL ./bin/flow worktime export today

# Test 3: Server URL set but no login — write command aborts with login hint
check "no-login: worktime start" 1 "nicht angemeldet" \
  env FLOW_SERVER_URL=http://localhost:18080 ./bin/flow worktime start --project test-project

# Test 4: Server URL set but no login — read command aborts with login hint
check "no-login: worktime export" 1 "nicht angemeldet" \
  env FLOW_SERVER_URL=http://localhost:18080 ./bin/flow worktime export today

# Test 5: whoami without any config — shows "nicht eingeloggt" with login hint
check "no-server-url: whoami" 1 "flow login" \
  env -u FLOW_SERVER_URL ./bin/flow whoami

# Test 6: whoami with server URL but no token — same graceful error
check "no-login: whoami" 1 "flow login" \
  env FLOW_SERVER_URL=http://localhost:18080 ./bin/flow whoami

echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
