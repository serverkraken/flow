# flow Client/Server — Phase 1 Design

**Datum:** 2026-06-02
**Status:** draft (brainstorm complete, awaiting user review)
**Scope:** Multi-Device-Sync für einen User (Soenne) — flow-server + Authentik-OIDC + WebUI + flow-mcp + bestehender TUI/CLI. Multi-User (Frau, Sharing) ist explizit **Phase 2** und kommt als reines Feature-Add ohne Architektur-Bruch.

## Problem

Soenne arbeitet regelmäßig an mehreren Notebooks. flow ist heute strikt single-device:

1. **Worktime läuft nur auf einem Laptop.** Sessions, Aktiv-State (`~/.tmux/worktime.state`), Pause-Marker, History — alles lokal in TSV/JSON pro Gerät. Wenn auf dem zweiten Laptop gearbeitet wird, fehlt der Tag in der History.
2. **Kompendium-Notes verteilen sich.** Heute git-backed pro Gerät; ohne aktiven Sync-Job liegen halbfertige Notes auf einem Notebook und sind woanders nicht erreichbar.
3. **Repo-Notes können nicht persistiert werden, wenn das Repo nicht uns gehört.** CLAUDE.md in einem fremden Repo zu committen geht nicht — die Notes leben damit nirgendwo systematisch, schon gar nicht für Claude/MCP zugänglich.

Phase 2 (separater Spec, später) löst zusätzlich: **Mit-Nutzer (z.B. Soennes Frau)** kriegen eigenen Account und Sharing von Repo-Notes wird möglich. Phase 1 baut die komplette Architektur dafür — inkl. Authentik-OIDC und User-Konzept im Datenmodell — liefert aber initial nur einen User aus.

## Entscheidungen aus Brainstorm 2026-06-02

| Frage | Entscheidung |
| --- | --- |
| Repo vs. neuer Repo | **Monorepo**: neue `cmd/flow-server/` + `cmd/flow-mcp/` neben bestehendem `cmd/flow/`. Domain + Ports geteilt. |
| Branch-Strategie | **main**, additive PRs. Kein long-lived `next`-Branch. Neue Binaries kommen erst ins Release-Target, wenn fertig. |
| Sync-Strategie | **Approach 1 — Lokales SQLite ist Truth, Server synct via REST.** Last-Writer-Wins für Notes; CRDT als spätere Migration falls Phase 2 Concurrent-Editing-Schmerz zeigt. |
| Active-Session | **Server-Truth statt Device-Truth.** Pro `(User, Project)` eine aktive Session; **mehrere parallel möglich** (Option 2). |
| Repo-Identifikation | Git-Remote-URL normalisiert als primärer Key; **lokaler Path-Hash als Fallback** für Repos ohne Remote. |
| Auth | **Authentik-OIDC ab Tag 1.** Phase 1 = Single-User-Allowlist (Soennes `sub`). Kein Stub-Auth. |
| Hosting | **Podman / docker-compose für lokales Dev, Kubernetes als Prod-Ziel** (Helm-Chart). Server ist aus dem Internet erreichbar. |
| WebUI Stack | **Templ + HTMX + Tailwind v4 + Alpine.js + ApexCharts + CodeMirror 6.** Gleicher Go-Server rendert API + HTML. |
| Migration bestehender TSV-Daten | **Backlog.** Phase 1 startet "from-scratch", optionaler `flow migrate-from-tsv`-CLI später. |

## Goals

- **Worktime + Notes synchron über alle Geräte** — am gleichen Schreibtisch oder unterwegs.
- **Offline-First:** jedes Client-Gerät arbeitet weiter, wenn der Server nicht erreichbar ist. Sync passiert im Hintergrund.
- **Claude/MCP-Zugriff auf Repo-Notes** ohne dass das Repo unsere Datei sieht — Notes leben im flow-Datenmodell, identifiziert über Repo-Key.
- **Authentik-OIDC** für sowohl WebUI (Auth-Code-Flow) als auch CLI/TUI/MCP (Device-Flow).
- **WebUI** ergänzend zur TUI/CLI — Browser-Zugriff von überall, Vorbereitung für Phase 2.
- **Container-Deployment** — Podman für lokales Dev, K8s mit Helm-Chart für Prod.
- **Architektur Phase-2-ready** — Datenmodell hat User-Konzept ab Tag 1, Multi-User wird kein Refactor.

