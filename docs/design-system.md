# Design System

Living reference for flow's TUI: the canonical token system, the
component inventory, the glyph whitelist, the German UI strings, and
the rules the screens follow.

This document is the **kuratierte Quelle**. It is updated together with
the code. The audit that drove the four-phase migration to this state
lives at [`design-system-audit.md`](design-system-audit.md) and reads
as a frozen snapshot — keep this file as the current map; reach for
the audit only when the *why* of a structural decision is needed.

---

## TL;DR

- One palette: `internal/frontend/tui/theme.Palette`. Tokyonight Night
  is canonical. Catppuccin Mocha is the alternate. tmux `@tn_*`
  user-options overlay per-machine.
- Components consume the **semantic** alias (`p.Sem().Accent`,
  `p.Sem().Danger`), not the raw hue (`p.Blue`).
- Glyph + colour, never colour alone (audit A11y-2). The glyph
  whitelist is fixed; new ones go through review.
- Builders before NewStyle. Use `theme.Dim(s, p)` (and friends) for
  text; reach for `lipgloss.NewStyle()` only for layout chains
  (Width / Padding / Border / Join / Place).
- A11y is part of the API: WCAG-AA contrast tested on every palette,
  no `Faint()` on prose, NO_COLOR-readable callouts.

---

## Token system

### Color tokens — `theme.Palette`

```go
// internal/frontend/tui/theme/palette.go
type Palette struct {
    Name string

    // Surface — backgrounds, dark to light.
    Bg, BgPanel, BgCode, BgChip, BgChipSoft, BgBar string
    BgDanger, BgSuccess                            string

    // Foreground — text stages, bright to dim.
    Fg, FgDim, FgMuted string

    // Hue — raw color points, no semantic meaning by themselves.
    Blue, Cyan, Green, Purple, Magenta, Yellow, Orange, Red, Teal string

    // Tag-rotation pool for hash-based chip colouring.
    TagPalette []string
}
```

Two canonical palettes: `theme.TokyonightNight` and
`theme.CatppuccinMocha`. The default is Night
(`theme.Default = TokyonightNight`).

**FgMuted lifted from upstream:** Tokyonight `comment` (`#565f89`)
fails WCAG-AA on Bg `#1a1b26` (~2.6:1). `theme.TokyonightNight.FgMuted
= #9aa5ce` clears AA. Catppuccin's `overlay0` (`#6c7086`) fails the
same test on Mocha Bg; `theme.CatppuccinMocha.FgMuted = #a6adc8` clears.

### Semantic alias — `Palette.Sem()`

```go
type Semantic struct {
    Accent       lipgloss.Color // primary interactive — Blue
    Active       lipgloss.Color // running / live — Cyan
    Success      lipgloss.Color // Green
    Warning      lipgloss.Color // Yellow
    Danger       lipgloss.Color // Red
    Info         lipgloss.Color // informative without action — Cyan
    Highlight    lipgloss.Color // attention-grabbing, non-actionable — Purple
    Border       lipgloss.Color // dim separator / progress-empty cells — BgCode
    BorderSubtle lipgloss.Color // light divider — BgChip
    BorderStrong lipgloss.Color // load-bearing border — FgMuted
}
```

**Components use `Sem()`, not the raw hues.** A future palette swap
that redefines "danger" as orange shifts every consumer in lockstep
without per-call-site renames. The hues stay accessible to renderers
that need free colour choice (markdown heading hierarchy, tag chips).

**Border-Trio:** drei Tokens für visuell zunehmend lautere Affordance.
`Border` (= `BgCode`) ist absichtlich sub-WCAG (≈ 1.3–2.0:1 auf Bg) —
es taucht im Status-Bar-▱-Empty, in Picker-Section-Dividern und
manchen Panel-Frames auf, wo Information durch parallel sichtbare
sem.Accent-/sem.Active-Glyphen getragen wird. Load-bearing Frames
(modal, help-overlay, picker AccentBar) müssen `BorderStrong` (=
`FgMuted`, ≥ 3:1 WCAG-Non-Text geprüft) verwenden. `BorderSubtle` ist
der Mittelweg für selection-Tints. Die WCAG-Trennung ist im
`theme/contrast_test.go` festgenagelt: BorderStrong-on-Bg/BgPanel
fail-fast, Border bekommt einen `t.Logf`-Info-Hook ohne Threshold.

### Spacing & layout — `theme.PadXS / PadSM / PadMD / …`

```go
// Padding (horizontal scale; vertical is 0 or 1 in a TUI).
PadNone = 0
PadXS   = 1   // chip / pill / status-bar inner
PadSM   = 2   // modal content L/R
PadMD   = 3   // modal vertical, spaced sections

// Layout — recurring column widths.
PillWidth     = 4    // status pill
KeyHintWidth  = 12   // help-overlay key column
DayLabelWidth = 3
DateColWidth  = 9
DefaultBox    = 60
NarrowBox     = 40
WideBox       = 80
```

