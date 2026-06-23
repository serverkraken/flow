#!/usr/bin/env bash
# Enforce the design rule: NO native browser popups in OUR WebUI code.
# Confirmations and alerts must use the in-design <dialog> components.
#
# Scope (CONTROLLER CORRECTION — the original plan scanned the whole webui dir,
# which falsely matched third-party vendor JS and test fixtures):
#   - EXCLUDE third-party vendored JS (static/vendor/**): htmx legitimately ships
#     a native confirm() for its hx-confirm attribute — that is not our code.
#   - EXCLUDE *_test.go: sanitization tests intentionally embed "<script>alert(1)"
#     as XSS input fixtures; those are not UI popups.
#   - Our own static/js/*.js (e.g. dialog.js) IS still scanned.
set -euo pipefail

# rg is required (repo convention); grep -E fallback keeps CI portable.
pattern='window\.(alert|confirm|prompt)|[^.a-zA-Z](alert|confirm|prompt)[[:space:]]*\('
dir="internal/adapter/webui"

if command -v rg >/dev/null 2>&1; then
  hits="$(rg -n --pcre2 "$pattern" "$dir" -g '!*_test.go' -g '!**/static/vendor/**' || true)"
else
  hits="$(grep -rnE "$pattern" "$dir" --include='*.go' --include='*.templ' --include='*.js' \
            --exclude='*_test.go' --exclude-dir=vendor || true)"
fi

if [ -n "$hits" ]; then
  echo "verify-no-popups: FAIL — native browser popups are banned (use Dialog/ConfirmDialog):" >&2
  echo "$hits" >&2
  exit 1
fi
echo "verify-no-popups: OK"