## Non-Goals (Phase 1)

- Mit-User. Frau kommt in **Phase 2**.
- Sharing von Notes zwischen Usern. Phase 2.
- Echte CRDT-Konfliktauflösung. **LWW reicht**, weil Phase 1 single-user-multi-device ist (keine echten Concurrent-Edits am gleichen Asset).
- Mobile-App / PWA. Responsive Web-UI reicht.
- Real-Time-Collaboration ("ich tippe, du siehst es"). Sync ist eventually-consistent mit Sekunden-Latenz.
- Notification-System (Stop-Reminders, Streak-Warnings push-to-device).
- Automatische Migration bestehender TSV-Sessions. Backlog.
- Selbst-bauen von OIDC. Wir verifizieren JWTs gegen Authentik-JWKS, mehr nicht.

## Architektur

```
┌─────────────────────────────────────────────────────────────────┐
│                          BROWSER                                │
│                       (Tokyonight UI)                           │
└────────────┬────────────────────────────────────────────────────┘
             │ HTTPS (Auth-Code-Flow OIDC)
             │
┌────────────▼─────────────────┐         ┌──────────────────────┐
│       flow-server            │◄────────│   Authentik (extern) │
│  (K8s / Podman Container)    │  JWKS   │   im K8s-Cluster     │
│  ┌────────────────────────┐  │         └──────────────────────┘
│  │ HTTP Handler           │  │
│  │  ├─ /api/v1/...   REST │  │
│  │  └─ /        WebUI     │  │
│  ├────────────────────────┤  │
│  │ Domain + Use Cases     │  │
│  ├────────────────────────┤  │
│  │ Adapters:              │  │
│  │  - sqliteserver        │  │
│  │  - oidcserver          │  │
│  │  - httpserver          │  │
│  │  - webui (Templ)       │  │
│  └────────────────────────┘  │
│  SQLite (Litestream-Backup)  │
└──────────────┬───────────────┘
               │ HTTPS (Device-Flow OIDC, Bearer Token)
               │
   ┌───────────┴──────────────┐
   │                          │
┌──▼──────────────┐    ┌──────▼──────────────┐
│   flow (TUI)    │    │   flow-mcp (stdio)  │
│   Notebook A    │    │   Notebook A        │
│                 │    │                     │
│ ┌─────────────┐ │    │ ┌─────────────────┐ │
│ │ Bubbletea   │ │    │ │ MCP Protocol    │ │
│ │ Use Cases   │ │    │ │ Use Cases       │ │
│ │ Adapters:   │ │    │ │ Adapters:       │ │
│ │  sqlitec    │◄┼────┼─┤  sqlitec        │ │  ◄── shared local cache
│ │  httpsync   │ │    │ │  httpsync       │ │     (one SQLite file)
│ │  oidcclient │ │    │ │  oidcclient     │ │
│ │  fs (legacy)│ │    │ └─────────────────┘ │
│ └─────────────┘ │    └─────────────────────┘
└─────────────────┘

         Notebook B: gleiche zwei Prozesse + eigener Cache
         Notebook C: gleiche zwei Prozesse + eigener Cache
```

**Drei Prozess-Typen:**
1. **flow-server** — Headless HTTP-Server, serviert REST-API + WebUI. State in SQLite. Container.
2. **flow** — bestehender TUI/CLI, neu mit Sync-Adapter. Hält lokales SQLite-Cache + redet mit Server.
3. **flow-mcp** — neuer stdio-MCP-Server für Claude/Cursor/Codex. Liest gleichen lokalen Cache wie `flow`, ist also offline-fähig.

**Wichtigster Punkt:** TUI und MCP **teilen sich denselben Client-Cache und denselben Sync-Adapter**. Beides sind Clients vom flow-server.

## Domain-Model Änderungen

Neue / erweiterte Entitäten:

