# Playwright (Phase 2)

Phase 2: end-to-end browser tests via Playwright. M7 ships handler unit + router-level + curl smoke.

## What lives here today

Nothing. This directory is a placeholder so the eventual `package.json` /
`playwright.config.ts` / `tests/*.spec.ts` tree has an obvious home, and
so the M7 plan can reference a concrete location for the Phase-2
follow-up.

## What M7 ships instead

| Layer                          | Where                                            |
|--------------------------------|--------------------------------------------------|
| Handler unit tests             | `internal/webui/handlers/*_test.go`              |
| Router-level wiring tests      | `internal/webui/handlers/*_test.go` (`httptest`) |
| SSE broadcaster unit tests     | `internal/webui/sse/broadcaster_test.go`         |
| Anonymous + asset smoke (curl) | `scripts/smoke-m6-webui.sh`                      |
| Cookie-auth mutation smoke     | `scripts/smoke-m7-webui-write.sh`                |

Run both smokes together with `make smoke-webui`. The mutation half
SKIPs gracefully when `FLOW_SMOKE_OIDC_ID_TOKEN` is unset (no dex / no
Authentik token in the calling shell), so the script stays useful as a
boot-time regression guard without requiring a real IdP.

## When to wire Playwright

Defer until at least one of:

- The TUI/CLI write surface is no longer the primary editor and the
  WebUI hosts flows that can't be cleanly asserted from curl (multi-step
  HTMX cascades, drag-and-drop, focus management).
- A regression slips past the handler-unit + curl-smoke layers and only
  shows up in a real browser (cookie SameSite, CSP, CodeMirror init
  races, etc.).
- The Phase-2 plan calls it out explicitly.
