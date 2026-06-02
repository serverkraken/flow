# flow Phase 1 — M2 + M3 Design: Domain-Erweiterung + Sessions-Sync

**Datum:** 2026-06-02
**Status:** draft (Brainstorm abgeschlossen, awaiting user review)
**Scope:** Refinement der M2+M3 Sub-Milestones des Phase-1-Specs (`2026-06-02-flow-client-server-phase1-design.md`). Konkretisiert Domain-Schema, DB-DDL, Sync-Protokoll, Konflikt-UX, TSV-Migration und CLI/TUI-Integration. Ist Vorgabe für `writing-plans` → "Plan B".

**Voraussetzung:** M1 (Branch `feat/phase1-m1-server-skeleton-oidc`, 31 Commits) ist implementiert: flow-server-Skeleton + OIDC-End-to-End + Browser-Login funktionieren. Domain/Daten kommen jetzt.

## Problem

Der Phase-1-Spec gab Domain-Modell und Sync-Protokoll als High-Level-Skizze vor. M2-M3-Implementierung braucht ausführbare Detail-Entscheidungen, die der Spec offen ließ:

1. **Naming-Kollision**: Heute existiert `domain.Project` als "Source-Directory in `$SOURCECODE_ROOT`". M2-Spec führt `Project` als "Worktime-Kategorie" ein. Beide gleichzeitig wäre Begriffs-Chaos.
2. **TSV-Migration**: Phase-1-Spec sagte "Phase 1 startet 'from-scratch' möglich". Aber Soenne hat reale Worktime-History auf seinem Hauptlaptop — Fresh-Start würde sie verlieren.
3. **Project-Resolution beim Worktime-Start**: Heute hat `flow worktime start` keinen Project-Begriff. Mit M2 ist Project required. Woher kommt er beim `s`-Press im TUI-Panel?
4. **Konflikt-UX**: Spec sagte nur "Server-Version übernehmen / Lokal überschreiben" — kein konkretes TUI-Component-Design.
5. **DB-Migrations-Tool**, **Default-Project-Semantik**, **TSV-Adapter-Lebensdauer**: alles ungeklärt.

## Entscheidungen aus Brainstorm 2026-06-02

| Frage | Entscheidung |
| --- | --- |
| Naming-Kollision | Existing `domain.Project` → **`domain.SourceDir`** (Source-Directory-Listing für Projects-Screen + tmux-attach). Neue Worktime-Kategorie ist `domain.Project`. |
| TSV-Migration | **Inkludiert in M2** als `flow worktime migrate-from-tsv` Subcommand. Idempotent (skipped schon migrierte Rows). Erstellt auto-`Allgemein` Project für untagged Sessions. |
| Project-Resolution | **TUI-Picker** beim `s`-Press: MRU-sortierte Project-Liste, fuzzy-filter (typing), Pfeil-Navigation, Enter wählt, "+ neues Projekt anlegen"-Entry am Ende. **CLI-Smart-Default** (`flow worktime start` ohne Flag): `$PWD → SourceDir → letzter Project → "Allgemein"`. Explicit `--project=foo` Override immer. |
| Konflikt-UI Session-Edit | **TUI-Overlay** (reuse `markdown_overlay`-Component-Pattern): *"Diese Session wurde auf [device] um [zeit] geändert. [s] Server übernehmen · [l] Lokal überschreiben · [esc] Abbrechen"* |
| Konflikt-UI Active-Session | Selber Overlay-Stil: *"Session [project] läuft auf [device] seit [zeit]. [t] Übernehmen (stoppt A) · [n] Neue Session erzwingen · [esc] Abbrechen"*. Option-2-Modus erlaubt parallele aktive Sessions, also `[n]` ist legitim. |
| DB-Migrations | **goose v3** embedded, SQL-Files unter `internal/adapter/sqlite{client,server}/migrations/`. Adapter runs auto-up on first connect. |
| Default-Project | Wird **nicht** auto-erstellt. Migration aus TSV legt "Allgemein" an für Untagged-Existing-Sessions. Frischer User: erster `s`-Press zeigt Picker mit nur "+ neu anlegen". |
| TSV-Adapter Parallel | Wird **gelöscht** sobald Migration einmal lief. Keine Config-Flag-Doppelwelt. M2 startet bereits SQLite-Mode (kein Server-Sync); M3 fügt Server-Sync drauf. |
| Wiring + Smoke | **Mandatorisch letzte Task** im Plan (Lehre aus M1, siehe `feedback_plan_main_wiring_task.md`): `cmd/flow` + `cmd/flow-server` end-to-end durchführen, Multi-Device-Smoke mit zwei flow-Instanzen. |
| Scope | **Eine** Plan-Datei für M2+M3 zusammen (M3 hängt an M2-Schema). |

## Goals (M2-M3)

