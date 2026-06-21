# flow `rebuild` Server Deployment — Design

**Goal:** Run the `rebuild`-branch flow-server live at **https://flow.thebackend.org** (replacing the abandoned `next`-code deployment), with full hybrid search, so the WebUI, `flow login`, and **flow-mcp** can be dogfooded against a real server instead of only the in-memory loopback / local dev stack.

**Status:** brainstormed + design-approved 2026-06-21. Next: `writing-plans` (likely split into flow-repo slices + a homelab-study slice). Not started.

## Context — the two-repo reality

flow is deployed via a **separate GitOps repo** `homelab-study` (`/Users/msoent/SourceCode/serverkraken/homelab-study`), reconciled by **ArgoCD** (auto-sync + selfHeal, `targetRevision: main`). The deploy mechanic is: edit `bootstrap/templates/**/*.j2` → `task configure` (makejinja render, delimiters `#{ }#` / `#% %#`, data from gitignored `config.yaml`+`secrets.yaml`) → `task bootstrap:secrets` (SOPS-encrypt `*.sops.yaml`) → commit the rendered `kubernetes/**` to `main` → ArgoCD applies.

The live deployment does **not** use the `next` Helm chart (that chart is SQLite+Litestream). It uses **raw Kustomize manifests** rendered from `.j2`, backed by **CloudNativePG (CNPG) Postgres**. So the relevant artifacts are the homelab-study raw manifests, not the flow Helm chart.

**Current live state (the `next` code):**
- **Image:** `ghcr.io/serverkraken/flow-server@sha256:…` (digest-pinned; `imagePullPolicy: Always`). Built by `next`'s `.github/workflows/build-server-image.yml` from `deploy/podman/Dockerfile.server`, pushed to GHCR on push to main/next (tags `:next`, `:latest`, `:<sha>`).
- **DB:** CNPG `Cluster` `flow-db` (PG17, `ghcr.io/cloudnative-pg/postgresql:17-bookworm`), 2 instances, **no pgvector**. DSN auto-secret `flow-db-app` key `uri` → env `FLOW_PG_DSN`. Backup: Barman→S3 (minio).
- **OIDC:** Authentik, blueprint `kubernetes/apps/identity/identity/config/blueprints/52-app-flow.yaml`. Two providers, `issuer_mode: per_provider`, `sub_mode: user_username`:
  - `flow-web` — confidential, issuer `https://id.thebackend.org/application/o/flow/`, grants `[authorization_code, refresh_token]`, redirect `https://flow.thebackend.org/auth/callback`.
  - `flow-cli` — public, issuer `https://id.thebackend.org/application/o/flow-cli/`, client_id literal `flow-cli`, grants `[device_code, refresh_token]`, redirect `urn:ietf:wg:oauth:2.0:oob`.
- **Ingress:** `ingressClassName: external` (ingress-nginx-external), host `flow.thebackend.org`, cert-manager `letsencrypt-production`, TLS secret `flow-thebackend-org-tls`, `external-dns target external.thebackend.org`.
- **Secrets:** SOPS+Age via `ksops`. `flow-server-secrets` (keys `oidc_client_id`, `oidc_client_secret`, `cookie_hash_key`, `cookie_block_key`, `allowed_subs`). The `oidc_client_id/secret` are a **manual mirror** of `identity-oidc.AK_OIDC_FLOW_ID/SECRET` (no auto-sync; documented invariant).
- **Ollama:** deployed in-cluster (`kubernetes/apps/ollama/ollama/`, image `ollama/ollama:0.30.10`, URL `http://ollama.ollama.svc.cluster.local:11434`) but **not wired to flow**.

## The gap — `rebuild` ≠ `next`

`rebuild` is a green-field server rewrite that **dropped all deploy artifacts** (no Dockerfile, no image CI, no chart) and **renamed its config surface** + **added a hard pgvector dependency**.

### Env-var mapping (live `next` manifest → `rebuild` server)

