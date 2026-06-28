---
type: agent
project: github.com/serverkraken/flow
---
# flow Kontext-Redesign · Baustein 2 — Tag-System — Detail-Spec

**Datum:** 2026-06-28 · **Branch:** `rebuild` (Slice-Branch beim Start des Plans) · **Status:** Design bestätigt (Speichermodell + Worktime-Migration + Body-Strip am 2026-06-28 entschieden); bereit für Implementation-Plan
**Übersichts-Spec:** `docs/superpowers/specs/2026-06-27-flow-kontext-redesign-design.md` (Achsen, Mechanik, D1–D11)
**Vorgänger (Hierarchie):** `docs/superpowers/specs/2026-06-27-flow-kontext-b1-hierarchie-bindings-design.md` (B1 gelandet — `nodes`-Hierarchie, Migr. 0015–0018)
**Tag-Ursprung (M2c):** `docs/superpowers/specs/2026-06-15-flow-rebuild-m2c-tags-filter-design.md` (Frontmatter-Tags, die B2 ablöst)

## Ziel

Tags von einer **Doc-lokalen, aus-Frontmatter-geparsten** `text[]`-Spalte zu einem **generischen,
polymorphen, explizit gesetzten** Tag-System machen — die *horizontale* Quer-Achse des Redesigns
(Thema/Querschnitt: `django`, `terraform`, `postgres` … über alle Entitäten), orthogonal zur
*vertikalen* Hierarchie aus B1 (Containment/Isolation). Tags werden **neutral** (keine Isolations-Logik
im Tag), **strukturiert** (Parameter/Relation statt YAML im Body) und **polymorph** (Doc, Node,
WorktimeSession — Asset folgt in Phase 2). Der Body wird zu **reinem Inhalt**. B2 ist quasi-unabhängig
von B1 und liefert das **Tag-Match-Primitiv**, das B3 (Kontext-Store) konsumiert.

## Scope

**In:**
- Tag-**Entität** (`tags`-Registry) + **polymorphe Zuordnung** (`taggings`-Junction) — Migration `0019`.
- Taggables: `document`, `node`, `work_session` (Asset = reservierter Typwert, Phase 2).
- **`tags []string` als expliziter API-Parameter** (MCP `flow_create_doc`/`flow_update_doc` + REST Doc-/Node-/Session-DTOs) statt Frontmatter-Parsing.
- **Cutover:** Write-Pfad ruft kein `ParseFrontmatter` mehr; Tags kommen nur noch aus dem Parameter.
- **Daten-Migration des Bestands** (destruktiv, verlustfrei, idempotent): Frontmatter parsen → `tags` in `taggings`, übrige Keys → `documents.extra`, `---`-Block aus `body` strippen; `work_sessions.tag` → `taggings`.
- **`documents.tags TEXT[]` + GIN (`0008`) droppen** · `work_sessions.tag` droppen; `Document.Tags` wird via Junction hydriert.
- Filter-Integration (AND) in `List`/`ListPage`/`Search`/`SemanticSearch` über die Junction; `ListTags` registry-gestützt + node-scopebar.
- Vault-Import (`flow docs import`): Fremd-Frontmatter nur noch **Konvertierungsschritt** in den `tags`-Parameter (die zwei divergenten Parser auf einen Pfad ziehen).
- **Consumer Worktime:** Sessions taggable + **Tag-Zeit-Auswertung** (Σ Dauer je Tag, node-/range-gefiltert).
- **Consumer-Primitiv für B3:** neutrale Cross-Node-Tag-Query (AND/OR über Scopes).
- **UI „Tagging überall":** Tag-Editor (add/remove) + Filter auf Docs/Nodes/Sessions in WebUI **und** TUI.

