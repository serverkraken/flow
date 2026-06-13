# flow — Green-Field Rebuild · Foundation Design

**Datum:** 2026-06-13
**Status:** Draft — Brainstorm abgeschlossen, wartet auf User-Review
**Scope:** Architektur-Fundament für den kompletten Neuaufbau von `flow` als server-zentrale Multi-User-App (WebUI + TUI). Hält die *querschnittlichen* Entscheidungen und die Milestone-Roadmap fest. Pro-Milestone-Details (M0–M5) bekommen eigene Specs/Pläne.
**Ersetzt:** den `next`-Branch (offline-first Client/Server) — **aufgegeben**.

## Warum Neuaufbau — Lehren aus `next`

`next` war offline-first: lokales SQLite als Wahrheit, eventual REST-Sync (Last-Writer-Wins), Litestream-Backup. Der Sync-Layer (httpsync, flockstate, sqliteclient, Konfliktlogik) quoll auf und wurde im „Server-only Rebuild" R1/R2a/R2b selbst wieder herausgerissen; danach 76 UX-Findings. Diagnose — zwei Root-Causes:

1. **Falsches Fundament:** Offline-First + Last-Writer-Wins kollidiert mit dem Kernwunsch „Änderung sofort auf der Gegenseite sichtbar". Beides gleichzeitig geht nicht sauber.
2. **Scope-Sprawl:** M1–M9 am Stück, dann PoC-Review, dann Rebuild-im-Rebuild — nie früh ein nutzbarer vertikaler Schnitt.

**Konsequenz:** (a) **server-authoritative** statt offline-first, (b) **vertikale Slices** mit früh nutzbarem Ergebnis.

## Goals

- **Eine Wahrheit, überall synchron:** Server ist Single Source of Truth; TUI und WebUI zeigen denselben Stand, Änderungen erscheinen **in Echtzeit** auf der Gegenseite (Timer-Start/Stop, Dokument angelegt/geändert).
- **Worktime** wie in v1 (`main`) — das Konzept war gut: Sessions mit Tag/Notiz, Tagesziel, Day-Offs, Stats, Burndown, dt. Feiertage, ICS-Export.
- **Kompendium** als Wissensbasis: Tages-, Projekt- und freie Notizen — plus **agentische Dev-Docs** (von Claude geschrieben) und die **CLAUDE.md/Brief** je Projekt. Gute Suche.
- **Project als geteilter First-Class-Hub** für Worktime + Kompendium, mit Metadaten/Bezügen (Repos, Geräte-Pfade, Auftraggeber, Links …).
- **MCP:** Claude liest/schreibt Kompendium-Dokumente, sucht (inkl. semantisch) und holt sich Projekt-Kontext — die agentische Memory-Bank lebt in flow.
- **Mobile-First WebUI**, die auch auf dem Desktop gut aussieht; **schicke TUI** (Tokyonight) für Worktime + Kompendium.
- **Multi-User ab Tag 1** (Authentik-OIDC); Teilen von Kompendium-Daten als Feature in M4.
- **Server-Betrieb von Anfang an:** Postgres via CloudNativePG, Kubernetes, ein Container.

## Non-Goals

- **Kein Offline-Schreiben.** Offline = read-only „letzter bekannter Stand" (Read-Cache). Timer/Schreiben brauchen den Server → kein Sync-Layer, keine Konfliktauflösung.
- **Kein Sidekick, keine Projekt-Launcher-View** (alter Sourcecode-/tmux-Ordner-Picker). Project bleibt als *Konzept*.
- **Kein Echtzeit-Co-Editing** am selben Dokument (Sekundenfrische ja, gleichzeitiges Tippen nein) — evtl. später.
- **Kein eigenes OIDC** — Verifikation gegen Authentik.
- **Keine externe Such-Engine** (Meilisearch/Typesense) — Postgres reicht.
- **Kein Geld-/Rechnungswesen.** `rate` ist optionales Insight (Σh×Satz), kein Billing.

## Kern-Entscheidungen (Brainstorm 2026-06-13)

