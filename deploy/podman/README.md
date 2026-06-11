# flow-server — Podman / Docker dev deployment

Local compose stack with everything needed to exercise flow-server end-to-end:

| Service       | Image                          | Purpose                              |
|---------------|--------------------------------|--------------------------------------|
| `flow-server` | built from `Dockerfile.server` | HTTP API + WebUI on `:8080`          |
| `postgres`    | `postgres:16-alpine`           | relational store on `:5432`          |
| `dex`         | `dexidp/dex:v2.41.1`           | local OIDC issuer on `:5556`         |

## Setup

Create `.env` in this directory (gitignored):

```
FLOW_SERVER_ADDR=:8080
FLOW_SERVER_BASE_URL=http://localhost:8080
FLOW_PG_DSN=postgres://flow:flow-dev@postgres:5432/flow?sslmode=disable
FLOW_OIDC_ISSUER=http://localhost:5556
FLOW_OIDC_CLIENT_ID=flow-server-dev
FLOW_OIDC_CLIENT_SECRET=dev-secret
FLOW_ALLOWED_SUBS=Cghsb2NhbGRldhIFbG9jYWw
FLOW_COOKIE_HASH_KEY=<openssl rand -hex 32>
FLOW_COOKIE_BLOCK_KEY=<openssl rand -hex 32>
```

## Run

```bash
podman compose up -d --build
# or: docker compose up -d --build
```

flow-server waits for postgres to pass its healthcheck before starting.

## Smoke

```bash
curl -fsS http://localhost:8080/healthz   # → "ok"
curl -fsS http://localhost:8080/readyz    # → "ready"
curl -fsS http://localhost:8080/api/v1/oidc/config | jq

# Browser login:
#   open http://localhost:8080/login
#   → redirected to dex, log in as soenne@local / password
#   → back at flow-server with the flow_session cookie set
# Then:
curl -fsS http://localhost:8080/api/v1/me --cookie "flow_session=..."
```

## Stop

```bash
podman compose down       # keeps pg-data volume
podman compose down -v    # wipes everything including the postgres data
```

## Healthcheck

The distroless runtime ships no shell or `curl`, and `flow-server` has no
`--healthcheck` flag. So this compose file deliberately omits a HEALTHCHECK
block for `flow-server` — probes happen externally (`curl http://localhost:8080/healthz`)
here and via the K8s Ingress in production. `postgres` has its own healthcheck
(`pg_isready`) so `flow-server` only starts once the DB is accepting connections.

## Notes

- Two OIDC clients are registered in dex (`deploy/podman/dex-config.yaml`):
  `flow-server-dev` (confidential, browser auth-code) and `flow-cli`
  (public, device-flow). flow-server uses `FLOW_OIDC_CLIENT_ID` /
  `FLOW_OIDC_CLIENT_SECRET` for the browser flow, and publishes the public
  CLI client_id from `FLOW_OIDC_CLI_CLIENT_ID` (default `flow-cli`) via
  `/api/v1/oidc/config` so the CLI/MCP can drive device-flow against the
  IdP directly.
- `FLOW_ALLOWED_SUBS` is pinned to the dex-encoded sub for the static
  `localdev` user. dex base64url-encodes `(userID, connectorID)` as a
  protobuf; that string is `Cghsb2NhbGRldhIFbG9jYWw` for our config.
  If you change `userID` in `dex-config.yaml`, recompute it (or watch the
  flow-server WARN log on first login).
- Dex 2.41 device-flow approval is browser-only — for full CLI auth
  testing use the prod Authentik deployment.
- Backups übernimmt im Homelab CNPG (Operator-Snapshots + PITR); der lokale
  compose-Stack ist Wegwerf-Dev — `podman volume rm` setzt ihn zurück.