**Out (bewusst):**
- **Rename/Merge als Bedien-Aktion** — das Junction-Modell *macht* es zum 1-Zeilen-`UPDATE` (der Payoff von B2-1); B2 liefert den `MergeTags`-Store-Helfer **als Primitiv** (Bestands-Hygiene, z.B. `postgres`/`postgresql` zusammenführen), aber die **Oberfläche** (Rename/Merge-Aktion in WebUI/TUI/CLI) gehört zur **Aufräum-Ansicht** (Querschnitt A/B).
- **Tag-`color`** + **Tag-`description`** + **Tag-Lifecycle** (`veraltet`): deferred. Lifecycle ist *pro Taggable* (Querschnitt A), nicht pro Tag — Tags bleiben neutrale Labels.
- **Asset-Tagging** (Phase 2, B4) — nur der `taggable_type`-Wert wird vorgehalten.
- **Bootstrap-Tag-Reichweite** (D7: nur global-getaggtes quer) — Policy lebt im **Kontext-Store (B3)**, nicht im Tag-System.
- **Tags im Such-Ranking** (tsvector/RRF) — Tags bleiben Filter, kein Rankingsignal (wie heute). Optional später.
- Tag-Synonyme/Aliase, Tag-Namespaces/Hierarchie (`lang/python` ist nur ein String).

## Entscheidungen (B2-spezifisch, erweitern D1–D11 der Übersicht)

- **B2-1** Speichermodell = **`tags`-Registry + polymorphe `taggings`-Junction** (normalisiert). Gegen „nur `text[]` je Tabelle" (keine Entität, kein Rename) und gegen „Hybrid Registry+Arrays" (Dual-Write, Rename fasst alle Arrays an). Join-Kosten bei dieser Datenmenge irrelevant; Rename/Merge = 1 Update; eine Distinct-Quelle. *(entschieden 2026-06-28)*
- **B2-2** Taggables = `document` · `node` · `work_session`. `asset` ist **reservierter** `taggable_type` (Phase 2) — null Schema-Änderung nötig. „Workspace" und „Repo" der Übersicht sind beide `node` (per `kind`), fallen zusammen.
- **B2-3** Identität = **`slug`** (lower/trim/dedup via bestehendes `normalizeTags`), `UNIQUE(owner_id, slug)`. **`display`** wird mitgeführt (Anzeigeform; default = slug; nur via Rename geändert, kein stilles Mutieren) — die Information ist nur beim Schreiben verfügbar, später nicht rekonstruierbar. **Flach**, keine Tag-Hierarchie.
- **B2-4** Worktime `tag` (Einzel-Freitext) wird **ersetzt + migriert**: bestehende nicht-leere Werte → `taggings`; Spalte gedroppt; `AddSession`/`StartSession`/`EditSession` + REST + CLI nehmen `tags []string`. Ein Tag-Konzept überall. *(entschieden 2026-06-28)*
- **B2-5** Body-Strip = **ganzer führender Frontmatter-Block raus, verlustfrei**: `tags:` → `taggings`, alle übrigen Keys → `documents.extra` (rekonstruierbar), `---`-Block aus `body`. *(entschieden 2026-06-28)*
- **B2-6** **Frontmatter abgeschafft am Write-Pfad** (Übersicht): `ParseFrontmatter` überlebt **nur** in (a) der Einmal-Migration und (b) dem Vault-Import als Konvertierungsschritt.
- **B2-7** Tags **neutral**: B2 liefert das Cross-Node-Match-**Primitiv** (AND/OR über Scopes). Die *Reichweiten-Policy* (D7) entscheidet der **Consumer** (B3-Bootstrap), nicht das Tag.
- **B2-8** `documents.tags TEXT[]` + GIN (`0008`) **gedroppt**; `Document.Tags []string` bleibt im Domain-Modell, wird aber via `taggings`-Join **hydriert** (`TagsForMany`), nicht mehr aus einer Spalte gescannt.
- **B2-9** Polymorphe FK ist **app-seitig** integer-gehalten: `taggings.tag_id` hat echten FK auf `tags` (`ON DELETE CASCADE`); `(taggable_type, taggable_id)` hat **keinen** DB-FK (kann nicht auf 3 Tabellen zeigen) → jedes Taggable-Delete räumt seine `taggings` via `TagStore.ClearTaggable`. Orphan-Tags (count 0) bleiben als Vokabular erhalten; Picker filtern auf in-use.

