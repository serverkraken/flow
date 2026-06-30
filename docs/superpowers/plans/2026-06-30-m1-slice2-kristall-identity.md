# M1 Slice 2 — Kristall Identity (design-system foundation) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Replace the cool/crisp "Studio" look with the approved **Kristall** identity — a fixed twilight-gradient + low-poly-facet backdrop, frosted-glass cards with a signature colored corner-triangle, a hexagon logo, NO emoji, in both light and dark — at the **token + shared-component + styleguide** level. Individual page surfaces (heute/frei/nodes/etc.) adopt the glass card in their own slices; this slice shifts the whole app's palette + backdrop + brand + the canonical `Card`, and showcases it at `/ui`.

**Architecture:** Evolve the CSS-var token blocks in `web/tailwind.css` to Kristall values (so every existing `bg-surface`/`text-ink`/hue utility re-skins instantly), add a fixed twilight backdrop layer, add glass + colored-corner component classes, make `Card` glass, swap the brand mark to a hexagon SVG and the theme-toggle emoji to sun/moon SVGs, then rebuild `app.css` + `_templ.go`.

**Tech Stack:** Tailwind CSS v4.1.5 (`make web` → `static/app.css`), templ v0.3.857 (`make generate` → `_templ.go`). `make ci` runs `verify-css` (fails on `app.css` drift) + `verify-generate` (fails on `_templ.go` drift) + `verify-no-popups` (no `alert/confirm/prompt`).

## Global Constraints
- **NO emojis** — only inline SVG / geometric marks. (Geometric Unicode like `▶ ◆ ● ▲` is allowed; color-emoji like `☀ ☾` is NOT.)
- **Both themes.** Dark values are lifted verbatim from the approved mockup; light values are the derived Kristall-light below. The theme mechanism (`data-theme` on `<html>`, localStorage, the no-flash bootstrap in `base.templ`) is UNCHANGED.
- **After every `tailwind.css` edit:** run `make web` and commit the regenerated `internal/adapter/webui/static/app.css` (else `verify-css` fails CI). **After every `.templ` edit:** run `make generate` and commit the regenerated `_templ.go` (else `verify-generate` fails CI).
- Do NOT migrate page-level inline cards (heute/frei/nodes/export) here — those land in their slices. This slice changes: `web/tailwind.css`, `base.templ`, `appshell.templ`, `themetoggle.templ`, `card.templ`, `styleguide.templ`, + regenerated assets.
- All commands from `/Users/msoent/SourceCode/serverkraken/flow-m1`.

## Exact Kristall values (the design contract)

### Dark tokens — `:root[data-theme="dark"]` (lifted from the approved mockup)
```
--canvas: 28 24 56     --surface: 40 33 64    --sunken: 22 18 44
--line: 58 52 84       --line2: 46 41 68
--ink: 236 233 245     --body: 185 178 207    --muted: 154 146 181    --faint: 122 115 151
--blue: 122 162 247    --cyan: 103 232 249    --green: 110 231 183     --purple: 196 181 253
--magenta: 247 110 168 --yellow: 224 175 104  --orange: 255 158 100    --red: 247 118 142   --teal: 115 218 202
--oncolor: 22 18 43
--grad-a: 196 181 253  --grad-b: 103 232 249           /* accent gradient lavender→cyan */
--glass: 255 255 255   --glass-a: .06   --glass-strong-a: .09   --glass-border-a: .12
--backdrop: linear-gradient(150deg,#1c1838,#2c1c42 55%,#102530)
--facet-a: .5
--shadow: 0 0 0        --shadow-accent: 103 232 249    --halo: 103 232 249  --halo-a: .25
--code-bg: 10 13 24    --code-fg: 201 212 245          --scrollthumb: 49 57 84
```

### Light tokens — `:root` (derived Kristall-light; this is the piece to eyeball at /ui)
```
--canvas: 244 243 251  --surface: 255 255 255 --sunken: 238 240 250
--line: 224 222 240    --line2: 234 233 246
--ink: 27 23 48        --body: 74 69 102      --muted: 122 115 151    --faint: 165 159 191
--blue: 79 111 240     --cyan: 11 165 214     --green: 39 165 103     --purple: 139 92 246
--magenta: 225 29 116  --yellow: 217 138 11   --orange: 234 106 43    --red: 224 69 91     --teal: 14 155 142
--oncolor: 255 255 255
--grad-a: 139 92 246   --grad-b: 11 165 214
--glass: 255 255 255   --glass-a: .55   --glass-strong-a: .7   --glass-border-a: .7
--backdrop: linear-gradient(150deg,#f4f2fb,#eef0fb 55%,#eaf4f6)
--facet-a: .22
--shadow: 40 50 100    --shadow-accent: 124 92 246     --halo: 124 92 246   --halo-a: .14
--code-bg: 15 20 38    --code-fg: 201 212 245          --scrollthumb: 217 223 236
```

