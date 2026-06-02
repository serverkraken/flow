#!/usr/bin/env bash
# Convenience launcher for `flow-server` against the local dex stack from
# `deploy/podman/`.
#
# Use this for local smoke testing. Production deployments should use the
# Helm chart (Phase F) or a hand-tuned podman-compose / k8s manifest.
#
# Required tools: openssl (cookie key generation), go build (`make
# build-server` will be invoked if `bin/flow-server` is missing).
#
# Override any default by exporting before running, e.g.:
#   FLOW_SERVER_ADDR=:9090 ./scripts/run-flow-server.sh
#   FLOW_OIDC_ISSUER=https://auth.real.com/realms/flow \
#     FLOW_OIDC_CLIENT_SECRET=… ./scripts/run-flow-server.sh
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ ! -x bin/flow-server ]]; then
  echo "→ building flow-server (bin/flow-server missing)..." >&2
  make build-server
fi

# Defaults match deploy/podman/dex-config.yaml so the local dex stack works
# out of the box. Override by exporting FLOW_* before running.
export FLOW_SERVER_ADDR="${FLOW_SERVER_ADDR:-:8080}"
export FLOW_SERVER_BASE_URL="${FLOW_SERVER_BASE_URL:-http://localhost:8080}"
export FLOW_OIDC_ISSUER="${FLOW_OIDC_ISSUER:-http://localhost:5556}"
export FLOW_OIDC_CLIENT_ID="${FLOW_OIDC_CLIENT_ID:-flow-server}"
export FLOW_OIDC_CLIENT_SECRET="${FLOW_OIDC_CLIENT_SECRET:-flow-server-secret}"
export FLOW_ALLOWED_SUBS="${FLOW_ALLOWED_SUBS:-alice-static-uid}"

# Cookie keys persist across runs in .flow-cookie-keys (gitignored) so the
# session cookie stays valid between restarts. Generate once if missing.
key_file=".flow-cookie-keys"
if [[ ! -f "$key_file" ]]; then
  echo "→ generating fresh cookie keys at $key_file (gitignored)..." >&2
  {
    echo "FLOW_COOKIE_HASH_KEY=$(openssl rand -hex 32)"
    echo "FLOW_COOKIE_BLOCK_KEY=$(openssl rand -hex 16)"
  } > "$key_file"
fi
# shellcheck disable=SC1090
source "$key_file"
export FLOW_COOKIE_HASH_KEY FLOW_COOKIE_BLOCK_KEY

echo "→ flow-server starting on ${FLOW_SERVER_ADDR}, IdP ${FLOW_OIDC_ISSUER}"
exec ./bin/flow-server
