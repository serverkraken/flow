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

printf 'waiting for ollama'
ok_ollama=
for _ in $(seq 1 30); do
  if curl -fsS http://localhost:11434/api/tags >/dev/null 2>&1; then ok_ollama=1; break; fi
  printf '.'; sleep 1
done
[ -n "$ok_ollama" ] && echo " ready" || { echo " TIMEOUT"; exit 1; }

echo "pulling embedding model (nomic-embed-text)…"
podman compose -f "$COMPOSE" exec -T ollama ollama pull nomic-embed-text

cat <<'EOF'

dev env up.
  start server:  make dev-run
  browser:       http://localhost:8080/      (login: msoent@dev.local / password)
  TUI token:     export FLOW_TOKEN=$(make -s dev-token)
  ollama:        http://localhost:11434      (embedding model nomic-embed-text)
  tear down:     make dev-down               (add ARGS=-v to drop the db volume)
EOF
