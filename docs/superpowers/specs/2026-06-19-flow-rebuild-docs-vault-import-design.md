# flow rebuild — `flow docs import` (typen-treuer Vault-Import) — Design Spec

**Date:** 2026-06-19
**Branch:** `rebuild`
**Status:** Approved (design)

## Goal

Bestandsdokumente aus einem lokalen Markdown-Vault (`~/notes`) typen-treu in flow
übernehmen: ein neues `flow docs import <dir>` Kommando, das daily/project/free
Dokumente mit ihrem **originalen Pfad, Datum und Projekt** anlegt — idempotent
und re-runnable.

## Context / Findings

Der Vault ist bereits **flow-geformt** (sieht aus wie ein früherer flow/Kompendium-Export):

- 25 `*.md`-Dateien; **keine** `[[wikilinks]]` (0 Dateien) → Link-Auflösung ist kein Thema.
- Jede Datei hat YAML-Frontmatter mit flow's Feldern:
  - `id:` = Relativpfad ohne `.md` (z.B. `daily/2026-04-28`, `notes/Onboarding`,
    `projects/gitlab.com/.../sql-credentials/_project`)
  - `type:` ∈ `daily | free | project` (= `domain.DocumentType`)
  - `date:` (bei daily, z.B. `"2026-04-28"`)
  - `project:` (bei project, als gitlab-Pfad, z.B. `gitlab.com/dataalliance/.../sql-credentials`)
- **Titel** = erste `# H1` nach dem Frontmatter (kein `title:`-Key im Frontmatter).
- Struktur: `daily/YYYY-MM-DD.md`, `notes/*.md`, `projects/gitlab.com/<pfad>/{_project.md, YYYY-MM-DD.md}`.
  Eine kuriose `gitlab.com>/`-Ordnervariante (Shell-Artefakt) existiert — pfad-treu unkritisch.

Relevante Code-Fakten (rebuild):

- `usecase.CreateDocument` **überschreibt** bei `type=daily`: `Date = now`,
  `Path = DailyPath(now)` (`internal/usecase/create_document.go:35-38`). Für historische
  Dailies also unbrauchbar → der Import braucht einen eigenen Pfad.
- `DailyPath(d) = "daily/" + d.Format("2006-01-02")` → der Vault-`id` `daily/2026-04-28`
  ist bereits flow-kanonisch (Pfad bleibt 1:1).
- `domain.ParseFrontmatter(body)` zieht **nur Tags** (yaml.v3) + `bodyStart`. Tags werden
  also serverseitig aus dem Body geparst — der Import schickt den Body **verbatim**.
- `domain.ResolveWikilink` matcht rein über `Path` — irrelevant hier (keine Wikilinks).
- `ports.ProjectStore`: `Create / List / Get / SetRate` (kein `GetByName` → List+Match).
  `domain.Project{ID, OwnerID, Name, Slug, Color, Glyph, Rate, Status, …}`.
- `DocumentType` ∈ `daily | project | free | agent`. `Document.Validate`: `project` braucht
  `ProjectID`, `daily` braucht `Date`.
- `gopkg.in/yaml.v3` ist bereits Dependency.

## Decisions (vom User bestätigt)

1. **Typen-treu** (daily/project/free) statt „alles als free".
2. **Architektur A**: dedizierter Import-Endpoint + clientseitiger CLI-Walker (statt
   `CreateDocument` aufbohren oder serverseitiger Bulk-Archiv-Import).
3. **Projekt-Mapping**: **find-or-create** — vorhandenes flow-Projekt matchen, sonst anlegen.

## Architecture

Server persistiert Dokumente verbatim über eine neue Usecase + Route; die CLI walkt
den Vault, parst Frontmatter, löst Projekte clientseitig find-or-create auf und ruft den
Import-Endpoint je Datei.

```
flow docs import <dir>
  └─ WalkDir *.md
       ├─ parse frontmatter (id/type/date/project) + H1 title + verbatim body
       ├─ resolve project (find-or-create, cached)         ─┐ reuse ListProjects/CreateProject
       └─ POST /api/v1/documents/import {type,path,title,    │
              body,date?,projectId?}                         │
                  └─ usecase.ImportDocument (verbatim)  ──────┘
```

### Komponenten / Einheiten

