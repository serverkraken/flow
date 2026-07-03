# Kristall K5 — Politur & Gate — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the Kristall redesign — fix the Live-Audit findings (mobile reflow, light contrast, CTA hierarchy, auth-page SSE), verify A11y, then gate + merge `cockpit-story → rebuild`.

**Architecture:** WebUI-only slice. templ components under `internal/adapter/webui`, Tailwind in `web/tailwind.css`, i18n in `internal/i18n/catalog_{de,en}.go`. Changes are additive/override — no new ports, no domain/usecase changes, no new server routes (except one demote already handled by CSS). Design spec: [[specs/2026-07-03-kristall-k5-politur-gate-design]].

**Tech Stack:** Go, templ (`go tool templ generate`), Tailwind v4 CLI (`make web`), goldmark N/A, testcontainers N/A (no pgstore change).

## Global Constraints

- **Multi-tenant, owner-scoped** stays binding — no fix introduces un-keyed global state, caches, or singletons. (Umbrella §0.)
- **No `make fmt`** (toolchain skew reformats the whole repo — AGENTS.md).
- **i18n de+en parity**: every new user-visible string gets a key in BOTH `internal/i18n/catalog_de.go` AND `internal/i18n/catalog_en.go`. (This slice adds few-to-none.)
- **templ**: after editing any `*.templ`, run `go tool templ generate` (`make generate`) and commit the resulting `*_templ.go`.
- **CSS**: after adding any new Tailwind utility class used in a `*.templ`, or editing `web/tailwind.css`, run `make web` (rebuilds `internal/adapter/webui/static/app.css`) and **commit app.css**. `make web` needs the `tailwindcss` CLI and is NOT part of `make ci`.
- **Guards stay sharp**: `bash scripts/verify-css.sh` (compiled utilities present in app.css) + `bash scripts/verify-no-popups.sh` (no `alert(`/`confirm(`/browser popups) must pass.
- **Gate**: `make ci` GREEN (lint + verify-generate + verify-css + verify-no-popups + cover ≥75% [`*_templ.go` excluded] + build) before "done".
- **Branch**: `cockpit-story` in worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. Base HEAD `791e4ba` (the K5 spec commit).
- **Dev stack for the live-gate** is already up: Postgres + Dex on `podman`, `flow-server` on `https://localhost:8080` (self-signed). Bearer token: `TOK=$(scripts/dev-token.sh)`. Screenshot harness: `/private/tmp/claude-501/-Users-msoent-SourceCode-serverkraken-flow-rebuild/b36fbc8c-8fa1-4fd8-a541-6f0613b1dcbb/scratchpad/shoot.mjs` (Playwright, Dex-login + light/mobile matrix).

---

### Task 1: M1 — shared `activityFeedRow` component + mobile reflow

The activity feed (`home.templ`) and cockpit pulse (`cockpit_uebersicht.templ`) render two near-duplicate `<li>` rows over the **same** type `ActivityRowVM`. At ≤~420px the single-line `flex items-center` forces verbs to char-wrap into skinny columns and pushes the target pill out of the card's right edge. Fix: extract ONE shared always-stacked row (actor+time on top, verb+pill+label wrapping below, contained in the card), and rewire both call sites.

**Files:**
- Create: `internal/adapter/webui/activity_row.templ`
- Modify: `internal/adapter/webui/home.templ:124-142` (activity `<li>` loop) and remove `activityTargetPill` (moves to the new file, currently `home.templ:168-179`)
- Modify: `internal/adapter/webui/cockpit_uebersicht.templ:162-180` (pulse `<li>` loop)
- Test: `internal/adapter/webui/activity_row_test.go`

**Interfaces:**
- Consumes: `ActivityRowVM` (`internal/adapter/webui/activity_row.go:12-24`, fields `ActorKind, ActorRef, VerbKey, Label, Href, RelTime, TargetName, TargetKind, TargetHref`), `components.ActorGlyph`, `components.T`, `NodeKindStyle`, `kindToneClass`.
- Produces: `templ activityFeedRow(row ActivityRowVM, showAgentChip bool, onPrefix bool)` and `templ activityTargetPill(row ActivityRowVM)` — both in package `webui`, callable from any `*.templ` in the package.

- [ ] **Step 1: Write the failing test.** Create `internal/adapter/webui/activity_row_test.go`:

