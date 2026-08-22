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
trap 'rm -f "$tmp" "$tmp.js" "$tmp.css"' EXIT
(cd "$SRC_DIR" && node build.mjs --out "$tmp")

if ! diff -q "$tmp.js" "$COMMITTED" >/dev/null; then
  echo "verify-editor: FAIL — $COMMITTED is out of date. Run 'make editor' and commit." >&2
  exit 1
fi
# esbuild schreibt das gebündelte Theme-CSS neben die JS-Datei (gleicher Name, .css).
if ! diff -q "$tmp.css" "${COMMITTED%.js}.css" >/dev/null; then
  echo "verify-editor: FAIL — ${COMMITTED%.js}.css is out of date. Run 'make editor' and commit." >&2
  exit 1
fi
echo "verify-editor: OK"
