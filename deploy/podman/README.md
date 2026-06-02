# flow-server — Podman dev deployment

Stack: flow-server + dex (OIDC IdP), both as containers.

## Setup

```bash
cp .env.example .env
sed -i.bak "s|FLOW_COOKIE_HASH_KEY=|FLOW_COOKIE_HASH_KEY=$(openssl rand -hex 32)|" .env
sed -i.bak "s|FLOW_COOKIE_BLOCK_KEY=|FLOW_COOKIE_BLOCK_KEY=$(openssl rand -hex 16)|" .env
rm .env.bak
```

## Run

```bash
podman-compose up -d
# or: docker-compose up -d
```

## Smoke

```bash
curl http://localhost:8080/healthz   # → "ok"
curl http://localhost:8080/readyz    # → "ready"
curl http://localhost:8080/api/v1/oidc/config | jq

# Browser-flow:
# Open http://localhost:8080/login → land on dex → log in as alice@example.com / password
# → redirected back, flow_session cookie set
# Then GET /api/v1/me should return alice's identity.

# CLI device-flow:
# In a separate shell:
flow login --server=http://localhost:8080 --client-id=flow-cli
# (this needs your built `flow` binary; dex 2.41.1 device-flow approval form
# is browser-only — for full CLI testing use Authentik in prod.)
```

## Stop

```bash
podman-compose down
```