- **Domain-Schicht erweitert** um User, Project (worktime), Repo, RepoNote; Session bekommt UserID + ProjectID + ID-UUID; SourceDir-Rename durchgezogen.
- **Lokaler SQLite-Store** (`sqliteclient`) ersetzt heutige TSV/JSON-Adapter für Worktime-Daten. Schema migriert via embedded goose-Migrations.
- **Existing TSV-History migriert** über `flow worktime migrate-from-tsv` — idempotent, ein-Befehl.
- **flow-server hat Sessions-API** unter `/api/v1/sessions` mit Pull (since-Lamport) + Push (ETag/Version) + Active-Session (server-authoritative POST).
- **Background-Sync-Worker** im Client-Adapter pusht Local-Writes, pollt Remote alle 30s, queued offline.
- **Conflict-UX** als TUI-Overlay-Component für 409s + Active-Session-Races.
- **Worktime-TUI** kriegt Project-Picker beim Start, Project-Anzeige in der Session-Liste, Sub-Tab für Project-Verwaltung.
- **CLI bleibt funktional**: `flow worktime start/stop/...`, `flow projects list/create/rename/delete`, sind sync-aware (gehen über sqliteclient).
- **Phase-2-ready Schema**: Spalten für `shared_with`, `permissions` etc. werden in M2 **nicht** befüllt aber das Schema reserviert die Plätze (`*_shares`-Tabellen sind nicht angelegt; werden in Phase-2 hinzugefügt).

## Non-Goals (M2-M3)

- **Notes-Sync (Kompendium + RepoNote)** — Plan C / M4.
- **flow-mcp stdio-Server** — Plan D / M5.
- **WebUI-Routes für Sessions/Projects** — Plan E / M6-M7. Nur die REST-API entsteht, kein HTML.
- **Sharing / Multi-User** — Phase 2.
- **Streak-Berechnung server-side** — bleibt client-side (Existing-Logik in `internal/domain/streak.go`); server-side-Move kommt eventuell mit M4 wenn nötig.
- **CRDT** — Phase 3.
- **Telemetry / Sync-Metrics** — minimal Logging reicht für M2-M3.

## Domain-Model — Final

```go
// internal/domain/source_dir.go (UMBENANNT von project.go)
//
// SourceDir is one entry in the Projects/SourceDir screen — a directory under
// $SOURCECODE_ROOT, optionally annotated with whether a tmux session of the
// same name is already running. Has nothing to do with worktime; it's the
// "where I can `cd` to start coding" listing.
type SourceDir struct {
	Name           string
	Path           string
	HasTmuxSession bool
}

// internal/domain/user.go (NEU)
type User struct {
	ID          string    // UUID v4
	OIDCSub     string    // Authentik 'sub' Claim, unique per server
	Email       string    // optional, from token
	DisplayName string    // from name claim
	CreatedAt   time.Time
}

// internal/domain/project.go (NEU — Worktime-Kategorie, KEIN Source-Dir mehr)
//
// Project is a worktime-tracking category. A Session is logged against
// exactly one Project. Users see and pick Projects in the TUI picker on
// `s` press.
type Project struct {
	ID        string    // UUID v4
	UserID    string
	Name      string    // human-readable, e.g. "flow", "Kompendium", "Allgemein"
	Slug      string    // URL-safe lowercase, unique per (UserID); generated from Name
	CreatedAt time.Time
	// MRU tracking (used for picker sort order). UpdatedAt-on-session-start.
	LastUsedAt time.Time
	// Soft-delete: hide from picker but keep historic sessions intact.
	ArchivedAt *time.Time
}

// internal/domain/repo.go (NEU — kommt in M4 voll zur Nutzung; M2 legt schon Schema an)
type Repo struct {
	ID            string  // UUID
	UserID        string
	CanonicalKey  string  // "git:github.com/foo/bar" | "path:sha256(/abs/path)"
	DisplayName   string  // human-readable
	CreatedAt     time.Time
}

// internal/domain/repo_note.go (NEU — Schema in M2, Sync-Tooling in M4)
type RepoNote struct {
	ID        string
	RepoID    string
	UserID    string
	Content   string    // Markdown
	Version   int64     // Lamport, increments on server-side update
	UpdatedAt time.Time
}

// internal/domain/session.go (ERWEITERT — siehe heute internal/domain/session.go)
type Session struct {
	ID         string    // NEU — UUID v4 (heute implizit über (Date, Start))
	UserID     string    // NEU — required after M2
	ProjectID  string    // NEU — required after M2; Migration setzt "Allgemein"-Projekt für historische Untagged-Sessions
	Date       time.Time // bleibt
	Start      time.Time // bleibt
	Stop       time.Time // bleibt
	Elapsed    time.Duration // bleibt; abgeleitet aus Stop-Start beim Speichern
	Tag        string    // bleibt — "deep" | "meeting" | freie Strings
	Note       string    // bleibt — one-liner
	Version    int64     // NEU — Lamport per Session-Row, increments bei Server-Side-Update
	UpdatedAt  time.Time // NEU — letzte Mutation
}

// internal/domain/active_session.go (NEU — ersetzt jsonflowstate-ActiveSessionStore)
//
// Mehrere ActiveSessions pro User möglich (Option-2: eine pro Project parallel).
// Primary Key (UserID, ProjectID).
type ActiveSession struct {
	UserID          string
	ProjectID       string
	StartedAt       time.Time
	StartedOnDevice string    // hostname; informativ für Conflict-UI
	Version         int64     // Optimistic Concurrency; server increments on each mutation
}

// Bestehende DayOff, Cheatsheet, FlowState-Pause-State: in M2-M3 nicht
// touched (Pause-State bleibt lokal in jsonflowstate — per-Device-State, nie
// synced). DayOff + Cheatsheet werden in einem späteren Mini-Refactor auf
// SQLite migriert; nicht Teil von M2-M3.
```

