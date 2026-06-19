# Design: docs "Kompendium-Look" + TUI Design-Sprache

- **Date:** 2026-06-19
- **Branch:** `rebuild`
- **Status:** Approved (brainstorm) — pending written-spec review
- **Topic:** Restore the rich kompendium list look in `flow docs`, dissolve the docs legacy-styling island, and establish a reusable visual design language for the wider TUI.

## 1. Context & Problem

The current `flow docs` list (`internal/tui/docs.go`, `renderList` at ~line 1094) renders a flat,
single-line-per-row list: `▸ daily  daily/2026-06-19  Title`. No date, no kind badge, no body
excerpt, no count header, no pagination. It uses the hardcoded `col*` constants from
`internal/tui/styles.go`, not the semantic `theme.Sem()` layer.

The old kompendium browse view (on the `main` branch, `internal/kompendium/frontend/tui/browse/`)
looked much richer: a count header (`kompendium — 24/24 Notizen · ● 10 täglich ◆ 9 projekt ○ 5 frei`),
a project chip, per-row `date + colored kind badge (TÄGL./PROJ./FREI) + 2-line body excerpt + left
cursor stripe`, and a paginator. The user wants that look back.

Beyond docs, the user finds the TUI overall "dröge" (dull) and wants a cleaner, modern, slightly
more colorful look aligned to the existing Tokyonight-Night shell colorscheme.

### Inventory findings (current `rebuild`)

- **One isolated legacy-styling island** — 5 files in top-level `internal/tui/` consuming
  `styles.go` `col*`/`style*`: `docs.go` (mixed: view mode already modern via `markdown_overlay`,
  list/filter/search legacy), `worktime.go`, `stats.go`, `dayoffs.go`, `export.go`.
- The latter four are **dead/parallel code** — already replaced by the modern `internal/tui/screen/worktime/*`
  routes. (Deletion is verification-gated; see §9.)
- Everything else (`shell/`, `home`, all `screen/*`, the `screen/docs` adapter) is modern: built on
  `theme.Palette` / `theme.Sem()` — which *is* the Tokyonight-Night scheme as a semantic layer.
- The design system (`internal/tui/ui/*`) is deliberately **hand-rolled** (M3a verbatim port of the
  liked `main` design). Of `charm.land/bubbles/v2`, only `textinput`, `viewport`, `key` are used;
  `list`, `table`, `paginator`, `help`, `spinner` are unused.

### Key insight

"Native bubbles v2" is **not** what makes a TUI feel polished — bubbles components are unstyled and
need the same lipgloss/theme work our hand-rolled ones already have. "Dröge" is a **visual-design**
problem (color, badges, hierarchy, whitespace, excerpts), not an architecture problem. We therefore
**polish the existing design system** rather than rewrite onto native components.

## 2. Goals

- Restore the kompendium richness in `flow docs`: count header, colored kind badges, date cell,
  2-line body excerpt, selected-row cursor stripe, paginator.