| Frage | Entscheidung |
|---|---|
| Offline | **Read-Cache (B).** Server = Wahrheit; Clients always-online für Writes; offline read-only. |
| Topologie | **Ein Go-Binary (A).** `flow-server` liefert JSON+SSE-API **und** server-rendered WebUI. TUI + `flow-mcp` = API-Clients. |
| WebUI-Stack | **templ + HTMX + Alpine.js + Tailwind v4**, PWA für Read-Cache. |
| Repo | **`flow` bleibt**, Neuaufbau auf `git checkout --orphan rebuild` (echte grüne Wiese). `next` aufgegeben. |
| Auth | **Authentik-OIDC ab Tag 1.** WebUI: Auth-Code-Flow. CLI/TUI/MCP: Device-Flow. Multi-User im Modell ab Tag 1. |
| DB | **Postgres via CloudNativePG** in K8s. |
| Sync | **SSE-Live-Push** pro User; Clients abonnieren, WebUI swappt HTML-Fragmente, TUI aktualisiert sein Modell. |
| Suche | **PG-FTS (`german`+`simple`) + `pg_trgm`**; `pgvector`-Semantik als M2/M3-Add-on. |
| Teilen | **Generisches Share-Primitiv** (Project\|Document → User\|Authentik-Gruppe, read\|write). Worktime privat. UX in M4. |
| MCP-Scope | **Breit:** documents CRUD (`agent`), search (kw+semantisch), project context (resolve/list/create, brief). User-scoped. |
| WebUI-Sprache | **B + A:** heller Canvas, Projektfarben als Buntheit, Gradient nur fürs Fokus-Element, horizontale Diagramme, mobile-first → Desktop. |
| TUI | **Tokyonight beibehalten** (tui-usability-Skill), dünner API-Client + SSE. |

## Architektur

```
                         ┌─────────────────────┐
        Auth-Code-Flow   │  Authentik (extern) │
      ┌──────────────────┤  id.thebackend.org  │
      │   JWKS / OIDC     └─────────┬───────────┘
      │                             │ Device-Flow (CLI/TUI/MCP)
┌─────▼─────────────────────────────────────────────────┐
│                     flow-server  (ein Binary)          │
│  HTTP:  /api/v1/...  (JSON REST)   +   /  (WebUI)       │
│         /api/v1/events  (SSE, per-User)                │
│  ┌──────────────────────────────────────────────────┐ │
│  │ httpserver · webui(templ/HTMX) · sse-broadcaster  │ │  ← driving/driven adapter
│  ├──────────────────────────────────────────────────┤ │
│  │ usecase (application services)                    │ │
│  ├──────────────────────────────────────────────────┤ │
│  │ domain (reine Typen + Regeln)                     │ │
│  ├──────────────────────────────────────────────────┤ │
│  │ ports → adapter: pgstore · oidc · search · ...    │ │
│  └──────────────────────────────────────────────────┘ │
└───────────────┬────────────────────────────────────────┘
                │ Postgres (CloudNativePG)
   ┌────────────┴───────────┬─────────────────────────┐
   │ HTTPS (Bearer)         │ HTTPS (Bearer)          │ HTTPS (Session)
┌──▼──────────┐      ┌──────▼────────┐         ┌──────▼───────┐
│ flow (TUI)  │      │  flow-mcp     │         │   Browser    │
│ Bubbletea   │      │  (stdio MCP)  │         │   (PWA)      │
│ +Read-Cache │      │  thin client  │         │ +Service-W.  │
└─────────────┘      └───────────────┘         └──────────────┘
```

- **Ein Composition-Root** (`cmd/flow-server/main.go`) verdrahtet alles; jede Route bekommt einen Smoke-Check (Lehre: explizite Wiring-Verifikation).
- Clients sind **dünn**: nur REST + SSE abonnieren. Keine Geschäftslogik doppelt.

## Datenmodell

Alle Entitäten gehören einem **User** (`ownerUserId`). Geteilte Sichtbarkeit via `Share`.

**User** — `id`, `oidcSub` (Authentik `sub`/`username`, unique), `displayName`, `email`, `defaultRate?` {amount, currency}, `createdAt`. Gruppen kommen aus dem OIDC-Token (nicht persistiert, ggf. gecacht).

**Project** (First-Class, von Worktime + Kompendium genutzt)
- Kern: `id`, `ownerUserId`, `name`, `slug` (unique/owner), `status` (active\|archived), `color`, `glyph`, `category?`, `tags[]`, `pinned`, `priority?`
- Bezüge: `aliases[]`, `links[]` {label,url}, `repos[]` (normalisierte Git-Remote-URLs = geräteunabhängige Identität), `paths[]` {deviceId, path} (pro Gerät, mehrere möglich)
- Frei: `meta` (JSONB) — beliebige Felder; bekannte Keys `client`/`costCenter`/`ticket`
- Insight: `rate?` {amount, currency} (überschreibt User-Default) → Σh×Satz, rein lesend
- Agent: `briefDocId?` → das Brief-Dokument (= CLAUDE.md)
- `createdAt`, `updatedAt`

