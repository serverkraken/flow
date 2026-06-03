# M2-M3 Smoke Test — Multi-Device Sync

Manual runbook for verifying Phase 1 M2+M3 (sessions-sync) end-to-end, covering:

- Two independent clients sharing projects and worktime sessions through `flow-server`.
- Server-side conflict detection when two clients attempt to start the same project concurrently.
- TSV migration smoke against a real `~/.tmux/worktime.log`.

The automated smoke script `scripts/smoke-m2-m3.sh` codifies all phases below; use this
runbook when you want to understand what is happening at each step or when running a partial
check against a real Authentik instance instead of the local dex stack.

---

## 1. Prerequisites

| Requirement | How to satisfy |
|---|---|
| **podman** | `brew install podman` + `podman machine init && podman machine start` |
| **podman-compose** | `brew install podman-compose` |
| **openssl** | macOS ships it; verify with `openssl version` |
| **dex stack** | `make dex-up` — starts dex at `http://localhost:5556` |
| **sqlite3 CLI** | Only needed for TSV idempotency check; `brew install sqlite` |
| **flow binaries** | Built by `make build-server && go build -o bin/flow ./cmd/flow` |

Dex static credentials (configured in `deploy/podman/dex-config.yaml`):

- User: `alice@example.com`, password: `password`
- Alice's `sub`: `ChBhbGljZS1zdGF0aWMtdWlkEgVsb2NhbA` (base64 of the static connector UID)

Both clients authenticate as Alice. This is intentional — the smoke verifies
that a single user's state propagates across two processes, not multi-tenancy.

---

## 2. Build

```bash
cd /path/to/flow

# Build both binaries.
make build-server
go build -o bin/flow ./cmd/flow

# Verify.
./bin/flow-server --help 2>&1 | head -3   # prints usage / exits cleanly
./bin/flow --help                          # lists all subcommands
```

---

## 3. Server Startup

```bash
# Terminal 1: start flow-server against local dex.
FLOW_SERVER_DB=/tmp/flow-smoke/server.db \
FLOW_OIDC_ISSUER=http://localhost:5556 \
FLOW_OIDC_CLIENT_ID=flow-server \
FLOW_OIDC_CLIENT_SECRET=flow-server-secret \
FLOW_COOKIE_HASH_KEY=$(openssl rand -hex 32) \
FLOW_COOKIE_BLOCK_KEY=$(openssl rand -hex 16) \
FLOW_ALLOWED_SUBS=ChBhbGljZS1zdGF0aWMtdWlkEgVsb2NhbA \
FLOW_SERVER_BASE_URL=http://localhost:8080 \
./bin/flow-server
```

Expected server log lines at startup (JSON):

```
{"level":"INFO","msg":"flow-server starting","addr":":8080","base_url":"http://localhost:8080","issuer":"http://localhost:5556","allowed_subs":1}
```

Healthcheck:

```bash
curl -s http://localhost:8080/healthz
# expected: {"ok":true}
```

---

## 4. Client A Flow

Client A represents the first device (e.g. your laptop).
Use `FLOW_CACHE_DB` to give each client a separate SQLite file.

```bash
# Terminal 2: client A.
mkdir -p /tmp/flow-smoke/{a,b}

# Step 1: login (opens browser / device-code prompt).
FLOW_CACHE_DB=/tmp/flow-smoke/a/cache.db ./bin/flow \
  login --server=http://localhost:8080 --client-id=flow-cli
```

The login command prints a URL and a short user-code. Open the URL in a
browser and enter the code. Approve in dex (alice@example.com / password).

```bash
# Step 2: create a project.
FLOW_CACHE_DB=/tmp/flow-smoke/a/cache.db ./bin/flow \
  projects create "Smoke A"
# expected: prints the new project UUID and slug "smoke-a"

# Step 3: start a session on that project.
FLOW_CACHE_DB=/tmp/flow-smoke/a/cache.db ./bin/flow \
  worktime start --project=smoke-a
# expected: "Session started for smoke-a"

# Wait 35 s for the sync worker to push to the server.
sleep 35
```

---

## 5. Client B Flow + Conflict Scenario

Client B represents the second device (e.g. a desktop or remote machine).

```bash
# Terminal 3: client B.

# Step 1: login as the same user.
FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db ./bin/flow \
  login --server=http://localhost:8080 --client-id=flow-cli
# Same device-code flow; approve as alice.

# Step 2: list projects — 'Smoke A' must appear (synced from A via server).
FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db ./bin/flow \
  projects list
# expected output includes a line with "Smoke A"

# Step 3: verify the active session is visible.
# worktime today prints the active session header when run in a pipe.
FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db ./bin/flow \
  worktime today 2>/dev/null | grep -i "Smoke A"
# expected: at least one match line showing the running session.

# Step 4: attempt to start the same project on client B (should conflict).
FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db ./bin/flow \
  worktime start --project=smoke-a
# expected: error message containing "konflikt", "conflict", "already", "läuft", or "running"
# The server returns HTTP 409 Conflict when a session is already active for the user/project.
```

---

## 6. Stop + Session Sync

