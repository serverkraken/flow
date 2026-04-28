#!/usr/bin/env bash
# Coverage gate — fails if total coverage drops below the given threshold.
set -euo pipefail

profile="${1:-coverage.out}"
threshold="${2:-70}"

if [[ ! -f "$profile" ]]; then
  printf 'coverage profile not found: %s\n' "$profile" >&2
  exit 1
fi

total="$(go tool cover -func="$profile" | awk '/^total:/ {gsub("%","",$3); print $3}')"
if [[ -z "$total" ]]; then
  printf 'could not determine coverage total from %s\n' "$profile" >&2
  exit 1
fi

if awk -v t="$total" -v th="$threshold" 'BEGIN { exit (t+0 >= th+0) ? 0 : 1 }'; then
  printf 'coverage: %s%% ≥ %s%%\n' "$total" "$threshold"
else
  printf 'coverage: %s%% < %s%% (gate failed)\n' "$total" "$threshold" >&2
  exit 1
fi
