# flow Kontext-Redesign · Querschnitt A — Memory-Lifecycle (Archivierung) — Detail-Spec

**Datum:** 2026-06-29 · **Branch:** `rebuild` · **Status:** Design bestätigt (Brainstorm), bereit für Plan
**Übersicht:** `specs/2026-06-27-flow-kontext-redesign-design` (§83 „Querschnitt A — Memory-Lifecycle")
**Vorgänger:** B3-Kern (Compose/Pinning) · B3d (DocType + Ist-Migration) · cap+rank (Budget). *(Git-kanonisch auf `rebuild`.)*

## Ziel

Memory *altert*. Symptom heute real: cap+rank ist live und budgetiert den Bootstrap, droppt aber ~65 leaf-Memories **still** unter dem Cap — großteils „DONE"-Milestones (`*_done`), die **korrekt, aber historisch** sind und nichts im Start-Kontext verloren haben. cap+rank sagt das selbst: „ein Inhalts-/Verrottungs-Problem, kein Ranking-Problem → Querschnitt A".

A-Kern liefert den **Lifecycle-Zustand `archiviert`**: ein Doc kann „raus aus dem Bootstrap + Default-Listen/Suche, aber findbar + reversibel" gestellt werden. Das ist das Fundament, das die übrigen Lifecycle-Mechanismen (Verfall A2, Verdichtung A3) später brauchen — und es realisiert den akuten Gewinn sofort, indem die Bestands-Done-Milestones in einem Pass archiviert werden.

`archived` spiegelt das bestehende `pinned`-Flag fast 1:1 (Boolean auf `documents`, gesetzt über den Write-/Import-Pfad + dedizierter Setter). Die Verdrahtung ist damit ein bekanntes, risikoarmes Muster.

## Scope-Schnitt

**In (A-Kern):**
- Boolean `archived` (+ `archived_at`) auf `documents`; Ausschluss aus Compose/Bootstrap, Default-Listen und Suche.
- `SetArchived` end-to-end: domain → ports → pgstore → usecase → REST → apiclient → **MCP** → CLI.
- Import-Manifest um eine `archived`-Disposition erweitert (künftige Imports landen direkt archiviert).
- Einmaliger, idempotenter **Bulk-Archive-Pass** über die Bestands-Done-Milestones (Heuristik → human-approve → apply).
- Leichte Provenance: `archived_at`; optional „ersetzt durch [[x]]" in `extra`.

**Out (bewusst — je eigener Baustein):**
- **Verfall / Review-Trigger** (`dauerhaft` vs. `flüchtig`, Ablauf-/Review-Fälligkeit) → **A2**.
- **Verdichtung** (Schwellwert-Warnung + Verdichtungs-Subagent, D9 halbautomatisch) → **A3**.
- **UI** (Lifecycle-Badges, Kontext-Inspektor, Aufräum-Ansicht, globale Cross-Scope-Suche) → **Querschnitt B**.
- **Soft-Delete / Papierkorb** (`gelöscht` als Tombstone): `gelöscht` bleibt der **bestehende Hard-Delete** (seltener, menschlicher Akt — D11); kein neuer Zustand.
- **source-repo/commit-Provenance:** Node/Scope kodiert „woher" nach B1 schon; YAGNI.
- **Globaler `include archived`-Filter** über alle Listen: kein Präzedenzfall im Code; Findbarkeit löst stattdessen die dedizierte `archived`-Sicht (§7).

## Entscheidungen (A-1 … A-8)

- **A-1** Ein Lifecycle-Zustand **`archiviert`** (nicht „veraltet" — die Done-Milestones sind korrekt, nur historisch). Boolean `archived` (+ `archived_at`), gespiegelt zu `pinned`. Achse `aktiv → archiviert → (selten) gelöscht`; **`gelöscht` = bestehender Hard-Delete**, kein eigener Zustand. „war falsch / ist ersetzt" ist **kein** eigener Zustand, sondern die optionale Notiz „ersetzt durch [[nachfolger]]".
- **A-2** **Ausschluss liegt im SQL** — ein `AND NOT archived` in fünf Queries. Die pure `Compose()`-Ranking-Funktion bleibt **unangetastet**; archivierte Docs erreichen das Kontextfenster nie.
- **A-3** `archived` ist ein **Write-/Import-Pfad-Feld wie `pinned`**: gesetzt bei `Create` / `UpsertByPath` **und** über dedizierten `SetArchived`. Idempotent: `UpsertByPath`s `ON CONFLICT` fasst das Flag nicht an → die Upsert-Usecase zieht `SetArchived` explizit nach (genau wie heute `SetPinned`), Re-Run kann umklassifizieren.
- **A-4** `archived` × `pinned` widersprechen sich (pin = immer laden, archived = nie). **`SetArchived(true)` löscht `pinned`; archived dominiert.** (Symmetrisch hebt das Setzen von `pinned` ein `archived` nicht automatisch auf — Un-Archivieren ist eine bewusste eigene Aktion.)
- **A-5** **Bulk-Archive = Heuristik-geseedete Review-TSV → human-approve → idempotenter Apply.** Gleiches Manifest-Muster wie B3d. Eine Mechanik, zwei Türen: künftige Imports (Tür A) und Bestand (Tür B).
- **A-6** **MCP-Tool `flow_archive_doc` ist aufgenommen** (über das strikte `pinned`-Spiegelbild hinaus): der Agent schreibt Memories über MCP und kann eine erledigte Milestone-Memory **in-session** archivieren — genau D11 („Markieren ist sicher + umkehrbar, *teils macht's ein Agent*", im Gegensatz zum menschlichen Hard-Delete). Motor gegen laufende Verrottung. REST + apiclient + CLI ebenfalls; **TUI/WebUI-Aktion + Badges = Querschnitt B**.
- **A-7** **Provenance leicht:** `archived_at` (wann) als Spalte; „ersetzt durch [[x]]" optional in `extra` JSONB (`superseded_by`). Kein source-repo/commit.
- **A-8** **Findbarkeit** über eine dedizierte Sicht `flow context archived` (eigener Store-Query `WHERE archived`) statt eines globalen include-Flags durch alle List-/Such-Pfade.

---

## 1 · Datenmodell — Migration `0022_documents_archived.sql`

Spiegelt `0021_documents_pinned.sql` (goose Up/Down):

```sql
-- +goose Up
ALTER TABLE documents ADD COLUMN archived    BOOLEAN     NOT NULL DEFAULT false;
ALTER TABLE documents ADD COLUMN archived_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE documents DROP COLUMN archived_at;
ALTER TABLE documents DROP COLUMN archived;
```

`archived_at` ist nullable und wird gesetzt, wenn `archived` auf `true` geht (und auf `NULL` zurück beim Un-Archivieren). Kein Index nötig: die fünf Bootstrap/List/Such-Queries filtern `AND NOT archived` als billiges Boolean-Prädikat; die `archived`-Sicht (§7) ist eine seltene, kleine Abfrage.

> goose-Annotationen sind Pflicht (`-- +goose Up`/`Down`) — sonst scheitert das Apply zur Laufzeit, der Build merkt's nicht (nur die pgstore-Docker-Tests). Siehe `feedback_pgstore_goose_migrations`.

## 2 · Mechanismus (domain → ports → pgstore)

**Domain** (`internal/domain/document.go`): `Archived bool \`json:"archived"\`` direkt nach `Pinned` (Zeile 73). `Validate()` unberührt. (`archived_at` wird im Store gesetzt, ist kein Domain-Validierungsfeld; optional als `ArchivedAt *time.Time` mitgeführt, wenn eine Sicht es braucht.)

**Ports** (`internal/ports/ports.go`, `DocumentStore` ab Zeile 172):
- neu `SetArchived(ctx context.Context, ownerID, id string, archived bool) error` (nach `SetPinned`:197).
- `UpsertByPath(...)` (Zeile 200) bekommt einen `archived bool`-Parameter neben `pinned bool`.

**pgstore** (`internal/adapter/pgstore/documents.go`):
- `archived` an `docCols` + `prefixedDocCols` (Zeile 30–32) anhängen; `archived_at` ebenso (oder bewusst nur in der `archived`-Sicht selektieren — Plan-Detail, Default: mit aufnehmen für Symmetrie).
- `Create` (Zeile 91): `$14 = d.Archived` (+ `$15 = d.ArchivedAt`).
- Scans `scanDocument` (502), `scanSearchHit` (362), `scanSemanticHit` (477): `&d.Archived` (+ `&d.ArchivedAt`) ergänzen.
- neuer `SetArchived` analog `SetPinned` (200):
  `UPDATE documents SET archived=$1, archived_at = CASE WHEN $1 THEN now() ELSE NULL END, updated_at=now() WHERE owner_id=$2 AND id=$3` — **plus** beim Archivieren `pinned=false` (A-4): einfacher als zwei Statements im selben `SET`.
- `UpsertByPath` (211): `archived` in die INSERT-Spaltenliste; `ON CONFLICT` lässt es (wie `pinned`) unberührt — die Usecase zieht `SetArchived` nach (§5).

**Ausschluss — ein `AND NOT archived` in fünf Queries** (A-2):

| pgstore-Methode | bestehende WHERE | Ergänzung |
|---|---|---|
| `List` (125) | `WHERE owner_id=$1` | `AND NOT archived` |
| `ListPage` (143) | `WHERE owner_id=$1` | `AND NOT archived` |
| `Search` (317) | `WHERE d.owner_id=$1` | `AND NOT d.archived` |
| `SemanticSearch` (432) | äußeres JOIN-WHERE | `AND NOT d.archived` |
| `ListForContext` (234) | `WHERE owner_id=$1 AND type = ANY($2)` | `AND NOT archived` |

`ListForContext` speist `ComposeContext` (`internal/usecase/compose_context.go`, Gather-Calls 266/281) — damit ist der Bootstrap-Ausschluss erledigt, **ohne** die `Compose()`-Funktion (130) oder das cap+rank-Ranking anzufassen.

## 3 · `archived` × `pinned` (A-4)

Widersprüchlich: pin = „immer laden", archived = „nie laden". Regel: **`SetArchived(true)` löscht `pinned`** (im selben UPDATE, s.o.). archived gewinnt im Bootstrap ohnehin (eigene Query ohne archived-Docs), aber das saubere Löschen verhindert einen inkonsistenten Zustand in der UI/Sicht. Un-Archivieren (`SetArchived(false)`) setzt **nicht** automatisch `pinned` zurück — Pinnen ist eine separate bewusste Aktion.

## 4 · Surfaces

**REST** (`internal/adapter/httpserver/`): `archiveReq{ Archived bool }` + `handleArchiveDocument` (spiegelt `pinReq`/`handlePinDocument` 240–261) → ruft `SetArchived.Execute`, publiziert `EventDocumentUpdated` (SSE `document.updated`), `204`. Route `POST /api/v1/documents/{id}/archive` (`server.go` nach 167); Feld `SetArchived usecase.SetArchived` am `Server`-Struct.

**apiclient** (`internal/adapter/apiclient/context.go`): `SetArchived(ctx, id string, archived bool) error` (spiegelt `SetPinned` 69–72); `Archived bool \`json:"archived"\`` an `UpsertByPathInput` (94).

**MCP** (`cmd/flow-mcp/server.go`): neues Tool **`flow_archive_doc`** — params `{ id string, archived bool }` (Default `archived=true`), optional `superseded_by string`. Ruft `apiclient.SetArchived` (+ schreibt `superseded_by` in `extra`, falls gesetzt). Das 14. Tool (heute 13). Tool-Beschreibung: „Mark a context doc archived (out of bootstrap + default lists/search, but findable + reversible) or un-archive it."

**usecase** (`internal/usecase/set_archived.go`): trivialer Delegator, verbatim-Spiegel von `set_pinned.go`.

**CLI** (`cmd/flow/`): zwei Verben (Pin hat keins — hier bewusst mehr):
- `flow context archive --from <tsv>` — Bulk-Apply (§5).
- `flow context archived` — listet archivierte Docs (§7).

**TUI/WebUI:** Archiv-Aktion + Lifecycle-Badges → **Querschnitt B (deferred).**

## 5 · Import-Dreiweg + Bulk-Pass — *eine* Mechanik, zwei Türen (A-3, A-5)

**Manifest** (`cmd/flow/context_migrate.go`, `manifestRow` 53): eine **6. Spalte `archived`** (`y`/`-`), rückwärtskompatibel (fehlende Spalte → `false`). Disposition-Lesart: `skip` (keep-Spalte) > `pin` > `archived` > `aktiv`; `pin` **und** `archived` gleichzeitig ist ungültig (archived gewinnt / Validierungs-Warnung). `memoryDoc` + `deriveMemoryDoc` (129) tragen `Archived`; der `UpsertByPathInput`-Call (209) reicht es durch.

**`UpsertDocumentByPath`** (`internal/usecase/upsert_document_by_path.go`): `Archived bool` an `UpsertByPathInput`; an `Docs.UpsertByPath` durchreichen; **nach** dem bestehenden expliziten `SetPinned` (50–53) ein analoges `SetArchived` (weil `ON CONFLICT` das Flag nicht anfasst → idempotent, Re-Run klassifiziert um).

**Tür A — künftige Imports:** die restlichen lokalen Memories (und künftige Vaults) können einzelne Zeilen direkt `archived` markiert importieren.

**Tür B — Bestand (der Sofort-Gewinn):**
1. **Kandidaten-Heuristik** (read-only): `flow context archive` ohne `--from` (oder `--candidates`) emittiert eine Review-TSV der wahrscheinlichen Done-Milestones — `type=memory`, deren Titel/Body „DONE" enthält bzw. deren Pfad/Name auf `*_done`/Milestone-Muster passt — je Zeile `path \t title \t archive` (vorbelegt `archive=y`).
2. **Human-Review:** Soenne kippt einzelne Zeilen auf `n` (behalten). Gleicher Workflow wie das B3d-Manifest, das Soenne kennt.
3. **Apply:** `flow context archive --from <tsv>` löst je Zeile `path → id` (über den bestehenden by-path-/List-Pfad) und ruft `apiclient.SetArchived(id, archive)`. **Idempotent** (SetArchived ist absolut, nicht toggelnd); `--dry-run` meldet „würde N archivieren", ohne zu schreiben.

> Die Heuristik ist nur ein **Seed**, keine Automatik — der Mensch segnet ab (D9/D11: Markieren statt automatisch löschen). Genaue Muster-Regex = Plan-Detail; im Zweifel `archive=n` (Compose budgetiert ohnehin via cap+rank).

## 6 · Provenance (leicht) (A-7)

- `archived_at` (Spalte): wann archiviert. `created_at`/`updated_at` existieren bereits („wann erstellt/zuletzt geändert").
- `superseded_by` (in `extra` JSONB, sparse): optionaler Wikilink-/Pfad-Verweis „ersetzt durch [[nachfolger]]". Gesetzt über `flow_archive_doc`/REST optional; **kein** Pflichtfeld.
- **Kein** source-repo/commit: nach B1 sagt `node_id`/Scope schon, *woher* ein Doc stammt.

## 7 · Findbarkeit (A-8)

archived ist raus aus allen Defaults, aber **nicht** verloren: `flow context archived` führt einen dedizierten Store-Query `... WHERE owner_id=$1 AND archived ORDER BY archived_at DESC` aus (neue schmale Port-Methode `ListArchived(ctx, ownerID)` oder Parameter-Variante — Plan-Detail). Kein globaler `includeArchived`-Flag durch List/Search (kein Präzedenzfall, unnötige Fläche). Die reiche Aufräum-/Archiv-**Ansicht** (gruppiert, mit Badges, Restore-Knopf) ist **Querschnitt B**.

## 8 · Tests (TDD)

**pgstore (Docker, Postgres):**
- `Create` mit `archived=true` → persistiert `archived`+`archived_at`; Scan liest beide.
- jede der fünf Queries (`List`/`ListPage`/`Search`/`SemanticSearch`/`ListForContext`) **schließt** ein archiviertes Doc aus; ein nicht-archiviertes erscheint.
- `SetArchived(true)` setzt `archived_at`, **löscht `pinned`** (A-4); `SetArchived(false)` nullt `archived_at`, lässt `pinned` unangetastet.
- `UpsertByPath` neu (INSERT) trägt `archived`; Re-Upsert (`ON CONFLICT`) + nachgezogenes `SetArchived` klassifiziert um (idempotent).
- `ListArchived` liefert nur archivierte, nach `archived_at` sortiert.

**usecase:** `SetArchived`-Delegator ruft Store; `ComposeContext`-Test (in-memory Store): ein archiviertes Memory erscheint **nie** im Compose-Ergebnis, unabhängig von Pin/Tag/Scope.

**httpserver:** `handleArchiveDocument` happy `204` + publiziert `document.updated`; Auth erzwungen; unbekannte id → sinnvoller Fehler.

**apiclient:** `SetArchived` postet korrekten Body an `/archive`; `UpsertByPathInput` serialisiert `archived`.

**MCP:** `flow_archive_doc` round-trip (id+archived → SetArchived aufgerufen; `superseded_by` → `extra`).

**CLI:** `context_migrate` parst die 6. Spalte (inkl. fehlend → false; `pin`+`archived` → Warnung/archived gewinnt); `flow context archive --from` löst Pfade auf + ruft SetArchived; `--dry-run` schreibt nicht; `flow context archived` rendert die Liste.

## 9 · Done-Gate

- `make ci` grün (lint inkl. `gofumpt`/`staticcheck`, templ, build, Tests, Coverage-Gate halten).
- **curl-smoke vs Postgres+Dex** (`FLOW_DEV=1`, `make dev-up`/`dev-run`):
  - `POST /documents/{id}/archive {"archived":true}` → `204` + `document.updated` SSE; das Doc verschwindet aus `flow context` (Compose) und aus Such-/Listen-Defaults; `flow context archived` zeigt es.
  - Un-Archivieren bringt es zurück; `pinned` ist nach Archivieren gelöscht.
- **MCP** `flow_archive_doc` round-trip gegen den laufenden Server.
- **Bulk-Pass real:** Kandidatenliste erzeugen → kleine Auswahl archivieren → `flow context` zeigt **weniger `Used`-Token** / die zuvor still gedroppten leaf-Memories sind keine Kandidaten mehr (der eigentliche Sofort-Gewinn, gegen den Footer aus cap+rank gegengeprüft).
- Browser-Dogfood entfällt (UI = Querschnitt B); TUI-Sicht ebenso.

## 10 · Geänderte/neue Dateien (Touch-point-Checkliste)

| Datei | Änderung |
|---|---|
| `internal/adapter/pgstore/migrations/0022_documents_archived.sql` | **neu** — `archived` + `archived_at` (goose Up/Down) |
| `internal/domain/document.go` | `Archived bool` (+ optional `ArchivedAt *time.Time`) nach `Pinned` |
| `internal/ports/ports.go` | `SetArchived(...)`; `archived bool` an `UpsertByPath`; ggf. `ListArchived(...)` |
| `internal/adapter/pgstore/documents.go` | Spalten+Scans+Create; `SetArchived` (löscht pin); `archived` in `UpsertByPath`; `AND NOT archived` in 5 Queries; `ListArchived` |
| `internal/usecase/set_archived.go` | **neu** — Delegator (Spiegel von `set_pinned.go`) |
| `internal/usecase/upsert_document_by_path.go` | `Archived` an Input; durchreichen; `SetArchived` nach `SetPinned` |
| `internal/usecase/compose_context.go` | **keine** — Ausschluss sitzt im SQL |
| `internal/adapter/httpserver/documents.go` | `archiveReq` + `handleArchiveDocument` |
| `internal/adapter/httpserver/server.go` | `SetArchived`-Feld + Route `POST …/{id}/archive` |
| `internal/adapter/apiclient/context.go` | `SetArchived`-Methode; `Archived` an `UpsertByPathInput` |
| `cmd/flow-mcp/server.go` | Tool `flow_archive_doc` (14.) |
| `cmd/flow/context_migrate.go` | 6. Manifest-Spalte `archived`; `memoryDoc`/`deriveMemoryDoc`/Upsert-Call |
| `cmd/flow/context_archive.go` (o.ä.) | **neu** — `flow context archive --from/--dry-run/--candidates` + `flow context archived` |

**Nicht angefasst:** `Compose()`-Ranking (pure), `list_documents.go`/`search_documents.go` (dünne Delegatoren; Filter im SQL).

---

## Nicht in diesem Slice (bewusst out-of-scope)

- **Verfall / Review-Fälligkeit (A2):** `dauerhaft` vs. `flüchtig`, Ablauf-Trigger — eigenes orthogonales Feld (z.B. `review_due_at`), baut auf diesem Status auf.
- **Verdichtung (A3):** Schwellwert-Warnung + Verdichtungs-Subagent (D9). Nutzt `flow_archive_doc` als sicheres Werkzeug.
- **Querschnitt B (UI):** Lifecycle-Badges, Kontext-Inspektor, Aufräum-Ansicht, Restore-Knopf, globale Cross-Scope-Suche.
- **Soft-Delete / Papierkorb:** `gelöscht` bleibt der bestehende Hard-Delete.
- **source-Provenance** (Repo/Commit) · **globaler `include archived`-Filter**.