**Device** (leicht) — `id`, `ownerUserId`, `label`/`hostname`, `lastSeenAt`. Trägt `Project.paths[]`; später für Sync-/Cache-Zuordnung.

**WorkSession** — `id`, `ownerUserId`, `projectId` (zum **Buchen Pflicht**; beim Start optional, spätestens beim Stop einem Projekt zuordnen — bestehend oder neu inline angelegt), `tag?`, `note?`, `start`, `stop?` (null = **läuft**, der aktive Timer), `elapsed` (abgeleitet), `createdAt`. **Genau ein laufender Timer pro User** (mehrere waren verwirrend): schnell starten ohne Projektwahl ist erlaubt, gebucht wird beim Stop; ein neuer Start setzt voraus, dass der laufende gestoppt (= gebucht) ist.

**DayOff** — `id`, `ownerUserId`, `date`, `kind` (holiday\|vacation\|sick), `label`, `targetOverride?` (Dauer). Dt. Feiertage als Seed.

**Document** (Kompendium-Notiz) — `id`, `ownerUserId`, `projectId?`, `type` (daily\|project\|free\|agent), `path` (menschenlesbarer Slug, unique/owner[+project] — z.B. `docs/architecture`, `plans/2026-06-13-rebuild`), `title`, `tags[]`, `body` (Markdown), `date?` (für daily), `role?` (brief), `extra` (JSONB — beliebige Frontmatter-Keys, wie v1 `Frontmatter.Extra`), `createdAt`, `updatedAt`.
- **Brief-Rolle:** genau ein Document/Project mit `role=brief` = die **CLAUDE.md**; `Project.briefDocId` zeigt darauf; MCP liefert sie als Kontext; syncbar mit der echten CLAUDE.md im Repo (auch fremde Repos) — löst das alte „Repo-Notes lassen sich nicht persistieren"-Problem.
- **Agentische Memory-Bank:** `agent`-Docs unter `memory/*` (active-context, patterns, decisions …), von Agenten via MCP gepflegt.

**DocLink** (abgeleitet, für Backlinks) — `srcDocId`, `targetSlug`/`targetDocId`, `display?`. Aus `[[wikilinks]]` im Body extrahiert.

**Share** — `id`, `subjectType` (project\|document), `subjectId`, `granteeType` (user\|group), `granteeId`, `level` (read\|write), `createdByUserId`, `createdAt`.

**Such-Index** — `tsvector`-Spalte (generated) auf Document + `pg_trgm`-Indizes; später `vector`-Spalte (pgvector) für Semantik.

## Real-Time-Sync (SSE)

- **`GET /api/v1/events`** — authentifizierter SSE-Stream, scoped auf den User (+ geteilte Subjects).
- **Events:** `session.started|stopped|updated`, `document.created|updated|deleted`, `project.created|updated|archived`, `dayoff.created|deleted`. Payload klein (IDs + Minimum).
- **WebUI (Hypermedia):** HTMX-SSE-Extension; Elemente lauschen via `hx-trigger="sse:session.stopped"` und re-fetchen ihr Fragment — der Server rendert den neuen Stand. Server weiß alles, schiebt HTML.
- **TUI:** abonniert denselben Stream als JSON, mappt Events auf `tea.Msg` → Modell-Update → Re-Render.
- **Read-Cache:** TUI persistiert letzten Stand lokal (Datei/SQLite, **nur Cache**, nicht Wahrheit); WebUI als PWA via Service-Worker. Bei Reconnect: Full-Refresh + Resubscribe. Offline → read-only Banner.

## Auth & Multi-User

- **WebUI:** OIDC Auth-Code-Flow gegen Authentik; Server-Session-Cookie. (`hx-boost="false"` auf Auth-Anker, sonst schluckt HTMX den 302.)
- **CLI/TUI/MCP:** OIDC **Device-Flow** (public Client, `device_code`-Grant, brand-level Flow mit `authentication: none`); Token im OS-Keyring. (macOS-Keyring kappt ~2 KiB/Item → Token-Felder splitten; `go-keyring-base64:`-Prefix beachten.)
- **Issuer/JWKS:** per-provider Issuer; Multi-Verifier oder `InsecureIssuerURLContext` (kein globaler Discovery-Endpoint). Statische Allowlist initial (`FLOW_ALLOWED_SUBS`); `sub` via `user_username`/`email` (nicht `hashed_user_id`).
- **Authentik-Blueprint:** OAuth2-Provider explizit `grant_types: [authorization_code, refresh_token, device_code]`.
- **Ownership** überall; **Share** + Authentik-**Gruppen** (Gruppen-Claim im Token).

