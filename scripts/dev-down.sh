#!/usr/bin/env bash
# Tear down the dev dependencies. Pass -v to also drop the postgres volume.
set -euo pipefail
cd "$(dirname "$0")/.."
podman compose -f deploy/dev/compose.yml down "$@"