---

## 1 · Datenmodell — `tags` + `taggings` (Migration 0019)

```sql
-- +goose Up
CREATE TABLE tags (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    slug       TEXT NOT NULL,                 -- normalisierte Identität (lower/trim/dedup)
    display    TEXT NOT NULL,                 -- Anzeigeform; default = slug
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, slug)
);

CREATE TABLE taggings (
    tag_id        TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    taggable_type TEXT NOT NULL CHECK (taggable_type IN ('document','node','work_session')),
    taggable_id   TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tag_id, taggable_type, taggable_id)
);
CREATE INDEX taggings_taggable ON taggings (taggable_type, taggable_id);  -- "Tags von X"
CREATE INDEX taggings_tag      ON taggings (tag_id);                      -- "alles mit Tag T"
```

- `taggable_id` ist über alle drei Ziele **TEXT** (alle PKs sind TEXT-ULIDs) → keine Typ-Sonderfälle.
- `taggable_type`-CHECK lässt `asset` (Phase 2) noch nicht zu; die Erweiterung ist ein `ALTER … DROP/ADD CONSTRAINT` ohne Datenmigration.
- `taggings.created_at` gibt „wann getaggt" gratis (leichte Provenance; nicht weiter ausgewertet in B2).
- **Goose-Annotations** beachten ([[feedback_pgstore_goose_migrations]]): jeder Schritt mit `-- +goose Up`/`Down`, sonst Apply-Fehler (vom Build nicht gefangen, nur pgstore-Docker-Tests).

> Reihenfolge in `0019`: erst `tags`/`taggings` anlegen, dann der Daten-Fixup (§6), **zuletzt** `DROP INDEX documents_tags_gin; ALTER TABLE documents DROP COLUMN tags; ALTER TABLE work_sessions DROP COLUMN tag;` — damit der Fixup die Alt-Werte noch lesen kann. `Down` legt Spalten/GIN wieder an und re-projiziert aus `taggings` (best-effort; Tags überleben, die Body-/extra-Umschichtung ist nicht voll reversibel → im Spec dokumentiert, kein verlustfreies Down erwartet).

## 2 · Domain — `Tag`, `TaggableType`

`internal/domain/tag.go` (neu). Die Tag-Normalisierung wandert aus `frontmatter.go` hierher (bleibt wiederverwendbar; `frontmatter.go` ruft sie weiter auf, solange es lebt):

```go
type TaggableType string

const (
    TaggableDocument    TaggableType = "document"
    TaggableNode        TaggableType = "node"
    TaggableWorkSession TaggableType = "work_session"
    // TaggableAsset reserviert (Phase 2)
)

type Tag struct {
    ID        string    `json:"id"`
    OwnerID   string    `json:"-"`
    Slug      string    `json:"slug"`
    Display   string    `json:"display"`
    CreatedAt time.Time `json:"createdAt"`
}

// NormalizeTag: trim, lower, "" → invalid. (Hebt das bestehende normalizeTags
// auf Einzel-Tag-Ebene; CollectTags/TagCount bleiben für die Zähl-Ansichten.)
func NormalizeTag(raw string) (slug string, ok bool)
func NormalizeTags(in []string) []string   // = bisheriges normalizeTags (dedup, first-seen)
```

- `TagCount{Tag, Count}` (heute in `frontmatter.go`) bleibt als View-DTO für Tag-Listen mit Häufigkeit.
- `display` ergibt sich beim Upsert aus dem **rohen** Eingabe-Tag (erste Schreibung gewinnt); `slug` aus `NormalizeTag`.

## 3 · Ports + Usecases

**Port `TagStore`** (`internal/ports/ports.go`, neu):

