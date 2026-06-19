# flow rebuild M3d-Docs — Fullscreen Markdown Document Viewer — Design

**Status:** approved (brainstorm complete), ready for the implementation plan.
**Date:** 2026-06-19
**Branch:** `rebuild`

## Goal

When the user opens a document in the `flow ui` Docs tab, show it **fullscreen** with the
**richly-rendered, polished viewer** that existed in the old Kompendium UI on `main` — instead
of today's plain line-by-line printer wrapped inside the shell chrome. This is the **Docs slice
of M3d** (the M3 overview spec lists the rest of M3d — Stats/Heatmap, DayOffs, Export, Projects —
which stay out of scope here).

## Background

- Parent: `docs/superpowers/specs/2026-06-16-flow-rebuild-m3-tui-design-system-shell.md`, slice
  **M3d** (line 115): "Docs (+ custom **Markdown-Viewer** goldmark→ANSI/Wikilinks/OSC-8 im viewport
  + markdown_overlay) … Docs-Wikilink-Drill-down + Back; Markdown gerendert." Design principle
  (lines 24/99): the custom goldmark→ANSI renderer **replaces glamour inside `markdown_overlay`**;
  search + code-copy stay; glamour is **not** introduced (theming ceiling + wikilink breakage).
- The M3c3 plan deferred "the Markdown-viewer + chrome cleanup" to M3d. The current Docs tab wraps
  the legacy `tui.DocsModel`, whose `modeView` renders the body line-by-line (`styleBodyLine`) with
  wikilink/weblink styling only — no headings/code/tables, no scroll viewport.
- On `main` the viewer was the `markdown_overlay` component (one component, five surfaces), built on
  a first-party `internal/frontend/tui/markdown/` goldmark→ANSI renderer (chroma syntax highlight,
  GFM tables, footnotes, 7 GitHub callouts, frontmatter card, "↩ Referenced by" backlinks footer,
  OSC-8 links, `/`-search with gutter match bars, code-copy). It rendered as a **fullscreen
  takeover** (its `View()` replaced the browse chrome entirely).

## Feasibility (verified)

- **Theme: zero gaps.** The rebuild `theme.Palette` (verbatim-ported in M3a) exposes every field the
  old `MarkdownRoles`/renderer reads. The `markdown/theme/` sub-package ports cleanly.
- **Deps:** `charm.land/bubbles/v2` v2.1.0 (viewport) and `goldmark` v1.8.2 are present. **Missing:
  `chroma/v2`** (code highlighting) and possibly `charmbracelet/x/cellbuf` — must be added.
- **Renderer/overlay port is mechanical:** copy from `main` + rewrite import paths
  (`internal/frontend/tui/components` → `internal/tui/ui`, `internal/frontend/tui/theme` →
  `internal/tui/theme`, `internal/frontend/tui/markdown` → `internal/tui/markdown`). The
  `ports.WikilinkResolver` reference becomes a local interface; `api.go` drops its `ports` import.
- **Wikilink resolution** already exists in the rebuild (`domain.FindWikilinks`,
  `domain.ResolveWikilink`, and `tui/docs.go`'s `buildBodyLinks`). **Frontmatter/tags** are on
  `domain.Document`; a **backlinks API** exists on `apiclient`. These feed the renderer's
  `WithFrontmatter`/`WithBacklinks`/`WithWikilinks` options.
- **The only genuinely new construction is the shell fullscreen takeover** (see below); everything
  else is a port + wiring.

## Architecture

### 1. Fullscreen takeover — `shell.FullScreener` (the new mechanism)

Add a small opt-in interface, mirroring the existing `shell.InputCapturer`:

```go
// FullScreener lets the active tab's top route take over the entire terminal,
// suppressing the shell's header/tabstrip/breadcrumb/footer. Used by the Docs
// viewer for an immersive document read, matching main's old ModeView takeover.
type FullScreener interface{ FullScreen() bool }
```

In `Shell.View`: when the active tab's top route implements `FullScreener` and reports `true`
(and no help/palette overlay is open), render **only** `top.View(Frame{Width: width, Height: H})`
over the full terminal height — skip header, tabstrip, breadcrumb, and footer. Otherwise the
existing chrome path is unchanged. Because the Docs route already reports `InputCapturer` while in
view mode, the shell already forwards all keys to it; `FullScreener` adds the visual takeover.
This also resolves the view-mode "double chrome".

### 2. The viewer lives inside `DocsModel.modeView` (least churn)

The ported `markdown_overlay` component is mounted as a field on the legacy `tui.DocsModel`:
- `modeList` / other modes: unchanged; the route is not fullscreen, list keeps current chrome.
- `modeView`: the overlay renders the document fullscreen.
- The `docs.Route` adapter's `FullScreen()` returns `true` exactly when the wrapped `DocsModel` is
  in `modeView`, via a new read-only `DocsModel.InViewMode()` accessor (mirrors the
  `CapturesInput()` accessor added earlier). The adapter delegates: `func (r *Route) FullScreen() bool { return r.m.InViewMode() }`.
- **Size bridging (rebuild-native):** the adapter translates the incoming `Frame` into the overlay's
  `SetSize(w, h)` at render time — the route gets its size from `View(Frame)`, never by storing a
  `tea.WindowSizeMsg`. (The old plan's `AltScreen`-on-inner-view + `WindowSizeMsg` approach does
  **not** work here: the adapter only reads `View().Content` and the shell owns the program view.)

Wikilink "back" reuses `DocsModel`'s existing `viewStack` (drill = push id, `Esc` = pop one level,
`Esc` at the root = leave view mode → list, chrome returns).