## Suche

- **Basis:** Postgres-FTS (`to_tsvector('german', …)` + `simple` für exakte Tokens) + `pg_trgm` (Trigram, Tippfehler/Teilstring). Ranking + `ts_headline`-Snippets. Filter: `type`, `project`, `tags`. Hinter `ports.Search`.
- **Add-on (M2/M3):** `pgvector` — Embedding-Spalte je Document, „verwandte Notizen" + agentisches Recall. Embedding selbst-hostbar (kleines Modell im Cluster); Modellwahl offen.

## MCP (flow-mcp)

Dünner stdio-MCP-Adapter über die **gleiche REST-API** (kein Sonderweg). Tools:
- **documents:** `create` / `read` / `update` (Typ `agent`, an Project), `delete` gegated/optional.
- **search:** keyword + (später) semantisch.
- **project:** `resolve` (aktuelles Repo→Project via `repos[]`/`paths[]`), `list`, `create`, `getBrief`/`setBrief`.
- **context:** `project.context` liefert Brief + Memory-Doc-Index in einem Call.
- User-scoped (Device-Flow/Token). **Worktime nicht im MCP.**
- **Memory-Bank-in-flow:** Agenten (inkl. memory-bank-synchronizer) schreiben Kontext-Docs via MCP nach flow statt nur lokal. Laden in Claudes Kontext (M3-Detail): MCP-Fetch (`project.context`) universell + optionaler dünner lokaler Mirror für eigene Repos (Harness-Autoload/Offline).

## Hexagonale Struktur (Skizze; exaktes Layout im M0-Plan)

```
cmd/flow-server   composition root: wire domain+usecase+adapter+http; smoke jede Route
cmd/flow          TUI + CLI (apiclient-Adapter, Bubbletea v2, Read-Cache)
cmd/flow-mcp      MCP-Server (apiclient-Adapter)
internal/
  domain/         reine Typen + Regeln (User, Project, WorkSession, DayOff, Document, Share, Link, Search…)
                  — dienen auch als JSON-DTOs (DRY)
  usecase/        Application-Services (hängen an ports)
  ports/          Interfaces: Stores, Clock, IDGen, OIDC, EventBus, Search, Embedder…
  adapter/
    pgstore/      Postgres (pgx) — alle Stores
    oidc/         Authentik JWKS verify + Device-Flow
    sse/          EventBus / Broadcaster
    search/       PG-FTS + pg_trgm (+ pgvector später)
    httpserver/   REST-Handler + SSE
    webui/        templ-Komponenten + HTMX-Handler
    apiclient/    Client-seitig: REST-Aufrufe für TUI/MCP
    markdown/     Renderer (carry-over + Ausbau)
  tui/            Bubbletea-App + Komponenten
```
Prinzipien: **DRY**, kleine fokussierte Files pro Verantwortung (ein Use-Case/Adapter/Helper = eine Datei), `main.go` nur Wiring. Keine Monolithen.

## Tech-Stack

- **Go** (aktuell), `pgx/v5`, **CloudNativePG** (Postgres) in K8s.
- HTTP: std `net/http` (+ leichter Router), **templ** (HTML), **HTMX** + **Alpine.js** + **Tailwind v4**, **SSE** (std).
- Auth: `coreos/go-oidc`, Authentik.
- TUI: **Bubbletea v2 / Lipgloss v2** (`charm.land/v2`). (Lipgloss-v2 `Width` = outer-total inkl. Border.)
- Migrationen: eingebettet (goose/atlas — Wahl im M0-Plan).
- Deploy: Docker Multi-Stage, **Helm**, cnpg, Authentik; rolling Image (immutable Digest/SHA-Tag pinnen, sonst stale Mirror).

## WebUI-Design-Sprache

- **Mobile-First**, dann Desktop-Enhancement: Single-Column-Basis (Daumen-Reichweite, Bottom-Nav), ab `md`/`lg` Mehrspaltigkeit (Sidebar-Nav, breitere Daten-/Listen-Ansichten, Split-View Liste+Detail).
- **B + A:** heller, ruhiger Canvas; **Projektfarben** (jedes Project hat `color`) tragen die Buntheit; **Gradient** nur fürs Fokus-Element (aktiver Timer); großzügiger Weißraum + klare Hierarchie.
- **Horizontale** Diagramme (Progress/Balken), keine vertikalen Bars.
- **PWA** (Service-Worker) für den Read-Cache.
- Pixelgenaue Ausarbeitung später via `frontend-design`-Skill.

