# flow rebuild M2b — Wikilinks & Backlinks (Design)

**Date:** 2026-06-15
**Branch:** `rebuild` (long-lived orphan, not merged)
**Predecessor:** M2a Document-Spine (`15046c6..8f24790`)
**Status:** Design approved — ready for implementation plan.

## Goal

Turn the M2a document store into a navigable compendium: documents reference
each other with `[[path]]` wikilinks, and each document shows which other
documents link to it ("Referenced by" / backlinks). Available in **both hosts**
(WebUI + TUI), following the generic-in-every-host principle.

This is the second slice of the M2 Kompendium vertical. Tags/Filter (M2c),
Search (M2d), and pgvector (M2e) come later and are out of scope here.

## Core decisions (locked during brainstorming)

1. **Reference key:** documents are addressed by their hierarchical `Path` slug.
2. **Backlink index:** a dedicated `document_links` table, maintained on
   create/update. (Not scan-on-read, not `extra`-JSONB.)
3. **Resolution scope:** **scope-isolated** — a `[[path]]` resolves first
   within the source document's own project scope, then to free/owner-level
   documents, and otherwise renders broken. A project document never links into
   a *foreign* project, even when the target slug is owner-wide unique. Cross-
   project references must go through a shared free document on purpose.
4. **Link extraction:** performed in the use case after persist (store stays a
   thin CRUD port). Non-atomic by design; a re-save heals any drift.
5. **TUI link behaviour:** wikilinks navigate **in-TUI** (focus cursor + Enter +
   back-stack); real weblinks open in the **OS default browser**.

## Architecture

Vertical slice mirroring M2a: **domain → store/migration → usecase → REST+SSE →
apiclient → WebUI → TUI**. Each unit below has one purpose and a defined
interface.

### 1. Domain — syntax & resolution authority

`internal/domain/wikilink.go` — pure, no adapter deps. The single place that
knows the wikilink syntax and the resolution rule.

- `type WikilinkSpan struct { Start, End int; Target, Display string }`
- `FindWikilinks(s string) []WikilinkSpan` — scans `[[target]]` and
  `[[target|display]]`. Aborts a candidate at a newline (a wikilink never spans
  a line break). Empty target → not a match. Returns byte offsets so the TUI can
  slice a line into styled segments.
- `WikilinkTargets(body string) []string` — ordered, de-duplicated target paths,
  for the link table.
- `ResolveWikilink(src Document, target string, all []Document) (Document, bool)`
  — scope-isolated:
  1. exact `path == target` within the **same project scope** as `src`
     (same `ProjectID`, treating nil as the free scope);
  2. else a free/owner-level document (`ProjectID == nil`) with `path == target`;
  3. else → `ok = false` (broken). A foreign-project match never resolves, even
     if it is owner-wide unique.

  `all` is the owner's document set; only `ID/Path/Title/Type/ProjectID` are
  needed (no bodies).

**Display fallback** (shared by both hosts): explicit `|display` wins; else the
resolved target's `Title`; else the raw target path.

### 2. Storage — `document_links` table

Migration `0007_document_links.sql`:

```sql
CREATE TABLE document_links (
    src_doc_id  UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    owner_id    TEXT NOT NULL,
    target_path TEXT NOT NULL
);
CREATE INDEX document_links_lookup ON document_links (owner_id, target_path);
CREATE INDEX document_links_src ON document_links (src_doc_id);
```

No `resolved_doc_id` — resolution is dynamic (the doc set changes; the
scope-aware tiebreak is applied at query time). `ON DELETE CASCADE` removes a
document's outbound rows when it is deleted; inbound references from other
documents simply stop resolving (rendered broken).

`ports.DocumentStore` gains:

- `ReplaceLinks(ctx, srcDocID, ownerID string, targets []string) error` —
  delete-then-insert the outbound links of one document.
- `Backlinks(ctx, ownerID, targetPath string) ([]Document, error)` — join
  `document_links` ↔ `documents`, returning candidate **source** documents whose
  link rows match `target_path` (with the fields needed to re-resolve).

`pgstore` + `FakeDocumentStore` both implement these.

### 3. Use cases

- `create_document` / `update_document`: after the document is persisted, call
  `domain.WikilinkTargets(body)` → `store.ReplaceLinks(docID, owner, targets)`.
- New `internal/usecase/backlinks.go` — `Backlinks(ctx, owner, docID)`:
  1. load the document → its `Path` and scope;
  2. `store.Backlinks(owner, path)` → candidate source docs;
  3. load `store.List(owner)` and keep only candidates where
     `ResolveWikilink(candidate, path, allDocs).ID == thisDoc.ID` (filters
     false-positives caused by scope collisions);
  4. return `[]BacklinkRef{ ID, Path, Title, Type }`.

`BacklinkRef` lives in the usecase (or domain) and is reused by REST/apiclient.

### 4. REST + SSE

- `GET /api/v1/documents/{id}/backlinks` → `200` `[]BacklinkRef`; `404` if the
  document is gone. Registered behind `s.auth` next to the other document routes.