1. **`usecase.ImportDocument`** (`internal/usecase/import_document.go`)
   - Input: `ImportDocumentInput{Type domain.DocumentType, Path, Title, Body string,
     Date *time.Time, ProjectID *string}`.
   - Verhalten: stempelt `ID` (`IDs.NewID()`), `OwnerID`, `CreatedAt/UpdatedAt = Clock.Now()`;
     übernimmt `Type/Path/Title/Body/Date/ProjectID` **verbatim** (kein DailyPath/now-Override);
     `StripHighlightSentinels` auf Title/Body (defensiv, wie CreateDocument);
     `Tags, bodyStart = ParseFrontmatter(Body)`; `d.Validate()`; `Docs.Create`;
     `Docs.ReplaceLinks(id, owner, WikilinkTargets(body[bodyStart:]))`; `Notifier.DocumentChanged()`.
   - Idempotenz: kein eigener Existenz-Check — verlässt sich auf den Unique-Path-Constraint
     des Stores; der bestehende Duplicate-Path-Fehler (vgl. `httpserver` `DuplicatePath`-Test)
     wird durchgereicht, damit der Caller skippen kann.
   - Abhängigkeiten: `ports.DocumentStore`, `ports.IDGen`, `ports.Clock`, `ports.DocChangeNotifier`
     (gleiche wie `CreateDocument`).

2. **HTTP-Handler** (`internal/adapter/httpserver/documents.go`)
   - `POST /api/v1/documents/import`, **dieselbe Auth/Owner-Ableitung wie `POST /documents`**
     (Owner aus Token; der Plan verifiziert das exakte Auth-Wrapper-Symbol am Bestand).
     Decodiert `ImportDocumentInput` (JSON: `type, path, title, body, date?, projectId?`),
     ruft die Usecase, antwortet `201` + Document, `409` bei Duplicate-Path, `400` bei
     Validierungsfehler. Route-Registrierung analog zur bestehenden `POST /documents`.

3. **apiclient** (`internal/adapter/apiclient/documents.go`)
   - `ImportDocumentInput` (mirror) + `func (c *Client) ImportDocument(ctx, in) (domain.Document, error)`.
   - 409 → typisierter Fehler (z.B. `ErrConflict` / `errors.Is`-fähig), damit die CLI „skip existing"
     von echten Fehlern unterscheidet.

4. **CLI `flow docs import`** (`cmd/flow/docs_import.go`)
   - Cobra-Subcommand unter dem bestehenden `docs`-Command (`flow docs` ohne Args bleibt TUI;
     `flow docs import <dir>` läuft den Import). Args: genau ein `<dir>`.
   - Flags: `--dry-run` (parst/auflöst/druckt Plan, schreibt nichts), `--update`
     (existierende per Pfad via `UpdateDocument` überschreiben statt skippen).
   - Ablauf:
     1. `ListDocuments` einmal → Set vorhandener `Path` (+ `Path→ID`-Map für `--update`).
     2. `filepath.WalkDir(dir)` über `*.md`.
     3. Pro Datei: Frontmatter parsen (§5), Felder ableiten (s.u.), Projekt auflösen (§ unten),
        Body verbatim lesen.
     4. Existiert Pfad schon und kein `--update` → **skip** (zählen). Sonst `ImportDocument`
        (bzw. `UpdateDocument` bei `--update`).
   - Feld-Ableitung pro Datei:
     - `Path` = Frontmatter `id`; Fallback: Relativpfad-ohne-`.md`.
     - `Type` = Frontmatter `type`; Fallback `free`; muss ∈ {daily,project,free,agent} sein.
     - `Date` = Frontmatter `date` (`2006-01-02`); Fallback: Dateiname `YYYY-MM-DD` wenn `type=daily`;
       sonst `nil`.
     - `Title` = erste `# H1`-Zeile nach dem Frontmatter (führendes `# ` entfernt);
       Fallback: Dateiname ohne `.md`.
     - `ProjectID` = aufgelöst aus Frontmatter `project` (s.u.); `nil` wenn kein `project`.
     - `Body` = vollständiger Dateiinhalt **verbatim** (inkl. Frontmatter).