| Live `next` env | `rebuild` env | Source / note |
|---|---|---|
| `FLOW_SERVER_ADDR` | `FLOW_LISTEN_ADDR` | rename; `:8080` |
| `FLOW_SERVER_BASE_URL` | `FLOW_PUBLIC_BASE_URL` | rename; `https://flow.thebackend.org` |
| `FLOW_PG_DSN` (secret `flow-db-app:uri`) | `DATABASE_URL` | rename; same CNPG `uri` secret |
| `FLOW_COOKIE_HASH_KEY` + `FLOW_COOKIE_BLOCK_KEY` | `FLOW_SESSION_SECRET` | **new single secret**, replaces the pair |
| `FLOW_OIDC_ISSUER` | `FLOW_OIDC_ISSUER` | keep; web issuer `…/o/flow/` |
| `FLOW_OIDC_CLI_ISSUER` | `FLOW_OIDC_CLI_ISSUER` | keep; cli issuer `…/o/flow-cli/` — **but rebuild must learn to consume it** (see OIDC) |
| `FLOW_OIDC_CLIENT_ID` (secret) | `FLOW_OIDC_CLIENT_ID` | keep; web client (mirror of `AK_OIDC_FLOW_ID`) |
| `FLOW_OIDC_CLIENT_SECRET` (secret) | `FLOW_OIDC_CLIENT_SECRET` | keep; web client secret |
| — | `FLOW_OIDC_CLI_CLIENT_ID` | **new**; literal `flow-cli` (the public CLI client) |
| `FLOW_ALLOWED_SUBS` (secret) | `FLOW_ALLOWED_SUBS` | keep; `msoent` (username sub_mode) |
| — | `FLOW_OLLAMA_HOST` | **new**; `http://ollama.ollama.svc.cluster.local:11434` |
| — | `FLOW_EMBED_MODEL` / `_INTERVAL` / `_BATCH` | **new**; `nomic-embed-text` / `15s` / `16` |
| — | `FLOW_DEV` | **must stay UNSET** in prod (Secure cookies; TLS terminates at ingress, plain HTTP to the pod — the browser↔nginx hop is HTTP/2 so the SSE-starvation concern is handled by nginx; the per-user SSE cap from the dos-fix still applies) |

### Other gaps
- **pgvector is mandatory:** migration `0010` does `CREATE EXTENSION vector`; without it the goose migration chain **fails entirely at boot**. The current CNPG image has no pgvector.
- **Fresh DB required:** `rebuild`'s goose migrations `0001–0012` are a different schema from `next`'s; reusing the existing `flow-db` would collide the goose version table. `next`'s data is throwaway dogfood data.
- **No image pipeline on `rebuild`:** must port the Dockerfile + CI, and **push the `rebuild` branch to origin** so CI can build it.

## Decisions (confirmed with the user)

1. **DB:** fresh CNPG cluster with a **pgvector-capable image**; empty schema, `rebuild` auto-migrates. `next` data discarded.
2. **Image:** **push `rebuild` to origin + port the CI workflow** (`build-server-image.yml`) — reproducible/digest-pinnable, like `next`.
3. **Semantic search:** **wire Ollama from day one** (pgvector is required anyway; Ollama already runs in-cluster).
4. **Helm chart:** **not ported** — the live deploy uses raw manifests (YAGNI).
5. **Topology:** **replace** the deployment at `flow.thebackend.org` (`next` is abandoned), not a parallel host.
6. **OIDC:** **re-add multi-issuer verification to the `rebuild` server** (rather than rebuild Authentik to one issuer) — see below.

## Design

### Workstream 1 — flow repo (`rebuild` branch)

Three changes, each a small TDD-able unit; then push the branch.

