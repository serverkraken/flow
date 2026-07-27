#!/usr/bin/env bash
# Run flow-server on the host against the dev dependencies (run `make dev-up` first).
set -euo pipefail
cd "$(dirname "$0")/.."
set -a; . deploy/dev/flow.env; set +a
exec go run ./cmd/flow-server
