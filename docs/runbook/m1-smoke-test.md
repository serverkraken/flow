# M1 Smoke Test — Manual

If the automated E2E test (`go test -tags integration ./internal/adapter/httpserver/`) does not pass against your dex setup (e.g. Docker unavailable, dex form layout changed), run this manual smoke against a real Authentik instance.

## Prerequisites

- Authentik running and reachable
- Authentik OIDC providers configured: `flow-web` (confidential, `authorization_code` + `refresh_token`) for browser login, **and** `flow-cli` (public, `urn:ietf:params:oauth:grant-type:device_code` + `refresh_token`) for the CLI/MCP device-flow. flow-server publishes the CLI client_id via `/api/v1/oidc/config` (configurable via `FLOW_OIDC_CLI_CLIENT_ID`, default `flow-cli`).
- Allowed redirect URI on the `flow-web` provider: `http://localhost:8080/auth/callback`
- Your Authentik user's `sub` claim added to `FLOW_ALLOWED_SUBS`

## Steps

```bash
# Terminal 1: start flow-server
cd /path/to/flow
go build -o flow-server ./cmd/flow-server
FLOW_OIDC_ISSUER=https://auth.example.com/realms/flow \
FLOW_OIDC_CLIENT_ID=flow-web \
FLOW_OIDC_CLIENT_SECRET=... \
FLOW_OIDC_CLI_CLIENT_ID=flow-cli \
FLOW_ALLOWED_SUBS=YOUR_AUTHENTIK_SUB \
FLOW_COOKIE_HASH_KEY=$(openssl rand -hex 32) \
FLOW_COOKIE_BLOCK_KEY=$(openssl rand -hex 16) \
FLOW_SERVER_BASE_URL=http://localhost:8080 \
./flow-server
```

```bash
# Terminal 2: drive the CLI
go build -o flow ./cmd/flow
./flow login --server=http://localhost:8080
# -> browser opens to Authentik device-authorization URL
# -> enter the user-code shown in the terminal
# -> approve in browser

./flow whoami --server=http://localhost:8080
# expected: prints Sub / Email / Name from /api/v1/me-bearer
```

## Browser smoke

```bash
# Visit http://localhost:8080/login in your browser
# -> redirect to Authentik login
# -> login + approve
# -> redirected to /
# -> visit http://localhost:8080/api/v1/me — should return sub/email/name as JSON
```

## What the automated test covers

The integration test (`//go:build integration`) in
`internal/adapter/httpserver/integration_e2e_test.go` exercises Path A
(browser auth-code flow against dex via its local-connector HTML form).
It self-skips with a clear message if:

- Docker is unavailable (dex container start fails)
- The dex login-form URL does not land on the dex issuer (form layout changed)
- The post-credential redirect does not return to the flow server (approval screen injected)

Run it with:

```bash
go test -tags integration -count=1 -timeout 90s -v ./internal/adapter/httpserver/
```