```go
package webui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/i18n"
)

func renderRow(t *testing.T, row ActivityRowVM, agentChip, onPrefix bool) string {
	t.Helper()
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	var b bytes.Buffer
	if err := activityFeedRow(row, agentChip, onPrefix).Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestActivityFeedRow_StacksActorAndTime(t *testing.T) {
	row := ActivityRowVM{ActorKind: "human", ActorRef: "msoent", VerbKey: "activity.verb.session.stopped", RelTime: "vor 1 Std", TargetName: "RTL Extern", TargetHref: "/nodes/x"}
	html := renderRow(t, row, false, true)
	// two-line structure: actor+time in the first flex row, detail in the second
	if strings.Count(html, "<div") < 2 {
		t.Fatalf("expected two stacked divs, got: %s", html)
	}
	// timestamp present, and NOT wrapped in a nowrap single-line container
	if !strings.Contains(html, "vor 1 Std") {
		t.Fatalf("reltime missing: %s", html)
	}
	// target pill contained (border pill), not a bare overflowing span
	if !strings.Contains(html, "RTL Extern") {
		t.Fatalf("target pill missing: %s", html)
	}
	// the detail row wraps (flex-wrap) so the pill can drop to a new line inside the card
	if !strings.Contains(html, "flex-wrap") {
		t.Fatalf("detail row must be flex-wrap: %s", html)
	}
}

func TestActivityFeedRow_AgentChipOnlyWhenAsked(t *testing.T) {
	agent := ActivityRowVM{ActorKind: "agent", ActorRef: "claude", VerbKey: "activity.verb.document.created", RelTime: "vor 2 Min"}
	// The AGENT chip is the only element carrying text-purple; assert on that
	// class rather than the localized chip text.
	withChip := renderRow(t, agent, true, false)
	without := renderRow(t, agent, false, false)
	if !strings.Contains(withChip, "text-purple") {
		t.Fatalf("agent chip expected when showAgentChip=true: %s", withChip)
	}
	if strings.Contains(without, "text-purple") {
		t.Fatalf("agent chip must be absent when showAgentChip=false: %s", without)
	}
}
```

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/adapter/webui/ -run TestActivityFeedRow -v` → FAIL (`activityFeedRow` undefined).

- [ ] **Step 3: Create the shared component.** Write `internal/adapter/webui/activity_row.templ`:

```templ
package webui

import "github.com/serverkraken/flow/internal/adapter/webui/components"

// activityFeedRow is the ONE shared activity/pulse row (Home feed + Cockpit
// pulse), rendering ActivityRowVM. It always stacks: actor + relative time on
// the top line, then verb + target pill + label on a flex-wrap detail line so
// the pill drops to a new line WITHIN the card instead of overflowing at
// narrow widths (K5 M1). showAgentChip adds the AGENT badge for agent actors
// (pulse only); onPrefix inserts the "auf/on" word before the pill (Home feed).
templ activityFeedRow(row ActivityRowVM, showAgentChip bool, onPrefix bool) {
	<li class="px-4 py-3 text-[.88rem]">
		<div class="flex items-baseline gap-2">
			<span class="shrink-0 text-muted">@components.ActorGlyph(row.ActorKind)</span>
			<span class="min-w-0 flex-1 truncate font-medium text-body">{ row.ActorRef }</span>
			<span class="shrink-0 text-[.75rem] text-faint tnum">{ row.RelTime }</span>
		</div>
		<div class="mt-0.5 flex flex-wrap items-center gap-x-1.5 gap-y-1 pl-6">
			if showAgentChip && row.ActorKind == "agent" {
				<span class="inline-flex shrink-0 items-center rounded-md border border-purple/40 px-1.5 py-0.5 text-[.62rem] font-semibold uppercase text-purple">{ components.T(ctx, "cockpit.pulse.agent") }</span>
			}
			<span class="text-muted">{ components.T(ctx, row.VerbKey) }</span>
			if row.TargetName != "" {
				if onPrefix {
					<span class="text-muted">{ components.T(ctx, "activity.on") }</span>
				}
				@activityTargetPill(row)
			}
			if row.Href != "" {
				<a href={ templ.SafeURL(row.Href) } class="min-w-0 truncate font-medium text-blue hover:underline">{ row.Label }</a>
			} else if row.Label != "" {
				<span class="min-w-0 truncate text-body">{ row.Label }</span>
			}
		</div>
	</li>
}

