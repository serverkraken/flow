# flow Rebuild · M2a — Document-Spine · Design

**Datum:** 2026-06-15
**Status:** Approved — Brainstorm abgeschlossen, User-Freigabe erteilt
**Scope:** Erster Slice der M2-Kompendium-Vertikale. Ein server-autoritativer **Document-Spine**: Document-Domain + Postgres-Store + REST-CRUD + apiclient + WebUI-`/docs` (volles CRUD) + TUI-`flow docs` (Liste/Ansicht + Anlegen/Bearbeiten via `$EDITOR`), **live-synced** via SSE, inkl. Markdown-Rendering. Architektur-Fundament siehe `2026-06-13-flow-rebuild-design.md` (Document/DocLink/Search-Modell); Worktime-Spine-Präzedenz siehe `2026-06-14-flow-rebuild-m1a-worktime-design.md`.
**Branch:** Code auf dem langlebigen Orphan-Branch `rebuild` (kein main-merge pro Slice); Planungs-Docs auf `main` (M0/M1*-Präzedenz).

## Warum dieser Schnitt (Slicing)

M2 (Kompendium-Vertikale) ist groß und wird wie M1 in Sub-Slices zerlegt:
- **M2a (dieses Dokument): Document-Spine** — CRUD + Pfad/Slug + Markdown + Live-Sync.
- M2b — Wikilinks + Backlinks (`[[wikilink]]`-Extraktion → DocLink + Backlink-Panel).
- M2c — Tags + Filter (type/project/tag).
- M2d — Suche (PG-FTS `german`+`simple` + `pg_trgm`).
- M2e — pgvector-Semantik (Add-on, ggf. nach M3).

M2a etabliert die Document-Entität end-to-end über alle Flächen — analog dazu, wie M1a den Worktime-Spine etablierte. Der Rebuild hat **noch keinen** Document-Code (Migrationen bis 0005); das v1-Kompendium auf `main` ist Filesystem+Git-basiert (fsstore/gitsnapshot/nvimeditor) → **Neubau** auf Postgres, Carry-over nur der reinen Domain-Ideen (Frontmatter/`Extra`, Pfad-Slugs; `ExtractLinks` erst M2b).

## Done-Gate (Akzeptanztest)

> Notiz in der WebUI (`/docs`) anlegen → sie erscheint **live** in `flow docs` (TUI). Im TUI per `$EDITOR` den Body editieren + speichern → die Änderung erscheint **live** in der WebUI-Ansicht. Markdown wird in der WebUI als HTML gerendert. `make ci` grün inkl. Coverage-Gate ≥80%.

## Kern-Entscheidungen (Brainstorm 2026-06-15)

| Frage | Entscheidung |
|---|---|
| Authoring-Flächen | **WebUI = volles CRUD** (Markdown-Textarea); **TUI = Liste/Ansicht + Anlegen/Bearbeiten via `$EDITOR`** (Shell-out, Tempfile). Beide live-synced. |
| Document-Typen | **Alle vier** (`daily`/`project`/`free`/`agent`) als Enum/Spaltenwerte. Das generische CRUD akzeptiert **jeden** der vier Typen (auch `agent`). Anlege-Flüsse: `free`/`project` user-authored per Slug, `daily` datums-gekeyed; `agent` hat **keinen** dedizierten Komfort-Flow im Spine (normal von der MCP in M3 geschrieben), ist aber über das generische Formular/POST anlegbar. |
| Pfad/Slug | **User gibt hierarchischen Slug** an (`docs/architecture`); `daily/path` = `daily/<date>` auto; **unique pro owner(+project bei project-Docs)**. |
| TUI-Fläche | **Eigenes `flow docs`-Model** (`internal/tui/docs.go`), kein Overlay im Worktime-Model (Kompendium = eigener Screen, Worktime-Model schon groß). |
| Schema-Breite | **Volle Document-Spalten in Migration 0006** (tags/role/date/extra nullable), obwohl tags/role/search erst spätere Slices nutzen — vermeidet ALTER-Churn. |
| Markdown | **goldmark** neu ins go.mod; WebUI Body→HTML mit `bluemonday`-Sanitizer (User-Content); TUI schlicht (Politur/Glamour später). |
| Live-Sync | **SSE** `document.created|updated|deleted` auf dem bestehenden Bus; WebUI + TUI abonnieren. |

## Datenmodell — Migration 0006

Neue Tabelle `documents` (inkrementell, embedded goose wie 0001–0005):

```sql
-- +goose Up
CREATE TABLE documents (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    project_id  TEXT REFERENCES projects(id),     -- NULL für free/daily/agent ohne Projekt
    type        TEXT NOT NULL,                     -- daily | project | free | agent
    path        TEXT NOT NULL,                     -- menschenlesbarer Slug
    title       TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL DEFAULT '',
    tags        TEXT[] NOT NULL DEFAULT '{}',       -- M2c
    doc_date    DATE,                               -- nur daily
    role        TEXT,                               -- nur brief (M3)
    extra       JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);
-- Slug-Eindeutigkeit pro Owner (+ Projekt). coalesce, damit NULL-Projekt als '' zählt.
CREATE UNIQUE INDEX documents_owner_project_path
    ON documents (owner_id, coalesce(project_id, ''), path);

-- +goose Down
DROP TABLE documents;
```