### Repo-Identifikation (Schema-Reserviert; voll genutzt in M4)

`CanonicalKey` ist einer von:
- `git:<host>/<owner>/<repo>` — extrahiert aus `git remote get-url origin`, normalisiert (lowercase, Suffix `.git` entfernt, SSH `git@host:owner/repo` und HTTPS `https://host/owner/repo` zusammengeführt).
- `path:<sha256-hex>` — Hash des absoluten Pfads, wenn kein Git-Remote.

Pro `(UserID, CanonicalKey)` existiert max eine Row. M2 legt nur Schema + Repo-Resolver-Helper an; eigentliche Note-Operationen kommen in M4.

## DB Schema — SQLite DDL

### Client-Schema (`internal/adapter/sqliteclient/migrations/`)

`0001_initial.sql`:

```sql
-- All client data for a single OIDC user. Multi-user is server-side only
-- (each device's cache holds exactly the logged-in user's slice).

CREATE TABLE users (
    id            TEXT    PRIMARY KEY,    -- UUID
    oidc_sub      TEXT    NOT NULL UNIQUE,
    email         TEXT    NOT NULL DEFAULT '',
    display_name  TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL        -- RFC3339
);

CREATE TABLE projects (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users(id),
    name          TEXT    NOT NULL,
    slug          TEXT    NOT NULL,
    created_at    TEXT    NOT NULL,
    last_used_at  TEXT    NOT NULL DEFAULT '',
    archived_at   TEXT,                   -- NULL = aktiv
    version       INTEGER NOT NULL DEFAULT 0,
    UNIQUE(user_id, slug)
);
CREATE INDEX idx_projects_user_last_used ON projects(user_id, last_used_at DESC);

CREATE TABLE sessions (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users(id),
    project_id    TEXT    NOT NULL REFERENCES projects(id),
    date          TEXT    NOT NULL,       -- YYYY-MM-DD
    start         TEXT    NOT NULL,       -- RFC3339
    stop          TEXT    NOT NULL,
    elapsed_ns    INTEGER NOT NULL,       -- nanoseconds
    tag           TEXT    NOT NULL DEFAULT '',
    note          TEXT    NOT NULL DEFAULT '',
    version       INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT    NOT NULL
);
CREATE INDEX idx_sessions_user_date ON sessions(user_id, date);
CREATE INDEX idx_sessions_user_project ON sessions(user_id, project_id);
CREATE INDEX idx_sessions_version ON sessions(version);

CREATE TABLE active_sessions (
    user_id            TEXT    NOT NULL REFERENCES users(id),
    project_id         TEXT    NOT NULL REFERENCES projects(id),
    started_at         TEXT    NOT NULL,
    started_on_device  TEXT    NOT NULL,
    version            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, project_id)
);

CREATE TABLE repos (
    id             TEXT    PRIMARY KEY,
    user_id        TEXT    NOT NULL REFERENCES users(id),
    canonical_key  TEXT    NOT NULL,
    display_name   TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    UNIQUE(user_id, canonical_key)
);

CREATE TABLE repo_notes (
    id         TEXT    PRIMARY KEY,
    repo_id    TEXT    NOT NULL REFERENCES repos(id),
    user_id    TEXT    NOT NULL REFERENCES users(id),
    content    TEXT    NOT NULL DEFAULT '',
    version    INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL
);
CREATE INDEX idx_repo_notes_user ON repo_notes(user_id);

-- Sync state: tracks per-resource the highest server-version we've ingested
-- (Lamport-watermark). Pull asks for "rows with version > watermark".
CREATE TABLE sync_state (
    resource    TEXT    PRIMARY KEY,      -- "sessions" | "projects" | "repo_notes" | ...
    watermark   INTEGER NOT NULL DEFAULT 0
);

-- Write queue: rows the client has locally written but not yet acked by
-- server. FIFO, replayed on connect. Idempotent via row.id at server side.
CREATE TABLE write_queue (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    resource   TEXT    NOT NULL,          -- "sessions" | "projects" | ...
    row_id     TEXT    NOT NULL,
    payload    TEXT    NOT NULL,          -- JSON-encoded row
    expected_version INTEGER NOT NULL,    -- ETag-If-Match
    enqueued_at TEXT   NOT NULL,
    last_error TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_write_queue_resource ON write_queue(resource);
```

### Server-Schema (`internal/adapter/sqliteserver/migrations/`)

`0001_initial.sql`:

```sql
-- Server holds multi-user data (Phase 1: single user via allowlist, but
-- schema is multi-user-ready). Each row carries user_id.

CREATE TABLE users (
    id            TEXT    PRIMARY KEY,
    oidc_sub      TEXT    NOT NULL UNIQUE,
    email         TEXT    NOT NULL DEFAULT '',
    display_name  TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL
);

CREATE TABLE projects (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users(id),
    name          TEXT    NOT NULL,
    slug          TEXT    NOT NULL,
    created_at    TEXT    NOT NULL,
    last_used_at  TEXT    NOT NULL DEFAULT '',
    archived_at   TEXT,
    version       INTEGER NOT NULL DEFAULT 0,
    UNIQUE(user_id, slug)
);
CREATE INDEX idx_projects_user_version ON projects(user_id, version);

CREATE TABLE sessions (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users(id),
    project_id    TEXT    NOT NULL REFERENCES projects(id),
    date          TEXT    NOT NULL,
    start         TEXT    NOT NULL,
    stop          TEXT    NOT NULL,
    elapsed_ns    INTEGER NOT NULL,
    tag           TEXT    NOT NULL DEFAULT '',
    note          TEXT    NOT NULL DEFAULT '',
    version       INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT    NOT NULL
);
CREATE INDEX idx_sessions_user_version ON sessions(user_id, version);
CREATE INDEX idx_sessions_user_date ON sessions(user_id, date);

CREATE TABLE active_sessions (
    user_id            TEXT    NOT NULL REFERENCES users(id),
    project_id         TEXT    NOT NULL REFERENCES projects(id),
    started_at         TEXT    NOT NULL,
    started_on_device  TEXT    NOT NULL,
    version            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, project_id)
);

CREATE TABLE repos (
    id             TEXT    PRIMARY KEY,
    user_id        TEXT    NOT NULL REFERENCES users(id),
    canonical_key  TEXT    NOT NULL,
    display_name   TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    UNIQUE(user_id, canonical_key)
);

CREATE TABLE repo_notes (
    id         TEXT    PRIMARY KEY,
    repo_id    TEXT    NOT NULL REFERENCES repos(id),
    user_id    TEXT    NOT NULL REFERENCES users(id),
    content    TEXT    NOT NULL DEFAULT '',
    version    INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL
);

-- Global lamport counter — every Resource-mutation pulls the next value.
-- One counter total (not per-user) so cross-user sharing in Phase 2 doesn't
-- need a re-key. M2-M3 uses it only for own resources.
CREATE TABLE lamport (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    counter   INTEGER NOT NULL DEFAULT 0
);
INSERT INTO lamport(id, counter) VALUES (1, 0);
```

### Schema-Konventionen

- **Timestamps** überall RFC3339-Strings (`2026-06-02T14:30:00Z`). Native SQLite-DateTime ist fiddly; RFC3339 sorts lexikographisch korrekt.
- **UUIDs** als TEXT, generiert client-seitig (`github.com/google/uuid`, schon in indirect deps).
- **Foreign Keys** sind PRAGMA-enabled (`PRAGMA foreign_keys=ON` beim connect-Hook).
- **Versions** sind monotonic-increasing INTEGER. Client liest "letzte gesehene Server-Version", schreibt mit `expected_version` als If-Match.

## Migrations — goose

Layout:

```
internal/adapter/sqliteclient/
  migrations/
    0001_initial.sql            ← oben, das gesamte Schema
    embed.go                    ← //go:embed *.sql + goose-Provider
  store.go                      ← öffnet DB + runs goose.UpContext() + holds sql.DB
  …

internal/adapter/sqliteserver/
  migrations/
    0001_initial.sql            ← server-Schema (siehe oben)
    embed.go
  store.go
```

**Adapter-Konvention:**
- `Open(path string) (*Store, error)` öffnet die DB, sets pragmas, runs migrations, returns ready store.
- `Close()` flushes WAL + closes connection.
- `goose.SetBaseFS()` mit der embed.FS sorgt dafür dass kein external migration-directory nötig ist.

**Backwards-Compat-Policy:** Migrations sind forward-only. Kein down. Wenn ein Down nötig wäre: neue forward-Migration die's repariert. Hobby-Projekt, single-user — kein Roll-Back-Druck.

## Sync-Protokoll — final

REST + JSON. Endpoints unter `/api/v1/`. Alle authenticated mit Bearer-Token (cookie für browser fällt für M2-M3 weg, nur REST).

### Resource-Endpoints

| Resource | GET pull | PUT push | POST start (special) |
| --- | --- | --- | --- |
| `projects` | `GET /api/v1/projects?since=<lamport>&limit=200` | `PUT /api/v1/projects/<id>` + `If-Match: <version>` | — |
| `sessions` | `GET /api/v1/sessions?since=<lamport>&limit=200` | `PUT /api/v1/sessions/<id>` + `If-Match: <version>` | — |
| `active_sessions` | `GET /api/v1/active` (alle aktuell aktiven) | `PUT /api/v1/active/<project-id>` + `If-Match: <version>` für state-update (z.B. Heartbeat) | `POST /api/v1/active/<project-id>/start` + `If-Match: 0` für Neustart |
| `repos` | `GET /api/v1/repos?since=<lamport>` | `PUT /api/v1/repos/<id>` | — |
| `repo_notes` | `GET /api/v1/repo-notes?since=<lamport>` | `PUT /api/v1/repo-notes/<id>` | — |

### JSON-Shapes