### Hexagon brand mark (inline SVG, viewBox 0 0 34 34)
```html
<svg viewBox="0 0 34 34" aria-hidden="true" class="..size..">
  <defs><linearGradient id="brandgrad" x1="0" y1="0" x2="1" y2="1">
    <stop offset="0" stop-color="rgb(var(--grad-a))"/><stop offset="1" stop-color="rgb(var(--grad-b))"/>
  </linearGradient></defs>
  <polygon points="17,2 31,9.5 31,24.5 17,32 3,24.5 3,9.5" fill="url(#brandgrad)" opacity=".92"/>
  <path d="M17,2 L17,32 M3,9.5 L31,24.5 M31,9.5 L3,24.5" stroke="rgb(var(--canvas))" stroke-width="1" opacity=".4"/>
</svg>
```
(The faceted hexagon: lavender→cyan gradient fill + 3 inner facet lines in the backdrop color. Two render sizes: 36px desktop, 28px mobile.)

### Theme-toggle icons (replace `☀`/`☾`)
- Sun: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>`
- Moon: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>`

---

### Task 1: Kristall tokens + twilight backdrop

**Files:** `web/tailwind.css` (token blocks + backdrop + glass vars), `internal/adapter/webui/components/base.templ` (fixed facet SVG layer), `internal/adapter/webui/static/app.css` (regenerated).

- [ ] **Step 1 — pre-flight:** `tailwindcss --help | head -1` and `go tool templ version` must both succeed. If not, STOP and report BLOCKED.
- [ ] **Step 2 — tokens:** in `web/tailwind.css`, replace the `:root` block values with the **Light tokens** above and the `:root[data-theme="dark"]` block values with the **Dark tokens** above (keep the existing var NAMES; add the new ones: `--grad-a`, `--grad-b`, `--glass`, `--glass-a`, `--glass-strong-a`, `--glass-border-a`, `--backdrop`, `--facet-a`). In the `@theme` block, add `--color-grad-a: rgb(var(--grad-a)); --color-grad-b: rgb(var(--grad-b));` (so `from-grad-a`/`to-grad-b` utilities exist). Keep all existing `@theme` mappings.
- [ ] **Step 3 — backdrop:** in `web/tailwind.css` `@layer base`, set `html { background: var(--backdrop) fixed; min-height: 100%; }` (replaces the old `html, body { background: rgb(var(--canvas)); }` — keep `body` transparent so the fixed gradient shows). Add `.kristall-facets { position: fixed; inset: 0; z-index: -1; pointer-events: none; opacity: var(--facet-a); }`.
- [ ] **Step 4 — facet SVG:** in `base.templ`, immediately inside `<body>`, add the fixed facet layer:
  ```html
  <svg class="kristall-facets" preserveAspectRatio="none" viewBox="0 0 800 560" aria-hidden="true">
    <polygon points="0,0 320,0 110,250"   fill="rgb(var(--purple))"  opacity=".18"/>
    <polygon points="800,0 800,250 470,60" fill="rgb(var(--cyan))"    opacity=".12"/>
    <polygon points="260,560 640,560 720,330" fill="rgb(var(--magenta))" opacity=".12"/>
    <polygon points="0,560 0,250 250,420"  fill="rgb(var(--blue))"    opacity=".16"/>
  </svg>
  ```
