# Design: Worktime Legacy Removal + Reusable Fuzzy/MRU Project Picker (Phase 2)

- **Date:** 2026-06-20
- **Branch:** `rebuild`
- **Status:** Approved (brainstorm) — pending written-spec review
- **Topic:** Remove the last legacy `tui.New(...)` worktime Model (and `styles.go`), make standalone `flow worktime` a worktime-only modern shell, and add a reusable fuzzy/MRU project picker used by both the worktime booking dialog and the docs project filter.
- **Predecessor:** Phase 1 = [docs kompendium-look + design language](2026-06-19-flow-rebuild-docs-kompendium-look-design.md).

## 1. Context & Problem

After Phase 1, `docs.go` is fully on `theme.Sem()`, but the legacy `internal/tui` top-level package still contains the old `tui.New(...)` worktime Model — `worktime.go` + `stats.go` + `dayoffs.go` + `export.go` — plus `styles.go` (hardcoded `col*` Tokyonight hex + `style*` vars). This Model is the *only* remaining consumer of `styles.go`, and it is wired solely by `cmd/flow/worktime.go:28` (`tui.New(client, os.Getenv("USER"))`) for the standalone `flow worktime` TUI. The modern world (`shell` + `screen/worktime/*` routes) already covers the same screens (Heute/Woche/Stats/Frei/Export) and is used by `flow ui`.

The user wants the legacy fully gone ("alles sauber und neu"). Removing the legacy Model gets the whole TUI off `col*`. Separately, the user's primary worktime workflow wants a project picker with **MRU sort + fuzzy filter + inline create** — which neither the legacy nor the current modern booking dialog has (both are plain `j/k` + type-to-create). We fold that upgrade in, as a reusable component shared with the docs project filter.

### Established facts (read-only investigation)

- `flow worktime` is a single TUI launch, no subcommands. Only production reference to the legacy Model is `cmd/flow/worktime.go:28`.
- `flow dayoff` (list/add/rm) and `flow export` are **non-TUI CLI commands** (apiclient → stdout) — independent of the legacy Model, untouched here.
- The modern `worktime.NewTodayRoute(api, now, pal, BuildRegistry(client, pal))` reaches Woche/Stats/Frei/Export via `w/t/d/e` (the `wtnav.Registry` pushes routes via `shell.SwitchRouteMsg`). A one-route shell containing only the Today route therefore has the full worktime surface.
- `shell.New(...).WithTabs([]Route{...})` accepts a single route; the tabstrip renders one cell cleanly.
- After deleting the 4 legacy files, `styles.go` becomes fully unused → deletable. `docs.go`/`docs_render.go`/`weblink.go` use no `styles.go` vars.
- `WorkSession{ProjectID *string, Start, Stop}` + server `GET /api/v1/sessions?since=RFC3339` give MRU data; `apiclient.ListSessions` currently has no `since` param.
- `apiclient.CreateProject(ctx, name) (domain.Project, error)` returns the created project (with ID) — usable for immediate booking.
- `picker.RowWithMatch(opts, p)` already renders per-rune match highlight from a `Match []int` of rune indices. No fuzzy library in go.mod; the shell Palette does substring-only filtering.

## 2. Goals

- Delete the legacy worktime Model (`worktime.go`/`stats.go`/`dayoffs.go`/`export.go`) and `styles.go`.
- Rewrite `cmd/flow/worktime.go` so `flow worktime` opens a **worktime-only** modern shell (single Today route + `w/t/d/e` siblings).
- Add two reusable, domain-free components: `ui/fuzzymatch` (subsequence matcher) and `ui/fuzzylist` (filterable list Model with inline-create affordance).
- Add MRU data: `apiclient.ListSessionsSince` + a pure `mruProjects` helper.
- Wire `ui/fuzzylist` into the worktime booking dialog (MRU order + fuzzy + inline-create) and the docs project filter (fuzzy + "Alle Projekte").

## 3. Non-Goals (this phase)

- No change to `flow dayoff` / `flow export` CLI commands.
- No server/REST changes (the `?since` query param already exists).
- No MRU in the docs project filter (no session context there — fuzzy over the existing project order only).
- **Phase 3 (committed follow-up, not here):** refactor `DocsModel` into a native `screen/docs` route and dissolve the top-level `internal/tui` package entirely. Pure architectural consistency, no feature gain; tracked separately.

## 4. Phase 2a — Legacy removal + worktime-only shell

Rewrite `cmd/flow/worktime.go`'s `RunE` to build a one-tab shell (keep the slog→tempfile redirect that protects the TUI):

