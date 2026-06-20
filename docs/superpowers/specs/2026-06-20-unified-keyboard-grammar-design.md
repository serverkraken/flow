# Unified Keyboard Grammar — flow rebuild

**Status:** approved 2026-06-20
**Branch:** `rebuild` (unmerged)
**Supersedes:** the vim-derived `j/k` grammar of the `tui-usability` skill, *for flow only*.

## Problem

Cursor navigation and back/quit semantics are reimplemented per screen with
subtly different rules:

| Surface | ↑/↓ arrows | j/k | g/G | edge | back |
| --- | --- | --- | --- | --- | --- |
| docs (list + search) | ❌ | ✓ | partial | clamp | internal `Esc`, shell `q`=quit |
| worktime Today | ✓ | ✓ | ✓ | **wrap** | internal |
| worktime dayoffs | ✓ | ✓ | — | **wrap** | internal |
| fuzzylist (picker) | ✓ | (typed) | — | clamp | — |

Concrete defects this causes:
- Real `↑/↓` arrows do **nothing** in the docs list.
- Edge behaviour disagrees (worktime wraps, everything else clamps).
- `q` always **quits** from the shell even when nav-stack depth > 1, so there is
  no consistent "go back one level".
- The standalone host (`flow docs`) intercepts `q`/`Esc` → quit *before* the
  route sees them (`shell/host.go:33`), ignoring both the text-input exception
  and the screen's own internal back navigation (document view → list).
- **Hints drift from behaviour:** each screen hand-writes its footer/`?` hint
  text separately from its key handler, so what's advertised and what's bound
  diverge over time.

## Goal

One keyboard grammar across all of flow's TUI, intuitive for a **default
computer user** with no vim/terminal background — explicitly *not* modelled on
vim. Mental model: **`↑/↓` moves *within*, the back key walks *out* one level at
a time.**

Crucially, the grammar is captured as **reusable UI infrastructure**, not
re-wired per screen: a single binding registry drives both behaviour and the
advertised hints, small capability interfaces are the reuse seam, and the back
logic lives in one shared function.

This is a deliberate user override of the `tui-usability` skill (User > Skill).
Updating that skill so it stops prescribing `j/k` is a **follow-up task**, out of
scope here.

## The grammar (the contract)

| Action | Key |
| --- | --- |
| Move selection | `↑` / `↓` |
| Switch tab | `Tab` · `1`–`9` |
| Open / confirm | `Enter` |
| **Back** (one level up); **quit** at the main screen | `q` · `Esc` (synonyms) |
| Hard quit (escape hatch) | `Ctrl+C` |
| Jump to top / bottom | `Home` / `End` |
| Page through long content | `PageUp` / `PageDown` |
| Search / filter | `/` |
| Help | `?` |
| Verbs (`n` new, `e` edit, `d` delete, `f` filter, `s` start/stop, …) | mnemonic, unchanged |

- `j/k` are **removed** as list navigation everywhere.
- `g/G` and `h/l` (vim) are dropped in favour of `Home/End` and `Tab`/numbers.
- `←/→` stay **unused** for now (reserved, not bound to tab-switching).
- Inside a true text-entry field the exception applies: every key (incl. `q`,
  `↑/↓`) goes to the field; `q` is literal, `Esc` cancels the field.

## Reuse model (the whole point)

Three reuse seams, from foundation up:

1. **`ui/grammar` registry** — one definition per action drives *both* the key
   handlers and the rendered hints, so behaviour and advertised hints can never
   drift (Building Block 0).
2. **Capability interfaces** — a route opts into grammar-conformant behaviour by
   embedding `listnav.Cursor` and implementing small interfaces
   (`InputCapturer`, `Backer`); the frame treats every route uniformly through
   them, exactly as it already does for `InputCapturer`/`FullScreener`. Nothing
   bespoke per screen.
3. **One shared back function** — the back-resolution chain is a single function
   consumed by both Shell and host, not duplicated.

## Building Block 0 — `ui/grammar` (single source of truth)