**1a. `deploy/podman/Dockerfile.server`** — port `next`'s, **simplified**:
- Builder `golang:1.25-alpine` (match go.mod) → distroless `gcr.io/distroless/static-debian12:nonroot`, uid 65532, `EXPOSE 8080`.
- **Drop all Node/npm** (`tools/tailwind`, `tools/codemirror` don't exist on `rebuild`; `internal/adapter/webui/static/app.css` is committed + `go:embed`-ed).
- Build: `go tool templ generate ./...` (templ is a `go.mod` tool directive on `rebuild`, not a pinned `go run …@version`), then `CGO_ENABLED=0 GOFLAGS=-trimpath go build -ldflags="-s -w" -o /out/flow-server ./cmd/flow-server`.
- Migrations + static assets are `go:embed`-ed (`internal/adapter/pgstore/migrations/*.sql`, `internal/adapter/webui/static/app.css`) — no COPY, no init job.

**1b. `.github/workflows/build-server-image.yml`** — port `next`'s:
- Trigger: push to `[main, rebuild]`, path filter on `cmd/flow-server/**`, `internal/**`, `web/tailwind.css`, `deploy/podman/Dockerfile.server`, `go.{mod,sum}`, `Makefile`.
- Image `ghcr.io/serverkraken/flow-server`, tags `:<sha>` always + `:rebuild` on the rebuild branch. buildx multi-arch `linux/amd64,linux/arm64`, GHA cache, `GITHUB_TOKEN` login.

**1c. Multi-issuer verifier** (the one real code change) — restore what `next` had and `rebuild` simplified away:
- `internal/config`: add `OIDCCliIssuer` from `FLOW_OIDC_CLI_ISSUER` (optional — when empty, behaviour is today's single-issuer).
- `internal/adapter/oidcverify`: accept a set of `(issuer, audiences)` pairs; build one `oidc.Provider`/verifier per issuer; `Verify` accepts a token if **any** issuer-verifier validates its signature+`iss`+`exp` **and** the token's `aud` is in that issuer's allowed set. (Per-issuer audiences is the clean default: web issuer→web client, cli issuer→cli client.)
- `cmd/flow-server/main.go`: replace `oidcverify.New(ctx, cfg.OIDCIssuer, []string{cfg.OIDCClientID, cfg.OIDCCliClientID})` with the multi-issuer form — `{ (FLOW_OIDC_ISSUER → [OIDCClientID]), (FLOW_OIDC_CLI_ISSUER → [OIDCCliClientID]) }` — falling back to single-issuer when `FLOW_OIDC_CLI_ISSUER` is empty so dev (one Dex issuer, two auds) is unaffected.
- `oidcauth` (web auth-code flow) is unchanged — it is correctly single-issuer (`FLOW_OIDC_ISSUER`, web client).
- **CLI side:** on the user's machine `flow login` sets `FLOW_OIDC_ISSUER` = the **cli** issuer (`…/o/flow-cli/`) for device-flow discovery, `FLOW_SERVER_URL=https://flow.thebackend.org`. (Same env name, different value than the server — matches the proven `next` setup.)

This mirrors `next`'s multi-verifier, which the homelab already proved necessary (Authentik per-provider issuers have no usable global discovery endpoint — prior lesson).

**Dev-env impact (verified — the one thing to keep deliberate):** the local dev stack needs **no change**. `deploy/dev/flow.env` already uses every `rebuild` name (`DATABASE_URL`, `FLOW_LISTEN_ADDR`, `FLOW_PUBLIC_BASE_URL`, `FLOW_SESSION_SECRET`, `FLOW_OLLAMA_HOST`+embed, `FLOW_OIDC_CLI_CLIENT_ID=flow-cli`); the dev DB image is already `pgvector/pgvector:pg16`; Ollama is already in `deploy/dev/compose.yml`; Dex is one issuer (`http://localhost:5556/dex`) with two clients (`flow-dev` confidential, `flow-cli` public). Because `FLOW_OIDC_CLI_ISSUER` is **optional with a single-issuer fallback**, this single-issuer-two-audience dev config keeps working unchanged. The multi-issuer *path* is exercised instead by (a) a focused unit/integration test with **two in-process OIDC issuers** (httptest discovery + JWKS: accept by matching iss+aud, reject cross-issuer / wrong-aud / expired) and (b) the prod live-gate (WS4). A faithful two-issuer dev stack (a second Dex) is deliberately **not** added — Dex is single-issuer by nature, and the unit fixtures + prod-gate cover the path more deterministically. The single-issuer fallback being behaviour-preserving for dev is itself a required test case.

**1d.** Push `rebuild` to `origin` so CI builds the image; record the resulting `@sha256` digest.

### Workstream 2 — homelab-study GitOps

**2a. pgvector CNPG image.** Provide a pgvector-capable Postgres-17 image for CNPG. Plan picks one of: (i) a tiny custom image `FROM ghcr.io/cloudnative-pg/postgresql:17-bookworm` adding `postgresql-17-pgvector`, built+pushed to GHCR; or (ii) a vetted community CNPG+pgvector image. Point the `flow-db` CNPG `Cluster` `imageName` at it. Confirm `CREATE EXTENSION vector` works (migration 0010).

**2b. Fresh DB.** Recreate `flow-db` clean (new cluster / dropped volume). `rebuild` auto-migrates `0001–0012` on first boot. Keep the existing Barman→S3 backup config.

**2c. flow-server Deployment env rewrite** (`bootstrap/templates/kubernetes/apps/flow/flow-server/deployment.yaml.j2`): apply the env-mapping table above — rename `FLOW_SERVER_ADDR/BASE_URL`→`FLOW_LISTEN_ADDR/PUBLIC_BASE_URL`, `FLOW_PG_DSN`→`DATABASE_URL`, drop the two cookie keys, add `FLOW_OIDC_CLI_CLIENT_ID=flow-cli`, add `FLOW_OLLAMA_HOST`+embed vars, keep `FLOW_OIDC_ISSUER`+`FLOW_OIDC_CLI_ISSUER`, ensure `FLOW_DEV` unset. Pin the new image digest from 1d.

**2d. Secrets** (`flow-server/secret.sops.yaml` + `secrets.yaml` template inputs): add `session_secret` (→ `FLOW_SESSION_SECRET`, freshly generated), remove `cookie_hash_key`/`cookie_block_key`. Keep `oidc_client_id/secret`/`allowed_subs` (and their identity-namespace mirror).

**2e. Ollama model.** Ensure `nomic-embed-text` is pulled in the in-cluster Ollama (a one-shot `ollama pull` Job, or documented manual pull). Until present, `rebuild` WARNs and degrades search to keyword — non-fatal.

### Workstream 3 — Authentik

Reuse the existing two-provider blueprint (`52-app-flow.yaml`) largely **as-is**. Verify with the `authentik-expert` skill: both providers carry the right grants (`flow-web`: authorization_code+refresh_token; `flow-cli`: device_code+refresh_token), `sub_mode: user_username` (so `FLOW_ALLOWED_SUBS=msoent` matches), and the redirect URIs match `rebuild`'s callback (`/auth/callback`) and the device OOB URN. No new provider expected.

### Workstream 4 — Dogfood (done-gate)

1. WebUI: open `https://flow.thebackend.org` → Authentik auth-code login → worktime + docs render; SSE live-sync works.
2. CLI: `flow login` (device-flow via the cli issuer) → `flow whoami` 200 → `flow worktime` / `flow docs` against prod.
3. **flow-mcp:** register the built `flow-mcp` binary in Claude Code via `.mcp.json` pointing at the prod server (using the stored token) → call `flow_project_context`, `flow_search_docs`, `flow_get_doc` against the real Kompendium.

## Sequencing

WS1 (Dockerfile + CI + multi-issuer verifier, `make ci` green) → push `rebuild` → CI builds image → record digest → WS2a (pgvector image) → WS2b–e (fresh DB + env rewrite + secret + Ollama model, digest pinned) → WS3 (Authentik verify) → render→sops→commit `main` → ArgoCD sync → WS4 verification.

## Verification

- flow-repo: `make ci` green (golangci-lint 0, coverage ≥ 80%, build) with the multi-issuer verifier unit-tested (single-issuer fallback + two-issuer accept/reject by iss+aud). Image built by CI, digest obtained.
- cluster: `flow-db` healthy with `vector` extension; flow-server pod `Ready`, migrations `0001–0012` applied at boot (logs); `/` serves the WebUI over the prod cert.
- auth: both OIDC paths (web auth-code + CLI device-flow) yield a verified identity for `msoent`.
- search: a semantic query returns results once `nomic-embed-text` is pulled; degrades to keyword otherwise.
- mcp: `flow_search_docs` returns real Kompendium hits through Claude Code.

## Risks / open items (resolve in the plan)

- **pgvector image choice** (custom build vs community) — verify the `vector` extension is loadable under CNPG.
- **Multi-issuer audience strictness** — per-issuer auds vs a union set; pick per-issuer for tightness.
- **Secret rotation** — generating `FLOW_SESSION_SECRET` invalidates existing sessions (none worth keeping; fresh deploy).
- **`rebuild` branch push** — first push of a large local-only branch to origin; ensure no unintended `next`/`main` interaction (CI path filters scoped to the branch).
- **CNPG cutover** — dropping `flow-db` is destructive; acceptable (throwaway data) but sequence the cluster recreate before pointing flow-server at it.

## Out of scope

- Porting/maintaining the `next` SQLite Helm chart.
- Migrating `next` data into the `rebuild` schema.
- Merging `rebuild` → `main` (stays on the long-lived integration branch; CI builds `:rebuild`).
- flow-mcp write tools / Resources (Slice 2c) and the formal `.mcp.json` 2d gate — this deploy enables the *read* dogfood now; later slices layer on.