### Layer — ordering hint, not a real z-axis

```go
LayerSurface  = 0   // Bg
LayerPanel    = 1   // BgPanel + NormalBorder
LayerHover    = 2   // BgChipSoft
LayerSelected = 3   // BgChip + accent bar
LayerOverlay  = 4   // RoundedBorder
LayerModal    = 5   // DoubleBorder + BgPanel + PadMD
```

### Contrast tested — `theme.ContrastRatio`

`theme/contrast_test.go` enforces WCAG 2.1 AA (4.5:1) on every
text-on-surface and bg-as-fg-on-accent pair for every registered
palette. A new palette doesn't ship until it clears this test.

---

## Glyph whitelist

Canonical glyphs live in `internal/frontend/tui/components/glyphs`.
Rules:

1. Monospace-only — every glyph is exactly one terminal cell. Tested
   via `ansi.StringWidth`.
2. No emoji — emoji-presentation pictographs render at variable
   width; tmux status segments and nvim sidebars stop aligning.
3. No half-fill characters (`◐`, `◓`, …) — emoji-width in some fonts.

| Glyph | Constant     | Meaning                            |
|-------|--------------|-----------------------------------|
| `▶`   | `Active`     | running session / live thing      |
| `■`   | `Stopped`    | halted                            |
| `‖`   | `Paused`     | paused                            |
| `✓`   | `Done`       | success / completed               |
| `✗`   | `Failed`     | failure                           |
| `▲`   | `Up`         | increase / streak                 |
| `▼`   | `Down`       | decrease                          |
| `●`   | `Filled`     | achieved goal / today             |
| `○`   | `Empty`      | missed goal / future              |
| `★`   | `Holiday`    | public holiday                    |
| `☼`   | `Vacation`   | vacation / personal-free          |
| `✚`   | `Extra`      | extra / additional entry          |
| `▎`   | `AccentBar`  | selection / focus indicator       |
| `▰ ▱` | `BarFilled / BarEmpty` | progress bar             |
| `╭ ╮ ╰ ╯` | `BoxRoundedTL/TR/BL/BR` | rounded box       |
| `┌ ┐ └ ┘` | `BoxNormalTL/TR/BL/BR`  | normal box        |
| `╔ ╗ ╚ ╝` | `BoxDoubleTL/TR/BL/BR`  | modal (double)    |
| `─ │ ═ ║` | box drawing             | horizontals/verticals |

Adding a glyph: open a PR that runs `go test
./internal/frontend/tui/components/glyphs/...` against the new
constant and updates this table. The cell-width test fails-fast on
non-conforming glyphs.

---

## Strings — kanonische DE-UI-Texte

Lives in `internal/frontend/tui/components/strings`. Centralised so
copy-paste drift is impossible:

```go
// Hints (status-bar / footer rows).
HintConfirm = "y/Enter → ja  ·  n/Esc → nein"
HintCancel  = "Esc → abbrechen"
HintFilter  = "/ → suchen"
HintHelp    = "? → Hilfe"
HintQuit    = "q → schließen"
HintNav     = "j/k → navigieren  ·  Enter → wählen"

// Block labels.
LabelLoading      = "lädt …"
LabelEmpty        = "Keine Treffer."
LabelError        = "Fehler:"
LabelNoSelection  = "Keine Auswahl."
LabelConfirmTitle = "Bestätigen"
```

Anything user-facing that's not generated content (note title, session
name, …) and appears in two or more places lives here.

---

## Component inventory

Every component sits in `internal/frontend/tui/components/<name>/`.
Naming is by role, not by widget.

### Pure-render components

These are functions: `Render(opts, p Palette) string`. No state, no
lifecycle. Use them when the screen owns the `tea.Model` and just
needs styled output for one cell of the layout.

| Component | API                                                 | Variants                       | Notes |
|-----------|-----------------------------------------------------|--------------------------------|-------|
| `titlebox` (legacy `box`) | `Render(title, body string, width int, p Palette)` | rounded                        | Wraps a body in a rounded-border box with a title strip. |
| `chip`    | `Render(Opts{Label, Color, Variant}, p Palette)` + `Hash(s, palette)` | Solid, Outline                 | Tag chip. `Hash` picks a stable colour from `p.TagPalette`. |
| `card`    | `Render(Opts{Badge, Title, Meta, Body, Width, Separator}, p)` | with/without badge / separator | Compact two-row header used in markdown frontmatter and worktime. |
| `tabs`    | `Render(items, active int, width int, variant, p)` | Underline (default), Pill     | Active tab = bold + accent + underline rule (A11y-2). |
| `modal`   | `Render(content string, Opts{Title, Kind, Width}, p)` | KindDefault, KindDanger, KindSafe | DoubleBorder + BgPanel + PadMD/PadSM. |
| `statusbar.Hints` | `Hints(text string, p Palette)`             | inline                         | Footer hint strip. |
| `statusbar.Bar`   | `Bar(pct, cells int, p Palette)`            | filled/empty                   | Progress bar via `▰` / `▱`. |
| `picker.Row`      | `Row(selected bool, label, hint string, width int, p)` | with hint                | Selection row with `▎` accent bar + bold-on-selected. |
| `picker.SectionHeader` | `SectionHeader(title string, width int, p)`     | —                              | Dim section divider. |
| `help`    | `Render(title string, sections, keyWidth, boxWidth int, p)` | overlay                        | `?`-overlay; uses `KeyHintWidth` for key column. |