```jsonc
// GET /api/v1/sessions?since=42&limit=200
{
  "items": [
    {
      "id": "01950a72-5f55-7c1c-9bba-…",
      "user_id": "01950a70-…",
      "project_id": "01950a71-…",
      "date": "2026-06-02",
      "start": "2026-06-02T09:12:00Z",
      "stop": "2026-06-02T11:42:00Z",
      "elapsed_ns": 9000000000000,
      "tag": "deep",
      "note": "M2-M3 spec",
      "version": 47,
      "updated_at": "2026-06-02T11:42:01Z"
    }
  ],
  "high_watermark": 47,    // höchste version in items; client setzt sync_state["sessions"] auf diesen Wert
  "has_more": false        // bei true: client soll erneut callen mit since=high_watermark
}

// PUT /api/v1/sessions/01950a72-...
// Body: einzelne Session-Row (gleiche Shape wie items[0])
// Headers: If-Match: 47 (current expected version)
// Response 200: { "version": 48, "updated_at": "..." }
// Response 409: { "current": { ...current server row... } }
// Response 201: bei row-not-found am Server (neue Row): { "version": 1, "updated_at": "..." }

// POST /api/v1/active/<project-id>/start
// If-Match: 0 (= "muss frei sein") oder current version (= "ich force-takeover from this version")
// Body: { "started_on_device": "macbook-soenne" }
// Response 200: { "version": 1, "started_at": "2026-06-02T14:30:00Z" } 
// Response 409: { "current": { "started_on_device": "macbook-other", "started_at": "...", "version": 5 } }
```

### Lamport-Semantik

- **Server-side** ist `lamport` eine globale Tabelle mit einem einzigen Counter. Jede Resource-Mutation (insert oder update an `sessions`/`projects`/`repo_notes`/`active_sessions`) macht `UPDATE lamport SET counter = counter + 1` und nimmt den neuen Wert als `version` für die Row.
- **Client-side** speichert `sync_state[resource]` als höchste je gesehene Server-Version pro Resource.
- **Pull-Reihenfolge**: Projects → Sessions → ActiveSessions → Repos → RepoNotes (Sessions/ActiveSessions referenzieren Projects).

### Background-Sync-Worker

In `internal/adapter/httpsync/`:

- **Pull-Tick**: alle 30s wenn die Verbindung steht. Für jeden Resource-Typ in der Reihenfolge oben: `GET .../<resource>?since=watermark&limit=200`. Falls `has_more`: erneut callen bis leer. Bei jedem Item: `INSERT … ON CONFLICT (id) DO UPDATE` ins lokale SQLite. Update Watermark.
- **Push-Drain**: bei Local-Write sofort getriggert (debounce 500ms). Liest `write_queue` FIFO. Für jeden Eintrag: `PUT/POST` mit `If-Match: expected_version`. Bei 200/201: aus Queue entfernen + lokale Row aktualisieren mit Server-Version. Bei 409: write_queue-Eintrag stehen lassen + Conflict-Channel an UI signalisieren mit current-server-row. Bei 5xx oder Netzwerk-Fehler: `last_error` setzen, exponential backoff (5s, 15s, 60s, 5min cap).
- **Conflict-Channel**: Go-Channel `chan ConflictMsg` den die TUI subscribed. ConflictMsg enthält local-row + server-row + resource-type + write-queue-seq. UI rendert Overlay; auf User-Auswahl wird write-queue-Eintrag entweder removed (akzeptiere Server) oder mit neuer `expected_version` re-enqueued (force-overwrite).
- **Identity-Bootstrap**: beim Login (M1 done) speichert oidcclient den `sub`. Beim sqliteclient-Open: lookup user-row by sub, falls nicht da: insert (id, sub, email, name). Local User-ID ist seitdem fix.

### Active-Session-Spezialfall

Active-Session ist **server-authoritative** weil Race zwischen Devices reale Doppel-Sessions erzeugen würde.

- `POST /api/v1/active/<project-id>/start` — Client behauptet "Session jetzt starten". Server prüft Row in `active_sessions` für `(user_id, project_id)`:
  - Existiert nicht → insert mit version=1, return 200.
  - Existiert + `If-Match: 0` → 409 mit current row.
  - Existiert + `If-Match: <current_version>` → update (force-takeover): startet_at=now, started_on_device=req.device, version+=1, return 200.
- `DELETE /api/v1/active/<project-id>` + `If-Match: <version>` — Stop. Server-Side atomic in einer Transaction: (a) löscht die `active_sessions`-Row, (b) inserts eine fertige `sessions`-Row aus dem `(started_at..now)`-Range (mit fresh UUID, Lamport-version aus Counter). Client sieht **beides** beim nächsten Pull (active_sessions verschwindet, neue session erscheint).

## Konflikt-UX — TUI-Overlay

Reuse existing `internal/frontend/tui/components/markdown_overlay`-Pattern (chrome-style, close on `esc`). New component:

```
internal/frontend/tui/components/conflict_overlay/
  component.go        ← bubbletea Model: title, message, choices
  styles.go           ← Tokyonight-Palette via theme.Palette
  variants.go         ← presets for session-edit + active-session-race
```

**Session-Edit-Konflikt (409 on PUT /sessions/<id>):**

```
╭─ Konflikt: Session "Spec schreiben" (2026-06-02) ────────────────╮
│                                                                  │
│  Diese Session wurde auf macbook-soenne um 14:32 geändert.       │
│                                                                  │
│  Lokal (deine Änderung):                                         │
│    Tag: deep · Note: M2-M3 spec                                  │
│                                                                  │
│  Server (neuere Version):                                        │
│    Tag: deep · Note: M2-M3 + dex-quirk-fix                       │
│                                                                  │
│  [s] Server-Version übernehmen                                   │
│  [l] Lokal überschreiben (force)                                 │
│  [esc] Abbrechen                                                 │
╰──────────────────────────────────────────────────────────────────╯
```

**Active-Session-Race (409 on POST /active/<project-id>/start):**

