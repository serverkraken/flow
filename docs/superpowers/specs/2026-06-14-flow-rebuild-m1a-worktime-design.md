# flow Rebuild · M1a — Worktime Live-Sync · Design

**Datum:** 2026-06-14
**Status:** Draft — Brainstorm abgeschlossen, wartet auf User-Review
**Scope:** Erster nutzbarer vertikaler Schnitt der `rebuild`-App: Timer in der TUI starten → in Echtzeit in der WebUI sichtbar (und umgekehrt). Hält die M1a-spezifischen Entscheidungen fest; Architektur-Fundament siehe `2026-06-13-flow-rebuild-design.md`, Spine-Stand siehe `2026-06-13-flow-rebuild-m0-spine.md`.
**Branch:** Code auf dem langlebigen Orphan-Branch `rebuild` (kein main-merge pro Milestone); Planungs-Docs auf `main` (M0-Präzedenz).

## Warum M1a (Slicing)

Der Foundation-Plan nennt **M1 = „Worktime vertical"** und bündelt darin sechs unabhängige Subsysteme über drei Oberflächen: Device-Flow-Login, Project, Worktime-Core, DayOff/Stats/Burndown/ICS, TUI, WebUI. Genau dieses Bündeln ist der Scope-Sprawl (Root-Cause #2 aus dem `next`-Postmortem), den der Neuaufbau vermeiden soll. Deshalb wird M1 in drei eigenständig abnehmbare Schnitte geteilt:

- **M1a (dieses Dokument):** dünnster Schnitt bis zum Done-Gate — Project-minimal + Session/Timer + SSE + OIDC-Auth-Code-Flow (Browser) + TUI-Worktime-Screen + WebUI-Worktime-Page.
- **M1b:** Device-Flow-Login (CLI/TUI/MCP) — ersetzt das `FLOW_TOKEN`-Env-Bearer überall.
- **M1c:** Worktime-Extras — DayOff, Stats, Burndown, dt. Feiertage, ICS-Export, Zeit-Export pro Projekt/Zeitraum.

Jeder Schnitt: eigener Spec → Plan → Ausführung, `make ci` grün als Tor.

## Done-Gate (Akzeptanztest)

> Timer in der TUI starten → erscheint binnen ~1 s in der WebUI; Stop in der WebUI → die TUI zieht nach.

Dieser eine Cross-Surface-Loop ist der Zweck von M1a. Alles unten ist das Minimum, um ihn **ehrlich** (kein Wegwerf-Code im kritischen Pfad) zu erreichen.

## Scope / Non-Goals

**In Scope:**
- `Project` (minimal) — Entity + Store + REST + Picker (Sessions buchen auf ein Projekt).
- `WorkSession`/Timer — Carry-over-Domain aus v1 (`main`) + Store + REST + SSE-Events.
- **OIDC Auth-Code-Flow** + Server-Session-Cookie für den Browser (echte WebUI-Auth, kein Shim).
- TUI: kleiner Shell + **ein** Worktime-Screen, SSE→`tea.Msg`.
- WebUI: Bootstrap `templ` + HTMX + Tailwind v4 + HTMX-SSE, **eine** Worktime-Page.

**Non-Goals (vertagt):**
- Device-Flow-Login → **M1b** (TUI bleibt bis dahin auf dem M0-`FLOW_TOKEN`-Env-Bearer).
- DayOff / Stats / Burndown / Feiertage / ICS / Zeit-Export → **M1c**.
- Read-Cache (TUI) / PWA-Service-Worker (WebUI) → später; M1a ist online-only (Done-Gate ist Live-Sync, nicht Offline).
- Volle `Project`-Felder (repos/paths/links/meta/rate/brief/tags/pinned/…) → spätere Migrationen, wenn Kompendium/Project-Hub sie brauchen.
- Kompendium/Documents → M2. Pixelgenaue UI-Politur → `frontend-design` / `tui-usability` (später).

## Kern-Entscheidungen (Brainstorm 2026-06-14)