### `tea.Model` components

These have their own state. The screen embeds them and forwards
messages.

| Component | Constructor                                  | Variants / Kinds                              | KeyMap Export | Notes |
|-----------|----------------------------------------------|-----------------------------------------------|---------------|-------|
| `spinner` | `New(label string, p Palette)`               | Dot                                           | —             | bubbles/spinner wrapper. |
| `toast`   | `New / NewSuccess / NewWarning / NewDanger / NewInfo` | Success ✓, Warning ▲, Danger ✗, Info › | —             | DefaultDuration = 2 s. Glyph + colour, never colour alone. |
| `confirm` | `New / NewDanger`                            | KindDefault (yellow), KindDanger (red)        | `Keys() KeyMap` (A11y-5) | y/Enter → confirm, n/Esc → cancel. Hint via `strings.HintConfirm`. |
| `form.TextInput` | `New(opts InputOpts, p)`              | default                                       | —             | Cursor + focus styled via Sem.Accent. |
| `form.ChoiceModel` | `New(items, width, p)`              | with-glyph                                    | —             | Bubble Tea picker for n-way choices. |
| `viewport` | bubbles wrapper                             | —                                             | —             | Scroll glyph + border styled from palette. |

### Pill (in `components/theme`)

`RenderPill(kind PillKind, label string, p Palette) string` — glyph +
colour status indicator. Kinds: `PillSuccess ✓`, `PillWarning ▲`,
`PillDanger ✗`, `PillActive ▶`, `PillInfo ›`, `PillSkip ○`,
`PillNeutral ·`. Audit A11y-2 enforced via test
`TestRenderPill_KindGlyphsAreDistinct`.

Legacy `Pill(state string, p)` keeps a fixed 4-cell width for
status-bar columnar rhythm; new code uses `RenderPill`.

---

## Builder catalog

Style builders are pure `(string, Palette) → string`. They replace
inline `lipgloss.NewStyle().Foreground(...).Render(...)` in screens.

### `components/theme.*`  (screen-side, 11-field Palette)

| Builder    | Visual           | Use for                                       |
|------------|------------------|-----------------------------------------------|
| `Body`     | `Fg`             | default paragraph                             |
| `Dim`      | `Dim` (= FgMuted)| hint / footer / "lädt…" placeholders          |
| `Strong`   | `Fg` + Bold      | emphasised body                               |
| `Heading`  | `Accent` + Bold  | section / panel title, prompt prefix          |
| `Highlight`| `Purple` + Bold  | identity / "active thing" accent              |
| `Success`  | `Green` + Bold   | status text — achievement                     |
| `Warning`  | `Yellow` + Bold  | status text — heads-up / pending              |
| `Danger`   | `Red` + Bold     | status text — failure / blocking              |
| `Err`      | `Red`            | error message paragraphs (no Bold; not a label) |
| `Info`     | `Cyan`           | informational meta — "läuft seit X"           |

### `theme.*` (canonical palette, role-named)

Same shape, takes the canonical 22-field `theme.Palette`. Used inside
the markdown renderer and any consumer that already has the canonical
palette in scope. See
[`internal/frontend/tui/theme/builders.go`](../internal/frontend/tui/theme/builders.go).

---

## A11y rules — part of the API

1. **WCAG 2.1 AA contrast on every text/glyph-on-surface pair.** Tested
   in `theme/contrast_test.go` for every registered palette. New
   palettes don't ship until the test passes.
2. **Glyph + colour, never colour alone.** Pills, toasts, pace dots,
   confirm-question, callout badges all carry a glyph distinct enough
   to read in NO_COLOR. Tested via
   `TestKindGlyphs` (toast),
   `TestRenderPill_KindGlyphsAreDistinct` (pill),
   `TestMarkdownRolesFor_NoColorPath` (markdown).
3. **No `Faint()` on prose.** Faint dims terminal foreground 30–50 %;
   on top of FgMuted that drops below AA. H6 enforced by
   `TestH6_NoFaint`.