```
╭─ Session läuft schon ─────────────────────────────────────────╮
│                                                               │
│  Projekt "flow" wird bereits getrackt:                        │
│    Gerät: macbook-soenne                                      │
│    Seit:  09:12 (vor 2h 18m)                                  │
│                                                               │
│  [t] Übernehmen (stoppt auf macbook-soenne, startet hier)     │
│  [n] Parallele Session starten (Option-2-Modus erlaubt das)   │
│  [esc] Abbrechen                                              │
╰───────────────────────────────────────────────────────────────╯
```

**Visuelle Konvention (aus `feedback_no_icons.md`):** Keine bunten Emoji. TUI-Glyphen ▶ ✓ ● erlaubt. Buchstaben-Hints in eckigen Klammern entsprechen der `s/l/esc/t/n` Keybindings.

**CLI-Variante:** `flow worktime start` (ohne TUI) auf Conflict → printet die selbe Info und bietet Y/N/Q-Prompt. Eskaliert zur TUI bei mehrdeutigen Fällen (z.B. mehrere parallele Projects laufen) — printet "Run `flow worktime` to resolve interactively" und exitet mit non-zero.

## CLI/TUI-Integration

### TUI-Worktime-Screen Erweiterungen

- **`s`-Press** (start): vorher direkt-start, neu → Project-Picker-Overlay aufmachen.
- **Project-Picker-Overlay** (neuer Component unter `internal/frontend/tui/components/project_picker/`):
  - Input-Feld oben für fuzzy-filter (sahilm/fuzzy, schon in deps)
  - Liste darunter, sortiert MRU (last_used_at DESC, then created_at DESC), filterbar live
  - Sticky-Action "+ Neues Projekt anlegen" am Ende der Liste (nicht filterbar)
  - Enter wählt highlight, esc cancels
  - Bei "+ neu" → kleines Input-Modal für Project-Name → erstellt + selektiert sofort
- **`r`-Press** (rename) auf Session-Row: erlaubt Project + Tag + Note inline-edit, schreibt via sqliteclient + httpsync-Queue.
- **Conflict-Overlay** wird durch Channel-Subscription des httpsync-Workers getriggert. Active-Session-Channel und Session-Edit-Channel sind separate.

### TUI-Projects-Screen (existing, refactored)

Heute zeigt Projects-Screen `SourceDir`-Listing (Verzeichnisse in `$SOURCECODE_ROOT`). Wird **erweitert** um einen zweiten Sub-Tab "Worktime-Projects":

```
╭─ Projekte ──────────────────────────────────────────────────────────╮
│  Quellverzeichnisse · Worktime-Projekte                             │
│                                                                     │
│  (Worktime-Projects-Sub-Tab)                                        │
│  ▶ flow             · zuletzt 2026-06-02 14:32 · 142 Sessions       │
│    Kompendium       · zuletzt 2026-05-30      ·  87 Sessions        │
│    Allgemein        · zuletzt 2026-05-22      ·  12 Sessions        │
│                                                                     │
│  [n] neu · [r] rename · [a] archive · [enter] sessions filtern      │
╰─────────────────────────────────────────────────────────────────────╯
```

Sub-Tab "Quellverzeichnisse" zeigt weiter das existing SourceDir-Listing unverändert.

### CLI-Subcommands neu / geändert

| Befehl | Wirkung |
| --- | --- |
| `flow worktime start [--project=foo] [--tag=deep] [--note=…]` | Smart-Default-Project-Resolution. Schreibt `active_sessions` lokal + queued POST /api/v1/active/.../start. |
| `flow worktime stop [--project=foo]` | DELETE /api/v1/active/<project-id>, erzeugt Session-Row aus dem (started_at..now)-Range. |
| `flow worktime status` | Erweitert: zeigt alle laufenden Sessions (Option-2), tmux-segment-format. |
| `flow worktime migrate-from-tsv [--tsv=path] [--default-project=Allgemein]` (**NEU**) | One-shot Migration: liest TSV-Log, erstellt für jede Row eine Session in SQLite, legt "Allgemein"-Project automatisch an wenn nötig, marked TSV als `.migrated`. Idempotent. |
| `flow projects list / create <name> / rename <id> <new-name> / archive <id>` (**NEU**) | CLI-Pendant zum Projects-Screen. |
| `flow sync status` (**NEU**) | Zeigt write_queue-Länge, letzten Pull-Timestamp, etwaige Conflicts pending. |
| `flow sync force-pull` (**NEU**) | Triggert ad-hoc Pull (für Debug). |

## TSV-Migration — Detail

`flow worktime migrate-from-tsv`:

1. **Inputs:** TSV-Pfad (default `~/.tmux/worktime.log`), Default-Project-Name (default "Allgemein"), force-Flag.
2. **User-Resolve:** wartet auf `flow login` zuvor; nimmt local-User aus sqliteclient. Wenn kein User: error "run `flow login` first".
3. **Default-Project-Ensure:** sucht Project mit slug=default-project-slug. Falls keiner: insert mit `created_at=now()`, `version=0` (wird vom Sync auf Server-Version gehoben).
4. **TSV-Parse:** liest Zeile für Zeile. Pro Row:
   - Generiere UUID (deterministic: hash(Date + Start + Tag + Note) → UUIDv5 mit fester Namespace-UUID. So ist Re-Migration idempotent: gleiche Eingaberow → gleiche UUID → `INSERT OR IGNORE` lässt sie weg).
   - Lookup Project: wenn Tag matched einen existing Project-Slug → nimm den, sonst → Default-Project.
   - Insert in `sessions` mit `version=0`, `updated_at=now()`.