// activityTargetPill renders the form-coded target of a session activity row:
// a kind-glyph + node name, tinted by the node kind, linking to the node when
// it still exists. Moved here from home.templ (K5 M1 — shared by feed+pulse).
templ activityTargetPill(row ActivityRowVM) {
	{{ k := NodeKindStyle(row.TargetKind) }}
	if row.TargetHref != "" {
		<a href={ templ.SafeURL(row.TargetHref) } class={ "inline-flex shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 text-[.75rem] font-medium hover:underline", kindToneClass(k.Tone) }>
			<span aria-hidden="true">{ k.Glyph }</span> { row.TargetName }
		</a>
	} else {
		<span class="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-line px-2 py-0.5 text-[.75rem] font-medium text-muted">
			{ row.TargetName }
		</span>
	}
}
```

- [ ] **Step 4: Remove the old `activityTargetPill` from `home.templ`.** Delete the `templ activityTargetPill(row ActivityRowVM) { … }` block at `home.templ:168-179` (it now lives in `activity_row.templ`; leaving it causes a duplicate-declaration compile error). Keep the surrounding comment on the feed loop if present.

- [ ] **Step 5: Rewire Home feed.** In `home.templ`, replace the `<li>…</li>` block (lines ~125-140, inside `for _, row := range vm.LogEntries {`) with a single call:

```templ
			for _, row := range vm.LogEntries {
				@activityFeedRow(row, false, true)
			}
```

- [ ] **Step 6: Rewire Cockpit pulse.** In `cockpit_uebersicht.templ`, replace the `<li>…</li>` block (lines ~163-179, inside `for _, row := range vm.Pulse {`) with:

```templ
				for _, row := range vm.Pulse {
					@activityFeedRow(row, true, false)
				}
```

- [ ] **Step 7: Generate + test.** `make generate && go test ./internal/adapter/webui/ -run 'TestActivityFeedRow' -v` → PASS. Then `go test ./internal/adapter/webui/... ./internal/adapter/httpserver/ -run 'Home|Cockpit|Pulse|Activity'` → green (existing home/cockpit render tests still pass; if an existing test asserted the old single-`<li>`-`flex items-center` markup, update that assertion to the new stacked structure).

- [ ] **Step 8: Rebuild CSS + guards.** `make web && bash scripts/verify-css.sh && bash scripts/verify-no-popups.sh` → all green (new utilities `gap-x-1.5`, `gap-y-1`, `pl-6`, `items-baseline` must appear in app.css).

- [ ] **Step 9: Commit.**

```bash
git add internal/adapter/webui/activity_row.templ internal/adapter/webui/activity_row_templ.go \
  internal/adapter/webui/home.templ internal/adapter/webui/home_templ.go \
  internal/adapter/webui/cockpit_uebersicht.templ internal/adapter/webui/cockpit_uebersicht_templ.go \
  internal/adapter/webui/activity_row_test.go internal/adapter/webui/static/app.css
git commit -m "fix(kristall): activity/pulse rows stack + wrap on mobile (K5 M1)"
```

---

### Task 2: M2 — cockpit tab strip scroll affordance + scroll-active-into-view

`.pill-tabs` already has `overflow-x:auto` (`web/tailwind.css:379`), so at 390px the strip IS swipeable — but there is no affordance (no visible scrollbar on mobile) and the active tab is not scrolled into view, so "Struktur/Bindings" read as unreachable. Fix: a right-edge fade mask when scrollable + a tiny inline script that scrolls the active tab into view on load/swap. No layout width change needed (`#cockpit-main` already has `min-w-0`, `cockpit.templ:39`).

**Files:**
- Modify: `web/tailwind.css:379` (add a `.pill-tabs` scroll-fade affordance)
- Modify: `internal/adapter/webui/cockpit.templ:238-244` (`CockpitTabsAndPanel` — add the scroll-into-view script + a wrapper hook)
- Test: assertion added to an existing cockpit render test (`internal/adapter/httpserver/webui_cockpit_test.go`)

**Interfaces:**
- Consumes: `CockpitTabs`, `cockpitTabLink` (unchanged).
- Produces: no new Go symbols; a `data-tabstrip` hook + CSS class `.pill-tabs-fade`.

- [ ] **Step 1: Write the failing assertion.** In `internal/adapter/httpserver/webui_cockpit_test.go`, in the test that renders a cockpit page (find the existing `TestCockpit…` that asserts the tab strip), add:

```go
	if !strings.Contains(body, "data-tabstrip") {
		t.Fatalf("cockpit tab strip must carry the data-tabstrip scroll hook")
	}
```

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/adapter/httpserver/ -run TestCockpit -v` → FAIL (`data-tabstrip` absent).

- [ ] **Step 3: Add the CSS affordance.** In `web/tailwind.css`, directly after line 379 (`.pill-tabs { … overflow-x:auto; }`), add inside the same `@layer components` block:

```css
  /* K5 M2: right-edge fade cues that the pill strip scrolls horizontally when
     it overflows (mobile has no visible scrollbar). Pure affordance — the
     strip already scrolls via overflow-x:auto. */
  .pill-tabs { position: relative; scrollbar-width: none; -webkit-overflow-scrolling: touch; scroll-snap-type: none; -webkit-mask-image: linear-gradient(to right, #000 calc(100% - 22px), transparent); mask-image: linear-gradient(to right, #000 calc(100% - 22px), transparent); }
  .pill-tabs::-webkit-scrollbar { display: none; }
```

Note: the mask fades the right edge always; the fade over an already-fully-visible strip is negligible (the last 22px of padding). This keeps the rule dependency-free (no JS scroll-state class).

- [ ] **Step 4: Add the hook + scroll-into-view script.** In `internal/adapter/webui/cockpit.templ`, change the `<nav class="pill-tabs">` in `CockpitTabsAndPanel` (line 238) to `<nav class="pill-tabs" data-tabstrip>` and append, immediately after the closing `</nav>` (before the `<div id="cockpit-panel"…>`):

```templ
	<script>
		(function () {
			// Scroll the active cockpit tab into view so Struktur/Bindings are
			// discoverable on narrow viewports (K5 M2). Runs on load and after
			// every htmx swap that re-renders the strip.
			function reveal() {
				document.querySelectorAll('[data-tabstrip]').forEach(function (nav) {
					var a = nav.querySelector('[aria-current="page"]');
					if (a && a.scrollIntoView) a.scrollIntoView({ inline: 'center', block: 'nearest' });
				});
			}
			reveal();
			document.body.addEventListener('htmx:afterSwap', reveal);
		})();
	</script>
```

- [ ] **Step 5: Generate + rebuild + test.** `make generate && go test ./internal/adapter/httpserver/ -run TestCockpit -v` → PASS. `make web && bash scripts/verify-css.sh && bash scripts/verify-no-popups.sh` → green (the script uses no `alert/confirm`, so verify-no-popups stays green; `scrollIntoView` is allowed).

- [ ] **Step 6: Commit.**

```bash
git add web/tailwind.css internal/adapter/webui/cockpit.templ internal/adapter/webui/cockpit_templ.go \
  internal/adapter/webui/static/app.css internal/adapter/httpserver/webui_cockpit_test.go
git commit -m "fix(kristall): cockpit tab strip scroll affordance + reveal active tab (K5 M2)"
```

---

### Task 3: L1 — light-theme `.field` border contrast

In light theme the `.field` background `rgb(var(--sunken)/.6)` (238 240 250 ≈ #EEF0FA) sits nearly on the white page and the `1px solid rgb(var(--line))` border (224 222 240) is too faint — empty fields (Einstellungen weekday inputs, Editor, Node form) read as barely-there. Fix: a light-only stronger field border via a `[data-theme=light] .field` override (smallest intervention; dark unchanged). Decision from spec §6: override, not a new token.

**Files:**
- Modify: `web/tailwind.css` (add a light override after the `.field` block, ~line 362)

- [ ] **Step 1: Add the override.** In `web/tailwind.css`, inside the same `@layer components` block, directly after `.field::placeholder { … }` (line 362), add:

```css
  /* K5 L1: on light glass, --line (pale lavender) nearly vanishes over the
     near-white field bg. Give light-theme fields a stronger, still-soft
     border so empty inputs read as fields. Dark keeps the original. */
  :root[data-theme="light"] .field { border-color: rgb(198 200 222); background: rgb(255 255 255 / .75); }
```

- [ ] **Step 2: Rebuild + guards.** `make web && bash scripts/verify-css.sh && bash scripts/verify-no-popups.sh` → green.

- [ ] **Step 3: Visual verify (screenshot).** Re-shoot Einstellungen + Editor + Node-form in light and confirm field borders read clearly (harness in Task 8; for a quick single check:

```bash
cd "/private/tmp/claude-501/-Users-msoent-SourceCode-serverkraken-flow-rebuild/b36fbc8c-8fa1-4fd8-a541-6f0613b1dcbb/scratchpad" && node shoot.mjs
```

then Read `shots/einstellungen__light-1440.png`). Borders on the weekday inputs must be clearly visible.

- [ ] **Step 4: Commit.**

```bash
git add web/tailwind.css internal/adapter/webui/static/app.css
git commit -m "fix(kristall): stronger .field border in light theme (K5 L1)"
```

---

### Task 4: L2 — sidebar nav scrollable (last item no longer clipped)

The sidebar `<aside>` is `fixed inset-y-4 overflow-hidden` (`appshell.templ:24`) with NO scrollable child — on short viewports the content (brand + timer widget + primary nav + node tree + divider + 4 secondary items + logout) exceeds the aside height and the last item ("Einstellungen") is clipped with nothing to scroll it into view. Fix: make the `SiteNav` `<nav class="flex-1">` a scroll region (`min-h-0 overflow-y-auto`), so the tree/nav scrolls while brand/timer (above) and logout (below) stay pinned.

**Files:**
- Modify: `internal/adapter/webui/components/sitenav.templ:43` (`<nav class="flex-1 px-3 space-y-1" …>`)
- Test: `internal/adapter/webui/components/sitenav_test.go` (or add to an existing sitenav render test)

**Interfaces:** no new symbols; a class change on the nav container.

- [ ] **Step 1: Write the failing assertion.** In a sitenav render test (create `internal/adapter/webui/components/sitenav_test.go` if none renders `SiteNav`):

```go
package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/i18n"
)

func TestSiteNav_ScrollRegion(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	var b bytes.Buffer
	if err := SiteNav("home").Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := b.String()
	if !strings.Contains(html, "overflow-y-auto") || !strings.Contains(html, "min-h-0") {
		t.Fatalf("SiteNav nav must be a scroll region (min-h-0 overflow-y-auto): %s", html)
	}
}
```

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/adapter/webui/components/ -run TestSiteNav_ScrollRegion -v` → FAIL.

- [ ] **Step 3: Make the nav scrollable.** In `internal/adapter/webui/components/sitenav.templ`, change line 43 from:

```templ
	<nav class="flex-1 px-3 space-y-1" aria-label={ T(ctx, "nav.primary") }>
```

to:

```templ
	<nav class="flex-1 min-h-0 overflow-y-auto px-3 space-y-1 scroll-thin" aria-label={ T(ctx, "nav.primary") }>
```

- [ ] **Step 4: Add a thin scrollbar style (optional polish).** In `web/tailwind.css` `@layer components`, add:

```css
  /* K5 L2: quiet scrollbar for the sidebar nav scroll region */
  .scroll-thin { scrollbar-width: thin; scrollbar-color: rgb(var(--scrollthumb)) transparent; }
  .scroll-thin::-webkit-scrollbar { width: 6px; }
  .scroll-thin::-webkit-scrollbar-thumb { background: rgb(var(--scrollthumb)); border-radius: 999px; }
```

(`--scrollthumb` already exists in both themes — light `217 223 236` at css:71, verify a dark value exists; if not, this rule still degrades to the browser default.)

- [ ] **Step 5: Generate + rebuild + test.** `make generate && go test ./internal/adapter/webui/components/ -run TestSiteNav -v` → PASS. `make web && bash scripts/verify-css.sh && bash scripts/verify-no-popups.sh` → green.

- [ ] **Step 6: Visual verify.** Re-shoot any page at a short viewport (e.g. 1440×720) and confirm "Einstellungen" + logout are reachable (scroll) rather than clipped.

- [ ] **Step 7: Commit.**

```bash
git add internal/adapter/webui/components/sitenav.templ internal/adapter/webui/components/sitenav_templ.go \
  internal/adapter/webui/components/sitenav_test.go web/tailwind.css internal/adapter/webui/static/app.css
git commit -m "fix(kristall): sidebar nav scrolls; last item no longer clipped (K5 L2)"
```

---

### Task 5: H1 — demote the idle timer-start CTA on node-cockpit pages

On a node cockpit the sidebar timer widget's **idle** state shows a full green "Timer starten" CTA that competes with the rail's green "Start". (When the timer runs, the widget shows a clock, not a Start CTA — so the conflict is idle-only.) The shell has zero route awareness (`AppShell` only gets the nav key), and threading route context through all 16 `AppShell` callers is disproportionate for a hierarchy polish. Chosen approach (supersedes the spec §6 "compact chip" idea — documented here): a pure-CSS demote scoped by `body:has(#cockpit-rail)` (the rail id exists only on cockpit pages), reducing the widget's start CTA to a quiet button so the rail Start is the one visual primary. No Go/handler/VM change.

**Files:**
- Modify: `web/tailwind.css` (add the `:has()` demote rule)

**Interfaces:** none. Relies on the stable ids `#cockpit-rail` (`cockpit.templ:33`) and `#timer-widget` (`appshell.templ:33`), and the widget's primary submit button rendered by `components.Button(components.BtnPrimary, …)` inside `#timer-widget`.

- [ ] **Step 1: Confirm the Button primary class.** Read `internal/adapter/webui/components/button.templ` to find the exact class the `BtnPrimary` variant emits (the green gradient CTA — e.g. `cta`/`bg-gradient-to-r from-green to-cyan`). Note the selector-stable class (call it `<PRIMARY_CLASS>`; most likely `.cta` or the gradient utilities). If BtnPrimary uses a single semantic class (e.g. `.cta`), target that; if it uses raw gradient utilities, target `#timer-widget button[type="submit"]`.

- [ ] **Step 2: Add the demote rule.** In `web/tailwind.css` `@layer components`, add:

```css
  /* K5 H1: on a node-cockpit page (only there does #cockpit-rail exist) the
     rail's object-bound "Start" is the primary action. Demote the GLOBAL
     sidebar timer widget's idle start button to a quiet secondary so two green
     Start CTAs don't compete. Running state shows a clock (no start button),
     so this only affects idle. :has() unsupported → widget stays full (safe). */
  body:has(#cockpit-rail) #timer-widget form[hx-post="/ui/timer/start"] button[type="submit"] {
    background: rgb(var(--glass) / .07) !important;
    color: rgb(var(--body)) !important;
    box-shadow: inset 0 0 0 1px rgb(var(--glass) / var(--glass-border-a));
  }
```

(If Step 1 found a single primary class like `.cta`, use `body:has(#cockpit-rail) #timer-widget .cta { … }` instead — same declarations. Prefer the class selector if it exists; it's more robust than the `form[hx-post]` path.)

- [ ] **Step 3: Rebuild + guards.** `make web && bash scripts/verify-css.sh && bash scripts/verify-no-popups.sh` → green.

- [ ] **Step 4: Visual verify (both states).** With the dev timer IDLE, shoot the cockpit page (`cockpit-eng`) in dark+light and confirm the sidebar widget's "Timer starten" is now a quiet button while the rail "Start" is the sole green CTA. Then confirm a NON-cockpit page (Home) still shows the full green widget CTA (the `:has` scope must not leak). Read `shots/cockpit-eng__dark-390.png` (or 1440) + `shots/home__light-1440.png`.

- [ ] **Step 5: Commit.**

```bash
git add web/tailwind.css internal/adapter/webui/static/app.css
git commit -m "fix(kristall): demote global timer start CTA on node-cockpit (K5 H1)"
```

---

### Task 6: A2 — auth pages without SSE (`BaseNoSSE`)

`components.Base` hard-codes `hx-ext="sse" sse-connect="/api/v1/events"` on `<body>` (`base.templ:37`). `AuthPage` renders through `Base`, so the logout landing + 3 OIDC-error pages open an SSE connection that 401s (no session) and retries. Fix: factor the hull into an internal `baseHull(active, sse, content)`; keep `Base` (sse=true, all existing callers unchanged) and add `BaseNoSSE` (sse=false); point `AuthPage` at `BaseNoSSE`.

**Files:**
- Modify: `internal/adapter/webui/components/base.templ` (introduce `baseHull`, keep `Base`, add `BaseNoSSE`)
- Modify: `internal/adapter/webui/auth.templ:19` (`AuthPage` → `components.BaseNoSSE`)
- Test: `internal/adapter/webui/auth_render_test.go` (extend the existing K4 auth test)

**Interfaces:**
- Produces: `templ Base(active string, content templ.Component)` (unchanged public signature), `templ BaseNoSSE(active string, content templ.Component)`, internal `templ baseHull(active string, sse bool, content templ.Component)`.

- [ ] **Step 1: Write the failing test.** In `internal/adapter/webui/auth_render_test.go` add:

```go
func TestAuthPage_NoSSEConnect(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	var b bytes.Buffer
	if err := AuthPage(AuthVM{TitleKey: "auth.loggedOut.title", MsgKey: "auth.loggedOut.msg", ShowLogin: true}).Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(b.String(), "sse-connect") {
		t.Fatalf("auth page must NOT open an SSE connection")
	}
}
```

(Ensure the file already imports `bytes`, `context`, `strings`, `testing`, and `i18n`; add any missing.)

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/adapter/webui/ -run TestAuthPage_NoSSEConnect -v` → FAIL (`sse-connect` present via `Base`).

- [ ] **Step 3: Refactor `base.templ`.** Open `internal/adapter/webui/components/base.templ`. Rename the existing `templ Base(active string, content templ.Component) { … }` body into a new internal `templ baseHull(active string, sse bool, content templ.Component) { … }`, and on the `<body>` (line 37) make the SSE attributes conditional:

```templ
		<body class="font-sans text-ink antialiased selection:bg-blue/15 selection:text-ink"
			if sse {
				hx-ext="sse" sse-connect="/api/v1/events"
			}
		>
```

Then add the two thin entry points:

```templ
// Base is the shared HTML hull with live-sync (SSE) enabled — the default for
// every authenticated page.
templ Base(active string, content templ.Component) {
	@baseHull(active, true, content)
}

// BaseNoSSE is the hull WITHOUT the SSE connection — for pre-auth pages
// (logout landing, OIDC-callback errors) where /api/v1/events would 401 and
// retry pointlessly (K5 A2).
templ BaseNoSSE(active string, content templ.Component) {
	@baseHull(active, false, content)
}
```

Keep everything else in the hull (head, theme no-flash script, timer JS, facets, etc.) exactly as-is.

- [ ] **Step 4: Point AuthPage at BaseNoSSE.** In `internal/adapter/webui/auth.templ:19`, change:

```templ
	@components.Base("auth", authBody(vm))
```

to:

```templ
	@components.BaseNoSSE("auth", authBody(vm))
```

- [ ] **Step 5: Generate + test.** `make generate && go test ./internal/adapter/webui/ -run 'TestAuthPage' -v` → PASS (new no-SSE test + existing K4 auth tests). Then `go test ./internal/adapter/webui/... ./internal/adapter/httpserver/ -run 'Base|Auth|Home|Logout|Callback'` → green (existing pages still get SSE via `Base`).

- [ ] **Step 6: Guards.** `make web && bash scripts/verify-css.sh && bash scripts/verify-no-popups.sh` → green (no new utilities; app.css unchanged is fine).

- [ ] **Step 7: Commit.**

```bash
git add internal/adapter/webui/components/base.templ internal/adapter/webui/components/base_templ.go \
  internal/adapter/webui/auth.templ internal/adapter/webui/auth_templ.go \
  internal/adapter/webui/auth_render_test.go
git commit -m "fix(kristall): auth pages render without SSE connect (K5 A2)"
```

---

### Task 7: A11y verification pass (verify; fix only real gaps)

The audit dossier found A11y largely already covered: the global `@media (prefers-reduced-motion: reduce)` block (`web/tailwind.css:240`) neutralizes all animations/transitions (so `breathe`/`rise`/pulse/glow honor it); the node-tree label has `title` (`navtree.templ:33`); the document delete button, mobile timer chip, MoreMenu close, and cockpit session-delete all carry `aria-label`; the shell timer start/stop/switch buttons render visible text (not icon-only). This task VERIFIES that baseline and fixes only genuine gaps it surfaces.

**Files:**
- Modify (only if a gap is found): the offending `*.templ`
- Modify (only if a new string is needed): `internal/i18n/catalog_de.go` + `internal/i18n/catalog_en.go`

- [ ] **Step 1: Reduced-motion coverage.** Confirm the rule exists and covers the animated elements:

```bash
rg -n "prefers-reduced-motion" web/tailwind.css
rg -n "animate-breathe|animate-rise|@keyframes" web/tailwind.css internal/adapter/webui --iglob '*.templ'
```

Expected: the `@media (prefers-reduced-motion: reduce) { *,*::before,*::after { animation:none…} }` at css:240 covers every `animate-*`. If any animation is applied via inline style or JS (not a CSS class caught by the blanket rule), add a guard. (Timer JS already checks `matchMedia('(prefers-reduced-motion: reduce)')` — confirm, don't duplicate.)

- [ ] **Step 2: Icon-only button audit.** Find every button whose only child is a glyph/svg and check for `aria-label`:

```bash
rg -n "<button" internal/adapter/webui --iglob '*.templ' -A3 | rg -n "aria-label" -B3 || true
# and the inverse — buttons WITHOUT an obvious text label:
rg -rn "<button" internal/adapter/webui --iglob '*.templ'
```

Manually confirm each icon-only button has `aria-label={ T(ctx, key) }`. Known-good (do NOT touch): themetoggle, document delete (`document.templ:76`), TimerChip (`timerwidget.templ:76`), MoreMenu close (`appshell.templ:118`), cockpit session delete (`cockpit.templ:462`). If a genuinely un-labeled icon-only button is found, add `aria-label` using the ThemeToggle pattern (`aria-label={ components.T(ctx, "<area>.<action>") }`) and add the key to both catalogs.

- [ ] **Step 3: Fade-truncation titles.** Confirm truncated labels expose the full text via `title`:

```bash
rg -n "fade-label|truncate" internal/adapter/webui --iglob '*.templ' | rg -i "name|label"
```

The node-tree anchor has `title={ row.Node.Name }` (`navtree.templ:33`) ✓. If any other `fade-label`/`truncate` name-bearing element lacks a `title`, add one. (The new `activityFeedRow` actor/label are short and truncate rarely — add `title` only if a real overflow case exists.)

- [ ] **Step 4: If any gap was fixed:** `make generate` (+ `make web` if a class changed), re-run the relevant render test, guards green, and commit:

```bash
git add -A && git commit -m "fix(kristall): a11y gaps from K5 verification pass"
```

If NO gap was found, record that in the task note and skip the commit — the pass is a clean verify. Either way, the reduced-motion + labels baseline is now confirmed for the K5 gate.

---

### Task 8: Wiring verification + full live re-audit (main gate task)

No new server routes/handlers were added (H1/A2 are template/CSS-scoped; the `/ui/timer` fragment is unchanged), so "wiring" here means: everything generated is committed, `make ci` is green, and the fixed surfaces are re-audited against the live stack with the same screenshot harness that found them. This is the plan's explicit verification task (per the "plans need a main-wiring task" rule) — even though this slice adds no composition-root wiring, the re-audit closes the loop.

**Files:** none (verification only).

- [ ] **Step 1: Generated + built artifacts committed.** `git status --porcelain` → clean. `make generate && git diff --exit-code` → no drift (all `*_templ.go` committed). `make web && git diff --exit-code internal/adapter/webui/static/app.css` → no drift (app.css committed).

- [ ] **Step 2: Full CI gate.**

```bash
make ci
```

Expected: GREEN (lint + verify-generate + verify-css + verify-no-popups + cover ≥75% + build). Fix anything red before proceeding. (If cover dipped below 75% because new templ helpers are excluded but new `.go` test-less code was added — there is none here; all new logic is templ or CSS — investigate any real regression.)

- [ ] **Step 3: Restart the dev server on the new build.** The dev server runs `go run ./cmd/flow-server`; restart it so the new templ/CSS are served (find + restart the `make dev-run` process, or re-run it). Confirm `curl -sk -o /dev/null -w '%{http_code}\n' https://localhost:8080/` → `302` (auth redirect) and, with a token, `curl -sk -H "Authorization: Bearer $(scripts/dev-token.sh)" -o /dev/null -w '%{http_code}\n' https://localhost:8080/api/v1/me` → `200`.

- [ ] **Step 4: Re-run the screenshot matrix.**

```bash
cd "/private/tmp/claude-501/-Users-msoent-SourceCode-serverkraken-flow-rebuild/b36fbc8c-8fa1-4fd8-a541-6f0613b1dcbb/scratchpad" && node shoot.mjs
```

- [ ] **Step 5: Verify each fix in the shots** (Read the PNGs):
  - **M1** — `home__light-390.png` + `cockpit-eng__light-390.png`: activity/pulse rows are two-line stacks, no node pill overflows the card's right edge, no char-wrapped verbs.
  - **M2** — `cockpit-eng__light-390.png`: the right-edge fade is visible on the tab strip (affordance); (swipe/scroll-into-view can't be screenshot-verified — note it as manually confirmed).
  - **L1** — `einstellungen__light-1440.png` + `editor__light-390.png` + `node-new__light-390.png`: field borders clearly visible.
  - **L2** — a short-viewport shot (add a `['light',1440,720,'light-720']` combo temporarily or resize): "Einstellungen" + logout reachable, not clipped.
  - **H1** — `cockpit-eng__dark-390.png`/`cockpit-eng__light-1440.png`: sidebar widget idle-start is quiet; rail "Start" is the sole green CTA. `home__light-1440.png`: Home widget still full green (no `:has` leak).
  - **A2** — not visual; confirmed by the render test (Task 6).

- [ ] **Step 6: Record the re-audit** — note per-finding pass/fail. Any regression → back to the owning task. No new commit needed (verification only) unless a shot exposes a miss.

---

### Task 9: Gate — dogfood + merge prep (human gate; NOT a subagent task)

This is the terminal gate. It is performed WITH Soenne, not by an implementation subagent.

- [ ] **Step 1: Opus whole-branch review.** Request a holistic review of the full `cockpit-story` branch diff (or at least the K1→K5 delta) — the past pattern has caught SSE-live-sync and focus-desync bugs that per-task reviews missed. Address Critical/Major findings before merge.
- [ ] **Step 2: Soenne dogfood.** Soenne walks the app: light-toggle across pages (contrast), mobile reflow (activity rows, cockpit tabs, sidebar), timer CTA hierarchy on a cockpit, logout/error pages (no console SSE 401 spam). Collect findings; fix as a follow-up commit round if any.
- [ ] **Step 3: Clean up the dev fixtures.** Remove the K5 audit seed document (created during the audit): `curl -sk -X DELETE -H "Authorization: Bearer $(scripts/dev-token.sh)" https://localhost:8080/api/v1/documents/e28e3f2a-8275-4f18-a6ff-4c5f5fe41ab2`. Remove the scratchpad Playwright harness/screenshots (session-isolated, but tidy: it lives under the session scratchpad and is auto-cleaned).
- [ ] **Step 4: Mirror the K5 spec + plan to flow** (`flow_create_doc`, types `spec`/`plan`) once the flow server's OIDC auth is reachable again (it was failing with "context canceled" during planning).
- [ ] **Step 5: Merge `cockpit-story → rebuild`.** Per superpowers:finishing-a-development-branch — with Soenne's go, integrate the whole Kristall program. Update the memory index entry for the Kristall redesign to DONE.

---

## Self-Review

**Spec coverage** (against [[specs/2026-07-03-kristall-k5-politur-gate-design]] §2):
- M1 (activity/pulse reflow) → Task 1 ✓
- M2 (cockpit tab strip) → Task 2 ✓
- L1 (light field border) → Task 3 ✓
- L2 (sidebar clip) → Task 4 ✓
- H1 (two-CTA demote) → Task 5 ✓ (implementation deviates to CSS `:has()` — documented in Task 5 + flagged for Soenne)
- A2 (no-SSE auth base) → Task 6 ✓
- A11y (verify + gaps) → Task 7 ✓
- Gate (make ci, live re-audit, dogfood, merge) → Tasks 8 + 9 ✓

**Placeholder scan:** no TBD/TODO; every code step shows real code; every command shows expected output. ✓

**Type consistency:** `activityFeedRow(row ActivityRowVM, showAgentChip bool, onPrefix bool)` and `activityTargetPill(row ActivityRowVM)` used consistently across Tasks 1/7. `Base`/`BaseNoSSE`/`baseHull` signatures consistent across Task 6. `ActivityRowVM` fields match the dossier (`activity_row.go:12-24`). ✓

**Open deviation to confirm at review:** Task 5 implements H1 as a CSS `:has()` demote rather than the spec §6 "compact chip", because the conflict is idle-only and the shell is route-blind (threading through 16 `AppShell` callers is disproportionate). Rationale in Task 5; surface to Soenne when presenting the plan / at dogfood.