```go
type TagStore interface {
    // Upsert findet-oder-legt-an je (owner, slug); display nur bei Neuanlage gesetzt.
    UpsertTags(ctx, ownerID string, raw []string) ([]domain.Tag, error)
    // SetTags ersetzt die Tags eines Taggables (diff: attach neue, detach entfallene).
    SetTags(ctx, ownerID string, t domain.TaggableType, id string, raw []string) ([]domain.Tag, error)
    // TagsForMany hydriert Tags für eine Menge gleichartiger Taggables (1 Query).
    TagsForMany(ctx, ownerID string, t domain.TaggableType, ids []string) (map[string][]domain.Tag, error)
    // FilterIDs liefert die taggable_ids, die ALLE (AND) bzw. EINEN (OR) der slugs tragen.
    FilterIDs(ctx, ownerID string, t domain.TaggableType, slugs []string, mode domain.TagMatch) ([]string, error)
    // ListTags: Registry mit Häufigkeit, optional auf einen Taggable-Typ / Node-Subtree beschränkt.
    ListTags(ctx, ownerID string, scope domain.TagScope) ([]domain.TagCount, error)
    // ClearTaggable entfernt alle taggings eines Taggables (beim Delete).
    ClearTaggable(ctx, ownerID string, t domain.TaggableType, id string) error
    // MergeTags (Hygiene-Helfer; UI in Querschnitt A): from-slug → into-slug umhängen, from droppen.
    MergeTags(ctx, ownerID, fromSlug, intoSlug string) error
}
```