```go
// NEU
type User struct {
    ID          string    // UUID
    OIDCSub     string    // Authentik 'sub' Claim, unique
    DisplayName string
    CreatedAt   time.Time
}

// NEU
type Project struct {
    ID        string    // UUID
    UserID    string
    Name      string
    Slug      string    // URL-safe, unique per User
    CreatedAt time.Time
}

// NEU
type Repo struct {
    ID            string  // UUID
    UserID        string
    CanonicalKey  string  // "git:github.com/foo/bar" oder "path:sha256(/abs/path)"
    DisplayName   string  // human-readable
    CreatedAt     time.Time
}

// NEU — pro Repo+User eine Note (mehrere möglich später, Phase 1 nur die "CLAUDE-Notiz")
type RepoNote struct {
    ID        string
    RepoID    string
    UserID    string
    Content   string    // Markdown
    Version   int64     // Lamport per (RepoID, UserID)
    UpdatedAt time.Time
}

// ERWEITERT — Session bekommt UserID + ProjectID
// Heute: Date, Start, Stop, Elapsed, Tag, Note (siehe internal/domain/session.go)
type Session struct {
    ID         string    // NEU — heute implizit über (Date, Start)
    UserID     string    // NEU
    ProjectID  string    // NEU (required; Default-Project "Allgemein" für ungetaggte)
    Date       time.Time
    Start      time.Time
    Stop       time.Time
    Elapsed    time.Duration
    Tag        string    // bleibt: "deep", "meeting" etc.
    Note       string    // bleibt
}

// NEU — ersetzt lokales ActiveSessionStore.GetActive() (flowstate.json)
// Mehrere pro User möglich (Option 2: eine pro Projekt)
type ActiveSession struct {
    UserID          string
    ProjectID       string
    StartedAt       time.Time
    StartedOnDevice string    // informativ, "macbook-soenne"
    Version         int64     // Optimistic Concurrency
}
// Primary Key (UserID, ProjectID) — ein User kann pro Projekt höchstens eine
// aktive Session haben, aber an mehreren Projekten gleichzeitig tracken.

// Bestehende: Cheatsheet, DayOff, FlowState, Palette, Wikilinks
// → kriegen UserID, ansonsten unverändert
```

**Repo-Identifikation (Detail):**

- **CanonicalKey** = einer von zwei Formen:
  - `git:<host>/<owner>/<repo>` — extrahiert aus `git remote get-url origin`, normalisiert (lowercase, ohne `.git`-Suffix, `git@` und `https://` zusammengeführt)
  - `path:<sha256-hex>` — Hash des absoluten lokalen Pfads, wenn kein Git-Remote
- Pro User+CanonicalKey existiert eine Repo-Row.
- **Konflikt-Szenario:** User klont Repo auf Laptop B unter anderem Pfad → CanonicalKey ist trotzdem identisch (weil Remote-URL gleich) → Note ist automatisch da. Für Repos ohne Remote: derselbe Pfad auf Laptop B ist Voraussetzung; sonst ist die Note dort schlicht nicht zugänglich. Dokumentiert.

## Sync-Protokoll

REST + JSON. Drei Mechanismen:

1. **Pull (incremental)** — `GET /api/v1/sessions?since=<lamport>` liefert alle Rows mit Version > Lamport. Client merged in lokales SQLite, updated lokalen Watermark.
2. **Push (single)** — `PUT /api/v1/sessions/<id>` mit Body + `If-Match: <version>` Header. Server antwortet 200 mit neuer Version oder 409 mit aktuellem Server-State.
3. **Active-Session (special, server-authoritative)** — `POST /api/v1/active/<project-id>/start` mit `If-Match: 0` (= "darf nicht existieren") oder existing version. Server-Race-Detection bei "läuft schon". Anders als Sessions/Notes (lokal-zuerst, eventually-consistent) wird Active-Session **immer am Server entschieden**, weil cross-device Race-Conditions sonst zu Doppel-Sessions führen.

**Lamport-Clock pro Resource-Typ**, gespeichert in `sync_state` Tabelle clientseitig:

```sql
CREATE TABLE sync_state (
    resource    TEXT PRIMARY KEY,   -- "sessions", "notes", etc.
    watermark   INTEGER NOT NULL    -- last seen server lamport
);
```

**Konflikt-Resolution:**

- Server ist autoritativ für Version-Counter.
- Bei 409 zeigt Client UI: *"Konflikt bei Session XYZ — Server-Version T2, deine lokale T1. [Server übernehmen] [Lokal behalten + überschreiben]"*. Phase 1 erlaubt manuelle Wahl; CRDT-Auto-Merge ist Phase 3.
- Active-Session-Race: Client zeigt *"Session läuft auf macbook-soenne seit 14:30. [Stoppen & neu starten] [Übernehmen] [Abbrechen]"*.