## TUI

- **Tokyonight** beibehalten (v1-Semantik/Paletten waren ausgearbeitet); `tui-usability`-Skill governt Farben, Keybinds, Spacing, Glyph-Whitelist.
- Worktime + Kompendium als Screens; dünner API-Client; SSE-Live-Updates; Project-Picker mit MRU-Sort + Fuzzy + inline-create (aus Project-Entitäten/`paths[]` statt Filesystem).
- Markdown-Viewer (carry-over) + Ausbau.

## Milestone-Roadmap (vertikale Slices)

- **M0 — Spine.** `flow-server`-Skelett, Postgres+cnpg lokal (compose), OIDC-Verify, User-Modell, EventBus/SSE-Infra, hexagonales Gerüst, **ein** dünner Client-Pfad verdrahtet. *Done:* Login → API → ein Live-Event end-to-end.
- **M1 — Worktime vertical.** Sessions/Timer/DayOff/Stats/Burndown/ICS in Server + TUI + WebUI, **live-synced**; Zeit-Export pro Projekt/Zeitraum. *Done:* Timer in TUI starten → sofort in WebUI (und umgekehrt).
- **M2 — Kompendium vertical.** Documents (daily/project/free/agent) + Pfade + Wikilinks/Backlinks + Tags + **Suche (FTS+trgm)** + Markdown in Server/TUI/WebUI, live-synced. pgvector-Semantik startet hier. *Done:* Notiz in WebUI → sofort in TUI; Suche liefert.
- **M3 — MCP.** `flow-mcp` mit breitem Scope; Brief/CLAUDE.md-Sync; Memory-Bank-in-flow. *Done:* Claude schreibt Plandoc + sucht + liest Brief.
- **M4 — Sharing.** Share-UX (Project/Document → User/Gruppe, read\|write). *Done:* zweiter User sieht ein geteiltes Projekt.
- **M5 — Deploy.** K8s/cnpg/Authentik/Helm im Homelab, `flow.thebackend.org`. *Done:* live + Digest-gepinnt.

Integration: ein langlebiger `rebuild`-Branch (kein main-merge pro Milestone).

## Carry-over-Inventar

**Übernehmen (als frische Commits in den Orphan-Branch):**
- **Markdown-Renderer** (`markdown-shell-render`-Stack) — übernehmen **und ausbauen**.
- **Worktime-Domain:** `session`, `dayoff`, `day`, `stats`, `burndown`, `holidays_de`, `ics`, `range`, `format/parse` — Logik, nicht die TSV-Adapter.
- **Kompendium-Domain:** `frontmatter` (inkl. `Extra`-Pattern), `link`/`ExtractLinks`, `id`/Pfad-Slugs, `search`-Query-Shape.
- **TUI-Komponenten/Patterns** (Bubbletea v2), Command-Palette, `markdown_overlay`.
- CLI-Verben (Cobra) als dünne API-Aufrufer.

**Wegwerfen:** alle File-Adapter (`tsvsessions`, `dayoffstsv`, `jsonflowstate`, `fsprojects`, `linkstsv`, `flockstate` …), `httpsync`/`sqliteclient`/Litestream, **Sidekick**, **Projekt-Launcher-View**, tmux-Bridge als Worktime-Quelle.

## Bekannte Stolperfallen (aus `next` gelernt — vorab einplanen)

- **Authentik:** Blueprint braucht explizite `grant_types`; Device-Flow = public Client + `device_code` + brand-level Flow `authentication: none`; `issuer_mode` per_provider; `sub` via username/email für statische Allowlist.
- **macOS-Keyring:** ~2 KiB/Item → Token-Felder splitten; `go-keyring-base64:`-Prefix strippen.
- **HTMX:** `hx-boost="false"` auf OIDC/externe Redirects.
- **Lipgloss v2:** `Style.Width(n)` = outer-total inkl. Border.
- **Deploy:** `:next`/`:latest` cached → immutable Digest/SHA-Tag pinnen.
- **slog→TUI:** strukturierte Logs dürfen die TUI nicht zerschießen (eigener Writer/Datei).

## Offene Punkte (für die jeweiligen Milestone-Specs)

- Embedding-Modell für pgvector (M2/M3).
- Memory-Bank-Lademechanismus: MCP-Fetch vs. lokaler Mirror (M3).
- Migrations-Tooling (M0).
- SSE-Reconnect-/Backpressure-Verhalten (M0/M1).
