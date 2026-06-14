#!/usr/bin/env bash
# Bring up the self-contained dev dependencies (Postgres + Dex OIDC) via podman.
# flow-server itself runs on the host — see deploy/dev/README.md.
set -euo pipefail
cd "$(dirname "$0")/.."
COMPOSE="deploy/dev/compose.yml"

podman compose -f "$COMPOSE" up -d

printf 'waiting for postgres'
ok_pg=
for _ in $(seq 1 30); do
  if podman compose -f "$COMPOSE" exec -T db pg_isready -U flow -d flow >/dev/null 2>&1; then ok_pg=1; break; fi
  printf '.'; sleep 1
done
[ -n "$ok_pg" ] && echo " ready" || { echo " TIMEOUT"; exit 1; }

printf 'waiting for dex'
ok_dex=
for _ in $(seq 1 30); do
  if curl -fsS http://localhost:5556/dex/.well-known/openid-configuration >/dev/null 2>&1; then ok_dex=1; break; fi
  printf '.'; sleep 1
done
[ -n "$ok_dex" ] && echo " ready" || { echo " TIMEOUT"; exit 1; }

cat <<'EOF'

dev env up.
  start server:  make dev-run
  browser:       http://localhost:8080/      (login: msoent@dev.local / password)
  TUI token:     export FLOW_TOKEN=$(make -s dev-token)
  tear down:     make dev-down               (add ARGS=-v to drop the db volume)
EOF