**Background-Sync-Worker im Client:**

- Pull alle 30 s wenn Verbindung steht.
- Push: Write-Queue, FIFO, retry mit exponential backoff bei Netz-Fehlern.
- HTTP SSE optional in Phase 1.5 für Push-Notification "neue Änderungen seit X".

## Authentication

**Authentik-OIDC als externer IDP.** Soenne hat Authentik im K8s schon laufen.

**Browser (WebUI):** Standard Authorization-Code-Flow.
- `/login` → redirect zu Authentik → Code zurück an `/auth/callback` → Server tauscht gegen JWT → Session-Cookie (HttpOnly, Secure, SameSite=Lax).
- Logout: Cookie löschen + OIDC end_session.

**CLI/TUI/MCP:** OIDC Device-Authorization-Flow.
- `flow login` öffnet Browser zu Authentik-Device-URL + zeigt User-Code.
- Authentik gibt Access+Refresh-Token nach Approval.
- **Token-Storage:** OS-Keychain via `github.com/zalando/go-keyring` (macOS Keychain / GNOME Keyring / KWallet / Linux Secret Service).
- Refresh-Logic: Access-Token vor Ablauf erneuern, Refresh-Token persistent.
- `flow logout` löscht beide Tokens.

**Server-Side JWT-Verifikation:**
- JWKS-Endpoint vom Authentik gepullt + gecached (refresh alle 12h oder bei `kid`-miss).
- Standard JWT-Verify: signature, `iss`, `aud`, `exp`.
- `sub`-Claim wird zum User-Lookup verwendet.
- **Phase 1 Allowlist:** `flow-server` config enthält erlaubte `sub`s (oder `email`s). Andere User: 403. Phase 2 ersetzt das durch User-Tabelle + Self-Service-Registration.

**MCP-Auth gleicher Mechanismus wie TUI:** `flow-mcp` liest Refresh-Token aus Keychain (gleicher Slot wie `flow`). Wenn nicht eingeloggt: MCP-Tools antworten mit Error-Resource "Run `flow login` first."

## Modul-Layout

```
flow/
  cmd/
    flow/                ← TUI + CLI (existing, wird Sync-aware)
      main.go            ← Wiring: client-Cache + httpsync-Adapter + fs-Adapter
    flow-mcp/            ← NEU: stdio-MCP-Server
      main.go            ← Wiring: client-Cache + httpsync + MCP-Protocol-Loop
    flow-server/         ← NEU: HTTP+WebUI-Server
      main.go            ← Wiring: sqlite-server + oidcserver + httpserver + webui
  internal/
    domain/              ← + User, Project, Repo, RepoNote, ActiveSession (multi)
    ports/               ← + AuthProvider, SyncTransport, RepoStore, NoteStore
    usecase/             ← + sync use cases, repo-note use cases, active-session
    adapter/
      atomicfile/        ← bleibt
      systemclock/       ← bleibt
      tsvsessions/       ← bleibt (Read-Only-Legacy für migrate-Befehl, Backlog)
      jsonflowstate/     ← bleibt für Standalone-CLI ohne Login
      ...
      sqliteclient/      ← NEU: lokaler Cache (~/.flow/cache.db)
      sqliteserver/      ← NEU: Server-DB
      httpsync/          ← NEU: Background-Worker, Pull+Push, Write-Queue
      httpserver/        ← NEU: Chi-Router, REST-Handler, Middleware
      oidcclient/        ← NEU: Device-Flow + Keychain via go-keyring
      oidcserver/        ← NEU: JWKS-Cache + Verifier-Middleware
    webui/               ← NEU
      handlers/          ← HTMX-aware Templ-Handler
      templates/         ← .templ-Files (typed Templates)
      static/            ← Tailwind-Output, Alpine.min.js, ApexCharts, CodeMirror
      assets.go          ← embed.FS
    syncengine/          ← NEU: Lamport-Logic, Conflict-Detection, Watermark-Walk
    keyringadapter/      ← NEU: dünner Wrapper um go-keyring (für Test-Fake)
  deploy/
    podman/
      docker-compose.yml ← lokales Dev: flow-server + lokales Authentik-Stub optional
      .env.example
      Dockerfile.server
    helm/
      flow-server/
        Chart.yaml
        values.yaml      ← image.tag, ingress.host, authentik.issuer, ...
        templates/
          deployment.yaml
          service.yaml
          ingress.yaml
          configmap.yaml
          secret.yaml    ← oder ExternalSecrets reference
          pvc.yaml       ← SQLite-Volume
```