```go
pal := theme.Load()
m := shell.New(client, os.Getenv("USER"), pal).
    WithTabs([]shell.Route{
        worktime.NewTodayRoute(client, time.Now, pal, worktime.BuildRegistry(client, pal)),
    })
_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
```

Then **delete**: `internal/tui/worktime.go`, `stats.go`, `dayoffs.go`, `export.go` and their `_test.go` files; and `internal/tui/styles.go`.

Guard: `eventsReadyMsg` is defined in both `worktime.go` and `docs.go` (same package). After deleting `worktime.go`, only `docs.go`'s definition remains — correct, no duplicate. Verify build.

## 5. ui/fuzzymatch + ui/fuzzylist (reusable core)

### `internal/tui/ui/fuzzymatch`

```go
// Match reports whether query is a case-insensitive subsequence of target.
// idx are the rune indices in target that matched (for highlight); score ranks
// quality (contiguous + early matches score higher). ok is false on no match.
func Match(query, target string) (idx []int, score int, ok bool)
```

Empty query → `ok=true`, `idx=nil`, `score=0` (everything matches, no highlight). Domain-free; table-tested.

### `internal/tui/ui/fuzzylist`

A value-type filterable list Model (domain-free). The caller owns Enter/Esc and the meaning of selection.

```go
type Item struct{ ID, Label string }

type Model struct { /* items, query, filtered+matches, cursor, pal, createHint */ }

func New(items []Item, pal theme.Palette) Model
func (m Model) WithCreateHint(hint string) Model // e.g. "neu: %s"; enables an inline-create row
func (m Model) SetItems(items []Item) Model       // refresh items, preserve query+refilter
func (m Model) Update(k tea.KeyPressMsg) Model     // text→query, Backspace, Up/Down + Ctrl+n/Ctrl+p → cursor
func (m Model) View(width int) string              // rows via picker.RowWithMatch + optional create row
func (m Model) Query() string
func (m Model) Selection() (it Item, isCreate, ok bool)
```

Behavior:
- **Filtering:** empty query → items in given order (caller supplies MRU order); non-empty → only matching items, sorted by `fuzzymatch` score desc, tie-break by original index (so MRU survives as tiebreak). Each rendered row uses `picker.RowWithMatch` with the match indices.
- **Navigation keys:** in fuzzy mode every rune (including `j`/`k`) goes to the query — navigation is `Up`/`Down` and `Ctrl+n`/`Ctrl+p` (fzf-style). This is a deliberate change from the old `j/k` picker.
- **Inline create:** when `createHint != ""`, query non-empty, and no item `Label` equals the query (case-insensitive), append a synthetic bottom row `✚ <hint-formatted-query>` (glyph `glyphs.Extra`). It is cursor-selectable; `Selection()` returns `isCreate=true` with the current `Query()` for it.
- **"All"/clear entry:** the caller injects an ordinary `Item{ID:"", Label:"Alle Projekte"}` as the first item (docs). No special-casing in the component; fuzzy filtering naturally hides it once the user types.
- `Selection()` returns the item under the cursor (`ok=false` if the filtered list is empty and no create row).

## 6. MRU data + wiring

### apiclient

```go
func (c *Client) ListSessionsSince(ctx context.Context, since time.Time) ([]domain.WorkSession, error)
// GET /api/v1/sessions?since=<since.Format(time.RFC3339)>
```

### MRU helper (in `screen/worktime`, pure + tested)

```go
func mruProjects(projects []domain.Project, sessions []domain.WorkSession) []domain.Project
```

Order projects by the most recent session referencing them (`Stop` if set, else `Start`), descending; projects with no session keep their original relative order, placed after used ones. Window for the session fetch: **90 days** (`since = now.AddDate(0,0,-90)`).

### Worktime booking dialog (`screen/worktime/dialogs.go`)

- `bookingState` replaces `projects []domain.Project, sel int, newName string` with a `list fuzzylist.Model` (`.WithCreateHint("neu: %s")`).
- `startOrStop` (on stop) loads `ListProjects` + `ListSessionsSince(now-90d)` (parallel), computes `mruProjects`, and `SetItems` on the list.
- Booking key handler: `Esc` → cancel; `Enter` → `Selection()`: if `isCreate` → `CreateProject(Query())` then `StopSession(id, newProject.ID)`; else → `StopSession(id, item.ID)`; everything else → `list.Update(k)`.
- `renderBooking` → `list.View(width)` with a one-line hint header (`tippen → filtern · ↑/↓ → wählen · enter → buchen · esc`).