```go
package grammar // internal/tui/ui/grammar

// Binding is one entry in the keyboard grammar: the keys that trigger an action
// and the canonical German hint advertised for it. One definition feeds both the
// handler (Matches) and the rendered hint (Hint), so they cannot drift.
type Binding struct {
    ID   string // stable, e.g. "move.down"
    Keys []Key  // one or more triggering keys (rune, special, or modified)
    Hint string // canonical German, "key → action" body, e.g. "↓ → bewegen"
}

func (b Binding) Matches(k tea.KeyPressMsg) bool

// Canonical structural bindings — the contract, defined once:
var (
    MoveUp, MoveDown   Binding // ↑ / ↓        "↑/↓ → bewegen"
    Top, Bottom        Binding // Home / End   "pos1/ende → sprung"
    PageUp, PageDown   Binding // PgUp / PgDn
    Open               Binding // Enter        "enter → öffnen"
    Back               Binding // q, Esc       "q → zurück"
    Quit               Binding // q, Esc       "q → beenden" (root-level alias of Back)
    Search             Binding // /            "/ → suchen"
    Help               Binding // ?            "? → Hilfe"
    NextTab            Binding // Tab
)
```

- Handlers match with `grammar.MoveDown.Matches(k)`, never raw `k.Code` checks.
- Footer + `?` overlay render from `[]grammar.Binding` via the canonical
  `key → action  ·  …` format — the hint text comes from the same `Binding`.
- Screen-specific **verb** keys (`n`/`e`/`d`/`f`/`s`) are *also* declared as
  `grammar.Binding` values local to each screen, so every advertised hint —
  structural or verb — flows from a `Binding`. No more hand-typed hint strings.
- The existing `ui/strings` hint constants migrate into bindings (or are derived
  from them) so there is one place to change a binding+label.

## Building Block 1 — `ui/listnav` (shared cursor primitive)

A small, domain-free **value** type (not a `tea.Model`) embedded by every list,
matching keys against the BB0 bindings:

```go
package listnav // internal/tui/ui/listnav

type Cursor struct{ idx int }

func (c Cursor) Index() int
func (c Cursor) Clamp(count int) Cursor
// Handle maps a key to a clamped movement against count using grammar bindings
// (MoveUp/MoveDown/Top/Bottom/PageUp/PageDown). ok=false when k is not a nav key.
// Always clamps to [0, count-1]; count==0 → index 0. No j/k.
func (c Cursor) Handle(k tea.KeyPressMsg, count, pageSize int) (Cursor, bool)
```

- **Clamp** semantics everywhere — worktime/dayoffs give up their wrap.
- One unit-tested rule; no screen reimplements cursor math.
- **Consumers (migrate):** docs (list cursor `m.sel`, search-results cursor
  `m.searchSel`), worktime Today (`r.cursor`), worktime dayoffs (`r.cursor`).
- **Align only:** `ui/fuzzylist` already clamps with arrows — route its movement
  through `listnav`/BB0 so it picks up `Home/End/PgUp/PgDn`; keep `Ctrl+n/Ctrl+p`
  for its live-filter context.
- **Not affected:** worktime week/stats (no cursor), export (datepicker form,
  already arrow-driven — not a list).

## Building Block 2 — unified back / quit (one shared function)

The back-resolution chain lives in **one function**, consumed by both the Shell
(`shell/shell.go`) and the standalone host (`shell/host.go`):

```go
// shell/back.go
type BackAction int
const (
    BackForward  BackAction = iota // key belongs to the route (text entry)
    BackOverlay                     // close help/palette
    BackRoute                       // route handles it internally (Backer)
    BackPop                         // pop the nav-stack
    BackQuit                        // quit the program
)

// ResolveBack decides what a back key (q/Esc) should do, given the active route,
// the nav-stack depth, and whether an overlay is open. Shell and host both call
// this and act on the result; host passes stackDepth=1 so it can only Quit.
func ResolveBack(top Route, stackDepth int, overlayOpen bool) BackAction
```