5. **Frontmatter-Reader** (klein, in `cmd/flow`)
   - `vaultFrontmatter{ID, Type, Date, Project string}` + Parser: `---\n … \n---`-Block finden
     (gleiche Fence-Logik wie `domain.ParseFrontmatter`), `yaml.Unmarshal`. Import-spezifisch im
     `cmd/flow`-Paket, kein Aufblähen von `domain`.

### Projekt-Mapping (find-or-create, CLI-seitig)

- Lazy: beim ersten `project:` einmal `ListProjects` → Index nach `Slug` und `Name`.
- Für einen `project:`-Pfad `p`:
  - Match wenn ein vorhandenes Projekt `Slug == p` ∥ `Name == p` ∥ `Name == lastSegment(p)` → dessen `ID`.
  - sonst `CreateProject(name = lastSegment(p), slug = p, Default Color/Glyph)` → neue `ID`.
- `lastSegment(p)` = Teil nach dem letzten `/`.
- Cache `p → ID` für den Lauf; neu angelegte Projekte in der Zusammenfassung zählen.
- `--dry-run`: nicht anlegen — „würde Projekt X anlegen" melden.

### Fehlerbehandlung

- Pro-Datei isoliert: Parse-/Validierungs-/4xx-Fehler → `Pfad: Grund` loggen, `failed++`,
  Walk läuft weiter.
- Duplicate-Path ohne `--update` = **skip**, kein Fehler.
- Exit-Code: `0` wenn alle importiert/übersprungen; `≠0` wenn `failed > 0` (damit Skripte es merken).

### Zusammenfassung (stdout, am Ende)

```
importiert N · übersprungen M (existieren) · Projekte angelegt P · Fehler K
```
plus je Fehler eine `Pfad: Grund`-Zeile.

## Data Flow

1. CLI liest Datei → `vaultFrontmatter` + Title + Body.
2. CLI löst `project:` → `projectId` (find-or-create, gecached).
3. CLI `POST /documents/import` mit `{type, path, title, body, date?, projectId?}`.
4. Server `ImportDocument`: stempelt id/owner/timestamps, übernimmt Felder verbatim, Tags aus
   Frontmatter, validiert, `Create`, `ReplaceLinks`, notify → `201` Doc (oder `409`/`400`).
5. CLI zählt imported/skipped/failed.

## Testing

- **`usecase.ImportDocument`** (in-memory store, wie `create_document_test`):
  daily mit **historischem** Datum+Pfad (kein now-Override) · project mit `ProjectID` · free ·
  Tags aus Frontmatter · Duplicate-Path → Fehler · Links extrahiert.
- **httpserver import handler**: happy `201` · bad type `400` · duplicate `409` · auth.
- **apiclient.ImportDocument**: postet korrekten Body · `409` → typisierter Conflict.
- **CLI**: Frontmatter/Title/Date-Ableitung (Unit auf die Parse-Helper) · find-or-create-Matching
  (Slug/Name/lastSegment) · `--dry-run` schreibt nichts (httptest zählt 0 POSTs) · idempotenter
  Re-Run skippt (zweiter Lauf 0 imported). httptest-Server wie `internal/tui/docs_test`.

## Scope Boundaries (YAGNI)

- Keine Wikilink-Umschreibung (Vault hat keine).
- Kein Archiv-/Zip-Import (Ansatz C verworfen).
- Kein Watch/Sync — einmaliger Import.
- `agent`-Typ passiv erlaubt (valider Typ), nicht sonderbehandelt.
- `_project.md` nicht sonderbehandelt — Typ kommt aus dem Frontmatter (`project`), Pfad aus `id`.
- Projekt-`Color`/`Glyph` = Defaults (nicht aus gitlab abgeleitet); `Rate` bleibt unset.

## Files

**New:**
- `internal/usecase/import_document.go` (+ `import_document_test.go`)
- `cmd/flow/docs_import.go` (+ `docs_import_test.go`)

**Modified:**
- `internal/adapter/httpserver/documents.go` (Handler) + Route-Registrierung
- `internal/adapter/httpserver/documents_test.go`
- `internal/adapter/apiclient/documents.go` (+ `documents_test.go`)
- `cmd/flow/docs.go` (Subcommand `import` an den `docs`-Command hängen)
- ggf. Wiring im Composition-Root (Usecase-Konstruktion + Route), inkl. curl-Smoke der neuen Route.