### Docs project filter (`docs.go`, `modeProjectFilter`)

- Replace the `projCursor` `j/k` loop + `renderProjectFilter` with a `fuzzylist.Model` (no create hint). Items = `{ID:"",Label:"Alle Projekte"}` + the projects (Slug labels).
- Keep a `fuzzylist.Model` field on `DocsModel`; on entering the mode, `SetItems` from `m.projects`. `Enter` → `Selection()` sets `m.projFilter = item.ID` ("" = all), `m.sel = 0`, back to `modeList`. `Esc` cancels. All other keys → `list.Update`.
- Footer/keyhints for the filter mode: `tippen → filtern · ↑/↓ → wählen · enter → anwenden · esc`.

## 7. Testing

- `fuzzymatch`: table tests — subsequence yes/no, case-insensitivity, score ordering (contiguous/early beats scattered/late), match-index correctness, empty-query semantics.
- `fuzzylist`: filter narrows + reorders by score; `Up/Down`+`Ctrl+n/p` move the cursor and clamp; typing routes to query (not navigation); inline-create row appears only with `createHint`+non-empty+no-exact-match and `Selection()` reports `isCreate`; `SetItems` preserves query; "all" item selectable.
- `mruProjects`: most-recent-first, running session (no `Stop`) uses `Start`, unused projects trail, stable tie order.
- Worktime booking: model-update test for create-vs-select branch (drive query+Enter).
- Docs filter: model-update test that selecting a project sets `projFilter` and "Alle" clears it (fuzzy path).
- `make ci` green incl. coverage gate. Done-gate: live dogfood — `flow worktime` opens worktime-only shell; booking dialog fuzzy-filters + MRU order + creates a new project inline; docs filter fuzzy-selects; `w/t/d/e` siblings still work.

## 8. File structure

**New:**
- `internal/tui/ui/fuzzymatch/fuzzymatch.go` (+ `_test.go`)
- `internal/tui/ui/fuzzylist/fuzzylist.go` (+ `_test.go`)
- MRU helper + test in `internal/tui/screen/worktime/` (e.g. `mru.go` + `mru_test.go`)

**Modified:**
- `cmd/flow/worktime.go` (worktime-only shell)
- `internal/adapter/apiclient/client.go` (`ListSessionsSince`)
- `internal/tui/screen/worktime/dialogs.go` (+ booking load in `route.go`)
- `internal/tui/docs.go` (`modeProjectFilter` → fuzzylist)

**Deleted:**
- `internal/tui/worktime.go`, `stats.go`, `dayoffs.go`, `export.go` (+ their `_test.go`)
- `internal/tui/styles.go`

## 9. Out of scope / follow-ups

- **Global-nav phase (committed, separate):** make `↑/↓` the *primary* navigation across every screen (with `j/k` kept as aliases), update hint wordings, and rewrite the now-overridden keybind standard in BOTH `~/.claude/skills/tui-usability` (§Keybind grammar + §Anti-patterns) and `flow/docs/design-system.md`. Cross-cutting; its own audit + spec + plan. NOTE: this phase does NOT alter the picker nav decided here — `ui/fuzzylist` already uses `↑/↓`+`Ctrl+n/p` because it is a live text-filter, which is correct under both the current and the future standard.
- **Phase 3 (committed):** refactor `DocsModel` (+ `docs_render.go`, `weblink.go`) into a native `screen/docs` route; dissolve the top-level `internal/tui` package. After Phase 3 the whole TUI is uniform; no feature gain, pure structure.
- MRU window (90 days) is a constant; revisit only if it feels wrong in dogfood.
- A potential later nicety: MRU in the docs filter too — deferred (docs has no session context today).

## 10. Plan sequencing

2a removal first (isolated, `make ci` green immediately) → `fuzzymatch` → `fuzzylist` → apiclient `ListSessionsSince` + `mruProjects` → worktime booking wiring → docs filter wiring → done-gate.

## 11. Resolved decisions

- Standalone `flow worktime` = worktime-only one-tab shell. ✔
- Project picker upgrade folded into this phase. ✔
- One reusable `ui/fuzzylist`, used by BOTH worktime booking and docs filter. ✔
- Fuzzy-mode navigation = `Up/Down` + `Ctrl+n/p` (not `j/k`). ✔
- MRU window = 90 days; MRU in worktime only, not docs. ✔
- `styles.go` deleted (fully unused after legacy removal). ✔
- DocsModel architectural refactor = Phase 3, later. ✔