## WebUI Spec

**Routes:**

| Route | Methode | Zweck |
| --- | --- | --- |
| `/` | GET | Dashboard: aktive Sessions, heute, schnell-Notes |
| `/login`, `/auth/callback`, `/logout` | GET | OIDC-Flow |
| `/worktime` | GET | Heute / Woche / Verlauf / Frei — wie TUI-Tabs |
| `/worktime/sessions/:id` | GET / PUT / DELETE | Edit/Delete einzelner Session via HTMX |
| `/worktime/active/:project-id/start` | POST (HTMX) | Start-Knopf |
| `/worktime/active/:project-id/stop` | POST (HTMX) | Stop-Knopf |
| `/notes` | GET | Note-List mit Suche (FTS5 server-side) |
| `/notes/:id` | GET / PUT | View + CodeMirror-Edit |
| `/notes/new` | GET / POST | Neue Free-Note |
| `/repos` | GET | Repo-Liste mit Note-Indikator |
| `/repos/:hash/note` | GET / PUT | Repo-Note Edit (analog `/notes/:id`) |
| `/projects` | GET / POST | Projekte verwalten |
| `/settings` | GET | User-Info, Device-Liste, Logout |
| `/api/v1/...` | REST | für TUI/MCP/Background-Sync (gleicher Server) |

**Live-Updates (Browser):** SSE-Stream `/api/v1/events?stream=ui` für aktive Session-Ticker und neue Sync-Events. HTMX `hx-ext="sse"` an entsprechenden Elementen. (Anmerkung: das ist UI-only — der TUI/MCP-Sync nutzt weiter 30s-Poll, SSE-für-Clients steht erst in Phase 1.5 an.)

**Tokyonight-Tailwind-Theme:**

- `tailwind.config.js` definiert Custom-Palette mit Hex aus dem TUI-Theme (siehe `internal/frontend/tui/theme`).
- Monospace-Headers (Tokyonight-Stil) — JetBrains Mono via local @font-face.
- Generous whitespace, keine bunten Emoji-Pictogramme (memory: `feedback_no_icons.md` gilt auch hier — TUI-Glyphen oder lucide-icons im stroke-Style).

**Markdown-Editor:** CodeMirror 6 mit:
- markdown-mode + syntax highlighting
- vim-Keybindings als Option (für später)
- preview-Pane right-side toggleable (server-rendered via gleicher Markdown-Renderer wie TUI — `internal/kompendium/...`)

## MCP-Server Spec

**Stdio MCP-Protokoll** (Anthropic-Spec). Tools:

| Tool | Args | Verhalten |
| --- | --- | --- |
| `flow_get_repo_note` | `repo_path: str` | Resolved zu CanonicalKey, lädt Note aus Cache. Wenn keine Note: leerer Content + `exists: false`. |
| `flow_save_repo_note` | `repo_path: str, content: str` | Schreibt in Cache, queued Push. Idempotent. |
| `flow_list_repo_notes` | `query?: str` | Liste aller Repo-Notes des Users (für Browse). |
| `flow_search_notes` | `query: str, limit?: int` | FTS5-Search im lokalen Cache (Kompendium + RepoNotes). |
| `flow_get_note` | `id: str` | Einzelne Kompendium-Note. |
| `flow_save_note` | `id: str, content: str` | Update Kompendium-Note. |
| `flow_worktime_status` | — | Aktive Sessions + heute-Saldo. |
| `flow_start_session` | `project: str, tag?: str, note?: str` | Active-Session-Start. Conflict-Errors propagiert. |
| `flow_stop_session` | `project: str` | Stop. |

**Resources** (MCP Resource-API):

- `flow://repos/<canonical-key>/note` — Repo-Note als auto-attached Resource bei passendem Working-Directory. Claude sieht das als Kontext-Quelle.

**Init-Verhalten:** Beim Start prüft `flow-mcp` ob OIDC-Token in Keyring vorhanden + nicht abgelaufen. Wenn nicht: alle Tools antworten mit Error "Login required: run `flow login` in a terminal first."

