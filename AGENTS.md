# AGENTS.md — flow (rebuild branch)

Conventions for any coding agent (Claude Code / Gemini CLI / Codex) working in this repo.

## Current work
Active plan: `docs/superpowers/plans/2026-06-25-flow-webui-wissen-markdown.md`
(design spec: `docs/superpowers/specs/2026-06-25-flow-webui-wissen-markdown-design.md`).
Execute it task-by-task starting at **Task 1** (Task 0 = this file, done). Each
task is self-contained: write the failing test, run it, implement, run it, commit.

## Build / test / lint
- `make ci` = `lint verify-generate verify-css verify-no-popups cover build`. Must be green before any task is "done".
- `make test` runs Go tests; `make cover` enforces the 75% coverage gate (`scripts/coverage-gate.sh`).
- `make generate` runs `templ generate`; commit generated `*_templ.go`. `make verify-generate` checks for drift.
- `make web` rebuilds Tailwind CSS into `internal/adapter/webui/static/app.css`; commit it. `make verify-css` checks for drift.
- `make verify-no-popups` fails if `window.alert/confirm/prompt` appears in templ/JS.

## Architecture (hexagonal)
- `internal/domain` — entities + pure logic, no I/O.
- `internal/ports` — interfaces (stores, buses).
- `internal/usecase` — application services (`Execute(...)`).
- `internal/adapter/...` — drivers: `httpserver` (HTTP + web handlers), `webui` (templ components), `pgstore` (Postgres), `tui`.
- Wiring is in `cmd/flow` + `internal/adapter/httpserver/server.go`.

## WebUI conventions
- templ + htmx + Tailwind, server-rendered. No SPA, no Node runtime.
- Pages: `XPage(vm)` wraps `components.Base(active, body)` → `components.AppShell(active, breadcrumb, subnav, content)`.
- htmx fragments: `XFragment(vm)`; SSE live via `hx-ext="sse"` (body) + `hx-trigger="sse:<event>"` on containers.
- i18n: NO hardcoded display strings; add keys to `internal/i18n/catalog_de.go` + `catalog_en.go` (Go maps, NOT TOML); use `components.T(ctx, "key")`.
- NO browser popups; confirms via `components.ConfirmDialog`.
- One responsibility per file ("keine Monolithen").

## Dev stack (live verification)
- `make dev-up` starts Postgres + Dex (OIDC). `make dev-run` runs the server. `make dev-token` mints a bearer token. (Cookie auth for the browser.)
- Live done-gate: each new route returns expected status; SSE reflects create/update/delete.

## TDD
- Write a failing test, run it (see it fail), implement minimal code, run it (see it pass), commit. Small commits.

## Commit style
- Conventional-commit subject (`feat(webui): …`, `feat(pgstore): …`, `docs: …`). One coherent deliverable per commit.