**Invarianten** (im Domain/Usecase erzwungen, nicht per CHECK — Einfachheit, wie 0005):
- `type` ∈ {daily, project, free, agent}.
- `project`-Docs: `project_id` gesetzt; andere Typen: optional.
- `daily`-Docs: `doc_date` gesetzt, `path` = `daily/<YYYY-MM-DD>`, höchstens eine pro (owner, date).
- `free`/`project`-Docs: `path` ist ein nicht-leerer slug-shaper String (`[a-z0-9]` + `/` + `-`), user-vergeben.

Domain (`internal/domain/`):
- `document.go` (neu): `Document`-Struct + `DocumentType`-Konstanten + `Validate()` (Typ, projectID-Regel, daily-Pfad, Slug-Form) + `DailyPath(date) string` + `slugOK(string) bool`.

## Domain & Usecase

**Neu — `internal/usecase/`** (je eigene Datei, „keine Monolithen"):
- `create_document.go` — `CreateDocument{Docs ports.DocumentStore; IDs ports.IDGen; Clock ports.Clock}`: validiert, vergibt ID + Timestamps, setzt für `daily` den Pfad aus dem Datum, persistiert; Slug-Kollision → `ports.ErrDocumentExists`.
- `update_document.go` — `UpdateDocument`: lädt owner-scoped (404 wenn fremd/fehlt), aktualisiert Felder + `updatedAt`.
- `delete_document.go` — `DeleteDocument`: owner-scoped Hard-Delete.
- `get_document.go` / `list_documents.go` — `GetDocument` (by id), `ListDocuments` (alle des Owners, newest-first; Filter type/project optional als Parameter, in M2a ungefiltert nutzbar).

**Port-Erweiterung** (`internal/ports/ports.go`): `DocumentStore{Create, Get, List, Update, Delete}` + `ErrDocumentNotFound`/`ErrDocumentExists`. Neuer `ports.Editor{Edit(ctx, initial []byte) ([]byte, error)}` (TUI-Body-Editing).

**pgstore** (`internal/adapter/pgstore/documents.go`): CRUD gegen `documents`; `scanDocument`; Unique-Verletzung (pgconn 23505) → `ErrDocumentExists`.

## HTTP-Routen

| Methode | Pfad | Auth | Zweck |
|---|---|---|---|
| `POST` | `/api/v1/documents` | `auth` | Anlegen `{type, projectId?, path?, title, body, date?}` |
| `GET` | `/api/v1/documents` | `auth` | Liste (owner-scoped; optional `?type=&project=`) |
| `GET` | `/api/v1/documents/{id}` | `auth` | Einzeln |
| `PUT` | `/api/v1/documents/{id}` | `auth` | Aktualisieren `{title, body, tags?}` |
| `DELETE` | `/api/v1/documents/{id}` | `auth` | Löschen |

Antworten owner-scoped; 400 (ungültiger type / fehlende projectID / Slug-Form), 404 (fremd/fehlt), 409 (Slug-Kollision). Nach erfolgreichem Create/Update/Delete: SSE-Event `document.created|updated|deleted` mit `{id}` auf dem Bus (`s.Bus.Publish`).

## apiclient

`internal/adapter/apiclient/documents.go`: `CreateDocument`, `ListDocuments`, `GetDocument`, `UpdateDocument`, `DeleteDocument` (DTO = `domain.Document`, DRY wie bei Project). `Events()` liefert bereits `ClientEvent` inkl. der neuen `document.*`-Typen (kein Client-Change nötig).

## WebUI `/docs`

- `internal/adapter/webui/docs.templ`: `DocsPage` (Liste: Typ-Badge + Pfad + Titel, Link je Doc) + `DocView` (Body als **Markdown→HTML** via goldmark+bluemonday) + `DocForm` (Anlegen/Bearbeiten: type-Select, projectID, path, title, body-Textarea) + Fragmente für HTMX-Refresh.
- Handler `internal/adapter/httpserver/webui_docs.go`: `GET /docs` (Liste), `GET /docs/{id}` (Ansicht), `GET /docs/new` + `POST /docs` (Anlegen), `GET /docs/{id}/edit` + `POST /docs/{id}` (Bearbeiten), `POST /docs/{id}/delete` — alle `webAuth`. SSE-getriggertes Listen-Refresh (HTMX-Fragment) für Live-Sync.
- Nav-Link „docs" konsistent zu worktime/dayoffs/stats/export (Symmetrie auf allen Seiten).
- Markdown-Render-Helper `internal/adapter/webui/markdown.go` (goldmark + bluemonday), testbar isoliert.

## TUI `flow docs`

- `internal/tui/docs.go`: eigenes Bubbletea-`Model` (`NewDocs(client, editor, user)`). Screens: **Liste** (j/k-Navigation, Typ + Pfad + Titel) → **Ansicht** (Body gerendert, schlicht) → **Anlegen** (`n`: Slug/Typ/Titel abfragen, dann Body via `$EDITOR`) / **Bearbeiten** (`e`: Body via `$EDITOR`) → **Löschen** (`d` + Confirm). Live-synced: abonniert `client.Events()`, mappt `document.*` → Reload.
- `cmd/flow/docs.go`: Cobra-Verb `flow docs` (Muster `cmd/flow/worktime.go`): baut client + Editor-Adapter, startet das Model, Logs in Datei (slog darf die TUI nicht zerschießen).
- Editor-Adapter `internal/adapter/editor/editor.go` (implementiert `ports.Editor`): schreibt `initial` in ein Tempfile (`*.md`), startet `$EDITOR` (Fallback `vi`) via `exec.Command` mit geerbtem TTY, liest die Datei zurück, räumt auf. (Carry-over-Idee aus v1 `nvimeditor` / R2b-Tempfile-Flow.)

## Markdown-Rendering

- Neu im `go.mod`: `github.com/yuin/goldmark` (+ `github.com/microcosm-cc/bluemonday` für HTML-Sanitizing).
- `webui/markdown.go`: `RenderMarkdown(md string) template.HTML` — goldmark→HTML, durch bluemonday-UGC-Policy gefiltert (User-Content, XSS-Schutz). Isoliert getestet.
- TUI: schlichte Anzeige des Bodys (raw mit Basis-Styling oder ein leichter goldmark-Text-Renderer) — Glamour/Politur in einem späteren Slice.

## Error-Handling

- Ungültiger `type` / fehlende `projectId` bei `project` / leerer-oder-ungültiger Slug → 400.
- Slug-Kollision (owner+project+path) → 409.
- Fremde/fehlende ID → 404.
- `$EDITOR` nicht gesetzt → Fallback `vi`; Editor-Exit ≠ 0 oder leerer Body → Abbruch ohne Speichern, Statuszeile.
- SSE-Event für nicht-sichtbares Doc → Liste reloadet idempotent.

## Testing

- **Domain:** `Document.Validate` (Typen, projectID-Regel, Slug-Form, daily-Pfad), `DailyPath`, `slugOK`.
- **Usecase:** Create/Update/Delete/Get/List (owner-scoping, daily-Pfad-Ableitung, Kollision→ErrDocumentExists), mit Fakes.
- **pgstore:** Round-trip CRUD, Unique-Kollision→ErrDocumentExists, owner-Isolation (testcontainers).
- **httpserver:** CRUD je Route (201/200/204), 400/404/409, SSE-Event-Publish (Bus-Fake), owner-Isolation.
- **apiclient:** je Methode (httptest).
- **webui:** `RenderMarkdown` (Sanitizing: `<script>` raus), `/docs`-Handler (200 + Liste/Ansicht-Marker), Nav-Symmetrie.
- **tui:** `docs.go`-Model (Liste-Load, Ansicht, `document.*`-Event→Reload, `$EDITOR`-Flow mit Fake-`ports.Editor`).
- **Editor-Adapter:** Tempfile-Roundtrip mit einem Fake-`$EDITOR`-Skript (z.B. `EDITOR=cat`/ein kleines Schreib-Skript).
- **Done-Gate manuell:** wie oben (WebUI-Create → TUI live; TUI-`$EDITOR`-Edit → WebUI live).

## Wiring-Verification (Pflicht-Abschlusstask)

Letzter Plan-Task ([[feedback_plan_main_wiring_task]]): Composition-Root (`cmd/flow-server/main.go`) verdrahtet `DocumentStore` + die Usecases + Handler; `cmd/flow` verdrahtet `flow docs` + Editor-Adapter; curl-Smoke trifft **jede** neue Route (POST/GET/GET-id/PUT/DELETE `/api/v1/documents`) inkl. SSE-Event-Beobachtung; `make ci` grün inkl. Coverage-Gate ≥80% (neue Handler/Render brauchen Happy-Path-Tests).

## Scope / Non-Goals (spätere Slices)

- **Wikilinks/Backlinks** → M2b (Domain hat `ExtractLinks`-Carry-over, hier ungenutzt).
- **Tag-Filter** → M2c (Spalte existiert, in M2a nur durchgereicht/leer).
- **Suche (FTS+trgm)** → M2d.
- **pgvector-Semantik** → M2e.
- **Brief-Rolle / CLAUDE.md-Sync** → M3 (Spalte `role` existiert, hier ungenutzt).
- **Sharing** → M4 (alles owner-scoped in M2a).
- **TUI-Glamour-Politur**, Mobile-First-WebUI-Feinschliff → später.
