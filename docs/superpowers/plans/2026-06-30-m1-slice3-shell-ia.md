# M1 Slice 3 — Shell & IA-Reframe — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Reframe the WebUI navigation to be project-centric: `/` becomes **Home**, the worktime detail moves to **`/zeit`**, the **Stats page is removed** (Tagesziel-Editor → new **Einstellungen** page; saldo/burndown → Home in Slice 4), the **Export** page is lifted onto the standard shell, the sidebar gains a **project-tree spine** + a working **mobile "More"** menu, and the structural dead-wood (`nav.templ`, `CategoryStrip`, `Glyph`) is deleted.

**Architecture:** Server-rendered templ + htmx. Routes registered in `internal/adapter/httpserver/server.go`; pages in `internal/adapter/webui/*.templ`; shared chrome in `internal/adapter/webui/components/`. Reuse the existing `BuildTree`/`ListNodes` for the sidebar tree. No backend/usecase changes except a new Einstellungen WebUI handler that reuses the existing `SetTarget` usecase.

**Tech Stack:** Go, templ v0.3.857 (`make generate`), Tailwind v4.1.5 (`make web`). `make ci = lint verify-generate verify-css verify-no-popups cover build`.

## Global Constraints
- **Build discipline:** every `.templ` edit → `make generate` → commit the `_templ.go`; every `web/tailwind.css` edit → `make web` → commit `app.css`. NO color-emoji (geometric marks ▶◆●⌂ ok); NO `alert/confirm/prompt`.
- The full per-file touchpoint detail is in `.superpowers/sdd/slice3-touchpoints.md` — read it for any file not fully specified in a task.
- Each task leaves `go build ./...` + `go test ./internal/...` green. Transient cross-task states (a nav link to a route removed in a later task) are acceptable; the final wiring task verifies every route.
- All commands from `/Users/msoent/SourceCode/serverkraken/flow-m1`.

---

### Task 1: i18n nav keys
**Files:** `internal/i18n/catalog_de.go`, `internal/i18n/catalog_en.go`.
- [ ] Add `"nav.home": "Home"` and `"nav.zeit": "Zeit"` to both catalogs (DE: "Home"/"Zeit"; EN: "Home"/"Time").
- [ ] Drop `"nav.knowledge"` from both (dead alias for `nav.wissen` — confirm `rg -n "nav.knowledge" internal` shows no live use first; if used, leave it).
- [ ] `go build ./... && go test ./internal/i18n/...` → green (the i18n completeness test, if any, must still pass — both catalogs must have the SAME keys).
- [ ] Commit `feat(i18n): nav.home + nav.zeit keys; drop dead nav.knowledge`.

---

### Task 2: Rename node-picker VM types (naming leak)
**Files:** `internal/adapter/webui/components/worktime_vm.go`, callers `webui_heute.go`, `webui_historie.go`, `heute_vm.go`, `historie_vm.go` (+ regenerated `_templ.go` for any `.templ` referencing them).
- [ ] Rename `FuzzyPickerVM`→`NodePickerVM` and `FuzzyProjectVM`→`NodePickerItem` (`worktime_vm.go:65-79`) and update the comment ("projectId/newProject" → "node id `n`/new node"). Use `rg -n "FuzzyPickerVM|FuzzyProjectVM" internal` to find ALL references; rename every one. If any `.templ` references the type, `make generate` after.
- [ ] `go build ./... && go test ./internal/adapter/...` → green.
- [ ] Commit `refactor(webui): NodePickerVM/NodePickerItem (node-consistent naming)`.

---

### Task 3: Delete dead components (`CategoryStrip`, `Glyph`)
**Files:** delete `components/categorystrip.templ`(+`_templ.go`), `components/glyph.templ`(+`_templ.go`); edit `components/coverage_test.go`.
- [ ] Confirm dead: `rg -n "CategoryStrip|@Glyph|components.Glyph" internal` shows only the defs + `coverage_test.go`. (If a live caller appears, STOP/report.)
- [ ] Delete the four files. In `coverage_test.go`, remove the `Glyph(...)`/`CategoryStrip(...)` render calls (and any now-unused imports). 
- [ ] `make generate` (ensure no stale `_templ.go` references), `go build ./... && go test ./internal/adapter/webui/...` → green.
- [ ] Commit `chore(webui): delete dead CategoryStrip + Glyph components`.

---

