# flow-server WebUI — Design System ("Editorial Terminal")

The visual language for the WebUI mounted by `flow-server` (Plans E / M6+M7).
This file is the source of truth. Templates reference it instead of redeciding
type scale, color use, or motion in each handler.

## Premise

The WebUI is the **visual extension of a terminal** — the same `flow` you live
in inside tmux, rendered through a browser when you want long-form notes,
sparklines, or device-flow logins. It must feel like a terminal that grew up:
keep the discipline (monospace, restrained color, glyph-not-icon, sharp
edges) but use what HTML can do that ANSI cannot — real typographic
hierarchy, real SVG line-art, real animated state changes, real charts.

## Locked constraints (from project memory)

- **Tokyonight-Night palette**, identical to `internal/frontend/tui/theme`.
- **JetBrains Mono** as the sole font family — TUI parity.
- **No emoji pictograms** (`feedback_no_icons`). Monospace glyphs only
  (`▶ ✓ ● ░▒▓█ ▰▱`) and stroke-style lucide SVGs.
- **Sharp corners universally.** `border-radius: 0 !important` (see
  `tools/tailwind/input.css`).

## Type system

One face, many voices — JetBrains Mono weights 100 → 800.

| Class         | Weight | Size      | Use                                       |
| ------------- | ------ | --------- | ----------------------------------------- |
| `.display-xl` | 100    | 72px      | Page hero on dashboard                    |
| `.display-lg` | 200    | 48px      | Section heroes (e.g. "Heute")             |
| `.display-md` | 300    | 30px      | Sub-section headers                       |
| `.readout-xl` | 700    | 32px      | Primary numeric — saldo, streak           |
| `.readout-lg` | 600    | 22px      | Secondary numeric — per-project totals    |
| `.readout-md` | 600    | 16px      | Inline numeric in tables                  |
| body          | 400    | 13px      | Default                                   |
| `.eyebrow`    | 500    | 11px caps | Pre-headline labels (tracked +0.18em)     |

Numeric content is set in tabular numerals (`font-variant-numeric: tabular-nums`)
on `.num`, `time`, `[data-num]`, and every `.readout-*` class.

## Color discipline

90% of pixels are `bg + fg + fg-dim`. Color is a **verb**, not decoration.

| Token        | Hex       | Role                                          |
| ------------ | --------- | --------------------------------------------- |
| `bg`         | `#1a1b26` | Page background (dominates)                   |
| `bg-dark`    | `#16161e` | Code blocks, editor surface, status spine     |
| `bg-soft`    | `#1f2335` | Row-hover, inline-code, subtle inset          |
| `fg`         | `#c0caf5` | Body text                                     |
| `fg-dim`     | `#9aa5ce` | Secondary text, labels, headers in tables     |
| `accent`     | `#7aa2f7` | **Interactive affordances only** + current state |
| `active`     | `#9ece6a` | **"Running right now"** — sessions, sync OK   |
| `warn`       | `#e0af68` | Conflict, sync stale, inline code highlights  |
| `err`        | `#f7768e` | Destructive actions, validation errors        |
| `muted`      | `#414868` | Borders, hairline rules                       |
| `muted-soft` | `#2a2e44` | Sub-rules, spine-bars without work            |

**Hard rules**
- `accent` never appears as a flat fill on a non-interactive element.
- `active` is reserved for live-running state. A stopped session is `fg-dim`.
- No drop shadows. No gradients (a thin grain overlay is the only texture).

## Spacing

4-px grid (`--spacing: 0.25rem`). Compose with Tailwind utilities.
- Section vertical gap: 24-32px
- Section-internal gap: 12-16px
- Inline cluster gap: 4-8px

## Layout

Editorial 70/30 — leading content column + thin right rail for metadata.
Apply with `.page-grid` (`grid-template-columns: minmax(0,1fr) 18rem` ≥ 1024px,
single column below). Section headers (`.section-head`) span both columns.

## Status spine

The signature: an 8-px-wide column glued to the left edge (`.spine` — `position:
fixed`), present on every authenticated page.

Top to bottom:
1. **State glyph** — `▶` if any session is active (color `active`, pulse 2.4s);
   otherwise `▰` (color `fg-dim`).
2. **Today's 24-hour mini-bars** — 24 vertical 4×2 cells (`.spine-bar`).
   `.has-work` → `accent` @ 0.6 opacity. The "current hour" cell gets
   `.is-now` → `active` @ 1.0.
3. **Sync dot** — `●` in `active` (recent push), `warn` (stale > 60s), or
   `fg-dim` (idle).

Updated by SSE `tick` events at 1Hz; degrades gracefully without SSE.

## Motion

Restrained but present. All transitions go through CSS, not JS.

| Trigger                  | Effect                           | Duration |
| ------------------------ | -------------------------------- | -------- |
| `hx-boost` page nav      | Opacity fade                     | 80 ms    |
| HTMX swap on partial     | `.htmx-swapping/.htmx-settling`  | 100 ms   |
| SSE-driven row update    | `.flash-row` accent background   | 600 ms   |
| Live "running since" lbl | `.live-pulse` opacity oscillation| 2 s loop |
| Spine state glyph        | `.spine-pulse`                   | 2.4 s loop |

## Components

Defined in `@layer components` of `tools/tailwind/input.css`:

- `.btn`, `.btn-primary`, `.btn-danger`, `.btn-sm` — flat single-pixel borders
- `.pill`, `.pill-active`, `.pill-warn`, `.pill-err`, `.pill-muted`
- `.nav-link`, `.subtab` — accent-underline for active state
- `.section-head`, `.rule`, `.eyebrow` — editorial chrome
- `.flash`, `.flash-ok`, `.flash-err`, `.flash-warn`
- `.editor-surface` — CodeMirror host with token-themed gutter
- `.prose-flow` — markdown render tone (replaces Tailwind's `prose-invert`)

## Charts

ApexCharts re-themed Tufte-style:
- No grid lines, no chart background.
- Axes in `muted`; ticks in `fg-dim`.
- Primary series in `accent`; "currently running" emphasis in `active`.
- Saldo bars: positive = `active`, negative = `err`.
- Tooltip background `bg-dark`, hairline `muted` border.
- Animations off (`chart.animations.enabled = false`) — they don't fit the
  static editorial vibe and a sparkline that animates on every SSE swap is
  noisy.

## Iconography

SVG inline only, stroke style (lucide-react conventions): `stroke-width: 1.5`,
`stroke="currentColor"`, no fills. Defined in
`internal/webui/templates/layout/icons.templ`.

## What we do NOT do

- No card shadows, no `border-radius`.
- No purple-gradient hero blocks ("AI slop" tell).
- No emoji pictograms (`📊🔥🎯`).
- No animation on chart redraws.
- No "Inter" / "Roboto" / "Space Grotesk" fallbacks — single family.
- No light theme. Tokyonight Night only (paired theme is on the roadmap, not in M6/M7).