4. **NO_COLOR end-to-end.** The markdown renderer + every pill / chip /
   border survives `termenv.Ascii` and stays content-readable.
5. **Keyboard-complete.** Every interactive component exports a
   `KeyMap` so the parent's `?`-overlay aggregates without
   hand-pasting (today: confirm). Esc cancels everywhere.
6. **Screen-reader-friendly glyphs.** ▶ ✓ ✗ read well; ▰ ▱ are okay
   as visual companions, not alone — the progress bar always carries
   a label.

---

## Style guidelines

### When to use a builder

- **Body styles** (single Foreground / Bold / Italic chains): use the
  builder. `theme.Dim(s, p)`, `theme.Heading(title, p)`, etc.
- **Anything in the role list above** (Body / Dim / Strong / Heading /
  Highlight / Success / Warning / Danger / Err / Info): builder.

### When to use `lipgloss.NewStyle`

- Layout chains: `Width(n).Padding(0, 1).Border(…)` — builders don't
  cover layout. Inline NewStyle is correct.
- Dynamic-color renders: when the colour is a function of state
  (`totalThresholdColor`, `kindColor`, `balColor`), the builder's
  fixed role doesn't fit. Inline.
- Pre-built styles in tight render loops: `dimStyle :=
  lipgloss.NewStyle().Foreground(p.Dim)` and call
  `dimStyle.Render(line)` per item is faster than constructing a new
  style per line. Keep inline.

### When to add a new builder

A pattern that appears 3+ times across screens with the same
Foreground+Bold combination is a builder candidate. Add it to
`components/theme/builders.go` with:

- a one-line role name (Hint? Marker? Caption?)
- a doc comment that names the use-case
- a smoke test in `builders_test.go`

### When to add a new component

A pattern that appears 2+ times AND owns its own layout (border /
panel / multi-row composition) is a component. Add a new directory
under `components/`. Follow the existing two-form choice:

- **Pure render** — `Render(opts, p) string`. Default for
  non-interactive widgets.
- **`tea.Model`** — when the widget owns timer / focus / input state.

Document the API in this file's component inventory in the same PR.

### When to NOT do either

If a screen has a one-off layout (worktime's pace dot row, history's
heatmap), keep it inline. Premature componentisation costs more than
the duplication it avoids.

---

## Lint regression baseline

`internal/frontend/tui/lint/screen_baseline_test.go` pins the maximum
`lipgloss.NewStyle()` count per screen file. Currently:

```
cheatsheet/model.go    1
palette/model.go       8
projects/model.go      2
worktime/dayoffs.go    5
worktime/history.go    29
worktime/model.go      2
worktime/today.go      3
worktime/week.go       13
                       ──
                       63
```

Adding new inline NewStyle bumps the relevant entry — visible in PR
diff, reviewable. Refactor that lowers a count: ratchet the baseline
down in the same commit so the next PR can't regress past the new
floor.

The audit's depguard goal — see [`design-system-audit.md`
§2.6](design-system-audit.md#26-konsumenten-disziplin) — is met in
spirit: a regression is a CI failure, not a hand-review escape hatch.

---

## Migration history

| Phase  | Commit pattern         | Outcome                                                                  |
|--------|------------------------|--------------------------------------------------------------------------|
| **F-WAVE** | `feat(tui): F-WAVE polish` | pre-design-system polish (cheatsheet command, in-process palette dispatch, help refactor) |
| **P1** | `feat(theme): P1 …`    | Canonical `internal/frontend/tui/theme` package: Palette + Sem + tokens + builders + WCAG-AA test. Storm dropped, Night canonical, FgMuted bumped for AA. |
| **P2** | `feat(markdown): P2 …` | `markdown/theme` decoupled from globals: `MarkdownRolesFor(r, p)` palette param, `Active`/`SetActive`/duplicate Palette removed, H6 ohne Faint, NO_COLOR test. |
| **P3** | `feat(components): P3 …` | Component-kit expand: chip, card, tabs, modal (4 new); pill/toast/confirm Kind variants; glyph whitelist; canonical DE strings; KeyMap-Export confirm. |
| **P4a**| `refactor(kompendium): P4a …` | Kompendium screens migrated off the deprecated markdown/theme shims; the shims deleted. Audit's "3 parallele Token-Quellen" reduced to 1. |
| **P4b**| `feat(components,screens): P4b …` | components/theme builders introduced; first screen rollout (worktime/today wrappers, projects, cheatsheet). 98 → 87 inline NewStyle. |
| **P4c**| `refactor(screens): P4c …` | Bulk inline NewStyle migration across worktime + palette; kompendium browse modal lifted onto components/modal. 87 → 63 inline NewStyle. |
| **P4d**| `chore(lint,docs): P4d …` | Regression-baseline test; this document. |

Each phase ships independently. The audit document is the historical
"why"; this document is the current "what".