- Add a **project chip + project filter** to docs (resolve project names; active-filter chip in the
  header using the project's own color/glyph).
- Migrate `docs.go` off `styles.go` `col*` onto `theme.Sem()` / builders.
- Extract a small set of **reusable `ui/` components** ("the design language") so the other screens
  can inherit the same look in follow-up plans.
- Keep all existing docs behavior working (view/fullscreen markdown, edit via `$EDITOR`, new, delete,
  tag filter, search, wikilink nav).

## 3. Non-Goals (this effort)

- No rewrite onto native `bubbles/v2` `list.Model`/`table.Model`. Native bubbles are used only where
  they clearly win and aren't yet used: the **paginator dots** (`bubbles/v2/paginator`) and existing
  `viewport` scroll.
- No visual refresh of the already-modern screens (Heute/Woche/Stats/Frei/Home) — they inherit the
  new components via **separate follow-up plans**.
- No server/REST changes. Project filtering is client-side (bodies + project list already available).
- No strict "only this project" exclusive filter mode (YAGNI; daily/free stay visible — see §7).

## 4. Visual Design (docs flagship)

```
 kompendium — 24/24 Notizen   ·   ● 10 täglich  ◆ 9 projekt  ○ 5 frei
 ⟨ serverkraken/flow ⟩                                    ← aktiver Projekt-Filter (Project.Color bg)
 ─────────────────────────────────────────────────────────────────────────

 notizen

 ▎ 2026-05-18   TÄGL.   daily/2026-05-18
   First on-call Schedule daily DPH-3658: Onboard team "Tech - ACT -
   AdBiz Tech" and product "RTTV" still missing EKS DPH-3776: New EKS…

   2026-06-09   PROJ.   serverkraken/flow · template-apps-demo-fastapi
   ● Memory-Bank-Sync durch. Kurzbericht für dich: Datei …

   2026-06-10   FREI    notes/foo
   (kein Text)

 ●○○○○○○○  1/24
 ─────────────────────────────────────────────────────────────────────────
 k/↑ hoch · j/↓ runter · enter öffnen · e bearbeiten · n neu · p projekt · f filter · / suchen · D löschen
```

### Colors (all via `theme.Sem()` / Tokyonight-Night)

- **Kind badges** (bg color + bold foreground = palette bg):
  - `TÄGL.` → `Sem.Accent` (blau)
  - `PROJ.` → `Sem.Success` (grün)
  - `FREI` → `Sem.Highlight` (magenta)
  - `AGENT` → `Sem.Warning` (the rebuild's 4th `DocumentType`, absent in old kompendium)
- **Count glyphs** `● ◆ ○` (and an `AGENT` glyph) use the **same** color as their badge — guaranteed
  by a single `kindcolor` mapping (§6).
- **Cursor stripe** `▎` on the selected row → `Sem.Active`.
- **Project chip** `⟨ … ⟩`: when a filter is active, background = `Project.Color` with `Project.Glyph`
  prefix; when inactive, hidden or dim "alle Projekte".
- **Date cell:** `daily` → `Document.Date`; others → `Document.UpdatedAt`.

## 5. Reusable components ("the design language")

Each is a small, focused `ui/` package (no monoliths) so other screens inherit the look later:

| Package | Purpose | First consumer |
|---|---|---|
| `ui/badge` | colored kind label from a Sem color | docs (later worktime/stats) |
| `ui/countbar` | `n/m Notizen · ● x täglich ◆ y …` counts line | docs (generic for any list) |
| `ui/listrow` (or extend `ui/picker`) | multi-line row: stripe + date + badge + title + excerpt | docs (template for other lists) |
| `ui/chip` | `⟨ label ⟩` filter/context chip | docs project filter |
| (paginator) | native `bubbles/v2/paginator` for `●○○ 1/24` | docs |

Decision: prefer **extending `ui/picker`** with a multi-line variant over a brand-new `ui/listrow`
package if the extension stays clean; otherwise add `ui/listrow`. The plan phase picks based on what
keeps `picker` cohesive.

## 6. `kindcolor` — single source of truth

A central mapping `DocumentType → (Sem color, count glyph, badge label)` lives in one place (mirrors
the old `main` `kind_color` adapter). Both the badge and the count glyph read from it, so a badge and
its count glyph can never drift in color. Exact location decided in the plan (candidate:
`internal/tui/ui/kindcolor` or a small map in the docs screen package if not reused yet — but since
countbar+badge both need it, a shared package is preferred).

## 7. Project filter (data flow + semantics)

**Data:** On `reload()`, `DocsModel` additionally calls `apiclient.ListProjects(ctx)` (parallel to
`ListDocuments`) and builds `projByID map[string]domain.Project`. This mirrors the existing worktime
Today resolution (`internal/tui/screen/worktime/today_state.go`). No server change.

**Per-row display:** `PROJ.` rows render `slug · title` (e.g. `serverkraken/flow · template-apps-demo-fastapi`).
Daily/free unchanged (`daily/2026-05-18`, `notes/foo`).

**Interaction:**
- Key **`p`** opens a project picker (reuses the `picker.Row` `j/k`+Enter pattern from the booking
  dialog), with a top **"Alle Projekte"** entry to clear.
- Selecting a project sets an active project-filter state; the list is filtered **client-side**.
- **Filter predicate (inclusive):** a doc is visible iff `ProjectID == selected` **OR** `ProjectID == nil`.
  Effect: "this project's notes + my global journal (daily/free)"; only *other* projects' notes are
  hidden. Rationale: daily notes carry the real project context but are typed `daily` with nil
  ProjectID, so excluding them would hide the most relevant journal.
- Header chip becomes active (`Project.Color` bg, `Project.Glyph` prefix). The count line reflects the
  visible (filtered) set: `m/24`.
- Composes with the tag filter (`f`) via **AND** and with search (`/`).
- `c` / Esc in the picker → back to "Alle" (filter cleared).

## 8. Rendering approach & testability

- **Rows:** hand-rolled multi-line rows via the reusable component (§5), not `list.Model`. The
  badge + date + 2-line excerpt layout is simpler with our `picker`-style rendering than with a
  `list.Model` custom delegate.
- **Paginator:** native `bubbles/v2/paginator` for the `●○○ 1/24` indicator; page size derived from
  available height. (Replaces today's flat, unpaginated scroll.)
- **Pure helpers for tests:** view-model logic (filter composition, count computation, excerpt
  building/truncation, date-cell selection, kind→color/label/glyph) is extracted into pure functions
  so it is table-testable without driving the bubbletea loop.

## 9. Legacy cleanup (verification-gated)

1. Migrate `docs.go` fully off `styles.go` (`col*`/`style*`) → `theme.Sem()` / builders.
2. After (1), re-check `styles.go` consumers. Expected remaining: only the 4 dead files
   (`worktime.go`, `stats.go`, `dayoffs.go`, `export.go`).
3. **Before deleting any of them**, verify `cmd/flow/main.go` wiring. If `flow worktime` / `stats` /
   etc. standalone subcommands still route to the legacy `Model`, those files **stay** (their
   migration/removal is a separate follow-up plan). Only files that are **provably unreferenced** are
   deleted in this effort, together with the now-unused parts of `styles.go`.
4. No "delete boldly." Each deletion is gated on `make ci` staying green and no broken wiring.

## 10. Behavior to preserve (regression checklist)

- `enter` → open/view (fullscreen markdown viewer, M3d) with wikilink nav (`Tab`/`Shift+Tab`/`Enter`).
- `e` → edit via `$EDITOR` (`tea.ExecProcess`).
- `n` → create (type cycle, title, editor launch).
- `D`/`d` → delete confirm.
- `f` → tag filter overlay; `/` → search mode with `<mark>`-style hit highlight.
- SSE live-reload of the list (`document.*` events).
- Works both standalone (`flow docs`) and as the `flow ui` Wissen/Docs tab (via `screen/docs` adapter).

## 11. Testing strategy

- Unit/render tests for each new `ui/` component (`badge`, `countbar`, `chip`, row variant) and the
  `kindcolor` mapping — string assertions in the style of existing `ui/` tests.
- Table tests for the extracted pure helpers (filter predicate incl. project+tag AND, counts,
  excerpt truncation, date-cell selection).
- `make ci` green, coverage gate respected.
- Manual done-gate: live dogfood against the Postgres+Dex dev stack — confirm kompendium look, badge
  colors, project chip+filter (incl. daily/free staying visible), paginator, and that view/edit/new/
  delete/tag-filter/search/SSE all still work.

## 12. Out of scope / follow-ups

- Visual refresh of Heute/Woche/Stats/Frei/Home using the new components — separate plan(s).
- Migration or deletion of any legacy file still wired as a standalone subcommand — separate plan.
- Strict "only this project" exclusive filter toggle — only if requested later.

## 13. Resolved decisions (no open questions)

- Direction: polish the hand-rolled design system (not native `list.Model`). ✔
- Scope: docs flagship + reusable design language; other screens via follow-ups. ✔
- Project chip: full display + client-side project filter. ✔
- Filter semantics: inclusive — daily/free always visible. ✔