- **No new SSE event type.** Backlink sets and forward-link validity change only
  when some document is created/updated/deleted, which already emits
  `document.created|updated|deleted`. Both hosts re-fetch on those events.

### 5. apiclient

`Backlinks(ctx, id string) ([]BacklinkRef, error)` calling the new endpoint.

### 6. WebUI

- `RenderDocument(body string, resolve func(target string) (href, title string, ok bool)) template.HTML`
  — a sibling of `RenderMarkdown` (plain callers untouched). A goldmark
  wikilink extension (inline parser + HTML renderer, ported from the old
  kompendium AST node) emits:
  - valid → `<a class="wikilink" href="/docs/{id}">→ display</a>`
  - broken → `<span class="wikilink-broken">⊘ display</span>`

  The bluemonday policy is extended to allow `class` on `a`/`span` and the
  `/docs/`-relative href. Regular markdown links/URLs render natively as
  `<a href>` (browser-external).
- `handleWebDocView`: fetch the document + `List(owner)`; build the resolver
  (path → `{id, title, ok}` via `ResolveWikilink`, href `/docs/{id}`); render the
  body via `RenderDocument`; call the backlinks use case; render a
  **"↩ Referenced by"** section. Existing SSE live-refresh re-renders the view.
- `docs.templ`: backlinks footer + wikilink CSS (valid/broken, Tokyonight
  semantic colours, no emoji).

### 7. TUI — in-TUI wikilink nav + browser weblinks

`internal/tui/docs.go`, `modeView`:

- `renderView` styles each body line by kind, using `domain.FindWikilinks` plus
  a weblink scanner (markdown `[text](http…)` and bare `http(s)://` URLs):
  - **wikilink** valid → `WikilinkValid`; broken → `WikilinkBroken` (`⊘`);
  - **weblink** → `WebLink` style + OSC-8 hyperlink (cmd-click opens browser).
- **Focus cursor:** `Tab` / `Shift+Tab` move a focus index over the ordered link
  set = forward wikilinks (body order, de-duped per target doc) + backlinks +
  weblinks. The focused link gets an extra highlight style.
- **`Enter`** acts by link kind:
  - wikilink → load the target document into the view, pushing the current
    document ID onto a back-stack (`viewStack []string`);
  - weblink → open in the OS default browser via the opener helper.
- **`Esc`** pops the back-stack (return to the previous document); empty stack →
  return to the list.
- `e` (edit) unchanged. Footer:
  `tab/⇧tab Link · enter folgen/öffnen · e edit · esc zurück · q quit`.
- The resolver is built from the already-loaded `m.docs` list.
- **Opener:** `internal/adapter/opener` (`open` on darwin, `xdg-open` on linux),
  exposed to the TUI via a small tui-local interface so tests pass `nil`
  (like `docEditor`). Composition roots inject the real one.
- New styles: `WikilinkValid`, `WikilinkBroken`, `WebLink`, plus a focus
  highlight. Glyphs: `→` (valid wikilink), `⊘` (broken), `↩` (backlinks header).

## Deliberate decisions / known limits

- **Glyphs** `→ ⊘ ↩` are plain Unicode (A11y-conform, no colour emoji).
- **TUI code fences:** the line-based scanner styles `[[…]]` even inside
  ```` ``` ```` blocks (WebUI goldmark does not). Minor; documented, not fixed.
- **Non-atomic link extraction:** a crash between document persist and
  `ReplaceLinks` could leave stale links; a re-save heals it. Acceptable for a
  personal tool.
- **Backlink precision:** the link table is a fast candidate filter; the
  use case re-applies `ResolveWikilink` so scope collisions never produce false
  backlinks.

## Testing & gate

- domain: `FindWikilinks` / `WikilinkTargets` / `ResolveWikilink` table-driven —
  pipe display, newline abort, nested/adjacent brackets, empty target,
  scope isolation (same-scope wins, free fallback, foreign-project → broken).
- store: `ReplaceLinks` (insert/replace/empty) + `Backlinks` against the fake and
  pgstore.
- usecase: backlinks filtering (candidate that resolves elsewhere is dropped).
- REST: `/backlinks` 200 shape + 404.
- WebUI: `RenderDocument` valid/broken/escaping; bluemonday still strips XSS.
- TUI: per-line styling segments; focus cursor movement; Enter dispatch
  (wikilink load vs weblink open) with a fake opener; back-stack/Esc.
- Migration `0007` applied against real Postgres in the done-gate.
- `make ci` green, coverage ≥ 80 % gate.

## Done-gate (live, like M2a)

Dev stack (Postgres + Dex), migration 0007 applied. curl-smoke the
`/backlinks` endpoint. Browser dogfood: create `a` linking `[[b]]`, view `b` and
see `a` under "Referenced by"; broken link renders broken; real URL opens
external. TUI dogfood: Tab through links, Enter follows a wikilink (Esc walks
back), Enter on a weblink opens the browser. User confirms before M2c.
