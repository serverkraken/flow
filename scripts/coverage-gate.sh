#!/usr/bin/env bash
set -euo pipefail
out="$1"; threshold="$2"
pct=$(go tool cover -func="$out" | awk '/^total:/ {gsub(/%/,"",$3); print $3}')
echo "coverage: ${pct}% (threshold ${threshold}%)"
awk -v p="$pct" -v t="$threshold" 'BEGIN { exit (p+0 >= t+0) ? 0 : 1 }' \
  || { echo "FAIL: coverage ${pct}% < ${threshold}%"; exit 1; }