### Task 4: Move worktime to `/zeit` (rename route + drop Stats subtab)
**Files:** `httpserver/server.go` (route), `httpserver/webui_heute.go` (handler name), `webui/heute.templ` (active key + subnav), the `worktimeSubnav` helper (find it: `rg -n "func worktimeSubnav|worktimeSubnav\("`).
- [ ] In `server.go`: change `mux.Handle("GET /{$}", s.webAuth(http.HandlerFunc(s.handleHeuteHome)))` → `mux.Handle("GET /zeit", s.webAuth(http.HandlerFunc(s.handleZeitHome)))`. (Leave `/{$}` unregistered for now — Task 7 adds Home there. Between tasks, `/` 404s; acceptable — verified at Task 11.)
- [ ] In `webui_heute.go`: rename `handleHeuteHome` → `handleZeitHome` (keep its body; it renders the same page).
- [ ] In `heute.templ`: change `@components.AppShell("today", ...)` active key to `"zeit"`. In `worktimeSubnav`, DROP the Stats tab (`{... "stats" ...}`) so the strip is Heute·Woche·Historie·Frei. (Woche/Historie pages also call `worktimeSubnav` — they get the updated strip automatically; their own active keys stay.)
- [ ] `make generate`; `go build ./...`; `go test ./internal/adapter/...` → green (update any test asserting the old `GET /` worktime or the Stats subtab; note it).
- [ ] Commit `feat(webui): worktime detail moves to /zeit (drop Stats subtab)`.

---

### Task 5: Einstellungen page + Tagesziel-Editor
**Files:** new `webui/einstellungen.templ` + `webui/einstellungen_vm.go`; new `httpserver/webui_einstellungen.go`; `server.go` (routes). Reuse: the `statsTargetCard` markup from `stats.templ` and `handleWebSetTarget` logic from `webui_stats.go` (read both).
- [ ] **Build the Einstellungen page:** `EinstellungenPage(vm)` = `@components.Base("einstellungen", ...)` + `@components.AppShell("einstellungen", nil, nil, ...)`. Content: the Tagesziel-Editor (default daily target + Mon–Fri overrides — ported from `statsTargetCard`, POST → `/ui/einstellungen/target`) and the Bundesland selector (reuse the existing dayoffs Bundesland control pattern or the `POST /api/v1/settings/bundesland` path — keep minimal: target editor is the must-have).
- [ ] **Handlers** in `webui_einstellungen.go`: `handleWebEinstellungenHome` (renders the page; builds VM from current settings via the settings store/usecase) and `handleWebSetTarget` (ported verbatim from `webui_stats.go` — validate, call `s.SetTarget.Execute()`, publish `EventSettingsChanged`, re-render the Einstellungen target fragment).
- [ ] **Routes** in `server.go`: `GET /einstellungen` → `handleWebEinstellungenHome`; `POST /ui/einstellungen/target` → `handleWebSetTarget`.
- [ ] Test: a handler test that `GET /einstellungen` renders the target editor + `POST /ui/einstellungen/target` updates the target (match the existing webui handler-test harness).
- [ ] `make generate`; `go build ./...`; `go test ./internal/adapter/httpserver/ ./internal/adapter/webui/` → green.
- [ ] Commit `feat(webui): Einstellungen page with Tagesziel editor (moved from Stats)`.

---

