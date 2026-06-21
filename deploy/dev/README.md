# flow dev environment

Self-contained local stack for dogfooding the worktime live-sync vertical:
**Postgres + Dex (OIDC)** in containers, **flow-server on the host**.

flow-server runs on the host (not in compose) on purpose: the OIDC issuer URL
must resolve to the *same* address for both the browser and flow-server, which
"everything in localhost" gives for free and an in-compose server would not
(without an issuer/networking workaround). You rebuild the server constantly
anyway, so host is the natural place for it.

## Prerequisites
- `podman` (machine running). There is no `docker` CLI here — all compose calls
  use `podman compose`.
- Go toolchain (for `make dev-run` / building the binaries).

## Quick start
```bash
make dev-up                              # Postgres + Dex (waits until both are ready)
make dev-run                             # flow-server on https://localhost:8080 (auto-migrates)
```
flow-server serves **HTTPS/HTTP/2** in dev (self-signed cert, cached under
`$TMPDIR/flow-dev-tls`). HTTP/2 multiplexes the persistent SSE stream and page
loads over one connection — without it, each open tab permanently eats one of
the browser's ~6 HTTP/1.1 connection slots and the app appears to hang after a
few tabs. First visit shows a "not private" warning: click through once, or
enable `chrome://flags/#allow-insecure-localhost`. (Cert issues? `rm -rf
$TMPDIR/flow-dev-tls` and restart `make dev-run`.)

Then dogfood:

**Browser (auth-code-flow, no token needed):**
1. Open <https://localhost:8080/> → accept the cert warning → redirected to Dex → log in **msoent@dev.local / password**.
2. Open a **second tab** on the same URL.
3. Start a timer in tab A → tab B updates within ~1 s (and stop propagates back).
   This proves the server-authoritative SSE live-sync loop end-to-end.

**TUI (bearer token):**
```bash
export FLOW_TOKEN=$(make -s dev-token)   # Dex id_token via the password grant
export FLOW_SERVER_URL=https://localhost:8080
export FLOW_INSECURE_TLS=1               # trust the dev server's self-signed cert
TOKEN="$FLOW_TOKEN" ./scripts/live-sync-check.sh   # server-side gate: REST start → SSE event
./bin/flow worktime                      # start/stop here ↔ see it in the browser
```

## Teardown
```bash
make dev-down            # stop containers (keeps the db volume)
make dev-down ARGS=-v    # also drop the Postgres volume (fresh DB next time)
```

## What's where
| File | Purpose |
|------|---------|
| `deploy/dev/compose.yml` | Postgres + Dex services (podman compose) |
| `deploy/dev/dex.yaml` | Dex config: `flow-dev` client, static user, password grant |
| `deploy/dev/flow.env` | flow-server env (DB + OIDC + `FLOW_DEV=1`) |
| `scripts/dev-up.sh` / `dev-down.sh` | bring the deps up / down |
| `scripts/dev-token.sh` | mint a Dex id_token for `FLOW_TOKEN` |
| `scripts/dev-run.sh` | run flow-server with `flow.env` sourced |

## Notes / not for production
- `FLOW_DEV=1` makes session cookies non-`Secure` (harmless over the dev https)
  and switches flow-server to the self-signed-TLS / HTTP/2 listener.
- `scripts/live-sync-check.sh` and the CLI/TUI need `FLOW_INSECURE_TLS=1` to
  accept the dev server's self-signed cert (`flow-cli.env` sets it).
- Dex storage is in-memory: every `make dev-up` after a full teardown starts fresh.
- The password grant (`scripts/dev-token.sh`) is a dev convenience standing in
  for the real device-flow login arriving in **M1b**.
- Credentials here (`flow-dev-secret`, `password`, the session secret) are dev-only.
