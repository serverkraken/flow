#!/usr/bin/env bash
set -euo pipefail
out="$1"; threshold="$2"
# Exclude generated templ files (*_templ.go) from the gate: generated render
# code — including the !IsBuffer defer cleanup paths that production (always
# buffered) never hits — should not count toward coverage. Hand-written Go is
# what the gate measures; handler tests assert the rendered HTML output instead.
filtered="$(mktemp)"
grep -v '_templ\.go:' "$out" > "$filtered" || true
pct=$(go tool cover -func="$filtered" | awk '/^total:/ {gsub(/%/,"",$3); print $3}')
rm -f "$filtered"
echo "coverage: ${pct}% (threshold ${threshold}%, excl. *_templ.go)"
awk -v p="$pct" -v t="$threshold" 'BEGIN { exit (p+0 >= t+0) ? 0 : 1 }' \
  || { echo "FAIL: coverage ${pct}% < ${threshold}%"; exit 1; }