| Frage | Entscheidung |
|---|---|
| M1-Slicing | **Thin-vertical first:** M1a (Live-Sync) → M1b (Device-Flow) → M1c (Extras). |
| Browser-Auth in M1a | **Echter Auth-Code-Flow jetzt** (Server-Session-Cookie), kein Dev-Shim. |
| `Project`-Umfang | **Minimal jetzt, später erweitern** (inkrementelle Migrationen). |
| TUI-Umfang | **Kleiner Shell + nur Worktime-Screen** (keine Palette/weitere Screens). |
| Offline | **Vertagt** — M1a online-only. |
| TUI-Auth in M1a | M0-`FLOW_TOKEN`-Env-Bearer (Device-Flow erst M1b). |

## Datenmodell-Deltas

Neue Migration `migrations/0002_project_worksession.sql` (inkrementell, embedded — Tooling wie M0).

**`Project` (minimal)** — `id`, `ownerUserId`, `name`, `slug` (unique pro Owner), `color`, `glyph`, `status` (active|archived), `createdAt`, `updatedAt`. Die schweren Foundation-Felder werden **nicht** spekulativ angelegt; sie kommen per späterer Migration.

**`WorkSession`** — `id`, `ownerUserId`, `projectId` (nullable bis Stop), `tag?`, `note?`, `start`, `stop?` (null = **läuft**, der aktive Timer), `createdAt`. `elapsed` ist **abgeleitet**, nicht gespeichert.

**Invariante** (Carry-over v1, im Usecase erzwungen): **genau ein laufender Timer pro User**. Start ohne Projekt erlaubt; **Buchen auf ein Projekt ist beim Stop Pflicht** (bestehend oder inline neu angelegt). Ein neuer Start setzt voraus, dass kein anderer Timer läuft.

## Domain & Usecases (Carry-over, kein Rewrite)

- v1-Timer-Logik nach `internal/domain` heben (reine Regeln), dünne Usecases darüber:
  `StartSession`, `StopSession` (inkl. Inline-Project-Booking), `ListSessions`, `CreateProject`, `ListProjects`.