5. **Mark Migrated:** rename `worktime.log` → `worktime.log.migrated-<RFC3339>`. Diese File wird vom `tsvsessions`-Adapter ignoriert (Adapter wird sowieso in M2 gelöscht).
6. **Report:** `✓ 247 Sessions migriert in Project "Allgemein" (3 Tag-Mappings, 244 ungetaggt). TSV archiviert nach worktime.log.migrated-2026-06-02T15:42:00Z.`

Migration-Command zählt zu **M2** (vor M3-Server-Sync), damit du auf einem fresh-installed-Account direkt full-history hast wenn dann der Sync startet.

## Modul-Layout — Final

```
flow/
  cmd/
    flow/                ← TUI + CLI; sync-aware nach M3
    flow-server/         ← bestehend (M1), wird in M3 erweitert um Sessions-API
    flow-mcp/            ← Plan D / M5
  internal/
    domain/
      source_dir.go      ← UMBENANNT von project.go
      project.go         ← NEU (Worktime-Kategorie)
      user.go            ← NEU
      session.go         ← ERWEITERT
      active_session.go  ← NEU
      repo.go            ← NEU
      repo_note.go       ← NEU
      dayoff.go          ← bleibt
      cheatsheet.go      ← bleibt
      streak.go          ← bleibt
    ports/
      source_dirs.go     ← UMBENANNT von projects.go (interface ProjectScanner → SourceDirScanner)
      projects.go        ← NEU (interface ProjectStore — worktime-projects)
      sessions.go        ← ERWEITERT (interface bleibt name, signatur ändert sich um UserID/ProjectID)
      active_sessions.go ← NEU
      users.go           ← NEU
      repos.go           ← NEU
      repo_notes.go      ← NEU
      sync.go            ← NEU (interface ConflictListener, SyncStatus)
    usecase/
      sessions.go        ← ERWEITERT
      projects.go        ← NEU
      migrate_tsv.go     ← NEU (use case für `migrate-from-tsv`)
      sync.go            ← NEU
    adapter/
      tsvsessions/       ← WIRD GELÖSCHT in M2h-Tail (nachdem migrate-from-tsv lief und sqliteclient die SessionStore-Implementierung übernommen hat). Falls User Plan B mit gefüllter TSV und ohne Migration-Run installiert: `flow worktime start` errort mit "TSV detected at <path>, run `flow worktime migrate-from-tsv` first" (Wiring-Guard im sessions-usecase).
      jsonflowstate/     ← WIRD AUF SQLITE REDUZIERT (pause-state bleibt JSON-lokal; active-state migriert nach sqliteclient)
      fsprojects/        ← bleibt; rename interner Funktionen (Project→SourceDir)
      sqliteclient/      ← NEU
        store.go
        migrations/0001_initial.sql + embed.go
        sessions.go      ← implements ports.SessionStore
        projects.go      ← implements ports.ProjectStore
        active_sessions.go
        users.go
        repos.go
        repo_notes.go
        sync_state.go
        write_queue.go
      sqliteserver/      ← NEU
        store.go
        migrations/0001_initial.sql + embed.go
        sessions.go
        projects.go
        active_sessions.go
        users.go
        lamport.go       ← UPDATE lamport counter atomic
        repos.go
        repo_notes.go
      httpsync/          ← NEU
        worker.go        ← Pull-Loop + Push-Drain
        client.go        ← REST-Calls
        conflict.go      ← ConflictMsg / Channel
        queue.go         ← write_queue management
      httpserver/        ← M1 da; M3 fügt Handler hinzu:
        sessions_handlers.go     ← GET/PUT /api/v1/sessions
        projects_handlers.go     ← GET/PUT /api/v1/projects
        active_sessions_handlers.go ← GET, POST .../start, DELETE
        users_handlers.go        ← ensure-on-first-call
        oidcclient/      ← bleibt
        keyringadapter/  ← bleibt
    frontend/
      cli/
        worktime/        ← erweitert
        projects/        ← NEU CLI commands für Worktime-Projects
        sync/            ← NEU CLI (status, force-pull)
        migrate/         ← NEU `migrate-from-tsv`
      tui/
        components/
          project_picker/  ← NEU
          conflict_overlay/← NEU
        screen/
          worktime/      ← erweitert um picker-trigger + conflict-listener
          projects/      ← sub-tabs SourceDirs + WorktimeProjects
```

## Testing-Strategie

- **Unit:** sqliteclient (CRUD-Roundtrips + sync_state/write_queue handling), sqliteserver (lamport-counter atomicity, version-check), httpsync (mock-server Pull+Push+Conflict-Channels), conflict_overlay (snapshot tests), migrate-tsv (idempotency, mapping logic).
- **Integration:** zwei flow-Instanzen gegen den gleichen flow-server starten, Session auf A starten, auf B pollen → erscheint; Session auf A edit, auf B edit parallel → einer kriegt 409 → Overlay rendered. Sequenzielles E2E ohne echte Browser/UI-Treiber, sondern direkt durch die Use-Case-Aufrufe + Adapter-Verifikation.
- **Migration-Tests:** ein 100-Row-TSV-Fixture, migrate, prüfe DB-Inhalt; re-migrate → keine duplikate; partial-tsv-corruption → klare Fehler.
- **Coverage-Erwartung:** ≥ 85% für neue Pakete. Bestehende Pakete dürfen nicht sinken.

