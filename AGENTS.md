# AGENTS.md — flow (rebuild branch)

Conventions for any coding agent (Claude Code / Gemini CLI / Codex) working in this repo.

## Current work
Active plan: `docs/superpowers/plans/2026-06-25-worktime-edit-running-start.md`
(design spec: `docs/superpowers/specs/2026-06-25-worktime-edit-running-start-design.md`).
Execute it task-by-task starting at **Task 1**. Each task is self-contained:
write the failing test, run it (see it fail), implement minimal code, run it
(see it pass), commit. Run `make ci` green before declaring the work done.

This is a **TUI** task (`internal/tui/...`), not WebUI — no templ/Tailwind/SSE
changes. `make verify-generate`/`verify-css` stay green untouched; the relevant
gates are `go test`, `gofumpt -l` (no output) and `staticcheck` via `make lint`.

### TUI conventions for this task
- The shell (`internal/tui/shell`) is a bubbletea/v2 root model holding tabs as
  nav-stacks. Routes implement `shell.Route`; cross-cutting behaviour is added
  via **optional interfaces** (`InputCapturer`, `FullScreener`, `BreadcrumbHider`
  …) — Task 1 adds `PaletteProvider` the same way. Follow that exact pattern.
- `TodayRoute` (`internal/tui/screen/worktime`) is a pointer receiver; its `st`
  field is the reconstructed `todayState` (`st.Running`, `st.Active *time.Time`,
  `st.ActiveID`). Dialogs follow the `editState`/`openEdit`/`submitEdit` shape in
  `dialogs.go` — mirror it for the new start-edit dialog.
- UI strings are **German**. Time input is `HH:MM` parsed via
  `wtfmt.ParseHM`. Errors surface as `toast.NewDanger(...)`, never popups.
- The backend already supports this: `apiclient.EditSession(..., stop *time.Time)`
  with `stop == nil` keeps a session running. Do **not** add server fields.
- Tests use the in-package `fakeAPI` (`route_test.go`) and `fixedNow`; the shell
  tests use the external `shell_test` package with the `stubRoute` helper.

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