### 3. The three ported layers (under `internal/tui/`)

1. **`internal/tui/markdown/theme/`** — `MarkdownRoles` + `MarkdownRolesFor(Palette)` +
   `CalloutBadge`/`CalloutBar`. Parallel-safe per-call palette.
2. **`internal/tui/markdown/`** — the goldmark→ANSI pipeline: `Render(source, width, ...Option)`,
   block/inline `nodeRenderer`, chroma code panels with language band, `[[wikilink]]` inline parser
   + resolver, OSC-8 URL post-processor, frontmatter card, backlinks footer. A local
   `WikilinkResolver` interface (in `interfaces.go`) replaces `ports.WikilinkResolver`.
3. **`internal/tui/ui/markdown_overlay/`** — the bubbletea sub-model wrapping a
   `bubbles/v2/viewport`: scroll (j/k, g/G, ctrl-d/u, pgup/pgdn), `/`-search with left-gutter match
   bars + n/N (scroll-to-match at ⅓ height), title line + separator, footer (progressive
   degradation), status bar (search badge · title path · "42% / N/M"), code-copy (`c`, OSC 52),
   close keys (`q`/`esc`).

### 4. Renderer wiring in DocsModel

- On `docViewMsg` (full doc body loaded), build the overlay with a `buildRenderFunc()` closure that
  calls `markdown.Render(body, width, WithWikilinks(adapter), WithFrontmatter(fm), WithBacklinks(bl),
  WithFocusedWikilink(state.focus))`.
- `wikiAdapter` (~20 lines) resolves `[[links]]` against the loaded doc set (`domain.ResolveWikilink`)
  and emits `flow://docs/<id>` URIs for valid links; the viewer turns Enter-on-wikilink into in-TUI
  navigation (push `viewStack`, reload), while bare web URLs open in the OS browser via the existing
  opener.
- Frontmatter card fed from `domain.Document` type/title/date/project/tags; backlinks footer fed
  from the `apiclient.Backlinks` call mapped from `domain.BacklinkRef{ID,Path,Title,Type}` to the
  renderer's `BacklinkRef{ID, Title}` shape.
- On render error, fall back to the raw body (never blank).

### 5. Keyboard wikilink navigation with visible focus (chosen: full)

The user is keyboard-primary and wants in-body focus, so the renderer gains a focus contract and
DocsModel drives it:

- **Renderer:** new `WithFocusedWikilink(idx int)` option (idx < 0 = none). The wikilink inline
  renderer keeps a per-render ordinal counter; the `idx`-th **valid** wikilink renders in a focused
  style (a `Focused` role added to `MarkdownRoles`, e.g. reverse/accent), the rest as today
  (valid `→` / broken `⊘`). Weblinks stay OSC-8 click-only (as in old Kompendium).