- Neue `ports`: `ProjectStore`, `SessionStore` (zu `internal/ports/ports.go` ergänzt, M0-Stil).
- pgstore-Adapter je Store (eine Datei pro Verantwortung — „keine Monolithen").
- Jede Mutation published ein `domain.Event` auf dem bestehenden `sse.Bus`.

## Auth — Auth-Code-Flow neben M0-Bearer

- Neue Config (confidential Client): `FLOW_OIDC_CLIENT_SECRET`, `FLOW_OIDC_REDIRECT_URL` (oder aus Base abgeleitet), `FLOW_SESSION_SECRET`.
- Routen: `GET /auth/login` → Redirect zu Authentik; `GET /auth/callback` → Code-Exchange, **denselben** `EnsureUser`-Usecase wie M0 fahren, signiertes Session-Cookie setzen; `POST /auth/logout`.
- Neue `webAuth`-Middleware: User aus **Session-Cookie** auflösen (parallel zur M0-Bearer-`auth`, die für `/api/v1/*` vom TUI bleibt).
- **SSE-Stolperfalle eingeplant:** Browser-`EventSource` kann **keinen** `Authorization`-Header setzen → `/api/v1/events` muss **Cookie ODER Bearer** akzeptieren. TUI nutzt Bearer, Browser nutzt Cookie.
- Authentik-Blueprint: `grant_types` explizit inkl. `authorization_code` + `refresh_token` (Device-Flow erst M1b). `hx-boost="false"` auf Auth-Anker (HTMX schluckt sonst den 302).

## API-Surface (neu)

REST:
- `POST /api/v1/sessions` — Timer starten (Projekt optional).
- `POST /api/v1/sessions/{id}/stop` — stoppen + buchen (Projekt Pflicht; inline-create möglich).
- `GET  /api/v1/sessions` — Liste (heute/Zeitraum-Filter minimal).
- `POST /api/v1/projects` — anlegen.
- `GET  /api/v1/projects` — Liste (für Picker).

SSE-Event-Typen (neu in `domain.EventType`): `session.started`, `session.stopped`, `session.updated`, `project.created`. Payload klein (IDs + Minimum). **`EventPing` + `POST /api/v1/debug/ping` werden entfernt** (M0 hat sie als M1-Wegwerf markiert).

## TUI — kleiner Shell + ein Worktime-Screen

`internal/tui` existiert noch nicht → M1a bootstrappt ein **kleines** Bubbletea-v2-Root-Model (Statuszeile + Help + SSE-Plumbing), das **nur** den Worktime-Screen hostet — keine Command-Palette, keine weiteren Screens (die kommen mit ihren Features).

Worktime-Screen: Project-Picker (MRU + Fuzzy + Inline-Create, aus der `projects`-API statt Filesystem), Start/Stop-Keys, laufender-Timer-Anzeige, heutige Sessions-Liste. SSE-Client mappt Events → `tea.Msg` → Model-Update → Re-Render. `apiclient` wächst um `StartSession/StopSession/ListSessions/Projects`. slog → Datei, nie stdout (TUI-Korruptions-Lehre).

## WebUI — Bootstrap templ + HTMX + Tailwind

Erstes `internal/adapter/webui`: `templ`-Komponenten + `templ generate` in CI, Tailwind-v4-Build, HTMX + HTMX-SSE-Extension. Eine Worktime-Page: Timer-Card (**Gradient nur wenn laufend** = Fokus-Element der Design-Sprache), Project-Picker, heutige Liste. Elemente lauschen `hx-trigger="sse:session.stopped"` und re-fetchen ihr Fragment; der Server rendert die Wahrheit. Mobile-first Single-Column. Politur explizit vertagt.

## Real-Time-Sync-Flow (Done-Walkthrough)

1. TUI: `POST /api/v1/sessions` (Bearer) → Usecase startet Session, published `session.started`.
2. Bus fächert an alle Abos desselben Users; Browser-`EventSource` auf `/api/v1/events` (Cookie) empfängt es.
3. WebUI-Element re-fetcht sein Fragment → Server rendert laufenden Timer.
4. WebUI: Stop-Button → `POST /api/v1/sessions/{id}/stop` → `session.stopped` → TUI-SSE-Client → `tea.Msg` → Timer verschwindet, Session in Liste.

## Verifikation (Tor)

- `make ci` grün (Coverage-Gate per M0-Konvention).
- **Per-Route-Smoke** im Composition-Root (Lehre „Pläne brauchen eine Wiring-Task": jede neue Route in `cmd/flow-server/main.go` verdrahtet + curl-smoke).
- **Two-Surface-Live-Sync-Check:** Session-Start via API treiben, asserten dass das SSE-Event landet; manuelle TUI↔Browser-Bestätigung. Das ist der eigentliche Abnahmetest des Milestones.

## Build-Reihenfolge (grob — speist writing-plans)

1. Migration `0002` + `Project`/`WorkSession`-Domain + Invariante (+ Tests).
2. `ProjectStore`/`SessionStore` Ports + pgstore-Adapter (+ Tests).
3. Usecases Start/Stop/List/CreateProject/ListProjects (+ Tests).
4. REST-Handler + neue SSE-Event-Typen; `debug/ping` entfernen; Wiring + Smoke.
5. OIDC Auth-Code-Flow + Session-Cookie + `webAuth` + SSE-Cookie-or-Bearer.
6. `apiclient`-Methoden + SSE-Client.
7. TUI-Shell + Worktime-Screen.
8. WebUI-Bootstrap (templ/HTMX/Tailwind) + Worktime-Page + HTMX-SSE.
9. Wiring-Verifikation + Two-Surface-Live-Sync-Check.

## Offene Punkte (für den Plan)

- Session-Store + `cookie`-Secret-Handling (gorilla/securecookie vs. eigenes signiertes Cookie) — Wahl im Plan.
- SSE-Reconnect-Verhalten im Browser/TUI: Full-Refresh + Resubscribe (M0-Bus droppt bei vollem Buffer) — minimal halten.
- Tailwind-v4-Build-Schritt in CI (Node-Toolchain im Docker-Multi-Stage) — Mechanik im Plan.
- `templ`-Codegen-Gate in `make ci`.
