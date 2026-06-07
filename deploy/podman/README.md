# flow-server — Podman / Docker dev deployment

Local compose stack with everything needed to exercise flow-server end-to-end:

| Service       | Image                          | Purpose                                       |
|---------------|--------------------------------|-----------------------------------------------|
| `flow-server` | built from `Dockerfile.server` | HTTP API + WebUI on `:8080`                   |
| `dex`         | `dexidp/dex:v2.41.1`           | local OIDC issuer on `:5556`                  |
| `minio`       | `minio/minio:latest`           | S3-compatible object store on `:9000`/`:9001` |
| `minio-setup` | `minio/mc:latest`              | one-shot: creates `flow-backups` bucket       |
| `litestream`  | `litestream/litestream:0.3.13` | replicates `server.db` → minio                |

## Setup

```bash
cp .env.example .env
# .env ships with dev-safe defaults; regenerate cookie keys before any
# non-laptop use:
#   openssl rand -hex 32   # FLOW_COOKIE_HASH_KEY
#   openssl rand -hex 32   # FLOW_COOKIE_BLOCK_KEY
```

## Run

```bash
podman compose up -d --build
# or: docker compose up -d --build
```

`minio-setup` exits 0 once the `flow-backups` bucket exists; this is normal.

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

# Litestream:
podman compose logs litestream | head -20
# expect: "initialized db" then "snapshot written" within ~10s
```

The MinIO console is reachable at <http://localhost:9001> (user `minio`,
password `minio123`).

## Stop

```bash
podman compose down       # keeps flow-data + minio-data volumes
podman compose down -v    # wipes everything including the SQLite + minio data
```

## Healthcheck

The distroless runtime ships no shell or `curl`, and `flow-server` has no
`--healthcheck` flag. So this compose file deliberately omits a HEALTHCHECK
block — probes happen externally (`curl http://localhost:8080/healthz`) here
and via the K8s Ingress in production (see Plan F · Task 4 helm chart).

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
- The `flow-backups` bucket is created idempotently by `minio-setup` —
  no manual `mc mb` needed.