```bash
# Terminal 2 (client A): stop the session.
FLOW_CACHE_DB=/tmp/flow-smoke/a/cache.db ./bin/flow \
  worktime stop --project=smoke-a
# expected: "Session stopped for smoke-a" + duration

# Wait for next pull cycle on client B.
sleep 35

# Terminal 3 (client B): verify the completed session appears in today's list.
FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db ./bin/flow \
  worktime today 2>/dev/null | grep -i "Smoke A"
# expected: the closed session with start/end times and duration.
```

---

## 7. TSV Migration Smoke

This step is independent — no server is required. It verifies that
`flow worktime migrate-from-tsv` correctly ingests a legacy `~/.tmux/worktime.log`
file into SQLite and is idempotent.

```bash
# Work on a copy so the original is not consumed.
cp ~/.tmux/worktime.log /tmp/flow-smoke/worktime.log.copy

# First run: import all rows.
FLOW_CACHE_DB=/tmp/flow-smoke/migrate.db ./bin/flow \
  worktime migrate-from-tsv --tsv=/tmp/flow-smoke/worktime.log.copy

# Check row count in the new DB.
sqlite3 /tmp/flow-smoke/migrate.db 'SELECT COUNT(*) FROM worktime_sessions;'
# expected: N > 0 (number of non-empty lines in worktime.log)

# The migrate command renames the source file to worktime.log.copy.migrated-<ts>.
# Restore a fresh copy for the idempotency check.
cp ~/.tmux/worktime.log /tmp/flow-smoke/worktime.log.copy2

# Second run: row count must not grow (UUIDs are stable v5 hashes of each line).
FLOW_CACHE_DB=/tmp/flow-smoke/migrate.db ./bin/flow \
  worktime migrate-from-tsv --tsv=/tmp/flow-smoke/worktime.log.copy2

sqlite3 /tmp/flow-smoke/migrate.db 'SELECT COUNT(*) FROM worktime_sessions;'
# expected: same N as before — no duplicates inserted.
```

---

## 8. Expected Outputs

| Phase | What to look for |
|---|---|
| Server startup | JSON log line with `"msg":"flow-server starting"` |
| Healthcheck | `{"ok":true}` |
| Login | Browser opens to dex; after approval CLI prints "Login successful" |
| Project create | New UUID + slug printed; project persisted in `a/cache.db` |
| Worktime start | "Session started" line |
| Project list (B) | "Smoke A" row after ≥35 s wait |
| Active session (B) | `worktime today` output mentions "Smoke A" with no end time |
| Conflict start (B) | Error containing "konflikt" / "conflict" / "already" / "läuft" |
| Stop (A) | "Session stopped" + duration |
| Session row (B) | Same row now has an end time visible after ≥35 s wait |
| TSV import | Positive integer from `SELECT COUNT(*)` |
| TSV idempotency | Count unchanged on second run |

---

## 9. Troubleshooting

### Server not reachable on :8080

- Check `FLOW_SERVER_BASE_URL` is set (not `FLOW_BASE_URL` — that env var does not exist).
- Check the dex stack is up: `make dex-logs` — look for "listening at 0.0.0.0:5556".
- macOS firewall may block loopback. Try `sudo /usr/libexec/ApplicationFirewall/socketfilterfw --setglobalstate off` temporarily.

### Dex container fails to start

```bash
make dex-logs          # look for the error
make dex-down          # clean up
podman system prune -f # clean dangling containers/images
make dex-up
```

If podman-compose reports a missing image, pull it manually:

```bash
podman pull ghcr.io/dex-idp/dex:v2.38.0
```

### SQLite database locked

Two `./bin/flow` processes using the same `FLOW_CACHE_DB` path will produce
"database is locked" errors. Verify each client uses a distinct path:
- Client A: `FLOW_CACHE_DB=/tmp/flow-smoke/a/cache.db`
- Client B: `FLOW_CACHE_DB=/tmp/flow-smoke/b/cache.db`

### `worktime today` does not show the active session

`worktime today` opens the TUI when stdout is a TTY. When piped, it falls
back to a plain text summary. If the plain-text path is not yet implemented,
the grep will fail. In that case:

1. Open the TUI manually in a terminal: `FLOW_CACHE_DB=... ./bin/flow worktime today`
2. Visually verify the "Heute" tab shows the active session row for "Smoke A".
3. Alternatively, query the DB directly:

```bash
sqlite3 /tmp/flow-smoke/b/cache.db \
  'SELECT project_id, started_at FROM worktime_active_sessions;'
```

### Conflict not detected

The 409 from the server is only returned when `active_sessions` already
has an entry for `(user_id, project_id)`. Verify the first session was
pushed before the conflict test:

```bash
sqlite3 /tmp/flow-smoke/server.db \
  'SELECT * FROM active_sessions;'
```

If the table is empty, the push from client A did not reach the server
(sync worker may not have started, or the login token was not stored in
`a/cache.db`). Re-run from Phase 5 with an extra `sleep 60` after the
`worktime start`.

### TSV migration produces 0 rows

- Verify the source file has at least one non-empty line:
  `wc -l ~/.tmux/worktime.log`
- Check the TSV format. Each line must be tab-separated:
  `START_ISO<TAB>END_ISO<TAB>...`
  Lines that do not parse are skipped with a warning.