## Rollout — Sub-Milestones

**Plan B** (eine Datei) deckt M2+M3 in einer Sequenz mit ~30-40 Tasks:

| Sub-Milestone | Was | LOC-Schätzung |
| --- | --- | --- |
| **M2a** | Rename `domain.Project` → `SourceDir` quer durch Codebase | 200 (touch many) |
| **M2b** | Neue Domain-Entities (User, Project, Repo, RepoNote, ActiveSession) + Session-Erweiterung | 250 |
| **M2c** | Ports + Fakes für neue Stores | 200 |
| **M2d** | sqliteclient + Migrations + goose-Embed | 700 |
| **M2e** | jsonflowstate-Reduction (active-state → sqliteclient) + tsvsessions-Read-Path beibehalten für Migration | 100 |
| **M2f** | Use Cases (sessions/projects use cases auf neue Ports) + CLI-Refactor | 400 |
| **M2g** | TUI-Project-Picker-Component + TUI-Projects-Sub-Tab-Refactor | 500 |
| **M2h** | `flow worktime migrate-from-tsv` Use Case + CLI + Tests | 250 |
| **M3a** | sqliteserver + Migrations + lamport-counter | 500 |
| **M3b** | httpserver Handlers (sessions, projects, users, active_sessions) | 500 |
| **M3c** | httpsync Worker + Conflict-Channel | 600 |
| **M3d** | conflict_overlay-Component (TUI) | 300 |
| **M3e** | Wiring + multi-device E2E-Smoke (mandatorisch) | 100 + smoke-script |

**Plan-B-Complete-Kriterium:**
- Soenne kann auf Notebook A `flow worktime migrate-from-tsv` laufen, dann `flow worktime start` (picker zeigt "Allgemein" + history), session stoppen.
- Auf Notebook B (frische SQLite, gleicher User via `flow login`): `flow worktime today` zeigt die Session von A nach max 30s Sync-Wartezeit.
- Konflikt-Szenarien (parallel-edit, start-on-A-while-running-on-B) rendern Overlay-Component korrekt.
- `make ci` grün, Coverage-Gate eingehalten.

## Risiken & Offene Punkte

1. **goose-Migration auf existing sqliteclient.db**: was passiert wenn ein User die DB von M2 hat und Plan C/D/E weitere Migrations einführt? goose handhabt forward-only sauber, aber wir müssen testen dass `0002_*.sql`-Migrations sauber on top von 0001 laufen. → Test-Helper.
2. **Two-laptop-clock-skew**: Lamport ist server-only, also kein Problem für version-counter. ABER `updated_at` ist local-time geschrieben. Wenn Notebooks-Uhren ~10min auseinander sind und beide haben offline-writes, könnte UI "in der Zukunft erstellt" zeigen. Akzeptiere für M2-M3, dokumentiere.
3. **write_queue-Wachstum offline**: wenn ein Device 2 Wochen offline ist, könnten 100+ Items queued sein. Push-Drain sollte das schaffen, aber Reihenfolge-Garantien? FIFO ist ausreichend.
4. **TUI-Picker-Performance** bei vielen Projekten: für >500 Projekte könnte fuzzy-filter spürbar werden. Soenne wird realistisch <50 Projekte haben — kein Optimierungsbedarf jetzt.
5. **Migration-Tag-Mapping**: TSV hat freie Tags; Projects haben Slugs. Migration kann Tag→Project-Mapping anbieten (mit `--map=deep=flow,meeting=Allgemein`), aber YAGNI bis Soenne es braucht. Default = alle in "Allgemein".
6. **Domain.Project-Rename-Konflikte im Worktree**: M1-Branch hat `domain.Project` noch unter altem Namen. Wenn Plan B parallel zu M1-Review läuft → Merge-Konflikte. Mitigation: Plan B startet **erst nach M1-PR-Merge**, oder rebased auf-the-fly.

## Phase-2-Hooks (für später-erinnerung, nicht-Scope-jetzt)

- `sharing` Tabelle mit `(resource, row_id, shared_with_user_id, perm)` — wird in Phase-2 hinzugefügt.
- `audit_log` Tabelle für Sharing-Aktionen — Phase-2.
- WebUI-Routes für Projects + Sessions — Plan E.
- MCP-Tools für `flow_get_session_status` etc. — Plan D.
- CRDT-Migration für note-body falls Sharing-Concurrent-Edits brennt — Phase 3.

## Memory-References

- `project_flow_client_server_phase1_spec.md` — Parent-Spec
- `feedback_plan_main_wiring_task.md` — wiring-task-mandate (befolgt)
- `reference_soenne_worktime_workflow.md` — Soenne's tmux+s-press workflow (Picker-Spec basiert darauf)
- `feedback_no_monoliths.md` — fokussierte kleine Files pro Verantwortung (Layout befolgt)
- `feedback_no_icons.md` — keine Emoji in TUI (Overlay-Spec befolgt)
- `feedback_dont_descope_hobby_projects.md` — voller Scope, kein descoping
- `feedback_navigation_discoverability_over_minimalism.md` — Sub-Tab-Strip für Projects-Screen vollständig zeigen
