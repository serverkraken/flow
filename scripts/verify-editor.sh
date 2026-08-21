#!/usr/bin/env bash
# Verify that the committed editor bundle matches a fresh esbuild build.
# Fails (exit 1) on drift so CI catches an un-rebuilt bundle — the twin of
# verify-css.sh for web/editor.
set -euo pipefail

SRC_DIR="web/editor"
COMMITTED="internal/adapter/webui/static/vendor/milkdown/editor.min.js"

if ! command -v npm >/dev/null 2>&1; then
  echo "verify-editor: npm not found on PATH (Node is a build requirement for the editor)" >&2
  exit 1
fi
if [ ! -f "$COMMITTED" ]; then
  echo "verify-editor: $COMMITTED is missing — run 'make editor'" >&2
  exit 1
fi
if [ ! -d "$SRC_DIR/node_modules" ]; then
  (cd "$SRC_DIR" && npm ci --silent --no-audit --no-fund)
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
(cd "$SRC_DIR" && npx esbuild editor.mjs --bundle --minify --format=esm --target=es2022 --outfile="$tmp" --log-level=warning)

if ! diff -q "$tmp" "$COMMITTED" >/dev/null; then
  echo "verify-editor: FAIL — $COMMITTED is out of date. Run 'make editor' and commit." >&2
  exit 1
fi
echo "verify-editor: OK"