Order encoded by `ResolveBack` (on `grammar.Back.Matches(k)`):
1. overlay open → `BackOverlay`
2. `top` is `InputCapturer` in a real text-entry mode → `BackForward`
3. `top` is a `Backer` with internal back available → `BackRoute`
4. `stackDepth > 1` → `BackPop`
5. else → `BackQuit`

New interface in `shell/route.go`:

```go
// Backer lets a route resolve a "back" within its own internal state before the
// frame pops the nav-stack or quits. ok=false means "nothing to go back to".
type Backer interface { Back() (Route, bool) }
```

Frame changes:
- `shell/shell.go`: the current unconditional `q` → `Quit` (~`:211`) and the
  separate `Esc` → `Pop` (~`:204`) fold into a single `ResolveBack` switch, so
  `q` and `Esc` become true synonyms.
- `shell/host.go`: replace the unconditional `q`/`Esc` → quit (`:33`) with the
  same `ResolveBack` call (stackDepth=1), so the text-input exception and the
  route's internal back work in standalone mode too.

Route changes:
- docs `CapturesInput()` is **narrowed** to real text-entry modes
  (`modeCreating`, `modeFiltering`, `modeSearch`) — **not** `modeView` /
  `modeDeleting` (currently `mode != modeList`, `docs.go:562`).
- docs implements `Backer`: `modeView` → `modeList`; an applied tag/project
  filter is cleared one step before leaving the list; `modeDeleting` cancels.
  docs' ad-hoc `Esc`/`q` handling is removed in favour of `Back()`.

## Building Block 3 — hints & help render from bindings

- Footer hints and the `?` overlay take `[]grammar.Binding` and render them via
  the canonical `key → action  ·  …` format; no screen hand-types hint text.
- The back hint flips by context: `grammar.Back.Hint` (`q → zurück`) in nested
  views, `grammar.Quit.Hint` (`q → beenden`) at the main screen — chosen from the
  same nav-depth the back chain uses.
- Net effect: no advertised hint can mention `j`, `k`, `g`, or `G`, and changing
  a binding updates behaviour + hint together.

## Testing

- `grammar`: `Matches` truth tables per binding; assert no binding contains
  `j`/`k`/`g`/`G`; assert each `Hint` uses the ` → ` connector.
- `listnav`: clamp at both ends, Home/End, PageUp/PageDown, count==0/1, and the
  "not a nav key" passthrough.
- `ResolveBack`: one case per branch (overlay, input-capture passthrough, Backer
  consumed, Pop at depth>1, Quit at root); host path asserts depth-1 → Quit but
  still honours input-capture + Backer.
- docs: `CapturesInput()` false in `modeView`/`modeDeleting`; `Back()` walks
  `modeView → filter-clear → modeList`; arrows move the list, `j/k` no longer do.
- Hint/golden tests assert footer + `?` output is generated from bindings and
  mentions no vim keys.

## Migration order (for the plan)

1. `ui/grammar` registry (BB0) + tests — bindings only, no call-sites yet.
2. `ui/listnav` (BB1) on top of BB0 + tests.
3. Wire `listnav` into docs (list + search), worktime Today, dayoffs; drop `j/k`
   and wrap. Align fuzzylist.
4. `Backer` + `ResolveBack` (BB2) in shell; narrow docs `CapturesInput()`; docs
   implements `Backer`; shell `q`/`Esc` fold into `ResolveBack`.
5. Mirror `ResolveBack` into host.go.
6. Hints/help/strings sweep (BB3): footer + `?` render from bindings; migrate
   `ui/strings` hint constants; remove `j/k`/`g/G` advertising.
7. Wiring-verification task: build, `make ci`, and a manual key-walk of every
   surface (arrows move; `q`/`Esc` step back through internal state → stack →
   quit; text fields keep `q` literal; footer/`?` show the new grammar) before
   the done-gate.

## Out of scope

- Updating the `tui-usability` skill / `design-system.md` to match (separate
  follow-up so the cross-tool design system stops prescribing `j/k`).
- `←/→` bindings (left reserved).
- Verb-key *changes* (kept mnemonic — but they do become `grammar.Binding`s).
- Mouse support.
