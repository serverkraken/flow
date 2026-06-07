# flow Client OIDC Identity Adoption + Pull-Remap — Design

Date: 2026-06-07
Status: Approved (brainstorming) — pending implementation plan
Scope: flow client only (`cmd/flow`, `cmd/flow-mcp`, `internal/adapter/httpsync`,
`internal/adapter/sqliteclient/users`). No server change, no redeploy.

## Problem

After the multi-device sync server went live, a freshly-connected client that
had been used offline cannot pull its own data: the sync worker logs

```
sync: pull active_sessions err="sqliteclient.ActiveSessions.Upsert:
constraint failed: FOREIGN KEY constraint failed (787)"
```

and the `active_sessions` watermark is stuck at 0.

### Root cause — identity mismatch

User rows are keyed by a per-store random UUID: both
`sqliteserver/users.go` and `sqliteclient/users.go` create users with
`uuid.NewString()`, independently. So the same person (OIDC sub `msoent`) has a
**different** `users.id` on each side. The client placeholder user is sub
`local` / id `c2971f71…`; the server user is sub `msoent` / a server UUID.

On every push the server overwrites `user_id` with its own authenticated
`user.ID` (`projects_handlers.go:72` `in.UserID = user.ID`; active via
`store.Start(user.ID, …)`). On pull the worker upserts rows **as-is**
(`worker.pullActivePage` → `w.active.Upsert(a)`), so the row carries the
server's user UUID — which does not exist in the client's local `users` table
→ local FK violation.

### Why projects "survive" but active_sessions don't

`projects` upsert uses `ON CONFLICT(id)`; a pulled project matches an
already-local row → the UPDATE branch runs and does **not** touch `user_id`
(it stays the valid local UUID) → no FK violation. `active_sessions` upsert
uses `ON CONFLICT(user_id, project_id)`; the pulled row's server UUID never
matches a local row → a fresh INSERT with the server UUID → user FK fails.
Projects only appear to work because they all originated locally; anything that
doesn't already exist locally would FK-fail the same way.

This is the unfinished identity wiring: `httpserver.Config` documents
`FLOW_LOCAL_USER_SUB … default "local"; real OIDC: Task 23`.

## Goals

- A logged-in client operates under its real OIDC identity, not the `local`
  placeholder.
- Pulled rows store cleanly (no FK errors); sync round-trips for all resources.
- Multi-user ready: distinct OIDC identities never bleed into each other,
  whether across machines or over time on one machine.
- Client-only: no server schema/API change, no redeploy.

## Non-goals

- Changing the user-id scheme to a stable, sub-derived id shared across stores
  (considered and rejected — needs a coordinated client+server migration and a
  server redeploy). UUIDs stay store-local; the OIDC sub is the canonical
  cross-store identity, enforced server-side by bearer auth and per-user pull
  scoping.
- Server-side changes of any kind.

## Design

### 1. Identity model

`user_id` columns stay store-local UUIDs. The canonical cross-store identity is
the OIDC `sub`. Each client process operates under exactly one local user: the
OIDC user when a token is present, otherwise the `local` placeholder.

### 2. Adoption = re-label the user row (no data re-keying)

When the user opts in (see §3), adoption is a single statement:

```sql
UPDATE users
   SET oidc_sub = :sub, email = :email, display_name = :name
 WHERE oidc_sub = 'local'   -- (the configured FLOW_LOCAL_USER_SUB)
```

The local user row (`c2971f71…`) **becomes** the OIDC user. All existing data
already references that UUID, so every project/session/active/repo/note is now
owned by the OIDC identity with zero row re-keying and zero FK risk. Safe on
first login because no `msoent` user row exists yet and `users.oidc_sub` is
UNIQUE.

### 3. First-login adoption prompt (`flow login`)

After `flow login` stores the token, the client checks: does pre-login data
exist under the `local` user **and** is there no OIDC user yet for this sub
(first login)? If so, prompt:

> Found N projects / M sessions under the offline profile.
> Adopt them under `<email>`? [y/N]

- **Yes** → re-label per §2; the client now operates as that user.
- **No** → `EnsureBySub(<sub>, …)` creates a fresh OIDC user (new UUID); the
  `local` data stays untouched under the `local` user.

The prompt fires only when there is unclaimed `local` data on first login; it
never fires for subsequent logins or for a second, different identity.

### 4. Current-user resolution (startup of `cmd/flow`, `cmd/flow-mcp`)

Determine the active sub at startup:

- A valid token in the keyring → use its `sub` (decoded from the stored access
  token, which carries `sub`). Display fields (email/name) are not in the
  access token; they are captured once at login time from `/api/v1/me-bearer`
  and persisted on the local user row, so startup resolution needs only the
  `sub`.
- Otherwise → `FLOW_LOCAL_USER_SUB` (default `local`).

Then `localUser = cacheUsers.EnsureBySub(activeSub, email, name)`. Everything
downstream (use cases, the sync worker's `userID`) uses this user. This
replaces today's unconditional `EnsureBySub("local")`.

### 5. Pull-remap (httpsync worker) — the load-bearing fix

In every pull path (`pullActivePage`, `pullProjects`, `pullSessions`,
`pullRepos`, `pullRepoNotes`), set `row.UserID = w.userID` before Upsert. This
is correct because the server scopes every pull to the authenticated user
(`PullSince … WHERE user_id = ?`), so every pulled row belongs to the
logged-in identity. A small helper keeps the five call sites uniform.

### 6. Multi-user correctness

- **Different users, different machines:** each client runs under its own OIDC
  identity and remaps pulls to its own local user; the server separates data by
  per-user pull scoping.
- **Different users, same machine over time:** the first real login may adopt
  the `local` data (via the prompt, re-labelling that row). A later, different
  login finds no `local` row to claim and gets a fresh OIDC user — it inherits
  nothing from the first identity.

### 7. Offline / logout

No token → operate as `local` (current behaviour); the `local` UUID persists.
A later login resolves back to the OIDC user (or triggers the first-login
adoption flow if `local` data is still unclaimed).

## Testing

- **Unit:** current-user resolution (token present → sub; absent → `local`);
  pull-remap helper rewrites `UserID` to the worker's user; re-label adoption
  updates `oidc_sub`/email/name and is idempotent / refuses when an OIDC row
  already exists.
- **Integration:** login → adopt prompt (yes) → re-label → sync round-trips all
  resources with no FK errors and advancing watermarks; login (no) → fresh user,
  `local` data untouched.

## Scope / files

- `internal/adapter/httpsync/worker.go` — pull-remap in the five pull paths.
- `cmd/flow/main.go`, `cmd/flow-mcp/main.go` — current-user resolution from the
  token; thread the resolved user as today.
- `flow login` command flow — first-login detection + adoption prompt.
- `internal/adapter/sqliteclient/users.go` — a `RelabelLocalUser(sub, email,
  name)` (or equivalent) helper + first-login/has-data probes.
- Token `sub`/claims decoding (reuse existing keyring + JWT decode paths).
- Tests as above.

## Prerequisite

The already-completed, `make ci`-green, live-verified **project-sync fix**
(`usecase/projects.go` enqueue + backfill, `cmd/flow` wiring) is independent and
lands first.