- [ ] **Step 5 — glass utilities** (added in Task 2; this task only does tokens+backdrop). Build: `make web` → confirm `static/app.css` regenerated; `make generate` (base.templ changed) → `_templ.go` regenerated.
- [ ] **Step 6 — verify + commit:** `bash scripts/verify-css.sh` (or `make web` then `git diff --quiet static/app.css` is unexpected — it SHOULD differ; verify-css passes when committed app.css == fresh build). Run `go build ./...`. Manually load `/ui` if possible is NOT required (no headless browser); the styleguide visual check is the human done-gate.
  ```bash
  git add web/tailwind.css internal/adapter/webui/components/base.templ internal/adapter/webui/components/base_templ.go internal/adapter/webui/static/app.css
  git commit -m "feat(webui): Kristall tokens + twilight facet backdrop (light+dark)"
  ```
  **There is no unit test for CSS** — the gate is `verify-css` (app.css matches `tailwind.css`) + `verify-generate` (templ) + `go build`. Note this in the report; do not fabricate a CSS test.

---

### Task 2: Glass card + colored corner

**Files:** `web/tailwind.css` (`@layer components`: `.glass`, `.glass-strong`, `.card-corner`), `internal/adapter/webui/components/card.templ`, regenerated `app.css` + `card_templ.go`.

- [ ] **Step 1 — CSS** in `web/tailwind.css` `@layer components`:
  ```css
  .glass { background: rgb(var(--glass) / var(--glass-a)); -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); border: 1px solid rgb(var(--glass) / var(--glass-border-a)); }
  .glass-strong { background: rgb(var(--glass) / var(--glass-strong-a)); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); border: 1px solid rgb(var(--glass) / var(--glass-border-a)); }
  /* colored corner triangle, top-left, clipped to the card's rounded corner via overflow-hidden on the card */
  .card-corner { position: absolute; top: 0; left: 0; width: 30px; height: 30px; clip-path: polygon(0 0, 100% 0, 0 100%); background: rgb(var(--c, var(--muted))); }
  ```
- [ ] **Step 2 — Card component** (`card.templ`): change the canonical card from `rounded-3xl bg-surface border border-line shadow-soft p-6` to a glass card that supports an optional corner hue. Read the current `Card` signature first. New shape (keep it backward-compatible — existing `@Card(class)` callers must still render):
  ```html
  templ Card(class string) {
    <article class={ "relative overflow-hidden rounded-2xl glass shadow-soft p-6", class }>
      { children... }
    </article>
  }
  // NEW variant with a project-hue corner (used by project cards later):
  templ CardCorner(hueVar, class string) {
    <article class={ "relative overflow-hidden rounded-2xl glass shadow-soft p-6 pl-7", class }>
      <span class="card-corner" style={ "--c:var(--" + hueVar + ")" } aria-hidden="true"></span>
      { children... }
    </article>
  }
  ```
  (`hueVar` is a token name like `blue`/`green`/`purple`/`cyan`. If the project carries a hex color, a follow-up maps it; here the token path is enough for the styleguide.)
- [ ] **Step 3 — build:** `make generate` + `make web`; `go build ./...`.
- [ ] **Step 4 — commit:** `feat(webui): glass Card + colored-corner CardCorner` (stage tailwind.css, card.templ, card_templ.go, app.css).

---

### Task 3: Hexagon brand mark

**Files:** `internal/adapter/webui/components/appshell.templ` (the two logo spots: lines ~11 desktop, ~29 mobile) — extract a reusable `BrandMark(sizeClass string)` templ for DRY, regenerated `appshell_templ.go`.