- `domain.TagMatch` = `{TagAnd, TagOr}`. `domain.TagScope` = `{Type *TaggableType, NodeSubtree *string}` (Subtree nutzt B1 `Ancestors`/`Children`-Logik; in B2 genügt owner-/typ-scope, NodeSubtree ist die Erweiterungs-Naht für B3).
- **Usecases** (`internal/usecase/`): `SetTags`, `GetTags` (einzeln, via `TagsForMany` mit 1 id), `ListTags` (ersetzt das alte „alle Docs laden + `CollectTags`"), plus Filter-Verdrahtung in den bestehenden List/Search-Usecases (übergeben `slugs` an `FilterIDs` bzw. inline-Subquery, siehe §5).

## 4 · API-Fläche — expliziter `tags`-Parameter

**MCP** (`cmd/flow-mcp/`):
- `createDocIn` (`tools_write.go:18`) + `updateDocIn` (`tools_write.go:60`) bekommen `Tags []string` (jsonschema „tags as a list; replaces the whole set"). Die Frontmatter-Schema-Notiz am `Body` fliegt raus.
- `update` ohne `tags`-Feld = Tags unverändert (Pointer/`omitempty`-Semantik wie `Title`/`Body`); leeres `tags:[]` = alle entfernen (explizit). Per Doku klarstellen.
- Node-/Session-Tagging in B2 via REST (kein neues MCP-Tool nötig; MCP-Doc-Tools decken den Claude-Schreibpfad ab — Worktime taggt Claude nicht).

**REST** (`internal/adapter/httpserver/documents.go`, `worktime.go`):
- `createDocReq`/`updateDocReq`/`importDocReq` + die Node-Create/Update- und Session-(`add`/`start`/`edit`)-DTOs bekommen `Tags []string`.
- Response: `domain.Document`/`Node`/`WorkSession` tragen weiter `tags` (jetzt hydriert, nicht gespaltet).
- List-Filter `?tag=foo&tag=bar` (`handleListDocuments`) bleibt **AND** (Semantik unverändert), intern auf die Junction umgestellt (§5).

**apiclient** (`internal/adapter/apiclient/documents.go`): `CreateDocumentInput`/`UpdateDocumentInput`/`ImportDocumentInput` + Session-Inputs bekommen `Tags []string`.

**Cutover (B2-6):** `usecase.CreateDocument`/`UpdateDocument`/`ImportDocument` rufen **kein** `ParseFrontmatter` mehr (außer Import, §6). Tags kommen aus dem Parameter → `SetTags`.

## 5 · Filter-Semantik + ListTags (registry-gestützt)

**AND-Filter** ersetzt `tags @> $N` (`documents.go:74`) durch eine Junction-Subquery, die mit FTS/Vector-Ranking komponiert ohne es zu stören:

```sql
AND d.id IN (
    SELECT tg.taggable_id
    FROM taggings tg JOIN tags t ON t.id = tg.tag_id
    WHERE t.owner_id = $own AND tg.taggable_type = 'document' AND t.slug = ANY($slugs)
    GROUP BY tg.taggable_id
    HAVING count(DISTINCT t.slug) = cardinality($slugs)
)
```
(OR-Variante = dieselbe Subquery ohne `GROUP BY/HAVING`; das ist das B3-Primitiv.)

**`Document.Tags`-Hydration** (B2-8): nach `List`/`Search`/`Semantic` die IDs einsammeln und `TagsForMany('document', ids)` → je Doc `Tags` setzen. Ein Helfer, identisch nutzbar für `node`/`work_session`.

**`ListTags`** (`usecase/list_tags.go`) wird:
```sql
SELECT t.slug, t.display, count(*) AS n
FROM tags t JOIN taggings tg ON tg.tag_id = t.id
WHERE t.owner_id = $own  [AND tg.taggable_type = $type]  [AND tg.taggable_id IN (<subtree>)]
GROUP BY t.slug, t.display ORDER BY n DESC, t.slug ASC
```
— ersetzt „alle Docs (inkl. Body) in den Speicher laden + `CollectTags`". Fixt nebenbei den TUI-`loadTags`-Cross-Project-Bug (`docs.go:278` zog owner-weit; jetzt `TagScope` mit Typ/Subtree).

## 6 · Migration (destruktiver Teil — verlustfrei, idempotent)

Alles in `0019` (oder gesplittet `0019` Schema / `0020` Daten-Fixup, falls die CHECK-/DROP-Reihenfolge das verlangt — Plan-Detail). Idempotent + owner-scopebar, **innerhalb einer Transaktion** (atomarer Rollback bei Fehler):

1. **Docs:** für jede Zeile mit führendem Frontmatter (`body` startet `---\n`, Heuristik = bestehendes `ParseFrontmatter`):
   - alle Keys nach `map[string]any` parsen; `tags` → `UpsertTags` + `taggings` (`taggable_type='document'`).
   - **übrige Keys → `documents.extra`** (merge, nicht überschreiben; z.B. `extra = extra || jsonb(rest)`), damit der Block rekonstruierbar bleibt.
   - **`body` = body[bodyStart:]** (Block entfernt, führende Leerzeile trimmen) → reiner Inhalt.
2. **Worktime (B2-4):** `INSERT INTO taggings SELECT … FROM work_sessions WHERE tag <> ''` (Slug via `NormalizeTag`, `UpsertTags` je Wert), `taggable_type='work_session'`.
3. **Spalten droppen:** `DROP INDEX documents_tags_gin` (`0008`) · `ALTER TABLE documents DROP COLUMN tags` · `ALTER TABLE work_sessions DROP COLUMN tag`.

> Die Frontmatter-Parse-Logik im Migrations-Schritt läuft heute schon auf **jedem Write** → das Verhalten auf dem Korpus ist bekannt, geringes Zusatzrisiko. Verify-Gate: pgstore-Docker-Test seedet Docs mit gemischtem Frontmatter (nur `tags`; `tags`+Fremdkeys; kein Frontmatter; `---` mitten im Body) + Sessions mit/ohne `tag`, prüft taggings, `extra`-Erhalt, Body-Strip, Idempotenz (zweiter Lauf = no-op).

**Vault-Import (`cmd/flow/docs_import.go` + `usecase/import_document.go`):** statt „rohen Body mit Frontmatter durchreichen, Server parst" liest der Import das Fremd-Frontmatter **clientseitig** vollständig (`parseVaultFrontmatter` erweitern: alle Keys), zieht `tags` in den **`tags`-Parameter** und schickt den **gestrippten** Body. `ImportDocument.Execute` ruft dann kein `ParseFrontmatter` mehr — ein Pfad, kein divergenter Zweit-Parser. Fremd-Keys außer den gemappten (id/type/date/project) → `extra`.

## 7 · Consumer

**Worktime — Tag-Zeit-Auswertung** (Übersicht: „gesamte django-Zeit über alle Engagements"):
- Sessions sind taggable (`taggable_type='work_session'`); Booking-/Edit-Usecases nehmen `tags []string` → `SetTags`.
- Neuer Aggregat-Usecase `TagTimeReport(owner, range, nodeSubtree?) → []{Tag, Σdur}`:
  ```sql
  SELECT t.slug, sum(coalesce(ws.stop_at, now()) - ws.start_at) AS dur
  FROM work_sessions ws
  JOIN taggings tg ON tg.taggable_type='work_session' AND tg.taggable_id = ws.id
  JOIN tags t ON t.id = tg.tag_id
  WHERE ws.owner_id = $own [AND ws.start_at >= $from AND ws.start_at < $to]
  GROUP BY t.slug ORDER BY dur DESC
  ```
- Minimaler Surface: CLI `flow session stats --by-tag [--from --to]` + eine Zeile/Sicht in der Worktime-Stats-TUI (kein großes neues Screen-Konstrukt).

**Kontext-Primitiv für B3:** `FilterIDs(..., TagOr)` über `taggable_type='document'` ist die Cross-Node-Tag-Query, die der Bootstrap-Compose (B3) konsumiert. B2 baut das neutrale Primitiv; **B3 begrenzt** die Reichweite auf global-getaggtes (D7).

## 8 · UI — „Tagging überall" (WebUI + TUI)

Heute gibt es **keinen** Tag-Editor (Tags kamen aus Frontmatter). B2 braucht überall **Setzen + Filtern**:

**WebUI** (`internal/adapter/webui/`, templ + htmx; `ui/chip`/`ui/badge`/`kindcolor`):
- **Tag-Editor** auf Doc-, Node-, Session-Edit-Form: Chip-Input (Tags hinzufügen/entfernen), Autocomplete aus `ListTags`. Submit schickt `tags []string`.
- **Filter-Bar** (Wissen/`wissen.templ`) bleibt, jetzt junction-gestützt; Tag-Chips auch an Node-Listen + Worktime-Historie.
- Bestehende Display-Chips (`components/chip.templ` `Tag`) wiederverwenden.

**TUI** (`internal/tui/`, shell-Routen; `theme`/`ui`, `ui/fuzzylist`):
- **Tag-Editor-Overlay** (add/remove via Fuzzy-Picker mit MRU + inline-create, `ui/fuzzylist`) auf Doc-/Node-/Session-Detail.
- Filter-Overlay (`docs.go` `f`, `:692`) bleibt; `loadTags` (`:278`) nutzt `TagScope` (Typ/Subtree) statt owner-weit → Cross-Project-Bug weg.

**Glyph/Farbe:** monospace, Whitelist; keine Emoji ([[feedback_no_icons]]). Tag-Chips neutral (`#slug`); kind-Badges weiter via `kindcolor`.

## 9 · Datei-Änderungs-Karte (für den Plan)

- **domain:** neu `tag.go` (`Tag`, `TaggableType`, `TagMatch`, `TagScope`, `NormalizeTag`); `frontmatter.go` (Normalisierung umziehen, `ParseFrontmatter` bleibt für Migration/Import, `tags` aus Vollparse); `document.go` (`Tags` bleibt, Quelle ändert sich); `worksession.go` (`Tag string` → entfällt, Sessions hydrieren `Tags []string`).
- **ports:** `ports.go` — neu `TagStore`; `DocumentStore.List/ListPage/Search/SemanticSearch` Tag-Filter auf Junction; `SessionStore`-Schreibsignaturen (`tag string` → `tags []string`).
- **pgstore:** neu `tags.go` (Registry + Junction + Filter + Hydration + Merge); `documents.go` (`docCols` ohne `tags`, Hydration via `TagsForMany`, Filter-Subquery); `worksessions.go` (kein `tag`-Scan, Hydration); Migration `0019` (+ ggf. `0020`).
- **usecase:** neu `set_tags.go`, `tag_time_report.go`; `list_tags.go` (registry-gestützt); `create_document.go`/`update_document.go`/`import_document.go` (kein Frontmatter-Parse, `SetTags`); `start_session.go`/`add_session.go`/`edit_session.go` (`tags`); Delete-Usecases (`ClearTaggable`).
- **adapter/httpserver:** `documents.go`/`worktime.go` DTOs + Filter; `server.go` Wiring `TagStore`.
- **adapter/apiclient:** `documents.go` + Session-Inputs `Tags`.
- **cmd/flow-mcp:** `tools_write.go` (`Tags` in create/update), `tools_docs.go` (`flow_list_tags` Scope).
- **cmd/flow:** `docs_import.go` (clientseitiger Vollparse + strip), `session.go` (`--tag` → `--tags` repeatable; `stats --by-tag`).
- **adapter/webui:** Tag-Editor (Doc/Node/Session-Forms), Filter junction-gestützt, Autocomplete.
- **internal/tui:** Tag-Editor-Overlay (Doc/Node/Session), `loadTags` Scope-Fix.

## 10 · Testing-Strategie

TDD durchgängig ([[feedback_plan_main_wiring_task]] — finaler Wiring-Task mit curl-Smoke jeder neuen Route/Param; [[feedback_subagent_git_commits_isolated]] — HEAD nach jedem Subagent prüfen):
- **Domain:** `NormalizeTag`/`NormalizeTags` (Edge: Casing, Dedup, Leerstring, Unicode), `TagMatch`/`TagScope`-Defaults — pure Tests.
- **pgstore (Docker):** `UpsertTags` (find-or-create, display-first-write-wins), `SetTags`-Diff (attach/detach), `FilterIDs` (AND `HAVING count`, OR), `TagsForMany` (Batch-Hydration), `ClearTaggable`, `MergeTags`; **Migration 0019** (Frontmatter→taggings + extra-Erhalt + Body-Strip + Idempotenz; `work_sessions.tag`→taggings; Spalten-Drop) auf gemischtem Seed-Korpus; CHECK auf `taggable_type`.
- **usecase:** `SetTags`/`GetTags`/`ListTags`/`TagTimeReport` via Fakes; Cutover (Create/Update parst kein Frontmatter mehr).
- **REST:** httptest create/update/import/list mit `tags`; `?tag=`-AND-Filter; Session-`tags`; `/documents/tags` registry-scope.
- **apiclient/webui(templ)/tui(fake-apiclient)/cli(cmd):** je Layer (Tag-Editor rendert/sendet; Filter-Overlay scope).
- **Done-Gate:** `make ci` grün (Coverage-Gate halten) + Live-Smoke vs. Postgres+Dex: Doc mit `tags`-Param anlegen (kein Frontmatter im Body), Filter zieht, Session taggen + `TagTimeReport`, Migration gegen Bestands-Snapshot verifizieren (Bodies sauber, `extra` befüllt, taggings korrekt), Browser/TUI-Tag-Editor dogfood.

## 11 · Abhängigkeiten / Reihenfolge / Slicing

B2 ist quasi-unabhängig von B1 (nutzt nur `node` als Taggable-Typ + optional `Ancestors` für Subtree-Scope) und liefert das Tag-Match-Primitiv für **B3**. Empfohlener Schnitt (der Plan macht Tasks daraus):

```
(a) Store+Domain+Migration   tags/taggings, TagStore-pgstore, Daten-Fixup, Hydration   → pgstore-Docker-Gate
(b) API-Cutover              tags-Param MCP+REST, kein Frontmatter-Write, ListTags,     → curl-Smoke
                             Vault-Import-Einpfad
(c) Consumer                 Worktime-Session-Tags + TagTimeReport; B3-Filter-Primitiv  → usecase/REST-Test
(d) UI                       Tag-Editor + Filter WebUI+TUI (Docs/Nodes/Sessions)        → templ/tui-Test
(e) Wiring + Done-Gate       Composition-Root, curl jede Route, Live-Dogfood            → make ci + Postgres+Dex
```

**Branch:** Slice-Branch von `rebuild` (z.B. `b2-tag-system`), am Ende auf `rebuild` integriert (unmerged) — wie B1. Worktree-Wahl beim Plan-Start (aktuell: Arbeit gehört in `flow-rebuild`).
