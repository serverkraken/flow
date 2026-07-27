# AGENTS.md — flow (rebuild branch)

Conventions for any coding agent (Claude Code / Gemini CLI / Codex) working in this repo.

## Grundsätze (verbindlich für JEDE Entscheidung)

- **flow ist eine MULTI-TENANT-App** für **Menschen UND AI-Agents** (M1-Spec
  §Kontext). Dass aktuell ein einzelner User dogfoodet, ist ein Deployment-
  Zustand, KEIN Design-Parameter. Konsequenzen:
  - Jeder Datenzugriff ist **owner-scoped** (ownerID in jeder Store-Query;
    Cross-Tenant-Leaks sind Critical-Findings).
  - Performance-/Sicherheits-Argumente müssen **pro Tenant** halten. „Ist nur
    ein User, also egal" ist als Begründung UNZULÄSSIG — in Code-Kommentaren,
    Reviews, Plänen und Spec-Trade-offs.
  - Keine globalen Caches/Singletons ohne Tenant-Schlüssel; Limits (Upload,
    Rate, Listengrößen) gelten pro User.
- **Menschen UND AI-Agents** sind gleichberechtigte Akteure (actor kind
  human/agent zieht sich durch Activity, Avatare, MCP).

## Current work
Long-lived integration branch **`cockpit-story`** (off `rebuild`): Slices 1–3
(Work/Privat, Node-Logos, Aktivität-Ziel) sind DONE. Es läuft das
**Kristall-Redesign-Programm** (komplette App + IA-Konsolidierung), Spec:
`docs/superpowers/specs/2026-07-02-kristall-redesign-design.md`, Slices K1–K5.
Fortschritts-Ledger: `.superpowers/sdd/progress.md` (gitignored, Worktree
`flow-rebuild`). Approved-Mockup: `docs/superpowers/specs/assets/2026-07-01-cockpit-story/direction-b-APPROVED.html`.

## Build / test / lint
- `make ci` = `lint verify-generate verify-css verify-no-popups cover build`. Must be green before any task is "done".
- `make test` runs Go tests; `make cover` enforces the 75% coverage gate (`scripts/coverage-gate.sh`, `*_templ.go` excluded — real output-asserting tests, no padding).
- `make generate` runs `templ generate`; commit generated `*_templ.go`. `make verify-generate` checks for drift (stage generated files before `make ci`).
- `make web` rebuilds Tailwind CSS into `internal/adapter/webui/static/app.css`; commit it. `make verify-css` checks for drift. Tailwind v4 scans doc comments in `.templ`/`.go` for class candidates; `docs/` + `.claude/` are excluded via `@source not`.
- `make verify-no-popups` fails if `window.alert/confirm/prompt` appears in templ/JS.
- NEVER run `make fmt` (toolchain skew reformats the whole repo).

## Architecture (hexagonal)
- `internal/domain` — entities + pure logic, no I/O.
- `internal/ports` — interfaces (stores, buses).
- `internal/usecase` — application services (`Execute(...)`).
- `internal/adapter/...` — drivers: `httpserver` (HTTP + web handlers), `webui` (templ components), `pgstore` (Postgres), `tui`.
- Wiring is in `cmd/flow-server/main.go` + `internal/adapter/httpserver/server.go`.

## WebUI conventions
- templ + htmx + Tailwind, server-rendered. No SPA, no Node runtime.
- Pages: `XPage(vm)` wraps `components.Base(active, body)` → `components.AppShell(active, breadcrumb, subnav, content)`.
- htmx fragments: `XFragment(vm)`; SSE live via `hx-ext="sse"` (body) + `hx-trigger="sse:<event>"` on containers.
- i18n: NO hardcoded display strings; add keys to `internal/i18n/catalog_de.go` + `catalog_en.go` (Go maps, NOT TOML); use `components.T(ctx, "key")`; de+en parity is test-enforced.
- NO browser popups; confirms via `components.ConfirmDialog`. No emoji pictograms (monospace glyphs ● ◆ ⬡ ▶ ■ + SVG only).
- One responsibility per file ("keine Monolithen").
- Cockpit htmx rule: everything re-rendering the tab area targets `#cockpit-main`; full-page forms use `hx-boost="false"`.

## Dev stack (live verification)
- `make dev-up` starts Postgres + Dex (OIDC). `make dev-run` runs the server (https://localhost:8080, self-signed). `make dev-token` mints a bearer token. (Cookie auth for the browser; scripted Dex login: POST credentials to the Dex form action.)
- Live done-gate: each new route returns expected status; SSE reflects create/update/delete.

## TDD
- Write a failing test, run it (see it fail), implement minimal code, run it (see it pass), commit. Small commits.

## Commit style
- Conventional-commit subject (`feat(webui): …`, `feat(pgstore): …`, `docs: …`). One coherent deliverable per commit.