- [ ] **Step 1:** add `templ BrandMark(sizeClass string)` rendering the hexagon SVG above (with `class={ sizeClass }` on the `<svg>`; gradient id must be unique-safe — fine to reuse `brandgrad` since it's defined in defs each render, but to avoid duplicate-id issues when rendered twice on one page, suffix the id, e.g. derive from sizeClass or render the `<defs>` once in base.templ — SIMPLEST: keep the gradient self-contained per-svg with id `brandgrad`; duplicate ids for identical gradients render fine in browsers. If the reviewer objects, switch to a CSS-var-filled polygon without a gradient def). 
- [ ] **Step 2:** replace `appshell.templ:11` desktop `<span class="grid place-items-center h-9 w-9 rounded-xl bg-gradient-to-br from-blue to-purple text-oncolor font-display font-semibold text-lg shadow-soft">f</span>` with `@BrandMark("h-9 w-9")`; replace `:29` mobile (`h-7 w-7`) with `@BrandMark("h-7 w-7")`.
- [ ] **Step 3:** `make generate`; `go build ./...`.
- [ ] **Step 4:** commit `feat(webui): hexagon brand mark replaces f-in-box`.

---

### Task 4: Theme-toggle sun/moon SVG (de-emoji)

**Files:** `internal/adapter/webui/components/themetoggle.templ`, regenerated `_templ.go`.

- [ ] **Step 1:** replace the `☀` span (`themetoggle.templ:16`) inner content with the Sun SVG above (class `toggle-sun`), and the `☾` span (`:17`) with the Moon SVG (class `toggle-moon`). Keep the existing `.toggle-sun`/`.toggle-moon` display-swap classes + the button wrapper UNCHANGED — only the icon glyph changes from emoji to SVG. Size the SVGs `h-[1.05rem] w-[1.05rem]`.
- [ ] **Step 2:** `make generate`; `go build ./...`. Confirm `rg -n "☀|☾" internal/adapter/webui` returns nothing.
- [ ] **Step 3:** commit `feat(webui): theme-toggle sun/moon SVG (no emoji)`.

---

### Task 5: Styleguide — Kristall showcase

**Files:** `internal/adapter/webui/components/styleguide.templ`, regenerated `_templ.go`.

- [ ] **Step 1:** add sections to the existing styleguide (it already uses `Base`+`AppShell`, so the backdrop/logo/toggle render automatically): (a) a **token swatch grid** showing canvas/surface/glass + each hue; (b) **glass card demo** — a row of `@CardCorner("blue", ...)`, `@CardCorner("green", ...)`, `@CardCorner("purple", ...)`, `@CardCorner("cyan", ...)` each with a title + a line of body text, so the colored corner + frosted glass are visible in every hue; (c) the **hexagon BrandMark** at 2 sizes; (d) a note that the page is the visual review surface for both themes (toggle top-right).
- [ ] **Step 2:** `make generate`; `go build ./...`.
- [ ] **Step 3:** commit `feat(webui): styleguide showcases Kristall identity`.

---

### Task 6: Verification

- [ ] **Step 1:** `go build ./... && go vet ./...` → clean.
- [ ] **Step 2:** `make web` then `git diff --exit-code internal/adapter/webui/static/app.css` → MUST be clean (committed app.css == fresh build); if it differs, `git add` + amend the last commit. Run `bash scripts/verify-css.sh` → pass.
- [ ] **Step 3:** `make generate` then `git diff --exit-code '*_templ.go'` → clean (else commit). Run the `verify-generate` + `verify-no-popups` steps (`rg -n "alert\(|confirm\(|prompt\(" internal/adapter/webui` → none introduced).
- [ ] **Step 4:** `go test ./internal/adapter/webui/... ./internal/adapter/httpserver/...` → green (the existing webui render tests must still pass; if a test asserted the old `from-blue to-purple` logo or `☀`, update it to the new markup).
- [ ] **Step 5:** Manual done-gate (human): load `/ui` in a browser, toggle light/dark — confirm the twilight backdrop, frosted-glass cards with colored corners, the hexagon logo, and the sun/moon toggle read as the approved Kristall identity. **The derived light theme is the thing to eyeball.**

---

## Self-Review (done)
1. **Coverage:** tokens+backdrop (T1), glass-card+corner (T2), logo (T3), de-emoji toggle (T4), styleguide (T5), verify (T6). Page-card migration is explicitly deferred to per-page slices.
2. **Placeholders:** dark token values are verbatim from the approved mockup; light values are a complete derived set (not "TBD"); the backdrop, glass, corner, hexagon, and toggle SVG/CSS are exact. The one judgement call flagged for the implementer (gradient-id duplication) names the concrete fallback.
3. **Build discipline:** every CSS task ends with `make web` + commit `app.css`; every templ task with `make generate` + commit `_templ.go`; verified by `verify-css`/`verify-generate` in T6.

## Notes for the executor
- There are NO unit tests for CSS appearance — the gates are `verify-css` (app.css ≡ tailwind.css build), `verify-generate`, `verify-no-popups`, `go build`, and the EXISTING webui render tests (which assert structure/strings — update any that pinned the old logo/emoji). The visual correctness is the human's `/ui` done-gate.
- Keep `body` background transparent so the fixed `html` twilight gradient shows through; the sidebar/mobile bars already use `bg-surface/80 backdrop-blur-xl` which now reads as Kristall glass over the gradient.
- This slice re-skins EVERY existing page's palette (tokens) + drops the twilight backdrop under everything, even before per-page glass migration — that intermediate state is intentional and coherent (Kristall colors everywhere; full glass per-page as slices land).