- **Focus state lives in a heap cell** `*viewerState{ focus int }` held by `DocsModel` and captured
  by the `buildRenderFunc` closure, so focus survives bubbletea's value-copies and the closure always
  reads the current focus. `Tab`/`⇧Tab` mutate `state.focus` (cycle over the doc's valid wikilinks),
  then trigger an overlay re-render (`overlay.Rerender()` re-runs the RenderFunc at the current size)
  and scroll the focused link into view.
- **Key ownership in `modeView`:** DocsModel intercepts `Tab`/`⇧Tab` (cycle focus), `Enter` (follow:
  wiki → push `viewStack` + reload; the focused wikilink id comes from the wikiAdapter's resolution),
  `e` (edit), and `Esc` (pop one `viewStack` level, or leave to list). **All other keys forward to
  the overlay** (scroll j/k/g/G/ctrl-d/u/pgup/pgdn, `/`-search + n/N, code-copy `c`). While the
  overlay is in its own `/`-search input, it reports `CapturesInput()==true` (new accessor, mirrors
  the docs/route pattern) and DocsModel forwards **every** key to it. The overlay is built with
  `WithCloseKeys()` empty so it never self-closes — DocsModel owns leaving.

## Components & responsibilities

- `internal/tui/markdown/theme` — palette → lipgloss role bundle. Pure, tested.
- `internal/tui/markdown` — markdown source → ANSI string. Pure (given width + options), tested with
  golden ANSI fixtures ported from `main`. **Adds** `WithFocusedWikilink(idx)` + a `Focused` role.
- `internal/tui/ui/markdown_overlay` — interactive viewport view-model (scroll/search/copy/chrome).
  Tested via reducer + golden render tests. **Adds** `Rerender()` (re-run RenderFunc at current size)
  and `CapturesInput() bool` (true while in `/`-search input).
- `internal/tui/shell` — `FullScreener` interface + `Shell.View` takeover branch. Tested: chrome
  suppressed when the top route reports fullscreen, present otherwise.
- `internal/tui/docs.go` (+ `internal/tui/screen/docs/route.go`) — mount overlay in `modeView`,
  `InViewMode()` accessor + adapter `FullScreen()`, `wikiAdapter`, `buildRenderFunc`, `Frame → SetSize` bridge.

## Data flow

open doc (`enter`) → `loadDoc` → `docViewMsg{doc}` → build overlay + render markdown (focus = -1) →
`modeView` → shell sees `FullScreen()==true` → renders overlay over full screen; keys forwarded
(`InputCapturer`). `Tab`/`⇧Tab` → mutate `state.focus` → `overlay.Rerender()` + scroll-to-focus.
`Enter` on focused wikilink → push `viewStack` → reload → re-render (focus reset). `Esc` → pop one
level, or leave to list (chrome returns). Scroll/`/`-search/`c`-copy keys → forwarded to overlay.

## Error handling

- Render failure → raw body fallback (logged), viewer still scrolls.
- Backlinks/frontmatter fetch failure → omit that section, render the body anyway.
- Unknown/unresolved wikilink → rendered as broken (`⊘`), not a fatal error.
- Missing terminal size before first `View(Frame)` → overlay uses a safe default until the first
  frame arrives (frames arrive every render, so this is momentary).

## Testing

- Golden ANSI tests for the renderer (port `main`'s fixtures): heading bands, code highlight band,
  GFM table, callouts, task lists, wikilink valid/broken, frontmatter card, backlinks footer.
- Reducer tests for the overlay: scroll bounds, `/`-search match cycling + scroll-to-match, code-copy
  target selection, close keys.
- Shell test: `FullScreener` true ⇒ no header/tabstrip/breadcrumb/footer in `View()`; false ⇒ chrome
  present.
- Renderer focus test: `WithFocusedWikilink(n)` styles the n-th valid wikilink differently from the
  others (ANSI-stripped position check + style-present check).
- Docs wiring tests: `FullScreen()` true only in `modeView`; `Tab` advances `state.focus` (cycles,
  wraps); `Enter` on focused wikilink pushes `viewStack` + loads target; `Esc` pops then leaves;
  in-doc `/`-search makes the overlay capture all keys; render-error fallback.
- `make ci` green, coverage ≥ 80 %.
- Live done-gate vs the dev stack: open a doc → fullscreen render of headings/code/table/callouts;
  `Tab`/`⇧Tab` cycles wikilinks with a **visible focus highlight** + scroll-to-focus, `Enter` drills
  in, `Esc` goes back/leaves; valid/broken wikilink markers; frontmatter card; backlinks footer;
  `/`-search; code-copy; weblink OSC-8 click; `Esc` returns to the list with shell chrome back;
  `flow docs` standalone still works.

## Scope boundaries (deliberately out)

- Only the **document view** goes fullscreen. The Docs **list** stays in the tab content (its own
  double-chrome is a separate, smaller cleanup, not this slice).
- The rest of M3d (Stats/Heatmap, DayOffs, Export, Projects as routes) is not in this slice.
- No WebUI change (that is M4).
- No glamour (per the M3 design principle).

## Risks

- **chroma/x dependency add** — new modules; pin versions matching `main`. (T-first task.)
- **markdown_overlay key/Frame integration** — the genuinely new glue (size bridge + FullScreener);
  covered by shell + adapter tests.
- **Two SSE subscriptions / list double-chrome** — pre-existing, untouched here.