### Task 6: Remove the Stats page
**Files:** delete `webui/stats.templ`(+`_templ.go`), `webui/stats_vm.go`; remove `handleWebStatsHome`, `handleWebStatsFragment`, `handleWebSetTarget` (moved in Task 5) + the `/stats`, `/ui/stats/fragment`, `/ui/stats/target` routes from `server.go` and delete `httpserver/webui_stats.go`.
- [ ] **Before deleting `webui_stats.go`:** it also defines `burndownBannerVM` + the saldo-tile VM builders (used by the Stats page). Check `rg -n "burndownBannerVM|statsSaldoTile|StatsVM" internal` — if ONLY the stats page uses them, delete with the file. (Home in Slice 4 will rebuild saldo from `StatsComputer`; do NOT preserve dead VM code here.) If `BurndownBanner` the COMPONENT (`components/burndownbanner.templ`) is referenced elsewhere, keep the component; only delete the stats-page VM glue.
- [ ] Remove the three `/stats*` route registrations from `server.go`.
- [ ] `make generate`; `go build ./...`; `go test ./...` → green (the I1 burndown-banner test from Slice 1 lived in a stats test file — if it's in `webui_stats_test.go`, MOVE that test to follow `BurndownBanner` wherever its VM now lives, or delete if the VM glue is gone; do NOT silently drop coverage of `BurndownBanner`. Report what you did with that test.)
- [ ] Commit `feat(webui): remove Stats page (Tagesziel→Einstellungen; saldo→Home in Slice 4)`.

---

### Task 7: Home route (`/`) — minimal landing
**Files:** new `webui/home.templ` + `webui/home_vm.go`; new/edit `httpserver/webui_home.go`; `server.go` route.
- [ ] **Minimal Home page:** `HomePage(vm)` = `@components.Base("home", ...)` + `@components.AppShell("home", nil, nil, homeContent(vm))`. Content (Slice-4 fills the real thing — keep MINIMAL but not empty): a Kristall hero card (`@components.Card`) with a greeting/heading ("Home"), and a row of `@components.Card` section links to **Zeit** (`/zeit`), **Wissen** (`/wissen`), **Projekte** (`/nodes`) — each a glass card the user can click. Add an HTML comment `<!-- Slice 4: timer-hero, saldo tiles, logstream, neueste Wissensartikel -->` marking where Slice 4 enriches. NO timer logic here (Slice 4).
- [ ] **Handler** `handleHomeHome` in `webui_home.go` (build a tiny `HomeVM` — e.g. the user's name/locale for the greeting). **Route** `server.go`: `mux.Handle("GET /{$}", s.webAuth(http.HandlerFunc(s.handleHomeHome)))`.
- [ ] Test: `GET /` renders the Home heading + the three section links (200).
- [ ] `make generate`; `go build ./...`; `go test ./internal/adapter/...` → green.
- [ ] Commit `feat(webui): Home landing at / (minimal; Slice 4 enriches)`.

---

### Task 8: Sitenav restructure + sidebar project-tree spine
**Files:** `components/sitenav.templ` (`PrimaryNav`/`SecondaryNav` + a new tree render), `components/appshell.templ` (desktop sidebar renders the tree), reuse `node_tree_vm.go` `BuildTree` + the `ListNodes` usecase. Read `sitenav.templ` + `appshell.templ` + `node_tree_vm.go` first.
- [ ] `PrimaryNav()` → `[]NavItem{ {"home","/","nav.home","⌂"}, {"docs","/wissen","nav.wissen","◆"}, {"projekte","/nodes","nav.projects","●"} }` (drop Stats). `SecondaryNav()` → `{ {"zeit","/zeit","nav.zeit","▶"}, {"frei","/dayoffs","nav.dayoffs","○"}, {"export","/export","nav.export","▰"}, {"einstellungen","/einstellungen","nav.settings","·"} }`. If `nav.stats` is now unused (`rg -n "nav.stats" internal` → only catalog), drop it from both catalogs.
- [ ] **Sidebar project-tree spine:** under the "Projekte" nav item in the DESKTOP sidebar (`SiteNav`/`appshell.templ`), render an expandable tree of the owner's nodes. Use CSS `<details>`/`<summary>` (NO JS): engagement nodes are `<summary>` (the toggle), their descendants nested `<details>`/links. Each leaf/node links to `/nodes/{id}`. Source the data: the AppShell needs the node list — thread it via a new optional `AppShell` param OR a sidebar-local fetch. SIMPLEST: add a `[]NavTreeNode` (built from `BuildTree`) to the data already passed to `AppShell` (the AppShell signature is `AppShell(active string, ???, subnav, content)` — read it; add the tree as a param or a context value). Keep the tree COLLAPSED by default (closed `<details>`); highlight the active node if the current route is `/nodes/{id}`. If threading the tree through AppShell is too invasive for this task, render a STATIC "Projekte" link only and note the inline-tree as a follow-up — but prefer the tree.
- [ ] `make generate`; `go build ./...`; `go test ./internal/adapter/...` → green (update nav tests for the new items).
- [ ] Commit `feat(webui): project-centric sidebar (Home·Wissen·Projekte + project tree + utilities)`.

---

### Task 9: Mobile "More" drawer
**Files:** `components/appshell.templ` (mobile bottom-nav), `components/sitenav.templ` (drawer content helper), reuse `webui/static/js/dialog.js` + the `dialog.templ` pattern.
- [ ] Mobile bottom-nav (4 cols): Home · Wissen · Projekte · **More**. The first three from the new `PrimaryNav()`; the 4th is a "More" button (geometric glyph, e.g. `≡`/`···`) opening a bottom-sheet `<dialog>` (or the existing dialog mechanism) listing `SecondaryNav()` items (Zeit/Frei/Export/Einstellungen) as full-width links.
- [ ] Ensure the drawer uses the existing `data-dialog-*` mechanism (no new `alert/confirm/prompt`). 
- [ ] `make generate`; `go build ./...`; `go test ./internal/adapter/webui/...` → green.
- [ ] Commit `feat(webui): mobile More drawer reaches Zeit/Frei/Export/Einstellungen`.

---

### Task 10: Lift Export onto the shell + delete `nav.templ`
**Files:** `webui/export.templ`, delete `webui/nav.templ`(+`_templ.go`), maybe `webui/export_vm.go`.
- [ ] Replace `export.templ`'s own `<!DOCTYPE html>`+`<head>`(CDN htmx)+`<body class="bg-slate-50 text-slate-800">` shell with `@components.Base("export", exportBody(d))` and `exportBody` = `@components.AppShell("export", nil, nil, exportContent(d))`. Remove the `@Nav("export", d.User)` call (AppShell provides nav). Replace `bg-slate-50 text-slate-800` with token classes (or rely on Base). Keep the `ExportFragment` content (the form + preview) intact inside `exportContent`.
- [ ] Delete `webui/nav.templ` + `nav_templ.go` (only caller was export). Confirm `rg -n "@Nav\(|func Nav\(" internal` is empty after.
- [ ] `make generate`; `go build ./...`; `go test ./internal/adapter/webui/...` → green.
- [ ] Commit `feat(webui): Export page on AppShell+tokens; delete nav.templ zombie`.

---

### Task 11: Wiring verification + done-gate
- [ ] `go build ./... && go vet ./...` → clean.
- [ ] `make web` + `git diff --exit-code static/app.css` → clean (commit if drift). `make generate` + `git diff --exit-code '*_templ.go'` → clean.
- [ ] **`make ci`** → green. If `cover` dips below the gate (this slice deletes the Stats page + adds minimal pages), report the exact %; do NOT add fake tests. If real lint/verify failures, report.
- [ ] **Route smoke (static check):** `rg -n "mux.Handle" internal/adapter/httpserver/server.go` — confirm registered: `GET /{$}`(Home), `GET /zeit`, `GET /einstellungen`, `POST /ui/einstellungen/target`, `GET /wissen`, `GET /nodes`, `GET /dayoffs`, `GET /export`, `GET /ui`; and NO `/stats*` remain. Confirm every `PrimaryNav`/`SecondaryNav` href has a matching route.
- [ ] **No dangling refs:** `rg -n "handleHeuteHome|handleWebStats|@Nav\(|CategoryStrip|@Glyph|FuzzyProjectVM|/stats" internal` → empty (all renamed/deleted).
- [ ] **Manual done-gate (human, vs dev stack):** `/` = Home; `/zeit` = worktime (Heute·Woche·Historie, no Stats tab); `/einstellungen` = Tagesziel editor (saves); `/stats` = 404; `/export` = styled like the rest; sidebar shows the project tree; mobile "More" reaches the utilities.
- [ ] Commit any fixups.

---

## Self-Review (done)
1. **Coverage vs spec §3/§12:** Home route (T7), project-tree sidebar (T8), Wissen stays global, Zeit utility (T4), Stats removed (T6), Export lifted (T10), nav.templ/dead components deleted (T3,T10), nav-activation + mobile reachability (T8,T9), `/einstellungen` real page (T5), naming leaks (T2), i18n (T1). Badge/Breadcrumb unification = NON-issue (no duplicates — verified in the map), so not a task.
2. **Placeholders:** routes, file deletions, and the i18n/naming changes are exact; the two genuinely-new pages (Einstellungen T5, Home T7) port existing markup (`statsTargetCard`) or are deliberately minimal (Home), with the Slice-4 boundary marked. The sidebar-tree threading (T8) names the concrete reuse (`BuildTree`/`ListNodes`) and a named fallback if AppShell-threading is too invasive.
3. **Ordering keeps build green:** dead-deletes (T2,T3) isolated; `/zeit` (T4) frees `/`; Einstellungen (T5) before Stats-removal (T6); Home (T7) reclaims `/`; nav (T8) after its targets exist; export-lift (T10) after nav.templ's only caller is gone; verify (T11) catches any transient dangling route.

## Notes for the executor
- The saldo tiles + burndown banner disappear from the UI between this slice and Slice 4 (they had no home once `/stats` is gone). This is intentional and noted in the done-gate. The `BurndownBanner` COMPONENT stays for Slice 4 to reuse — only the stats-page VM glue is deleted.
- T8's sidebar tree is the slice's signature; give it real effort, but the named fallback (plain "Projekte" link) keeps the slice shippable if AppShell-threading balloons.