**SessionStart-Hook-Integration (optional, Soenne-spezifisch):**
- `.claude/hooks/load-repo-note.sh` läuft beim Session-Start, ruft `flow-mcp` tool `flow_get_repo_note` mit `$PWD`, injiziert Output in Context. So bekommt Claude die Repo-Note automatisch ohne Tool-Aufruf zu brauchen.

## Server-Implementierung

**Router:** `github.com/go-chi/chi/v5` — leichtgewichtig, idiomatisch.

**DB:** SQLite via `modernc.org/sqlite` (pure-Go, kein CGo — passt zu flow's Pure-Go-Stance). Migrations via `github.com/pressly/goose/v3` oder embedded golang-migrate.

**Templ:** `github.com/a-h/templ` — typed Go-Templates, kompiliert zu Go-Code.

**Backup:** Litestream als **Sidecar-Container** (eigener Pod-Container im K8s, gemeinsames Volume; bei Podman-Compose ein zweiter Service). Replicas zu S3-kompatiblem Storage (Minio, Backblaze, was auch immer Soenne im Cluster hat).

**Embedded Assets:** Tailwind-Output, Alpine.js, ApexCharts via `//go:embed` in `webui/assets.go` — gleiches Single-Binary-Prinzip wie das heutige `flow`.

**Healthcheck:** `/healthz` (liveness, immer 200 wenn Prozess läuft) + `/readyz` (readiness, prüft DB-Connect + JWKS-Cache).

**Metrics:** `/metrics` Prometheus-Endpoint mit Standard-Counter (req_total, req_duration, db_query_duration, sync_conflicts_total). Phase 1 minimal.

## Client-Adapter

**sqliteclient (`internal/adapter/sqliteclient/`)**:

- Lokale Cache-DB bei `~/.flow/cache.db` (XDG-Pfad: `$XDG_DATA_HOME/flow/cache.db`).
- Schema-Subset des Server-Schemas (gleiche Tabellen, gleiche Spalten, plus `sync_state`-Tabelle).
- Implementiert dieselben Ports wie heute `tsvsessions`, `kompendium`, etc. — Use-Cases sehen keinen Unterschied zur lokalen TSV-Version.
- Foreign-Key-Constraints lokal: nein (Server validiert).

**httpsync (`internal/adapter/httpsync/`)**:

- Background-Goroutine, startet beim `flow`/`flow-mcp` Boot.
- Pull-Tick alle 30s. Push-Tick: bei Local-Write sofort triggern (debounce 500ms).
- Write-Queue persistent in `sync_state.queue` Tabelle — überlebt Crash/Restart.
- Bei 401: triggert Token-Refresh über `oidcclient`. Bei Refresh-Fail: signalisiert "Login required" über Channel an UI.

**oidcclient (`internal/adapter/oidcclient/`)**:

- Device-Flow-Implementation (RFC 8628).
- `flow login` ist ein use-case in `cmd/flow`, der diesen Adapter callt.
- Token-Storage: `keyringadapter` (zalando/go-keyring), Slot-Name `flow-server-tokens` (gleich für `flow` und `flow-mcp`).

## Testing-Strategie

**Bestehende Tests:** laufen weiter. `tsvsessions`-Adapter bleibt, sein Coverage zählt weiter. Der Refactor "Session bekommt UserID" passt die Tests an.

**Neue Test-Schichten:**

- **Unit:** syncengine (Lamport-Walk, Conflict-Detection), oidcclient (mit Mock-IdP), httpserver-Handlers (mit Fake-Stores).
- **Integration:** docker-compose mit echtem flow-server-Container + curl-basierte API-Tests. Authentik gemockt via `dexidp/dex` als leichter OIDC-IdP für Tests.
- **End-to-End:** Playwright (Headless-Chromium) gegen lokalen WebUI-Container. Smoke: Login, Session starten/stoppen, Note editieren.
- **MCP:** stdio-Loopback-Test — Test spawnt `flow-mcp`, sendet MCP-Init + Tool-Call, verifiziert Response.

**Coverage-Erwartung Phase 1:** ≥ 80% für neue Pakete (etwas niedriger als die heutigen 85.6% — neue HTTP-Pfade haben mehr Edge-Cases die nur in E2E auffallen). Bestehende Coverage darf nicht sinken.

## Rollout-Plan (Milestone-Sequenz)

Jedes Milestone ist eigener PR-Set zu `main`, mergeable, nichts brechend:

1. **M1 — Skeleton:** `cmd/flow-server` mit `/healthz`, `/login`, `/auth/callback`, JWKS-Cache. `cmd/flow login` mit Device-Flow + Keychain. Es passiert noch nichts mit Daten. Deployable.
2. **M2 — Domain-Erweiterung + sqliteclient:** User/Project/Repo/RepoNote ins Domain. `sqliteclient`-Adapter, lokale DB initialisiert sich beim ersten Start. Bestehende TUI nutzt noch die FS-Adapter parallel.
3. **M3 — Sessions-Sync:** httpserver für `/api/v1/sessions`. httpsync-Adapter. TUI kann zwischen "lokal" (FS-Adapter) und "synced" (sqliteclient + httpsync) umstellen per Config-Flag. Active-Session-Logic.
4. **M4 — Notes-Sync:** Kompendium-Notes + RepoNotes über Server. FTS5 server-side. Conflict-UI in TUI.
5. **M5 — flow-mcp:** stdio-Server, alle Tools, SessionStart-Hook-Doku.
6. **M6 — WebUI Read-Only:** Routes für Worktime + Notes anzeigen. Tailwind+Templ Setup.
7. **M7 — WebUI Write:** Start/Stop, Session-Edit, Note-Edit, Repo-Note-Edit, CodeMirror, ApexCharts.
8. **M8 — Deployment:** Dockerfile, docker-compose für Podman, Helm-Chart für K8s. Litestream-Sidecar. Production-Deploy.
9. **M9 — Hardening:** Metrics-Endpoint, structured logging, retry-Policies, Backup-Verifizierung. Release `flow-server` v0.1.0.

**Phase-1-Complete-Kriterium:** Soenne nutzt `flow-server` produktiv von zwei Laptops + Browser. Auth funktioniert. Repo-Note via MCP klappt in Claude Code. WebUI ist benutzbar.

## Risiken & Offene Punkte

1. **Pure-Go-SQLite ist langsamer als CGo-SQLite.** `modernc.org/sqlite` ist ca. 30% langsamer als `mattn/go-sqlite3` bei Write-Heavy-Workloads. Für Hobby-Scale (1 User, dann 2) irrelevant. Falls jemals Performance-Schmerz: einfach Build-Tag tauschen.
2. **Authentik-Device-Flow muss enabled sein.** Default-Config evtl. nicht. Spec sollte Dokumentation für Authentik-Konfiguration enthalten (Provider/Application-Setup).
3. **Litestream-Restore-Drill** muss Teil von M9 sein, sonst ist das Backup nur theoretisch.
4. **Repo-CanonicalKey-Edge-Cases:** Repos mit mehreren Remotes (origin + fork), Worktrees (Sub-Pfade desselben Repo), Forks mit gleicher Upstream-URL. Spec sagt "origin nimmst", aber das ist nicht abschließend.
5. **Backlog:** Migration der bestehenden TSV-Sessions auf Soennes Hauptlaptop. Manueller `flow migrate-from-tsv`-Befehl als optionaler Schritt vor M3-Rollout.
6. **Backlog:** PWA / Mobile-Optimierung — responsive Tailwind reicht für Phase 1.
7. **Backlog:** Streak-/Saldo-Berechnung server-side (aktuell client-side, könnte für WebUI doppelt implementiert werden — sauberer wäre ein gemeinsamer Use-Case).

## Phase-2-Vorschau (nicht in diesem Spec)

- Frau als zweiter User in Authentik anlegen, Allowlist erweitern.
- Sharing-Modell: per-Repo `repo_shares(repo_id, shared_with_user_id, perm)` — wenn du einen Repo teilst, sieht der andere User die Repo-Note zu diesem Repo. Notes-Body bleibt LWW.
- User-Management-UI: `/admin/users` für Allowlist-Pflege; oder Self-Registration mit Approval.
- Sharing-UI im WebUI: per-Note-Share-Button, "geteilt mit"-Indikator.
- Architektur bleibt unverändert — Phase 2 ist Feature-Add ohne Refactor.

## Phase-3-Vorschau (bei Bedarf)

- CRDT-Migration für Note-Body falls Concurrent-Editing in Phase 2 schmerzt.
- Real-Time-Collaboration via WebSocket.
- Mobile-App / PWA mit Offline-Support.
