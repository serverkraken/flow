#!/usr/bin/env bash
# Verify that the committed app.css matches a fresh tailwindcss build.
# Fails (exit 1) on drift so CI catches an un-rebuilt stylesheet.
set -euo pipefail

SRC="web/tailwind.css"
COMMITTED="internal/adapter/webui/static/app.css"

if ! command -v tailwindcss >/dev/null 2>&1; then
  echo "verify-css: tailwindcss CLI not found on PATH" >&2
  exit 1
fi
if [ ! -f "$COMMITTED" ]; then
  echo "verify-css: $COMMITTED is missing — run 'make web'" >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
tailwindcss --input "$SRC" --output "$tmp" --minify >/dev/null 2>&1

if ! diff -q "$tmp" "$COMMITTED" >/dev/null; then
  echo "verify-css: FAIL — $COMMITTED is out of date. Run 'make web' and commit." >&2
  diff "$COMMITTED" "$tmp" | head -40 >&2 || true
  exit 1
fi
echo "verify-css: OK"
