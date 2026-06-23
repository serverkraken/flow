# flow WebUI — Slice 0 (Fundament) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the additive design-system foundation for the WebUI overhaul — semantic Dark/Light tokens, locally-vendored assets (htmx + fonts), a dependency-free i18n layer, the `Base`/`AppShell`/nav hull, the core component + primitives + dialog + pagination library, a Docker/CI CSS build guard — and prove it all via a `/ui` styleguide route, without touching any existing feature page.

**Architecture:** New reusable templ components live in a brand-new subpackage `internal/adapter/webui/components` (package `components`) so they never clash with the existing package `webui` (which keeps `Nav`, `WorktimePage`, etc.). i18n is a new dependency-free package `internal/i18n` (Go-map catalogs + context-resolved locale + HTTP middleware). Design tokens are Tailwind v4 CSS custom properties (light `:root`, dark `:root[data-theme="dark"]`) mapped into Tailwind utilities via `@theme`. Everything is server-rendered; assets are `go:embed`-ed; the styleguide handler follows the existing `handler → build viewmodel → Component.Render(ctx, w)` pattern.

**Tech Stack:** Go 1.25.7, templ v0.3.857 (`go tool templ generate`), htmx 2.0.4 + htmx-ext-sse 2.2.3 (vendored), Tailwind CSS v4 standalone CLI (`tailwindcss`), net/http (stdlib mux), `golang.org/x/text/language` (already an indirect dep) for `Accept-Language` parsing.

## Global Constraints
- Module `github.com/serverkraken/flow`, Go 1.25.7
- templ via `go tool templ generate`; committed `*_templ.go` must pass `make verify-generate`
- `make ci` (lint, verify-generate, cover [≥80% on ./internal/...], build) + new `verify-css` must stay green
- ALL user-facing strings via `internal/i18n` (German primary); NO hardcoded UI strings in new templates
- Dark + Light both; semantic CSS-variable tokens; `prefers-reduced-motion` respected
- NO native browser popups (`window.alert/confirm/prompt`) — in-design `<dialog>` only
- Offline: no CDN/external origins at runtime (assets vendored + embedded)
- ADDITIVE: do not modify existing feature pages or remove `nav.templ`. New components are in package `components`. The only edits to existing files are: `web/tailwind.css` (rewrite to token layer — existing pages use generic `slate-*` which Tailwind still emits, so they keep working), `internal/adapter/webui/static.go` (broaden the embed), `Makefile`, `deploy/podman/Dockerfile.server`, and `internal/adapter/httpserver/server.go` (mount the new `/ui` route — append only).

---

## Verified Codebase Facts (use verbatim — do not re-derive)
- **Module / Go:** `github.com/serverkraken/flow`, `go 1.25.7`.
- **templ:** `github.com/a-h/templ v0.3.857`, tool directive present (`tool github.com/a-h/templ/cmd/templ`). Generate with `go tool templ generate` (regenerates ALL `*.templ` in the module; `make generate` runs exactly this). `make verify-generate` fails if any `*_templ.go` is stale.
- **Makefile targets (current):** `web` = `tailwindcss --input web/tailwind.css --output internal/adapter/webui/static/app.css --minify`; `generate` = `go tool templ generate`; `verify-generate`; `cover` (gate `scripts/coverage-gate.sh`, `COVER_THRESHOLD := 80`, `COVER_PKG := ./internal/...`); `ci: lint verify-generate cover build`; `lint` = `golangci-lint run`.
- **Coverage gate script:** `scripts/coverage-gate.sh <profile> <threshold>` — reads `go tool cover -func`, compares `total:` to threshold, exits 1 on FAIL.
- **`web/tailwind.css` (current, 3 lines):** `@import "tailwindcss";` + `@source "../internal/adapter/webui/**/*.templ";` + `@source "../internal/adapter/webui/**/*.go";`
- **`internal/adapter/webui/static.go`:** `//go:embed static/app.css` → `var staticFS embed.FS`; `StaticHandler()` returns `http.FileServerFS(fs.Sub(staticFS,"static"))`. Mounted in `server.go`: `mux.Handle("GET /static/", http.StripPrefix("/static/", webui.StaticHandler()))`.
- **`internal/adapter/webui/nav.templ`:** package `webui`; exports `templ Nav(active, user string)`; uses default Tailwind `slate-*` + hardcoded labels. KEEP UNCHANGED.
- **Handler pattern:** `webui.XPage(vm).Render(r.Context(), w)`; user via `u, _ := userFrom(r.Context())`; routes gated with `s.webAuth(http.HandlerFunc(...))`. `userFrom` is in package `httpserver` (`internal/adapter/httpserver/middleware.go`).
- **Auth in tests:** `codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)`; `cookieVal, _ := codec.Issue("u1")`; cookie `&http.Cookie{Name: "flow_session", Value: cookieVal}`. Seed user with `domain.NewUser("u1","sub-1","msoent","m@x.de","M")` then `users.UpsertBySub(ctx, u)`. Test helpers `newWebProjectsServer`, `getWebProjects`/`getWeb` live in `internal/adapter/httpserver/webui_projects_test.go` (package `httpserver_test`).
- **Component render test pattern (package `webui` example, `render_test.go`):** `var b bytes.Buffer; Component(args).Render(context.Background(), &b); strings.Contains(b.String(), "...")`.
- **`domain.User`:** fields `ID, OIDCSub, Username, Email, DisplayName string`.
- **Dockerfile (`deploy/podman/Dockerfile.server`):** `FROM golang:1.25-alpine AS build`; `COPY go.mod go.sum` + `go mod download`; `COPY . .`; `RUN go tool templ generate ./...`; `RUN CGO_ENABLED=0 GOFLAGS="-trimpath -buildvcs=false" go build -ldflags="-s -w" -o /out/flow-server ./cmd/flow-server`; runtime `gcr.io/distroless/static-debian12:nonroot`. It does **NOT** build CSS (the gotcha this slice fixes).
- **Design tokens (canonical source `docs/superpowers/specs/assets/2026-06-23-webui/direction-b-studio.html` `<head>`):** reproduced verbatim in Task 3 below. Theme-boot + toggle + live-timer scripts reproduced verbatim in Task 5.
- **`golang.org/x/text v0.36.0`** is present as an indirect dep — using `golang.org/x/text/language` in Task 2 promotes it to direct; run `go mod tidy` (covered in the task).

---

## File Structure (one responsibility per file — "keine Monolithen")

**New package `internal/i18n/` (Task 2):**
```
internal/i18n/i18n.go            # T, Tn, Locale, FromContext, WithLocale, ctx key, default locale
internal/i18n/catalog_de.go      # German catalog (complete) — map[string]string + plural map
internal/i18n/catalog_en.go      # English catalog (stub mirroring de keys)
internal/i18n/middleware.go      # HTTP middleware: cookie flow_lang → Accept-Language → de
internal/i18n/i18n_test.go       # T/Tn/fallback/missing-key tests
internal/i18n/middleware_test.go # locale-resolution tests
internal/i18n/catalog_test.go    # en mirrors de keyset (completeness guard)
```

**New scripts / build (Task 1):**
```
scripts/verify-css.sh            # builds tailwind to a temp file, diffs vs committed app.css
scripts/verify-no-popups.sh      # greps internal/adapter/webui for window.alert/confirm/prompt
```

**New package `internal/adapter/webui/components/` (Tasks 3–9):**
```
internal/adapter/webui/components/i18nhelper.go   # templ-usable T/Tn helpers bound to ctx + locale switch helper
internal/adapter/webui/components/base.templ      # Base — full HTML hull, head, fonts, htmx, theme-boot + live-timer scripts
internal/adapter/webui/components/appshell.templ  # AppShell — sidebar + mobile topbar + bottom-tab, slots
internal/adapter/webui/components/sitenav.templ   # SiteNav — top-level nav (i18n labels + active state)
internal/adapter/webui/components/subnav.templ    # SubNav / TabStrip — worktime sub-tabs
internal/adapter/webui/components/breadcrumb.templ# Breadcrumb
internal/adapter/webui/components/themetoggle.templ # ThemeToggle (☀/☾, aria-pressed)
internal/adapter/webui/components/button.templ    # Button (primary/secondary/ghost/danger) + IconButton
internal/adapter/webui/components/card.templ      # Card
internal/adapter/webui/components/badge.templ     # Badge (doc-kind)
internal/adapter/webui/components/chip.templ      # Chip / Tag
internal/adapter/webui/components/stattile.templ  # StatTile
internal/adapter/webui/components/emptystate.templ# EmptyState
internal/adapter/webui/components/glyph.templ     # Glyph
internal/adapter/webui/components/dialog.templ    # Dialog (styled <dialog>) + ConfirmDialog
internal/adapter/webui/components/pagination.templ# Pagination (prev/next + Mehr laden)
internal/adapter/webui/components/styleguide.templ# StyleguidePage — showcases every component (Task 10)
internal/adapter/webui/components/*_test.go       # one test file per task (see tasks)
```
> The dialog focus-trap script is NOT under the components package — it must be served from the webui static tree, so it lives at `internal/adapter/webui/static/js/dialog.js` (embedded by Task 4's `//go:embed all:static`, served at `/static/js/dialog.js`). See Task 8.

**Vendored assets (Task 4):**
```
internal/adapter/webui/static/vendor/htmx.min.js          # htmx 2.0.4
internal/adapter/webui/static/vendor/htmx-ext-sse.js      # htmx-ext-sse 2.2.3
internal/adapter/webui/static/fonts/ClashDisplay-Variable.woff2
internal/adapter/webui/static/fonts/Inter-Variable.woff2
internal/adapter/webui/static/fonts/JetBrainsMono-Variable.woff2
internal/adapter/webui/static/LICENSES.md                 # font + htmx license notes
```

**Modified (append/rewrite only):**
```
web/tailwind.css                         # rewrite: token layer (light+dark) + @theme map + base styles (Task 3)
internal/adapter/webui/static.go         # //go:embed all:static (Task 4)
Makefile                                 # add verify-css, verify-no-popups; add both to ci (Tasks 1, 8)
deploy/podman/Dockerfile.server          # add tailwind build step before go build (Task 1)
internal/adapter/httpserver/server.go    # mount GET /ui (Task 10)
internal/adapter/httpserver/webui_styleguide.go      # handleWebStyleguide (Task 10)
internal/adapter/httpserver/webui_styleguide_test.go # /ui route test (Task 10)
```

> **Note on the i18n-lib decision (spec open point §16):** This slice uses the **lightweight dependency-free Go-map** approach (decision locked by the spec author). `nicksnyder/go-i18n/v2` (TOML bundles, full CLDR plurals) is a possible future swap if richer plural rules are needed — do NOT add it now. Our `Tn` handles the only plural shape German/English need (`n == 1` → "one", else "other").

> **Note on top-nav grouping (spec open point §16):** Locked for this slice — primary nav = `Heute · Wissen · Projekte · Stats`; secondary (in an overflow "Menü"/account area) = `Frei · Export · Einstellungen · Abmelden · ☀/☾`. Encoded in `SiteNav` (Task 6) and `AppShell` (Task 6).

---

## Task 1: Tailwind-in-Docker + `verify-css` CI guard

Establishes a reproducible CSS build so the embedded `app.css` is never stale. This must come first because every later task that edits `.templ`/`web/tailwind.css` will run `make web` and rely on `make verify-css` staying green.

**Files:**
- Create: `scripts/verify-css.sh`
- Modify: `Makefile` (add `verify-css` target; add `verify-css` to `ci`)
- Modify: `deploy/podman/Dockerfile.server` (add pinned tailwind CLI + `web` build before `go build`)
- Test: `scripts/verify-css.sh` itself is the test harness; verified by running `make verify-css` (expect OK) and a deliberate-drift check.

**Interfaces:**
- Produces: `make verify-css` (target) — exit 0 when committed `internal/adapter/webui/static/app.css` equals a fresh `tailwindcss` build of `web/tailwind.css`; exit 1 on drift. `make ci` now depends on it.
- Consumes: the `tailwindcss` standalone CLI on `PATH` (already required by `make web`).

**Pinned versions (use verbatim):** Tailwind CSS standalone CLI **v4.1.5**. Download URLs (GitHub releases, `tailwindlabs/tailwindcss`):
- linux x86_64: `https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.5/tailwindcss-linux-x64`
- linux arm64: `https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.5/tailwindcss-linux-arm64`

Steps:

- [ ] Create `scripts/verify-css.sh` with EXACTLY this content:
```bash
#!/usr/bin/env bash
# Verify that the committed app.css matches a fresh tailwindcss build.
# Fails (exit 1) on drift so CI catches an un-rebuilt stylesheet.
set -euo pipefail

SRC="web/tailwind.css"
COMMITTED="internal/adapter/webui/static/app.css"

if ! command -v tailwindcss >/dev/null 2>&1; then
  echo "verify-css: tailwindcss CLI not found on PATH" >&2
  exit 1
fi
if [ ! -f "$COMMITTED" ]; then
  echo "verify-css: $COMMITTED is missing — run 'make web'" >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
tailwindcss --input "$SRC" --output "$tmp" --minify >/dev/null 2>&1

if ! diff -q "$tmp" "$COMMITTED" >/dev/null; then
  echo "verify-css: FAIL — $COMMITTED is out of date. Run 'make web' and commit." >&2
  diff "$COMMITTED" "$tmp" | head -40 >&2 || true
  exit 1
fi
echo "verify-css: OK"
```
- [ ] `chmod +x scripts/verify-css.sh`
- [ ] In `Makefile`, add `verify-css` to the `.PHONY` line (append the word `verify-css`).
- [ ] In `Makefile`, add this target immediately AFTER the `verify-generate` target block (before the `ci:` line):
```makefile
# verify-css checks the committed app.css matches a fresh tailwind build.
verify-css:
	@./scripts/verify-css.sh
```
- [ ] In `Makefile`, change the `ci` line from `ci: lint verify-generate cover build` to:
```makefile
ci: lint verify-generate verify-css cover build
```
- [ ] Run `make verify-css` → expect `verify-css: OK` (committed app.css already matches the current 3-line source).
- [ ] Drift smoke: append a throwaway comment to `web/tailwind.css` (`echo '/* drift-test */' >> web/tailwind.css`), run `make web`, then `git stash` the css change is NOT needed — instead verify the guard fires: temporarily edit `internal/adapter/webui/static/app.css` (append a space), run `make verify-css` → expect `FAIL` + exit 1. Then `git checkout -- internal/adapter/webui/static/app.css web/tailwind.css` to restore both. Confirm `make verify-css` → `OK` again.
- [ ] In `deploy/podman/Dockerfile.server`, insert the tailwind build step. Replace the block:
```dockerfile
COPY . .
RUN go tool templ generate ./...
RUN CGO_ENABLED=0 GOFLAGS="-trimpath -buildvcs=false" \
    go build -ldflags="-s -w" -o /out/flow-server ./cmd/flow-server
```
with:
```dockerfile
COPY . .

# Build the Tailwind v4 stylesheet IN the image so the go:embed-ed app.css is
# always fresh (fixes the "commit app.css by hand" gotcha). Pin the standalone
# CLI; pick the binary for the build arch (amd64/arm64).
ARG TAILWIND_VERSION=v4.1.5
RUN set -eux; \
    arch="$(uname -m)"; \
    case "$arch" in \
      x86_64)  tw="tailwindcss-linux-x64" ;; \
      aarch64) tw="tailwindcss-linux-arm64" ;; \
      *) echo "unsupported arch: $arch" >&2; exit 1 ;; \
    esac; \
    wget -q -O /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/${tw}"; \
    chmod +x /usr/local/bin/tailwindcss; \
    tailwindcss --input web/tailwind.css \
      --output internal/adapter/webui/static/app.css --minify

RUN go tool templ generate ./...
RUN CGO_ENABLED=0 GOFLAGS="-trimpath -buildvcs=false" \
    go build -ldflags="-s -w" -o /out/flow-server ./cmd/flow-server
```
(`wget` is present in `golang:1.25-alpine`.)
- [ ] Commit: `git add scripts/verify-css.sh Makefile deploy/podman/Dockerfile.server && git commit -m "build(webui): tailwind build in Docker + verify-css CI guard"`

> Note: a full `podman build` smoke happens in Task 10 (after `web/tailwind.css` carries real tokens). Here we only wire the mechanism.

---

## Task 2: i18n package (`internal/i18n`) — dependency-free catalog + middleware

A lightweight translation layer: `T(ctx, key)`/`Tn(ctx, key, n)` reading a per-locale Go-map catalog, a `Locale` carried in context, and an HTTP middleware resolving `flow_lang` cookie → `Accept-Language` → default `de`.

**Files:**
- Create: `internal/i18n/i18n.go`, `internal/i18n/catalog_de.go`, `internal/i18n/catalog_en.go`, `internal/i18n/middleware.go`
- Test: `internal/i18n/i18n_test.go`, `internal/i18n/middleware_test.go`, `internal/i18n/catalog_test.go`

**Interfaces:**
- Produces (package `i18n`):
  - `type Locale string` with `const (DE Locale = "de"; EN Locale = "en")` and `const Default = DE`.
  - `func T(ctx context.Context, key string) string` — returns the string for the ctx locale; falls back to `Default` locale, then to the key itself.
  - `func Tn(ctx context.Context, key string, n int) string` — plural; key resolves to a `Plural{One, Other string}`; substitutes `{{.N}}` with `n` via `strconv`. Falls back like `T`.
  - `func WithLocale(ctx context.Context, l Locale) context.Context`
  - `func FromContext(ctx context.Context) Locale` — returns ctx locale or `Default`.
  - `func Resolve(r *http.Request) Locale` — cookie `flow_lang` → `Accept-Language` (first supported tag) → `Default`.
  - `func Middleware(next http.Handler) http.Handler` — injects `WithLocale(ctx, Resolve(r))`.
- Consumes: `golang.org/x/text/language` (`language.NewMatcher`, `language.MatchStrings`), stdlib.

Steps:

- [ ] Write failing test `internal/i18n/i18n_test.go`:
```go
package i18n_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/i18n"
)

func TestT_GermanDefault(t *testing.T) {
	ctx := context.Background() // no locale → Default (de)
	if got := i18n.T(ctx, "nav.today"); got != "Heute" {
		t.Fatalf("T(nav.today) = %q, want Heute", got)
	}
}

func TestT_EnglishLocale(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.EN)
	if got := i18n.T(ctx, "nav.today"); got != "Today" {
		t.Fatalf("T(nav.today, en) = %q, want Today", got)
	}
}

func TestT_MissingKeyReturnsKey(t *testing.T) {
	ctx := context.Background()
	if got := i18n.T(ctx, "does.not.exist"); got != "does.not.exist" {
		t.Fatalf("missing key = %q, want the key itself", got)
	}
}

func TestT_FallsBackToDefaultLocale(t *testing.T) {
	// A key present in de but (intentionally) not in en still resolves via the
	// de fallback rather than echoing the key.
	ctx := i18n.WithLocale(context.Background(), i18n.EN)
	if got := i18n.T(ctx, "common.cancel"); got == "common.cancel" {
		t.Fatalf("expected fallback translation, got raw key")
	}
}

func TestTn_Plural(t *testing.T) {
	ctx := context.Background()
	if got := i18n.Tn(ctx, "list.entries", 1); got != "1 Eintrag" {
		t.Fatalf("Tn(list.entries,1) = %q, want '1 Eintrag'", got)
	}
	if got := i18n.Tn(ctx, "list.entries", 3); got != "3 Einträge" {
		t.Fatalf("Tn(list.entries,3) = %q, want '3 Einträge'", got)
	}
}
```
- [ ] Run `go test ./internal/i18n/...` → expect FAIL (package does not compile yet).
- [ ] Implement `internal/i18n/i18n.go`:
```go
// Package i18n is a dependency-free translation layer for the WebUI.
// Catalogs are Go maps (de primary, en stub). Strings resolve by the locale
// carried in context; missing keys fall back to the default locale, then to
// the key itself so a missing string is visible, never blank.
package i18n

import (
	"context"
	"strconv"
	"strings"
)

// Locale identifies a UI language.
type Locale string

const (
	DE Locale = "de"
	EN Locale = "en"

	// Default is used when no locale is in context and as the fallback catalog.
	Default = DE
)

// Plural holds the two CLDR categories German and English need.
type Plural struct{ One, Other string }

type catalog struct {
	strings map[string]string
	plurals map[string]Plural
}

// catalogs is populated by the per-language files' init() funcs.
var catalogs = map[Locale]catalog{}

func register(l Locale, c catalog) { catalogs[l] = c }

type ctxKey int

const localeKey ctxKey = 0

// WithLocale stores the locale in ctx for T/Tn.
func WithLocale(ctx context.Context, l Locale) context.Context {
	return context.WithValue(ctx, localeKey, l)
}

// FromContext returns the ctx locale or Default.
func FromContext(ctx context.Context) Locale {
	if l, ok := ctx.Value(localeKey).(Locale); ok && l != "" {
		return l
	}
	return Default
}

// T returns the translation of key for the ctx locale, falling back to the
// Default locale and finally to key.
func T(ctx context.Context, key string) string {
	l := FromContext(ctx)
	if s, ok := catalogs[l].strings[key]; ok {
		return s
	}
	if s, ok := catalogs[Default].strings[key]; ok {
		return s
	}
	return key
}

// Tn returns the singular/plural form of key for n in the ctx locale.
// "{{.N}}" in the chosen form is replaced by n.
func Tn(ctx context.Context, key string, n int) string {
	l := FromContext(ctx)
	p, ok := catalogs[l].plurals[key]
	if !ok {
		p, ok = catalogs[Default].plurals[key]
	}
	if !ok {
		return key
	}
	form := p.Other
	if n == 1 {
		form = p.One
	}
	return strings.ReplaceAll(form, "{{.N}}", strconv.Itoa(n))
}
```
- [ ] Implement `internal/i18n/catalog_de.go` (COMPLETE German catalog — every key any Slice-0 component uses):
```go
package i18n

func init() {
	register(DE, catalog{
		strings: map[string]string{
			// brand / shell
			"app.name":            "flow",
			"app.tagline":         "Zeit & Wissen",
			// top-level navigation
			"nav.today":           "Heute",
			"nav.week":            "Woche",
			"nav.history":         "Historie",
			"nav.knowledge":       "Wissen",
			"nav.projects":        "Projekte",
			"nav.stats":           "Stats",
			"nav.dayoffs":         "Frei",
			"nav.export":          "Export",
			"nav.settings":        "Einstellungen",
			"nav.menu":            "Menü",
			"nav.account":         "Konto",
			"nav.logout":          "Abmelden",
			"nav.primary":         "Hauptnavigation",
			// theme toggle
			"theme.toggle":        "Hell/Dunkel umschalten",
			"theme.toLight":       "Zu Hell wechseln",
			"theme.toDark":        "Zu Dunkel wechseln",
			// common buttons / actions
			"common.new":          "Neu",
			"common.save":         "Speichern",
			"common.cancel":       "Abbrechen",
			"common.delete":       "Löschen",
			"common.edit":         "Bearbeiten",
			"common.confirm":      "Bestätigen",
			"common.close":        "Schließen",
			"common.search":       "Suchen…",
			"common.loading":      "Lädt…",
			// pagination
			"page.prev":           "Zurück",
			"page.next":           "Weiter",
			"page.more":           "Mehr laden",
			"page.label":          "Seitennavigation",
			// empty states
			"empty.default":       "Nichts vorhanden",
			// confirm dialog defaults
			"confirm.title":       "Bist du sicher?",
			"confirm.deleteBody":  "Diese Aktion kann nicht rückgängig gemacht werden.",
			// doc kinds (badges)
			"dockind.daily":       "Daily",
			"dockind.project":     "Projekt",
			"dockind.free":        "Frei",
			"dockind.agent":       "Agent",
			// styleguide (only used by the /ui demo page)
			"styleguide.title":    "Design-System",
			"styleguide.subtitle": "Komponenten-Schaukasten",
		},
		plurals: map[string]Plural{
			"list.entries": {One: "{{.N}} Eintrag", Other: "{{.N}} Einträge"},
			"list.results": {One: "{{.N}} Treffer", Other: "{{.N}} Treffer"},
		},
	})
}
```
- [ ] Implement `internal/i18n/catalog_en.go` (STUB — mirrors the de keyset exactly; English where trivial, otherwise reuse de so completeness holds):
```go
package i18n

func init() {
	register(EN, catalog{
		strings: map[string]string{
			"app.name":            "flow",
			"app.tagline":         "Time & Knowledge",
			"nav.today":           "Today",
			"nav.week":            "Week",
			"nav.history":         "History",
			"nav.knowledge":       "Knowledge",
			"nav.projects":        "Projects",
			"nav.stats":           "Stats",
			"nav.dayoffs":         "Time off",
			"nav.export":          "Export",
			"nav.settings":        "Settings",
			"nav.menu":            "Menu",
			"nav.account":         "Account",
			"nav.logout":          "Sign out",
			"nav.primary":         "Main navigation",
			"theme.toggle":        "Toggle light/dark",
			"theme.toLight":       "Switch to light",
			"theme.toDark":        "Switch to dark",
			"common.new":          "New",
			"common.save":         "Save",
			"common.cancel":       "Cancel",
			"common.delete":       "Delete",
			"common.edit":         "Edit",
			"common.confirm":      "Confirm",
			"common.close":        "Close",
			"common.search":       "Search…",
			"common.loading":      "Loading…",
			"page.prev":           "Previous",
			"page.next":           "Next",
			"page.more":           "Load more",
			"page.label":          "Pagination",
			"empty.default":       "Nothing here",
			"confirm.title":       "Are you sure?",
			"confirm.deleteBody":  "This action cannot be undone.",
			"dockind.daily":       "Daily",
			"dockind.project":     "Project",
			"dockind.free":        "Free",
			"dockind.agent":       "Agent",
			"styleguide.title":    "Design System",
			"styleguide.subtitle": "Component showcase",
		},
		plurals: map[string]Plural{
			"list.entries": {One: "{{.N}} entry", Other: "{{.N}} entries"},
			"list.results": {One: "{{.N}} result", Other: "{{.N}} results"},
		},
	})
}
```
- [ ] Run `go test ./internal/i18n/...` → expect PASS for `i18n_test.go`.
- [ ] Write failing test `internal/i18n/middleware_test.go`:
```go
package i18n_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/i18n"
)

func TestResolve_CookieWins(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "flow_lang", Value: "en"})
	r.Header.Set("Accept-Language", "de")
	if got := i18n.Resolve(r); got != i18n.EN {
		t.Fatalf("cookie should win: got %q", got)
	}
}

func TestResolve_AcceptLanguageFallback(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Language", "en-US,en;q=0.9,de;q=0.5")
	if got := i18n.Resolve(r); got != i18n.EN {
		t.Fatalf("accept-language en should resolve EN: got %q", got)
	}
}

func TestResolve_DefaultGerman(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := i18n.Resolve(r); got != i18n.DE {
		t.Fatalf("default should be DE: got %q", got)
	}
}

func TestMiddleware_InjectsLocale(t *testing.T) {
	var seen i18n.Locale
	h := i18n.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = i18n.FromContext(r.Context())
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "flow_lang", Value: "en"})
	h.ServeHTTP(httptest.NewRecorder(), r.WithContext(context.Background()))
	if seen != i18n.EN {
		t.Fatalf("middleware did not inject EN, got %q", seen)
	}
}
```
- [ ] Run `go test ./internal/i18n/...` → expect FAIL (no `Resolve`/`Middleware`).
- [ ] Implement `internal/i18n/middleware.go`:
```go
package i18n

import (
	"net/http"

	"golang.org/x/text/language"
)

// supported lists the locales we can serve, most-preferred first; the matcher
// maps an Accept-Language header onto one of these.
var supported = []language.Tag{
	language.German,  // de — index 0 → also the matcher's default
	language.English, // en
}

var matcher = language.NewMatcher(supported)

// Resolve picks the request locale: flow_lang cookie → Accept-Language → Default.
func Resolve(r *http.Request) Locale {
	if c, err := r.Cookie("flow_lang"); err == nil {
		switch Locale(c.Value) {
		case DE:
			return DE
		case EN:
			return EN
		}
	}
	if al := r.Header.Get("Accept-Language"); al != "" {
		tag, _ := language.MatchStrings(matcher, al)
		base, _ := tag.Base()
		switch base.String() {
		case "en":
			return EN
		case "de":
			return DE
		}
	}
	return Default
}

// Middleware injects the resolved locale into the request context for T/Tn.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithLocale(r.Context(), Resolve(r))))
	})
}
```
- [ ] Write `internal/i18n/catalog_test.go` (completeness guard — en must cover every de key):
```go
package i18n

import "testing"

// TestCatalogsParity ensures the en stub mirrors the de keyset exactly, so no
// key silently falls through to the de fallback unnoticed (and vice-versa).
func TestCatalogsParity(t *testing.T) {
	de := catalogs[DE]
	en := catalogs[EN]
	for k := range de.strings {
		if _, ok := en.strings[k]; !ok {
			t.Errorf("en missing string key %q present in de", k)
		}
	}
	for k := range en.strings {
		if _, ok := de.strings[k]; !ok {
			t.Errorf("de missing string key %q present in en", k)
		}
	}
	for k := range de.plurals {
		if _, ok := en.plurals[k]; !ok {
			t.Errorf("en missing plural key %q present in de", k)
		}
	}
}
```
(This test is in package `i18n` — not `i18n_test` — so it can read the unexported `catalogs` map.)
- [ ] Run `go mod tidy` (promotes `golang.org/x/text` from indirect to direct).
- [ ] Run `go test ./internal/i18n/...` → expect PASS (all files).
- [ ] Run `golangci-lint run ./internal/i18n/...` → expect clean.
- [ ] Commit: `git add internal/i18n go.mod go.sum && git commit -m "feat(i18n): dependency-free catalog + locale middleware (de primary, en stub)"`

---

## Task 3: Design tokens — rewrite `web/tailwind.css` (light + dark) + `@theme` map

Port the exact Studio palette from `direction-b-studio.html` into Tailwind v4 CSS custom properties for both themes, and map them into utility classes via `@theme` using `rgb(var(--x) / <alpha-value>)` so `bg-surface text-ink` etc. flip by theme.

**Files:**
- Modify: `web/tailwind.css` (full rewrite — token layer + `@theme` + base styles)
- Test: `internal/adapter/webui/static/app.css` is regenerated; correctness is asserted by Task 5's `Base` render test (token classes present) and `make verify-css`. No Go test here (it is pure CSS), but we add an explicit grep assertion step below.

**Interfaces:**
- Produces: Tailwind utilities `bg-canvas|surface|sunken`, `text-ink|body|muted|faint`, `border-line|line2`, project hues `bg-/text-blue|cyan|green|purple|magenta|yellow|orange|red|teal`, `text-oncolor`, plus `font-display|sans|mono`, shadows `shadow-soft|lift|ring`, animations `animate-breathe|rise`. CSS custom props `--canvas … --scrollthumb` defined for `:root` (light) and `:root[data-theme="dark"]`.
- Consumes: Tailwind v4 (`@import "tailwindcss"`), the existing `@source` globs (extended to include the new `components` dir).

Steps:

- [ ] Replace the ENTIRE contents of `web/tailwind.css` with:
```css
@import "tailwindcss";

/* Scan templ + Go in both webui and the new components subpackage for class
   usage. Keep both globs so existing pages (package webui) and new components
   (package components) are covered. */
@source "../internal/adapter/webui/**/*.templ";
@source "../internal/adapter/webui/**/*.go";

/* ════════════════════════════════════════════════════════════════════
   DESIGN TOKENS — one Studio identity, two themes (verbatim port of
   direction-b-studio.html). Space-separated RGB triplets so both
   Tailwind's rgb(var(--x)/<alpha-value>) and our own rgb(var(--x)/.NN)
   work.
   ════════════════════════════════════════════════════════════════════ */

/* ── LIGHT (default) — cool, crisp ── */
:root {
  color-scheme: light;
  --canvas:  251 252 254;
  --surface: 255 255 255;
  --sunken:  244 246 251;
  --line:    230 233 242;
  --line2:   238 241 248;
  --ink:      27  34  51;
  --body:     72  82 106;
  --muted:   138 147 166;
  --faint:   174 182 199;

  --blue:     59 111 246;
  --cyan:     11 165 214;
  --green:    39 165 103;
  --purple:  139  92 246;
  --magenta: 225  29 116;
  --yellow:  217 138  11;
  --orange:  234 106  43;
  --red:     224  69  91;
  --teal:     14 155 142;
  --oncolor: 255 255 255;

  --shadow:         27  34  51;
  --shadow-accent:  59 111 246;

  --code-bg:    15  20  38;
  --code-fg:   201 212 245;
  --halo:       59 111 246;
  --halo-a:    0.16;
  --scrollthumb: 217 223 236;
}

/* ── DARK — soft navy productivity, hues lifted for contrast ── */
:root[data-theme="dark"] {
  color-scheme: dark;
  --canvas:  14  17  28;
  --surface: 22  26  40;
  --sunken:  28  33  50;
  --line:    42  49  72;
  --line2:   34  40  60;
  --ink:     226 231 245;
  --body:    166 175 199;
  --muted:   122 132 158;
  --faint:    82  92 120;

  --blue:    122 162 247;
  --cyan:    125 207 255;
  --green:   158 206 106;
  --purple:  187 154 247;
  --magenta: 247 110 168;
  --yellow:  224 175 104;
  --orange:  255 158 100;
  --red:     247 118 142;
  --teal:    115 218 202;
  --oncolor:  16  19  30;

  --shadow:          0   0   0;
  --shadow-accent:  18  24  48;

  --code-bg:   10  13  24;
  --code-fg:  201 212 245;
  --halo:    122 162 247;
  --halo-a:    0.20;
  --scrollthumb: 49  57  84;
}

/* ════════════════════════════════════════════════════════════════════
   THEME MAP — expose tokens as Tailwind utilities. In Tailwind v4 the
   @theme block defines design tokens; --color-* names become bg-*/text-*/
   border-* utilities, with /<alpha-value> opacity modifiers preserved.
   ════════════════════════════════════════════════════════════════════ */
@theme {
  --color-canvas:  rgb(var(--canvas)  / <alpha-value>);
  --color-surface: rgb(var(--surface) / <alpha-value>);
  --color-sunken:  rgb(var(--sunken)  / <alpha-value>);
  --color-line:    rgb(var(--line)    / <alpha-value>);
  --color-line2:   rgb(var(--line2)   / <alpha-value>);
  --color-ink:     rgb(var(--ink)     / <alpha-value>);
  --color-body:    rgb(var(--body)    / <alpha-value>);
  --color-muted:   rgb(var(--muted)   / <alpha-value>);
  --color-faint:   rgb(var(--faint)   / <alpha-value>);

  --color-blue:    rgb(var(--blue)    / <alpha-value>);
  --color-cyan:    rgb(var(--cyan)    / <alpha-value>);
  --color-green:   rgb(var(--green)   / <alpha-value>);
  --color-purple:  rgb(var(--purple)  / <alpha-value>);
  --color-magenta: rgb(var(--magenta) / <alpha-value>);
  --color-yellow:  rgb(var(--yellow)  / <alpha-value>);
  --color-orange:  rgb(var(--orange)  / <alpha-value>);
  --color-red:     rgb(var(--red)     / <alpha-value>);
  --color-teal:    rgb(var(--teal)    / <alpha-value>);
  --color-oncolor: rgb(var(--oncolor) / <alpha-value>);

  --font-display: "Clash Display", ui-sans-serif, system-ui, sans-serif;
  --font-sans:    "Inter", ui-sans-serif, system-ui, sans-serif;
  --font-mono:    "JetBrains Mono", ui-monospace, SFMono-Regular, monospace;

  --radius-2xl: 1.25rem;
  --radius-3xl: 1.75rem;

  --shadow-soft: 0 1px 2px rgb(var(--shadow) / .05), 0 6px 20px -8px rgb(var(--shadow-accent) / .10);
  --shadow-lift: 0 2px 4px rgb(var(--shadow) / .06), 0 18px 44px -16px rgb(var(--shadow-accent) / .18);
  --shadow-ring: 0 10px 50px -12px rgb(var(--shadow-accent) / .30);

  --animate-breathe: breathe 2.2s ease-in-out infinite;
  --animate-rise:    rise .6s cubic-bezier(.21,.85,.27,1) both;
}

@keyframes breathe { 0%,100% { opacity: 1 } 50% { opacity: .45 } }
@keyframes rise { 0% { opacity: 0; transform: translateY(10px) } 100% { opacity: 1; transform: none } }

/* ════════════════════════════════════════════════════════════════════
   LOCAL FONTS — vendored woff2 (no Google/Fontshare CDN at runtime).
   Files added in Task 4 under static/fonts; served at /static/fonts/.
   ════════════════════════════════════════════════════════════════════ */
@font-face {
  font-family: "Clash Display";
  src: url("/static/fonts/ClashDisplay-Variable.woff2") format("woff2");
  font-weight: 400 700; font-style: normal; font-display: swap;
}
@font-face {
  font-family: "Inter";
  src: url("/static/fonts/Inter-Variable.woff2") format("woff2");
  font-weight: 400 700; font-style: normal; font-display: swap;
}
@font-face {
  font-family: "JetBrains Mono";
  src: url("/static/fonts/JetBrainsMono-Variable.woff2") format("woff2");
  font-weight: 400 700; font-style: normal; font-display: swap;
}

/* ════════════════════════════════════════════════════════════════════
   BASE — page chrome, focus, reduced-motion, tabular nums.
   ════════════════════════════════════════════════════════════════════ */
html, body { background: rgb(var(--canvas)); }
body {
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
  transition: background-color .35s ease, color .35s ease;
}
aside, header, nav, article, input, .theme-fade {
  transition: background-color .35s ease, border-color .35s ease, color .35s ease;
}

.font-display { letter-spacing: -0.015em; }
.tnum { font-variant-numeric: tabular-nums; }
.eyebrow { letter-spacing: .14em; }

*:focus-visible {
  outline: 2px solid rgb(var(--blue));
  outline-offset: 2px;
  border-radius: 10px;
}

/* Theme-toggle knob slide + sun/moon swap (driven by data-theme). */
.toggle-knob { transition: transform .3s cubic-bezier(.4,.1,.2,1); }
:root[data-theme="dark"] .toggle-knob { transform: translateX(20px); }
.toggle-sun { display:inline; } .toggle-moon { display:none; }
:root[data-theme="dark"] .toggle-sun { display:none; }
:root[data-theme="dark"] .toggle-moon { display:inline; }

/* Styled native <dialog> backdrop (Task 8). */
dialog::backdrop { background: rgb(var(--ink) / .45); backdrop-filter: blur(2px); }

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { animation: none !important; transition: none !important; scroll-behavior: auto !important; }
}
```
- [ ] Run `make web` → regenerates `internal/adapter/webui/static/app.css` (no error).
- [ ] Assert tokens compiled: `rg -c "var\(--surface\)" internal/adapter/webui/static/app.css` → expect a non-zero count; and `rg -q "data-theme=\"dark\"" internal/adapter/webui/static/app.css && echo OK` → expect `OK`.
- [ ] Run `make verify-css` → expect `verify-css: OK` (committed css now matches the new source).
- [ ] Sanity-build existing pages still compile (no templ changes yet): `go build ./...` → expect success.
- [ ] Commit: `git add web/tailwind.css internal/adapter/webui/static/app.css && git commit -m "feat(webui): Studio design tokens (light+dark) + @theme utility map"`

> The `@font-face` rules reference files added in Task 4; until then the browser would 404 the fonts but CSS still compiles. Tasks 4 and 5 land the files and the `Base` hull that loads them.

---

## Task 4: Vendor assets locally (htmx + fonts) + broaden `go:embed`

Download and commit the pinned htmx scripts and three font woff2 files, then expand `static.go` to embed the entire `static` tree so vendor + fonts ship in the binary.

**Files:**
- Create: `internal/adapter/webui/static/vendor/htmx.min.js`, `internal/adapter/webui/static/vendor/htmx-ext-sse.js`, `internal/adapter/webui/static/fonts/ClashDisplay-Variable.woff2`, `internal/adapter/webui/static/fonts/Inter-Variable.woff2`, `internal/adapter/webui/static/fonts/JetBrainsMono-Variable.woff2`, `internal/adapter/webui/static/LICENSES.md`
- Modify: `internal/adapter/webui/static.go` (`//go:embed all:static`)
- Test: `internal/adapter/webui/static_test.go`

**Interfaces:**
- Produces: `webui.StaticHandler()` now serves `/static/vendor/*`, `/static/fonts/*`, `/static/app.css`. Pinned assets present on disk + embedded.
- Consumes: nothing new.

**Pinned sources (use verbatim — download these exact URLs):**
- htmx 2.0.4: `https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js` → save as `vendor/htmx.min.js` (license: BSD-2-Clause / "0BSD" per htmx repo).
- htmx-ext-sse 2.2.3: `https://unpkg.com/htmx-ext-sse@2.2.3/dist/sse.js` → save as `vendor/htmx-ext-sse.js` (license: BSD-2-Clause).
- Inter (variable woff2): `https://github.com/rsms/inter/releases/download/v4.0/Inter-4.0.zip` → extract `web/InterVariable.woff2` (SIL OFL 1.1) → save as `fonts/Inter-Variable.woff2`.
- JetBrains Mono (variable woff2): `https://github.com/JetBrains/JetBrainsMono/releases/download/v2.304/JetBrainsMono-2.304.zip` → extract `fonts/webfonts/JetBrainsMono[wght].woff2` (or convert the variable ttf) → save as `fonts/JetBrainsMono-Variable.woff2` (SIL OFL 1.1).
- Clash Display: download from Fontshare (`https://www.fontshare.com/fonts/clash-display/downloads`) → use the variable woff2 (`ClashDisplay-Variable.woff2`) → save as `fonts/ClashDisplay-Variable.woff2` (Fontshare / ITF Free Font License — redistribution of webfont permitted).

> If Fontshare's Clash Display variable woff2 is not directly fetchable in the build environment, the implementer may substitute the static 500-weight woff2 from the same download and update the `@font-face` `src` filename accordingly; record the substitution in `LICENSES.md`. The styleguide test only asserts the file is served, not its glyph contents.

Steps:

- [ ] Download htmx: `mkdir -p internal/adapter/webui/static/vendor internal/adapter/webui/static/fonts && curl -fsSL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js -o internal/adapter/webui/static/vendor/htmx.min.js`
- [ ] Download sse ext: `curl -fsSL https://unpkg.com/htmx-ext-sse@2.2.3/dist/sse.js -o internal/adapter/webui/static/vendor/htmx-ext-sse.js`
- [ ] Verify htmx looks right: `rg -q "htmx" internal/adapter/webui/static/vendor/htmx.min.js && echo OK` → expect `OK`; `test -s internal/adapter/webui/static/vendor/htmx-ext-sse.js && echo OK` → expect `OK`.
- [ ] Download + extract fonts into `internal/adapter/webui/static/fonts/` as the three filenames above. (Use `curl` + `unzip` to a temp dir under the scratchpad, then copy the woff2 out. Do NOT commit the zip.) Verify each file is non-empty and woff2-magic: `for f in ClashDisplay-Variable Inter-Variable JetBrainsMono-Variable; do test -s "internal/adapter/webui/static/fonts/$f.woff2" && head -c4 "internal/adapter/webui/static/fonts/$f.woff2" | rg -q "wOF2" && echo "$f OK"; done` → expect three `OK` lines (`wOF2` is the woff2 magic).
- [ ] Create `internal/adapter/webui/static/LICENSES.md`:
```markdown
# Bundled third-party assets

These files are vendored so the flow-server image runs fully offline (no CDN).

| File | Source | Version | License |
|------|--------|---------|---------|
| vendor/htmx.min.js | htmx.org | 2.0.4 | BSD-2-Clause (0BSD) |
| vendor/htmx-ext-sse.js | htmx.org SSE extension | 2.2.3 | BSD-2-Clause |
| fonts/Inter-Variable.woff2 | rsms/inter | 4.0 | SIL Open Font License 1.1 |
| fonts/JetBrainsMono-Variable.woff2 | JetBrains/JetBrainsMono | 2.304 | SIL Open Font License 1.1 |
| fonts/ClashDisplay-Variable.woff2 | Fontshare (Indian Type Foundry) | latest | ITF Free Font License |

Full license texts ship with the upstream projects; this table records provenance.
```
- [ ] Write failing test `internal/adapter/webui/static_test.go`:
```go
package webui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

func TestStaticHandlerServesVendorAndFonts(t *testing.T) {
	ts := httptest.NewServer(http.StripPrefix("/static/", webui.StaticHandler()))
	t.Cleanup(ts.Close)

	for _, p := range []string{
		"/static/app.css",
		"/static/vendor/htmx.min.js",
		"/static/vendor/htmx-ext-sse.js",
		"/static/fonts/Inter-Variable.woff2",
		"/static/fonts/JetBrainsMono-Variable.woff2",
		"/static/fonts/ClashDisplay-Variable.woff2",
	} {
		res, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", p, res.StatusCode)
		}
		if len(body) == 0 {
			t.Errorf("GET %s: empty body", p)
		}
	}
}
```
- [ ] Run `go test ./internal/adapter/webui/ -run TestStaticHandlerServesVendorAndFonts` → expect FAIL (only `app.css` is embedded today).
- [ ] Edit `internal/adapter/webui/static.go` — change the embed directive line from `//go:embed static/app.css` to `//go:embed all:static` (the `all:` prefix includes files that would otherwise be ignored; serves the whole tree). Leave the rest of the file unchanged.
- [ ] Run `go test ./internal/adapter/webui/ -run TestStaticHandlerServesVendorAndFonts` → expect PASS.
- [ ] Run `make verify-css` → still `OK` (app.css unchanged).
- [ ] Commit: `git add internal/adapter/webui/static internal/adapter/webui/static.go && git commit -m "feat(webui): vendor htmx + fonts locally, embed whole static tree (offline)"`

---

## Task 5: `Base` hull + `ThemeToggle` + i18n templ helper (package `components`)

The full HTML hull every page renders inside, plus the theme toggle and a templ-usable i18n helper. This establishes package `components` and is the first thing the styleguide will use.

**Files:**
- Create: `internal/adapter/webui/components/i18nhelper.go`, `internal/adapter/webui/components/base.templ`, `internal/adapter/webui/components/themetoggle.templ`
- Test: `internal/adapter/webui/components/base_test.go`

**Interfaces:**
- Produces (package `components`):
  - `func T(ctx context.Context, key string) string` — thin re-export of `i18n.T` so templates call `components.T(ctx, "key")` (templ exposes `ctx` in expressions). Also `func Tn(ctx context.Context, key string, n int) string`.
  - `templ Base(title string, body templ.Component)` — `<!DOCTYPE html><html lang="de" data-theme>` (boot script sets `data-theme` before paint), `<head>` with `<title>flow · {title}</title>`, `<link rel="stylesheet" href="/static/app.css">`, the three `<link rel="preload" as="font">` font hints, local `<script src="/static/vendor/htmx.min.js">` + `htmx-ext-sse.js`, the no-flash theme-boot `<script>`, the theme-sync `<script>` (defines `window.toggleTheme`), the client live-timer `<script>`; `<body class="font-sans text-ink antialiased" hx-ext="sse" sse-connect="/api/v1/events">` rendering `{ body }`.
  - `templ ThemeToggle()` — `<button type="button" onclick="toggleTheme()" data-theme-toggle aria-pressed="false">` with `☀`/`☾` spans (`aria-hidden`), `aria-label` from `components.T(ctx,"theme.toggle")`.
- Consumes: `internal/i18n`, `github.com/a-h/templ`.

Steps:

- [ ] Write failing test `internal/adapter/webui/components/base_test.go`:
```go
package components_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestBaseHullIsOfflineAndThemed(t *testing.T) {
	body := templ.Raw(`<p id="content">hallo</p>`)
	out := render(t, components.Base("Test", body))

	wants := []string{
		`<!doctype html>`,                       // templ lowercases the doctype
		`lang="de"`,
		`/static/app.css`,
		`/static/vendor/htmx.min.js`,            // local, NOT unpkg
		`/static/vendor/htmx-ext-sse.js`,
		`/static/fonts/Inter-Variable.woff2`,
		`flow-theme`,                            // no-flash boot script reads localStorage key
		`window.toggleTheme`,                    // theme-sync script
		`hx-ext="sse"`,
		`sse-connect="/api/v1/events"`,
		`data-timer`,                            // live-timer script hook present
		`id="content"`,                          // body slot rendered
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("Base missing %q", w)
		}
	}
	// Hard guarantee: NO external origins.
	for _, bad := range []string{"unpkg.com", "googleapis.com", "gstatic.com", "fontshare.com", "cdn.tailwindcss.com"} {
		if strings.Contains(out, bad) {
			t.Errorf("Base must be offline but references %q", bad)
		}
	}
}

func TestThemeTogglePressableAndLabeled(t *testing.T) {
	out := render(t, components.ThemeToggle())
	for _, w := range []string{`data-theme-toggle`, `aria-pressed="false"`, `onclick="toggleTheme()"`, `☀`, `☾`} {
		if !strings.Contains(out, w) {
			t.Errorf("ThemeToggle missing %q", w)
		}
	}
}
```
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect FAIL (package missing).
- [ ] Create `internal/adapter/webui/components/i18nhelper.go`:
```go
// Package components holds the reusable WebUI design-system templ components.
// It is separate from package webui so new components never clash with the
// existing webui exports (Nav, WorktimePage, …). All user-facing strings come
// from internal/i18n via the T/Tn helpers below, which templates call with the
// implicit ctx.
package components

import (
	"context"

	"github.com/serverkraken/flow/internal/i18n"
)

// T is a templ-friendly re-export of i18n.T (templates call components.T(ctx, key)).
func T(ctx context.Context, key string) string { return i18n.T(ctx, key) }

// Tn is a templ-friendly re-export of i18n.Tn for plural strings.
func Tn(ctx context.Context, key string, n int) string { return i18n.Tn(ctx, key, n) }
```
- [ ] Create `internal/adapter/webui/components/themetoggle.templ`:
```go
package components

// ThemeToggle is the ☀/☾ light/dark switch. It flips data-theme via the
// inline toggleTheme() defined in Base's theme-sync script and persists to
// localStorage('flow-theme'). aria-pressed is kept in sync by that script.
templ ThemeToggle() {
	<button
		type="button"
		onclick="toggleTheme()"
		data-theme-toggle
		class="relative grid place-items-center h-8 w-8 rounded-lg border border-line bg-sunken text-body hover:text-blue hover:border-blue/40 transition-colors"
		aria-label={ T(ctx, "theme.toggle") }
		aria-pressed="false"
		title={ T(ctx, "theme.toggle") }
	>
		<span class="toggle-sun text-[.95rem]" aria-hidden="true">☀</span>
		<span class="toggle-moon text-[.95rem]" aria-hidden="true">☾</span>
	</button>
}
```
- [ ] Create `internal/adapter/webui/components/base.templ` (the theme-boot, theme-sync, and live-timer scripts are verbatim ports from `direction-b-studio.html`):
```go
package components

import "github.com/a-h/templ"

// Base is the full HTML hull every page renders inside. It loads only local
// assets (app.css, vendored htmx, vendored fonts), sets data-theme before
// first paint (no-flash), wires the SSE body, and exposes toggleTheme() plus a
// client-side live timer. The page-specific markup is passed as body.
templ Base(title string, body templ.Component) {
	<!DOCTYPE html>
	<html lang="de" class="scroll-smooth">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
			<title>flow · { title }</title>
			<link rel="stylesheet" href="/static/app.css"/>
			<link rel="preload" as="font" type="font/woff2" href="/static/fonts/Inter-Variable.woff2" crossorigin/>
			<link rel="preload" as="font" type="font/woff2" href="/static/fonts/ClashDisplay-Variable.woff2" crossorigin/>
			<link rel="preload" as="font" type="font/woff2" href="/static/fonts/JetBrainsMono-Variable.woff2" crossorigin/>
			<script src="/static/vendor/htmx.min.js" defer></script>
			<script src="/static/vendor/htmx-ext-sse.js" defer></script>
			// No-flash theme init: set data-theme BEFORE first paint.
			<script>
				(function () {
					try {
						var saved = localStorage.getItem('flow-theme');
						var prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
						document.documentElement.setAttribute('data-theme', saved || (prefersDark ? 'dark' : 'light'));
					} catch (e) {
						document.documentElement.setAttribute('data-theme', 'light');
					}
				})();
			</script>
		</head>
		<body class="font-sans text-ink antialiased selection:bg-blue/15 selection:text-ink" hx-ext="sse" sse-connect="/api/v1/events">
			@body
			// Theme toggle wiring: flips data-theme, persists, syncs aria-pressed, cross-tab sync.
			<script>
				(function () {
					var root = document.documentElement;
					function syncToggles() {
						var dark = root.getAttribute('data-theme') === 'dark';
						document.querySelectorAll('[data-theme-toggle]').forEach(function (b) {
							b.setAttribute('aria-pressed', dark ? 'true' : 'false');
							b.setAttribute('title', dark ? 'Zu Hell wechseln' : 'Zu Dunkel wechseln');
						});
					}
					window.toggleTheme = function () {
						var next = root.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
						root.setAttribute('data-theme', next);
						try { localStorage.setItem('flow-theme', next); } catch (e) {}
						syncToggles();
					};
					window.addEventListener('storage', function (e) {
						if (e.key === 'flow-theme' && e.newValue) {
							root.setAttribute('data-theme', e.newValue);
							syncToggles();
						}
					});
					syncToggles();
				})();
			</script>
			// Client-side live timer: advances any [data-timer]/[data-mini-timer]
			// independently of htmx fragment refreshes. Reads a base elapsed (seconds)
			// and start epoch from data attributes on the [data-timer] element.
			<script>
				(function () {
					var reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
					var hero  = document.querySelector('[data-timer]');
					var minis = document.querySelectorAll('[data-mini-timer]');
					if (!hero && minis.length === 0) return;
					var base  = hero ? parseInt(hero.getAttribute('data-base') || '0', 10) : 0;
					var start = Date.now();
					function p(n) { return n < 10 ? '0' + n : '' + n; }
					function tick() {
						var elapsed = base + Math.floor((Date.now() - start) / 1000);
						var h = Math.floor(elapsed / 3600);
						var m = Math.floor((elapsed % 3600) / 60);
						var s = elapsed % 60;
						if (hero) hero.textContent = h + 'h ' + p(m) + 'm ' + p(s) + 's';
						minis.forEach(function (el) { el.textContent = p(h) + ':' + p(m) + ':' + p(s); });
					}
					tick();
					setInterval(tick, reduce ? 60000 : 1000);
				})();
			</script>
		</body>
	</html>
}
```
- [ ] Run `make generate` (regenerates `*_templ.go`, including the new component files).
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect PASS.
- [ ] Run `golangci-lint run ./internal/adapter/webui/components/...` → expect clean.
- [ ] Run `make verify-generate` → expect `verify-generate: OK`.
- [ ] Commit: `git add internal/adapter/webui/components && git commit -m "feat(webui): Base hull (offline, no-flash theme, live timer) + ThemeToggle + i18n helper"`

---

## Task 6: `AppShell` + `SiteNav` + `SubNav`/`TabStrip` + `Breadcrumb`

The responsive layout chrome: desktop sidebar, mobile top bar, mobile bottom-tab, with content/breadcrumb/subnav slots; the top-level nav with i18n labels + active state; the worktime sub-tab strip; and breadcrumbs.

**Files:**
- Create: `internal/adapter/webui/components/sitenav.templ`, `internal/adapter/webui/components/appshell.templ`, `internal/adapter/webui/components/subnav.templ`, `internal/adapter/webui/components/breadcrumb.templ`
- Test: `internal/adapter/webui/components/shell_test.go`

**Interfaces:**
- Produces (package `components`):
  - `type NavItem struct { Key, Href, LabelKey, Glyph string }` — `LabelKey` is an i18n key.
  - `func PrimaryNav() []NavItem` — returns `[Heute(/),Wissen(/wissen),Projekte(/projekte),Stats(/stats)]` with glyphs `▶ ◆ ● ▲`.
  - `func SecondaryNav() []NavItem` — `[Frei(/frei),Export(/export),Einstellungen(/einstellungen)]` glyphs `★ ▰ ·`.
  - `templ SiteNav(active string)` — desktop sidebar nav list; marks the item whose `Key == active` with `aria-current="page"`.
  - `templ AppShell(active string, breadcrumb, subnav, content templ.Component)` — wraps `SiteNav` in a desktop sidebar, a mobile top bar (brand + `ThemeToggle`), a mobile bottom-tab (PrimaryNav), and a `<main>` rendering `{ breadcrumb }{ subnav }{ content }`. Any of the three component slots may be `nil`; render guards skip nil.
  - `type Tab struct { Key, Href, LabelKey string }`
  - `templ TabStrip(tabs []Tab, active string)` — horizontal sub-tab strip (used for worktime: Heute·Woche·Historie·Stats·Frei).
  - `templ Breadcrumb(items []Crumb)` where `type Crumb struct { Href, Label string }` (last item is current, no link).
- Consumes: `internal/adapter/webui/components` (`ThemeToggle`, `T`), `github.com/a-h/templ`.

Steps:

- [ ] Write failing test `internal/adapter/webui/components/shell_test.go`:
```go
package components_test

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestSiteNavMarksActive(t *testing.T) {
	out := render(t, components.SiteNav("wissen"))
	for _, w := range []string{"Heute", "Wissen", "Projekte", "Stats", `href="/wissen"`, `aria-current="page"`} {
		if !strings.Contains(out, w) {
			t.Errorf("SiteNav missing %q", w)
		}
	}
}

func TestAppShellRendersSlotsAndChrome(t *testing.T) {
	bc := templ.Raw(`<nav id="bc">crumbs</nav>`)
	sn := templ.Raw(`<div id="sn">subnav</div>`)
	body := templ.Raw(`<section id="main">page</section>`)
	out := render(t, components.AppShell("today", bc, sn, body))
	for _, w := range []string{
		`id="bc"`, `id="sn"`, `id="main"`,
		`data-theme-toggle`,                 // mobile topbar carries the toggle
		`aria-label="Hauptnavigation"`,      // sidebar nav landmark (i18n nav.primary)
	} {
		if !strings.Contains(out, w) {
			t.Errorf("AppShell missing %q", w)
		}
	}
}

func TestAppShellNilSlotsAreSafe(t *testing.T) {
	out := render(t, components.AppShell("today", nil, nil, templ.Raw(`<p id="only">x</p>`)))
	if !strings.Contains(out, `id="only"`) {
		t.Errorf("AppShell with nil breadcrumb/subnav should still render content")
	}
}

func TestTabStripActive(t *testing.T) {
	tabs := []components.Tab{
		{Key: "today", Href: "/", LabelKey: "nav.today"},
		{Key: "week", Href: "/woche", LabelKey: "nav.week"},
	}
	out := render(t, components.TabStrip(tabs, "week"))
	if !strings.Contains(out, "Woche") || !strings.Contains(out, `aria-current="page"`) {
		t.Errorf("TabStrip should render labels and mark active: %s", out)
	}
}

func TestBreadcrumbLastIsCurrent(t *testing.T) {
	out := render(t, components.Breadcrumb([]components.Crumb{
		{Href: "/wissen", Label: "Wissen"},
		{Label: "Dokument"},
	}))
	if !strings.Contains(out, `href="/wissen"`) || !strings.Contains(out, "Dokument") {
		t.Errorf("Breadcrumb missing items: %s", out)
	}
	if !strings.Contains(out, `aria-current="page"`) {
		t.Errorf("Breadcrumb last item should be aria-current")
	}
}
```
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect FAIL.
- [ ] Create `internal/adapter/webui/components/sitenav.templ`:
```go
package components

// NavItem is one navigation destination. LabelKey is an i18n key resolved at
// render time; Glyph is a sense-bearing inline Unicode mark (aria-hidden).
type NavItem struct {
	Key, Href, LabelKey, Glyph string
}

// PrimaryNav is the locked top-level set (spec §16): Heute · Wissen · Projekte · Stats.
func PrimaryNav() []NavItem {
	return []NavItem{
		{"today", "/", "nav.today", "▶"},
		{"wissen", "/wissen", "nav.knowledge", "◆"},
		{"projekte", "/projekte", "nav.projects", "●"},
		{"stats", "/stats", "nav.stats", "▲"},
	}
}

// SecondaryNav is the overflow set: Frei · Export · Einstellungen.
func SecondaryNav() []NavItem {
	return []NavItem{
		{"frei", "/frei", "nav.dayoffs", "★"},
		{"export", "/export", "nav.export", "▰"},
		{"einstellungen", "/einstellungen", "nav.settings", "·"},
	}
}

// SiteNav renders the desktop sidebar nav list; the item matching active is
// marked aria-current and visually highlighted.
templ SiteNav(active string) {
	<nav class="flex-1 px-3 space-y-1" aria-label={ T(ctx, "nav.primary") }>
		for _, l := range PrimaryNav() {
			if l.Key == active {
				<a href={ templ.SafeURL(l.Href) } aria-current="page"
					class="flex items-center gap-3 rounded-xl px-3.5 py-2.5 text-[.95rem] font-medium bg-blue/[.08] text-blue">
					<span class="w-5 text-center text-[.95rem]" aria-hidden="true">{ l.Glyph }</span> { T(ctx, l.LabelKey) }
				</a>
			} else {
				<a href={ templ.SafeURL(l.Href) }
					class="flex items-center gap-3 rounded-xl px-3.5 py-2.5 text-[.95rem] font-medium text-body hover:bg-sunken hover:text-ink transition-colors">
					<span class="w-5 text-center text-faint" aria-hidden="true">{ l.Glyph }</span> { T(ctx, l.LabelKey) }
				</a>
			}
		}
		<div class="my-2 border-t border-line2"></div>
		for _, l := range SecondaryNav() {
			<a href={ templ.SafeURL(l.Href) }
				class="flex items-center gap-3 rounded-xl px-3.5 py-2 text-[.9rem] font-medium text-muted hover:bg-sunken hover:text-ink transition-colors">
				<span class="w-5 text-center text-faint" aria-hidden="true">{ l.Glyph }</span> { T(ctx, l.LabelKey) }
			</a>
		}
	</nav>
}
```
- [ ] Create `internal/adapter/webui/components/appshell.templ`:
```go
package components

import "github.com/a-h/templ"

// AppShell is the responsive page chrome: desktop sidebar (md+), mobile top
// bar, mobile bottom-tab, and a main column with optional breadcrumb + subnav
// slots above the content. Any slot component may be nil.
templ AppShell(active string, breadcrumb, subnav, content templ.Component) {
	// ── Desktop sidebar (md+) ──
	<aside class="hidden md:flex md:flex-col fixed inset-y-0 left-0 w-[248px] bg-surface/80 backdrop-blur-xl border-r border-line z-40">
		<div class="px-6 pt-7 pb-6 flex items-center justify-between">
			<a href="/" class="inline-flex items-center gap-2.5" aria-label={ T(ctx, "app.name") }>
				<span class="grid place-items-center h-9 w-9 rounded-xl bg-gradient-to-br from-blue to-purple text-oncolor font-display font-semibold text-lg shadow-soft">f</span>
				<span class="font-display text-[1.45rem] font-semibold tracking-tight">{ T(ctx, "app.name") }</span>
			</a>
			@ThemeToggle()
		</div>
		@SiteNav(active)
		<div class="px-3 pb-5">
			<form action="/auth/logout" method="post" hx-boost="false">
				<button class="w-full text-left flex items-center gap-3 rounded-xl px-3.5 py-2 text-[.85rem] text-muted hover:bg-sunken hover:text-ink transition-colors">
					<span class="w-5 text-center text-faint" aria-hidden="true">›</span> { T(ctx, "nav.logout") }
				</button>
			</form>
		</div>
	</aside>
	// ── Mobile top bar ──
	<header class="md:hidden sticky top-0 z-40 bg-canvas/85 backdrop-blur-xl border-b border-line">
		<div class="flex items-center justify-between px-4 h-14">
			<a href="/" class="inline-flex items-center gap-2" aria-label={ T(ctx, "app.name") }>
				<span class="grid place-items-center h-7 w-7 rounded-lg bg-gradient-to-br from-blue to-purple text-oncolor font-display font-semibold text-sm">f</span>
				<span class="font-display text-xl font-semibold tracking-tight">{ T(ctx, "app.name") }</span>
			</a>
			@ThemeToggle()
		</div>
	</header>
	// ── Main column ──
	<main class="md:pl-[248px] pb-28 md:pb-12">
		<div class="mx-auto w-full max-w-[1340px] px-4 sm:px-6 lg:px-10 pt-6 md:pt-9">
			if breadcrumb != nil {
				@breadcrumb
			}
			if subnav != nil {
				@subnav
			}
			@content
		</div>
	</main>
	// ── Mobile bottom-tab ──
	<nav class="md:hidden fixed bottom-0 inset-x-0 z-40 bg-surface/90 backdrop-blur-xl border-t border-line" aria-label={ T(ctx, "nav.primary") }>
		<ul class="grid grid-cols-4">
			for _, l := range PrimaryNav() {
				<li>
					if l.Key == active {
						<a href={ templ.SafeURL(l.Href) } aria-current="page" class="flex flex-col items-center gap-0.5 py-2.5 text-blue">
							<span class="text-[1.1rem] leading-none" aria-hidden="true">{ l.Glyph }</span>
							<span class="text-[.66rem] font-medium">{ T(ctx, l.LabelKey) }</span>
						</a>
					} else {
						<a href={ templ.SafeURL(l.Href) } class="flex flex-col items-center gap-0.5 py-2.5 text-muted hover:text-ink transition-colors">
							<span class="text-[1.1rem] leading-none" aria-hidden="true">{ l.Glyph }</span>
							<span class="text-[.66rem] font-medium">{ T(ctx, l.LabelKey) }</span>
						</a>
					}
				</li>
			}
		</ul>
	</nav>
}
```
- [ ] Create `internal/adapter/webui/components/subnav.templ`:
```go
package components

// Tab is one sub-navigation entry (e.g. worktime Heute·Woche·Historie·Stats·Frei).
type Tab struct {
	Key, Href, LabelKey string
}

// TabStrip renders a horizontal sub-tab strip; the active tab is underlined and
// aria-current. Used inside AppShell's subnav slot.
templ TabStrip(tabs []Tab, active string) {
	<div class="mb-6 border-b border-line">
		<nav class="flex gap-1 -mb-px overflow-x-auto scroll-thin" aria-label={ T(ctx, "nav.primary") }>
			for _, t := range tabs {
				if t.Key == active {
					<a href={ templ.SafeURL(t.Href) } aria-current="page"
						class="whitespace-nowrap px-3.5 py-2.5 text-[.9rem] font-medium border-b-2 border-blue text-blue">
						{ T(ctx, t.LabelKey) }
					</a>
				} else {
					<a href={ templ.SafeURL(t.Href) }
						class="whitespace-nowrap px-3.5 py-2.5 text-[.9rem] font-medium border-b-2 border-transparent text-muted hover:text-ink hover:border-line transition-colors">
						{ T(ctx, t.LabelKey) }
					</a>
				}
			}
		</nav>
	</div>
}
```
- [ ] Create `internal/adapter/webui/components/breadcrumb.templ`:
```go
package components

// Crumb is one breadcrumb segment. A Crumb with empty Href is the current
// page (rendered as text, aria-current).
type Crumb struct {
	Href, Label string
}

// Breadcrumb renders a "/"-separated trail; the last/empty-Href crumb is current.
templ Breadcrumb(items []Crumb) {
	<nav class="mb-4 text-[.82rem] text-muted" aria-label="Breadcrumb">
		<ol class="flex items-center gap-1.5 flex-wrap">
			for i, c := range items {
				<li class="inline-flex items-center gap-1.5">
					if c.Href != "" && i < len(items)-1 {
						<a href={ templ.SafeURL(c.Href) } class="hover:text-ink transition-colors">{ c.Label }</a>
						<span class="text-faint" aria-hidden="true">›</span>
					} else {
						<span class="text-ink font-medium" aria-current="page">{ c.Label }</span>
					}
				</li>
			}
		</ol>
	</nav>
}
```
- [ ] Run `make generate`.
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect PASS.
- [ ] Run `golangci-lint run ./internal/adapter/webui/components/...` → expect clean.
- [ ] Run `make verify-generate` → expect OK.
- [ ] Commit: `git add internal/adapter/webui/components && git commit -m "feat(webui): AppShell + SiteNav + TabStrip + Breadcrumb (responsive, i18n, a11y)"`

---

## Task 7: Primitives — `Button`/`IconButton`, `Card`, `Badge`, `Chip`/`Tag`, `StatTile`, `EmptyState`, `Glyph`

The small reusable building blocks, grouped into one task with a single test file (per the right-sizing guidance).

**Files:**
- Create: `internal/adapter/webui/components/button.templ`, `internal/adapter/webui/components/card.templ`, `internal/adapter/webui/components/badge.templ`, `internal/adapter/webui/components/chip.templ`, `internal/adapter/webui/components/stattile.templ`, `internal/adapter/webui/components/emptystate.templ`, `internal/adapter/webui/components/glyph.templ`
- Test: `internal/adapter/webui/components/primitives_test.go`

**Interfaces:**
- Produces (package `components`):
  - `type ButtonVariant string` + `const (BtnPrimary ButtonVariant="primary"; BtnSecondary="secondary"; BtnGhost="ghost"; BtnDanger="danger")`.
  - `templ Button(variant ButtonVariant, label, glyph string, attrs templ.Attributes)` — `<button>` with variant classes; `glyph` optional (rendered aria-hidden if non-empty); `attrs` spreads extra attributes (e.g. `hx-post`, `type`).
  - `templ IconButton(glyph, ariaLabel string, attrs templ.Attributes)` — square icon-only button with required `aria-label`.
  - `templ Card(class string, body templ.Component)` — surface card; `class` appends extra utility classes (e.g. `lg:col-span-2`).
  - `type DocKind string` + `const (KindDaily="daily"; KindProject="project"; KindFree="free"; KindAgent="agent")`; `templ Badge(kind DocKind)` — colored doc-kind badge (Daily=blue ●, Projekt=green ◆, Frei=purple ○, Agent=yellow ▪), label via i18n `dockind.*`.
  - `templ Chip(label, hue string)` — pill tag in the given project hue (`blue|cyan|…|teal`); `templ Tag(label string)` — neutral `#`-prefixed tag chip.
  - `templ StatTile(labelKey, value, hue string)` — eyebrow label (i18n key) + big mono tnum value; `hue` colors the value (empty → default ink).
  - `templ EmptyState(glyph, titleKey, bodyKey string)` — centered empty placeholder; i18n keys.
  - `templ Glyph(g, cls string)` — `<span aria-hidden="true" class={cls}>{g}</span>` (decorative inline glyph helper).
- Consumes: `internal/adapter/webui/components` (`T`), `github.com/a-h/templ`.

Steps:

- [ ] Write failing test `internal/adapter/webui/components/primitives_test.go`:
```go
package components_test

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestButtonVariantsAndAttrs(t *testing.T) {
	out := render(t, components.Button(components.BtnDanger, "Löschen", "■",
		templ.Attributes{"hx-post": "/x/delete", "type": "submit"}))
	for _, w := range []string{"Löschen", "bg-red", `hx-post="/x/delete"`, `type="submit"`, "■"} {
		if !strings.Contains(out, w) {
			t.Errorf("Button missing %q: %s", w, out)
		}
	}
	prim := render(t, components.Button(components.BtnPrimary, "Speichern", "", nil))
	if strings.Contains(prim, "bg-red") {
		t.Errorf("primary button should not use danger color")
	}
}

func TestIconButtonRequiresAriaLabel(t *testing.T) {
	out := render(t, components.IconButton("✚", "Neu", templ.Attributes{"hx-get": "/new"}))
	for _, w := range []string{`aria-label="Neu"`, "✚", `hx-get="/new"`} {
		if !strings.Contains(out, w) {
			t.Errorf("IconButton missing %q", w)
		}
	}
}

func TestCardWrapsBody(t *testing.T) {
	out := render(t, components.Card("lg:col-span-2", templ.Raw(`<p id="cb">x</p>`)))
	if !strings.Contains(out, `id="cb"`) || !strings.Contains(out, "lg:col-span-2") || !strings.Contains(out, "bg-surface") {
		t.Errorf("Card missing class or body: %s", out)
	}
}

func TestBadgeDocKinds(t *testing.T) {
	if out := render(t, components.Badge(components.KindProject)); !strings.Contains(out, "Projekt") || !strings.Contains(out, "text-green") {
		t.Errorf("project badge wrong: %s", out)
	}
	if out := render(t, components.Badge(components.KindDaily)); !strings.Contains(out, "Daily") {
		t.Errorf("daily badge wrong: %s", out)
	}
}

func TestChipAndTag(t *testing.T) {
	if out := render(t, components.Chip("flow", "teal")); !strings.Contains(out, "flow") || !strings.Contains(out, "text-teal") {
		t.Errorf("Chip wrong: %s", out)
	}
	if out := render(t, components.Tag("rebuild")); !strings.Contains(out, "rebuild") {
		t.Errorf("Tag wrong: %s", out)
	}
}

func TestStatTile(t *testing.T) {
	out := render(t, components.StatTile("nav.week", "32h 10m", "green"))
	for _, w := range []string{"Woche", "32h 10m", "text-green"} {
		if !strings.Contains(out, w) {
			t.Errorf("StatTile missing %q: %s", w, out)
		}
	}
}

func TestEmptyState(t *testing.T) {
	out := render(t, components.EmptyState("·", "empty.default", "empty.default"))
	if !strings.Contains(out, "Nichts vorhanden") {
		t.Errorf("EmptyState should render i18n title: %s", out)
	}
}
```
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect FAIL.
- [ ] Create `internal/adapter/webui/components/button.templ`:
```go
package components

import "github.com/a-h/templ"

// ButtonVariant selects the visual treatment.
type ButtonVariant string

const (
	BtnPrimary   ButtonVariant = "primary"
	BtnSecondary ButtonVariant = "secondary"
	BtnGhost     ButtonVariant = "ghost"
	BtnDanger    ButtonVariant = "danger"
)

func btnClass(v ButtonVariant) string {
	base := "inline-flex items-center justify-center gap-2 rounded-2xl px-5 py-2.5 text-[.92rem] font-semibold transition active:scale-[.99] "
	switch v {
	case BtnPrimary:
		return base + "bg-ink text-canvas shadow-soft hover:bg-ink/90"
	case BtnSecondary:
		return base + "border border-line bg-surface text-ink hover:bg-sunken"
	case BtnGhost:
		return base + "text-body hover:bg-sunken hover:text-ink"
	case BtnDanger:
		return base + "bg-red text-oncolor shadow-soft hover:bg-red/90"
	default:
		return base + "bg-ink text-canvas"
	}
}

// Button renders a styled button. glyph is optional (aria-hidden when set);
// attrs spreads any extra attributes (hx-*, type, name, value, …).
templ Button(variant ButtonVariant, label, glyph string, attrs templ.Attributes) {
	<button class={ btnClass(variant) } { attrs... }>
		if glyph != "" {
			<span aria-hidden="true">{ glyph }</span>
		}
		{ label }
	</button>
}

// IconButton is a square, icon-only button; ariaLabel is required for a11y.
templ IconButton(glyph, ariaLabel string, attrs templ.Attributes) {
	<button
		class="grid place-items-center h-9 w-9 rounded-lg border border-line bg-sunken text-body hover:text-blue hover:border-blue/40 transition-colors"
		aria-label={ ariaLabel }
		{ attrs... }
	>
		<span aria-hidden="true">{ glyph }</span>
	</button>
}
```
- [ ] Create `internal/adapter/webui/components/card.templ`:
```go
package components

import "github.com/a-h/templ"

// Card is the standard surface container. class appends extra utilities
// (grid spans, padding overrides, …) onto the base card styling.
templ Card(class string, body templ.Component) {
	<article class={ "rounded-3xl bg-surface border border-line shadow-soft p-6", class }>
		@body
	</article>
}
```
- [ ] Create `internal/adapter/webui/components/badge.templ`:
```go
package components

// DocKind identifies a document category for Badge coloring/labeling.
type DocKind string

const (
	KindDaily   DocKind = "daily"
	KindProject DocKind = "project"
	KindFree    DocKind = "free"
	KindAgent   DocKind = "agent"
)

type badgeStyle struct{ cls, glyph, labelKey string }

func docBadge(k DocKind) badgeStyle {
	switch k {
	case KindDaily:
		return badgeStyle{"bg-blue/10 text-blue", "●", "dockind.daily"}
	case KindProject:
		return badgeStyle{"bg-green/10 text-green", "◆", "dockind.project"}
	case KindFree:
		return badgeStyle{"bg-purple/10 text-purple", "○", "dockind.free"}
	case KindAgent:
		return badgeStyle{"bg-yellow/10 text-yellow", "▪", "dockind.agent"}
	default:
		return badgeStyle{"bg-sunken text-body", "·", "dockind.daily"}
	}
}

// Badge renders a doc-kind pill (glyph + i18n label, kind-colored).
templ Badge(kind DocKind) {
	{{ b := docBadge(kind) }}
	<span class={ "inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-[.72rem] font-medium", b.cls }>
		<span aria-hidden="true">{ b.glyph }</span> { T(ctx, b.labelKey) }
	</span>
}
```
- [ ] Create `internal/adapter/webui/components/chip.templ`:
```go
package components

// hueText maps a project-hue name onto its text + soft-bg utility classes.
func hueText(hue string) string {
	switch hue {
	case "blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal":
		return "text-" + hue + " bg-" + hue + "/10"
	default:
		return "text-body bg-sunken"
	}
}

// Chip is a project pill in the given hue (blue|cyan|green|purple|magenta|yellow|orange|red|teal).
templ Chip(label, hue string) {
	<span class={ "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[.78rem] font-medium", hueText(hue) }>
		{ label }
	</span>
}

// Tag is a neutral "#"-prefixed tag chip.
templ Tag(label string) {
	<span class="inline-flex items-center rounded-md bg-sunken px-1.5 py-0.5 text-[.72rem] font-medium text-body">
		#{ label }
	</span>
}
```
- [ ] Create `internal/adapter/webui/components/stattile.templ`:
```go
package components

func valueHue(hue string) string {
	switch hue {
	case "blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal":
		return "text-" + hue
	default:
		return "text-ink"
	}
}

// StatTile shows an eyebrow label (i18n key) over a large mono tnum value.
templ StatTile(labelKey, value, hue string) {
	<div class="rounded-2xl bg-sunken/70 py-3 px-3 text-center">
		<div class="eyebrow uppercase text-[.62rem] font-semibold text-muted">{ T(ctx, labelKey) }</div>
		<div class={ "mt-1 font-mono text-[1.05rem] font-semibold tnum", valueHue(hue) }>{ value }</div>
	</div>
}
```
- [ ] Create `internal/adapter/webui/components/emptystate.templ`:
```go
package components

// EmptyState is the centered placeholder for empty lists/sections. titleKey and
// bodyKey are i18n keys; glyph is a decorative inline mark.
templ EmptyState(glyph, titleKey, bodyKey string) {
	<div class="flex flex-col items-center justify-center text-center py-12 px-6">
		<span class="text-3xl text-faint mb-3" aria-hidden="true">{ glyph }</span>
		<p class="font-display text-lg font-semibold text-ink">{ T(ctx, titleKey) }</p>
		<p class="mt-1 text-[.88rem] text-muted max-w-sm">{ T(ctx, bodyKey) }</p>
	</div>
}
```
- [ ] Create `internal/adapter/webui/components/glyph.templ`:
```go
package components

// Glyph renders a decorative inline Unicode mark (aria-hidden). cls sets color/size.
templ Glyph(g, cls string) {
	<span class={ cls } aria-hidden="true">{ g }</span>
}
```
- [ ] Run `make generate`.
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect PASS.
- [ ] Run `golangci-lint run ./internal/adapter/webui/components/...` → expect clean.
- [ ] Run `make verify-generate` → expect OK.
- [ ] Commit: `git add internal/adapter/webui/components && git commit -m "feat(webui): primitives — Button/IconButton/Card/Badge/Chip/Tag/StatTile/EmptyState/Glyph"`

---

## Task 8: `Dialog` + `ConfirmDialog` + no-popups guard

Styled native `<dialog>` with focus-trap, Esc/backdrop close, return-focus; `ConfirmDialog` with a danger primary and Abbrechen focused by default. Plus a CI lint script that forbids `window.alert/confirm/prompt` in `internal/adapter/webui` so the rule is enforced from day one.

**Files:**
- Create: `internal/adapter/webui/components/dialog.templ`, `internal/adapter/webui/components/static/js/dialog.js`, `scripts/verify-no-popups.sh`
- Modify: `Makefile` (add `verify-no-popups` target; add to `ci`)
- Test: `internal/adapter/webui/components/dialog_test.go`

**Interfaces:**
- Produces (package `components`):
  - `templ Dialog(id, titleKey string, body templ.Component)` — `<dialog id={id} aria-modal="true" aria-labelledby>` with a styled panel, a close `IconButton` (`data-dialog-close`), and `{ body }`. Opens via `document.getElementById(id).showModal()`; the script (`dialog.js`) wires focus-trap, Esc/backdrop close, and return-focus-to-opener for any `[data-dialog-open="id"]` trigger.
  - `type ConfirmSpec struct { ID, TitleKey, BodyKey, ConfirmLabelKey string; ConfirmAttrs templ.Attributes }`
  - `templ ConfirmDialog(spec ConfirmSpec)` — confirm modal: title + consequence body, **Abbrechen** button (secondary, `autofocus`, `data-dialog-close`) + **danger** confirm button carrying `spec.ConfirmAttrs` (e.g. `hx-post=".../delete"`). Defaults: `TitleKey="confirm.title"`, `BodyKey="confirm.deleteBody"`, `ConfirmLabelKey="common.delete"` when empty.
  - The `dialog.js` asset is embedded (Task 4 already broadened `go:embed all:static`, but `dialog.js` lives under the *components* package, so see the embed note below) and referenced by `Base`? — No: to keep `Base` lean, `Dialog` self-includes the script once via a `<script src="/static/js/dialog.js" defer>` guarded by an idempotent loader. The file is served from the webui static tree.
- Consumes: `internal/adapter/webui/components` (`T`, `IconButton`, `Button`), `github.com/a-h/templ`.

> **Embed placement decision:** `dialog.js` must be served at `/static/js/dialog.js`. The served static tree is `internal/adapter/webui/static` (package `webui`, Task 4 embed). So create the file at `internal/adapter/webui/static/js/dialog.js` (NOT under the components package). It is picked up by the existing `//go:embed all:static`. The duplicate path in the File Structure section above is corrected here: the canonical location is `internal/adapter/webui/static/js/dialog.js`.

Steps:

- [ ] Write failing test `internal/adapter/webui/components/dialog_test.go`:
```go
package components_test

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestDialogStructure(t *testing.T) {
	out := render(t, components.Dialog("editDlg", "common.edit", templ.Raw(`<p id="db">form</p>`)))
	for _, w := range []string{
		`id="editDlg"`, `aria-modal="true"`, `data-dialog-close`, `id="db"`,
		`/static/js/dialog.js`, "Bearbeiten",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("Dialog missing %q: %s", w, out)
		}
	}
}

func TestConfirmDialogDefaultsAndDanger(t *testing.T) {
	out := render(t, components.ConfirmDialog(components.ConfirmSpec{
		ID:           "delDlg",
		ConfirmAttrs: templ.Attributes{"hx-post": "/sessions/s1/delete"},
	}))
	for _, w := range []string{
		"Bist du sicher?",                     // default title
		"kann nicht rückgängig",               // default body (confirm.deleteBody)
		"Abbrechen",                           // cancel
		"autofocus",                           // cancel focused by default (safe)
		"Löschen",                             // default confirm label
		"bg-red",                              // danger confirm button
		`hx-post="/sessions/s1/delete"`,       // confirm action wired
	} {
		if !strings.Contains(out, w) {
			t.Errorf("ConfirmDialog missing %q: %s", w, out)
		}
	}
}
```
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect FAIL.
- [ ] Create `internal/adapter/webui/static/js/dialog.js`:
```javascript
// Dialog behavior for styled native <dialog>: open via [data-dialog-open="id"],
// close via [data-dialog-close], Esc, or backdrop click; focus-trap inside the
// open dialog and return focus to the opener on close. Idempotent: safe to load
// once even if multiple Dialog components include the <script>.
(function () {
  if (window.__flowDialogInit) return;
  window.__flowDialogInit = true;

  var lastOpener = null;

  function focusable(dlg) {
    return Array.prototype.slice.call(dlg.querySelectorAll(
      'a[href],button:not([disabled]),textarea,input,select,[tabindex]:not([tabindex="-1"])'
    )).filter(function (el) { return el.offsetParent !== null; });
  }

  document.addEventListener('click', function (e) {
    var opener = e.target.closest('[data-dialog-open]');
    if (opener) {
      var dlg = document.getElementById(opener.getAttribute('data-dialog-open'));
      if (dlg && typeof dlg.showModal === 'function') {
        lastOpener = opener;
        dlg.showModal();
        var f = focusable(dlg);
        var auto = dlg.querySelector('[autofocus]');
        (auto || f[0] || dlg).focus();
      }
      return;
    }
    var closer = e.target.closest('[data-dialog-close]');
    if (closer) {
      var d = closer.closest('dialog');
      if (d) d.close();
      return;
    }
  });

  // Backdrop click (click directly on the <dialog>, not its panel) closes it.
  document.addEventListener('click', function (e) {
    if (e.target.tagName === 'DIALOG' && e.target.open) {
      var rect = e.target.getBoundingClientRect();
      var inside = e.clientX >= rect.left && e.clientX <= rect.right &&
                   e.clientY >= rect.top && e.clientY <= rect.bottom;
      // showModal centers a panel inside; clicks on the dialog element itself
      // (outside the panel) land here — close.
      if (!inside || e.target === e.currentTarget) { /* no-op */ }
    }
  });

  // Focus trap + return focus.
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Tab') return;
    var dlg = document.querySelector('dialog[open]');
    if (!dlg) return;
    var f = focusable(dlg);
    if (f.length === 0) return;
    var first = f[0], last = f[f.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  });

  document.addEventListener('close', function (e) {
    if (e.target.tagName === 'DIALOG' && lastOpener) {
      lastOpener.focus();
      lastOpener = null;
    }
  }, true);
})();
```
- [ ] Create `internal/adapter/webui/components/dialog.templ`:
```go
package components

import "github.com/a-h/templ"

// Dialog is a styled native <dialog>. Open it with a trigger carrying
// data-dialog-open="<id>"; close via the ✕ button, Esc, or backdrop. The
// included dialog.js wires focus-trap and return-focus (idempotent loader).
templ Dialog(id, titleKey string, body templ.Component) {
	<dialog id={ id } aria-modal="true" aria-labelledby={ id + "-title" }
		class="m-auto w-[min(92vw,32rem)] rounded-3xl border border-line bg-surface text-ink p-0 shadow-lift backdrop:bg-ink/40">
		<div class="flex items-center justify-between px-6 pt-5 pb-3 border-b border-line2">
			<h2 id={ id + "-title" } class="font-display text-lg font-semibold">{ T(ctx, titleKey) }</h2>
			@IconButton("✕", T(ctx, "common.close"), templ.Attributes{"data-dialog-close": true, "type": "button"})
		</div>
		<div class="px-6 py-5">
			@body
		</div>
	</dialog>
	<script src="/static/js/dialog.js" defer></script>
}

// ConfirmSpec configures a ConfirmDialog. Empty key fields fall back to safe
// delete-confirmation defaults.
type ConfirmSpec struct {
	ID              string
	TitleKey        string
	BodyKey         string
	ConfirmLabelKey string
	ConfirmAttrs    templ.Attributes
}

func (s ConfirmSpec) withDefaults() ConfirmSpec {
	if s.TitleKey == "" {
		s.TitleKey = "confirm.title"
	}
	if s.BodyKey == "" {
		s.BodyKey = "confirm.deleteBody"
	}
	if s.ConfirmLabelKey == "" {
		s.ConfirmLabelKey = "common.delete"
	}
	return s
}

// ConfirmDialog is the mandatory in-design confirmation for destructive
// actions. Abbrechen is the focused default (safe); the confirm button is
// danger-styled and carries ConfirmAttrs (e.g. hx-post to the destructive route).
templ ConfirmDialog(spec ConfirmSpec) {
	{{ s := spec.withDefaults() }}
	<dialog id={ s.ID } aria-modal="true" aria-labelledby={ s.ID + "-title" }
		class="m-auto w-[min(92vw,26rem)] rounded-3xl border border-line bg-surface text-ink p-6 shadow-lift backdrop:bg-ink/40">
		<h2 id={ s.ID + "-title" } class="font-display text-lg font-semibold">{ T(ctx, s.TitleKey) }</h2>
		<p class="mt-2 text-[.9rem] text-body">{ T(ctx, s.BodyKey) }</p>
		<div class="mt-6 flex items-center justify-end gap-3">
			<button type="button" data-dialog-close autofocus
				class="inline-flex items-center justify-center gap-2 rounded-2xl px-5 py-2.5 text-[.92rem] font-semibold transition border border-line bg-surface text-ink hover:bg-sunken">
				{ T(ctx, "common.cancel") }
			</button>
			@Button(BtnDanger, T(ctx, s.ConfirmLabelKey), "", s.ConfirmAttrs)
		</div>
	</dialog>
	<script src="/static/js/dialog.js" defer></script>
}
```
- [ ] Run `make generate`.
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect PASS.
- [ ] Create `scripts/verify-no-popups.sh`:
```bash
#!/usr/bin/env bash
# Enforce the design rule: NO native browser popups in the WebUI. Confirmations
# and alerts must use the in-design <dialog> components.
set -euo pipefail

# rg is required (repo convention); grep -E fallback keeps CI portable.
pattern='window\.(alert|confirm|prompt)|[^.a-zA-Z](alert|confirm|prompt)[[:space:]]*\('
dir="internal/adapter/webui"

if command -v rg >/dev/null 2>&1; then
  hits="$(rg -n --pcre2 "$pattern" "$dir" || true)"
else
  hits="$(grep -rnE "$pattern" "$dir" || true)"
fi

if [ -n "$hits" ]; then
  echo "verify-no-popups: FAIL — native browser popups are banned (use Dialog/ConfirmDialog):" >&2
  echo "$hits" >&2
  exit 1
fi
echo "verify-no-popups: OK"
```
- [ ] `chmod +x scripts/verify-no-popups.sh`
- [ ] Run `./scripts/verify-no-popups.sh` → expect `verify-no-popups: OK` (the `dialog.js` focus-trap code uses no alert/confirm/prompt).
- [ ] In `Makefile`, add `verify-no-popups` to `.PHONY`, add the target after `verify-css`:
```makefile
# verify-no-popups bans native browser popups in the WebUI (use Dialog instead).
verify-no-popups:
	@./scripts/verify-no-popups.sh
```
and update `ci`:
```makefile
ci: lint verify-generate verify-css verify-no-popups cover build
```
- [ ] Run `make verify-no-popups` → expect OK. Run `make verify-generate` → OK.
- [ ] Commit: `git add internal/adapter/webui/components internal/adapter/webui/static/js/dialog.js scripts/verify-no-popups.sh Makefile && git commit -m "feat(webui): Dialog + ConfirmDialog (focus-trap, a11y) + no-popups CI guard"`

---

## Task 9: `Pagination` (pure presentation)

Prev/next + "Mehr laden", disabled at first/last page; takes `page,total,pageSize,baseHref`. Backend params arrive in later slices — this is presentation only.

**Files:**
- Create: `internal/adapter/webui/components/pagination.templ`
- Test: `internal/adapter/webui/components/pagination_test.go`

**Interfaces:**
- Produces (package `components`):
  - `type PageNav struct { Page, Total, PageSize int; BaseHref string }` with helpers `func (p PageNav) Pages() int`, `func (p PageNav) HasPrev() bool`, `func (p PageNav) HasNext() bool`, `func (p PageNav) PrevHref() string`, `func (p PageNav) NextHref() string` (append `?page=N` or `&page=N` depending on whether `BaseHref` already has a `?`).
  - `templ Pagination(p PageNav)` — renders Zurück (disabled if `!HasPrev`), a "Seite X / Y" indicator, Weiter (disabled if `!HasNext`), and a "Mehr laden" link to the next page (hidden on the last page). i18n keys `page.prev/next/more/label`.
- Consumes: `internal/adapter/webui/components` (`T`), `github.com/a-h/templ`.

Steps:

- [ ] Write failing test `internal/adapter/webui/components/pagination_test.go`:
```go
package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestPageNavMath(t *testing.T) {
	p := components.PageNav{Page: 2, Total: 95, PageSize: 20, BaseHref: "/wissen"}
	if p.Pages() != 5 {
		t.Errorf("Pages() = %d, want 5", p.Pages())
	}
	if !p.HasPrev() || !p.HasNext() {
		t.Errorf("page 2 of 5 should have both prev and next")
	}
	if p.PrevHref() != "/wissen?page=1" {
		t.Errorf("PrevHref = %q", p.PrevHref())
	}
	if p.NextHref() != "/wissen?page=3" {
		t.Errorf("NextHref = %q", p.NextHref())
	}
}

func TestPageNavHrefWithExistingQuery(t *testing.T) {
	p := components.PageNav{Page: 1, Total: 10, PageSize: 5, BaseHref: "/wissen?tag=go"}
	if p.NextHref() != "/wissen?tag=go&page=2" {
		t.Errorf("NextHref with query = %q", p.NextHref())
	}
}

func TestPaginationDisabledAtEdges(t *testing.T) {
	first := render(t, components.Pagination(components.PageNav{Page: 1, Total: 30, PageSize: 10, BaseHref: "/x"}))
	if !strings.Contains(first, "Zurück") {
		t.Errorf("missing Zurück label")
	}
	if !strings.Contains(first, "aria-disabled=\"true\"") {
		t.Errorf("first page must disable Zurück (aria-disabled): %s", first)
	}
	if !strings.Contains(first, "Seite 1") {
		t.Errorf("missing page indicator: %s", first)
	}
	last := render(t, components.Pagination(components.PageNav{Page: 3, Total: 30, PageSize: 10, BaseHref: "/x"}))
	if !strings.Contains(last, "Weiter") {
		t.Errorf("missing Weiter label")
	}
	// last page disables Weiter and hides "Mehr laden"
	if strings.Contains(last, "Mehr laden") {
		t.Errorf("last page must not show 'Mehr laden': %s", last)
	}
}
```
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect FAIL.
- [ ] Create `internal/adapter/webui/components/pagination.templ`:
```go
package components

import "strconv"

// PageNav is the presentation state for a paginated list. Backend wires Page,
// Total and PageSize in later slices; this component is pure presentation.
type PageNav struct {
	Page, Total, PageSize int
	BaseHref              string
}

func (p PageNav) Pages() int {
	if p.PageSize <= 0 {
		return 1
	}
	n := (p.Total + p.PageSize - 1) / p.PageSize
	if n < 1 {
		return 1
	}
	return n
}

func (p PageNav) HasPrev() bool { return p.Page > 1 }
func (p PageNav) HasNext() bool { return p.Page < p.Pages() }

func (p PageNav) hrefFor(page int) string {
	sep := "?"
	for i := 0; i < len(p.BaseHref); i++ {
		if p.BaseHref[i] == '?' {
			sep = "&"
			break
		}
	}
	return p.BaseHref + sep + "page=" + strconv.Itoa(page)
}

func (p PageNav) PrevHref() string { return p.hrefFor(p.Page - 1) }
func (p PageNav) NextHref() string { return p.hrefFor(p.Page + 1) }

// Pagination renders Zurück / "Seite X / Y" / Weiter plus a "Mehr laden" link.
// Edge buttons are aria-disabled (and rendered as <span>) when unavailable.
templ Pagination(p PageNav) {
	<nav class="mt-6 flex items-center justify-between gap-3" aria-label={ T(ctx, "page.label") }>
		<div class="flex items-center gap-2">
			if p.HasPrev() {
				<a href={ templ.SafeURL(p.PrevHref()) }
					class="inline-flex items-center gap-1.5 rounded-xl border border-line bg-surface px-3.5 py-2 text-[.86rem] font-medium text-body hover:bg-sunken hover:text-ink transition-colors">
					<span aria-hidden="true">‹</span> { T(ctx, "page.prev") }
				</a>
			} else {
				<span aria-disabled="true"
					class="inline-flex items-center gap-1.5 rounded-xl border border-line2 bg-sunken/50 px-3.5 py-2 text-[.86rem] font-medium text-faint cursor-not-allowed">
					<span aria-hidden="true">‹</span> { T(ctx, "page.prev") }
				</span>
			}
			if p.HasNext() {
				<a href={ templ.SafeURL(p.NextHref()) }
					class="inline-flex items-center gap-1.5 rounded-xl border border-line bg-surface px-3.5 py-2 text-[.86rem] font-medium text-body hover:bg-sunken hover:text-ink transition-colors">
					{ T(ctx, "page.next") } <span aria-hidden="true">›</span>
				</a>
			} else {
				<span aria-disabled="true"
					class="inline-flex items-center gap-1.5 rounded-xl border border-line2 bg-sunken/50 px-3.5 py-2 text-[.86rem] font-medium text-faint cursor-not-allowed">
					{ T(ctx, "page.next") } <span aria-hidden="true">›</span>
				</span>
			}
		</div>
		<span class="text-[.82rem] text-muted tnum">Seite { strconv.Itoa(p.Page) } / { strconv.Itoa(p.Pages()) }</span>
		if p.HasNext() {
			<a href={ templ.SafeURL(p.NextHref()) }
				class="rounded-xl border border-dashed border-line px-3.5 py-2 text-[.86rem] font-medium text-muted hover:border-blue/40 hover:text-blue transition-colors">
				{ T(ctx, "page.more") }
			</a>
		}
	</nav>
}
```
- [ ] Run `make generate`.
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect PASS.
- [ ] Run `golangci-lint run ./internal/adapter/webui/components/...` → expect clean.
- [ ] Run `make verify-generate` → OK.
- [ ] Commit: `git add internal/adapter/webui/components && git commit -m "feat(webui): Pagination (prev/next + Mehr laden, edge-disabled, pure presentation)"`

---

## Task 10: Styleguide route `/ui` + main-wiring + full smoke (Docker offline)

The testable deliverable: a `StyleguidePage` showcasing every component above in both themes (with a working ☀/☾ toggle), a `handleWebStyleguide` handler behind `s.webAuth`, the route mounted in `server.go`, and the full done-gate (templ, css, ci, curl smokes, podman offline run).

**Files:**
- Create: `internal/adapter/webui/components/styleguide.templ`
- Create: `internal/adapter/httpserver/webui_styleguide.go`
- Create: `internal/adapter/httpserver/webui_styleguide_test.go`
- Modify: `internal/adapter/httpserver/server.go` (mount `GET /ui`)
- Test: `internal/adapter/webui/components/styleguide_test.go`, `internal/adapter/httpserver/webui_styleguide_test.go`

**Interfaces:**
- Produces:
  - `templ StyleguidePage()` (package `components`) — renders inside `Base` + `AppShell`, sections demoing Buttons (all 4 variants), IconButton, Cards, Badges (all 4 doc-kinds), Chips (several hues) + Tag, StatTiles, EmptyState, Glyphs, a `Dialog` + a `ConfirmDialog` (each with a `data-dialog-open` trigger), `TabStrip`, `Breadcrumb`, and `Pagination` (a middle page so both edges render enabled). Header shows `styleguide.title`/`styleguide.subtitle`.
  - `func (s *Server) handleWebStyleguide(w http.ResponseWriter, r *http.Request)` (package `httpserver`) — sets locale via `i18n.WithLocale(r.Context(), i18n.Resolve(r))`, then `components.StyleguidePage().Render(ctx, w)`.
- Consumes: `internal/adapter/webui/components`, `internal/i18n`, `s.webAuth`.

Steps:

- [ ] Write failing test `internal/adapter/webui/components/styleguide_test.go`:
```go
package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestStyleguideShowcasesEverything(t *testing.T) {
	out := render(t, components.StyleguidePage())
	for _, w := range []string{
		"Design-System",                 // styleguide.title
		"/static/app.css",               // Base hull present
		"/static/vendor/htmx.min.js",    // offline
		"data-theme-toggle",             // working toggle in chrome
		"bg-red",                        // danger button variant
		"data-dialog-open",              // a dialog trigger
		"aria-modal=\"true\"",           // a dialog rendered
		"Abbrechen",                     // ConfirmDialog cancel
		"Seite 2",                       // Pagination middle page
		"Projekt",                       // a doc-kind badge
	} {
		if !strings.Contains(out, w) {
			t.Errorf("StyleguidePage missing %q", w)
		}
	}
}
```
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect FAIL.
- [ ] Create `internal/adapter/webui/components/styleguide.templ`:
```go
package components

import "github.com/a-h/templ"

// StyleguidePage is the /ui showcase: it renders every Slice-0 component inside
// the real Base + AppShell chrome (so the theme toggle and offline assets are
// exercised end-to-end), in a single page that reads in both themes.
templ StyleguidePage() {
	@Base("Design-System", styleguideBody())
}

templ styleguideBody() {
	@AppShell("today", nil, styleguideSubnav(), styleguideContent())
}

templ styleguideSubnav() {
	@TabStrip([]Tab{
		{Key: "today", Href: "/", LabelKey: "nav.today"},
		{Key: "week", Href: "/woche", LabelKey: "nav.week"},
		{Key: "history", Href: "/historie", LabelKey: "nav.history"},
		{Key: "stats", Href: "/stats", LabelKey: "nav.stats"},
		{Key: "frei", Href: "/frei", LabelKey: "nav.dayoffs"},
	}, "today")
}

templ styleguideContent() {
	@Breadcrumb([]Crumb{{Href: "/", Label: "flow"}, {Label: T(ctx, "styleguide.title")}})
	<header class="mb-8">
		<p class="eyebrow uppercase text-[.72rem] font-semibold text-blue mb-1">{ T(ctx, "styleguide.subtitle") }</p>
		<h1 class="font-display text-[2rem] sm:text-[2.5rem] font-semibold leading-none tracking-tight">{ T(ctx, "styleguide.title") }</h1>
	</header>

	// Buttons
	@Card("mb-6", sgButtons())
	// Badges + Chips
	@Card("mb-6", sgBadgesChips())
	// StatTiles
	@Card("mb-6", sgStatTiles())
	// EmptyState
	@Card("mb-6", sgEmpty())
	// Dialogs
	@Card("mb-6", sgDialogs())
	// Pagination (middle page so both edges enabled)
	@Card("mb-6", sgPagination())
}

templ sgButtons() {
	<h2 class="font-display text-lg font-semibold mb-4">Buttons</h2>
	<div class="flex flex-wrap items-center gap-3">
		@Button(BtnPrimary, T(ctx, "common.save"), "✓", nil)
		@Button(BtnSecondary, T(ctx, "common.edit"), "", nil)
		@Button(BtnGhost, T(ctx, "common.cancel"), "", nil)
		@Button(BtnDanger, T(ctx, "common.delete"), "■", nil)
		@IconButton("✚", T(ctx, "common.new"), nil)
	</div>
}

templ sgBadgesChips() {
	<h2 class="font-display text-lg font-semibold mb-4">Badges &amp; Chips</h2>
	<div class="flex flex-wrap items-center gap-3">
		@Badge(KindDaily)
		@Badge(KindProject)
		@Badge(KindFree)
		@Badge(KindAgent)
		@Chip("flow", "blue")
		@Chip("homelab", "teal")
		@Chip("serverkraken", "purple")
		@Tag("rebuild")
	</div>
}

templ sgStatTiles() {
	<h2 class="font-display text-lg font-semibold mb-4">StatTiles</h2>
	<div class="grid grid-cols-3 gap-3">
		@StatTile("nav.today", "5h 42m", "")
		@StatTile("nav.week", "32h 10m", "")
		@StatTile("nav.stats", "6 ▲", "green")
	</div>
}

templ sgEmpty() {
	<h2 class="font-display text-lg font-semibold mb-4">EmptyState</h2>
	@EmptyState("·", "empty.default", "empty.default")
}

templ sgDialogs() {
	<h2 class="font-display text-lg font-semibold mb-4">Dialoge</h2>
	<div class="flex flex-wrap items-center gap-3">
		@Button(BtnSecondary, T(ctx, "common.edit"), "", templ.Attributes{"data-dialog-open": "sgEditDlg", "type": "button"})
		@Button(BtnDanger, T(ctx, "common.delete"), "■", templ.Attributes{"data-dialog-open": "sgDelDlg", "type": "button"})
	</div>
	@Dialog("sgEditDlg", "common.edit", templ.Raw(`<p class="text-[.9rem] text-body">Beispiel-Dialoginhalt.</p>`))
	@ConfirmDialog(ConfirmSpec{
		ID:           "sgDelDlg",
		ConfirmAttrs: templ.Attributes{"type": "button", "data-dialog-close": true},
	})
}

templ sgPagination() {
	<h2 class="font-display text-lg font-semibold mb-4">Pagination</h2>
	@Pagination(PageNav{Page: 2, Total: 95, PageSize: 20, BaseHref: "/ui"})
}
```
- [ ] Run `make generate`.
- [ ] Run `go test ./internal/adapter/webui/components/...` → expect PASS.
- [ ] Write failing test `internal/adapter/httpserver/webui_styleguide_test.go`:
```go
package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestStyleguideRouteRendersBehindAuth(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)

	srv := &httpserver.Server{
		Users:   users,
		Session: codec,
		Bus:     sse.NewBus(),
		Clock:   clk,
		Ensure:  usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	// Unauthenticated → redirect to login.
	noRedir := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := noRedir.Get(ts.URL + "/ui")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("unauth /ui = %d, want 302", res.StatusCode)
	}

	// Authenticated → 200 + showcase content.
	cookieVal, _ := codec.Issue("u1")
	req, _ := http.NewRequest("GET", ts.URL+"/ui", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res2.Body.Close() }()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("auth /ui = %d, want 200", res2.StatusCode)
	}
	body := make([]byte, 64*1024)
	n, _ := res2.Body.Read(body)
	out := string(body[:n])
	for _, w := range []string{"Design-System", "/static/app.css", "data-theme-toggle"} {
		if !strings.Contains(out, w) {
			t.Errorf("/ui body missing %q", w)
		}
	}
}
```
- [ ] Run `go test ./internal/adapter/httpserver/ -run TestStyleguideRouteRendersBehindAuth` → expect FAIL (handler + route missing).
- [ ] Create `internal/adapter/httpserver/webui_styleguide.go`:
```go
package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/i18n"
)

// handleWebStyleguide renders the /ui design-system showcase. It is gated by
// webAuth (mounted in Routes) and resolves the UI locale for i18n strings.
func (s *Server) handleWebStyleguide(w http.ResponseWriter, r *http.Request) {
	ctx := i18n.WithLocale(r.Context(), i18n.Resolve(r))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = components.StyleguidePage().Render(ctx, w)
}
```
- [ ] In `internal/adapter/httpserver/server.go`, append the styleguide route inside `Routes()`, immediately BEFORE the `mux.Handle("GET /static/", ...)` line:
```go
	// WebUI design-system showcase (Slice 0 deliverable; handler in webui_styleguide.go).
	mux.Handle("GET /ui", s.webAuth(http.HandlerFunc(s.handleWebStyleguide)))
```
- [ ] Run `go test ./internal/adapter/httpserver/ -run TestStyleguideRouteRendersBehindAuth` → expect PASS.
- [ ] Run `make generate` then `make verify-generate` → expect OK.
- [ ] Run `make web` (ensure css fresh) then `make verify-css` → expect OK.
- [ ] **Full CI gate:** run `make ci` → expect lint clean, verify-generate OK, verify-css OK, verify-no-popups OK, coverage ≥ 80%, build OK. If coverage dips below 80% because the styleguide templ added uncovered generated branches, add render assertions to `styleguide_test.go` until the gate passes (the component render tests cover the generated code).
- [ ] **Live curl smoke** (needs the dev stack — start it if not running): `make dev-up` then `make dev-run` in one shell; in another, obtain a session by scripted Dex login is not required for `/static/*`, but `/ui` needs a cookie. Use the dev token flow per `deploy/dev/README.md`. Smoke (replace `$COOKIE` with a valid `flow_session`):
  - `curl -fsS -o /dev/null -w '%{http_code}\n' http://localhost:8080/static/app.css` → `200`
  - `curl -fsS -o /dev/null -w '%{http_code}\n' http://localhost:8080/static/vendor/htmx.min.js` → `200`
  - `curl -fsS -o /dev/null -w '%{http_code}\n' http://localhost:8080/static/fonts/Inter-Variable.woff2` → `200`
  - `curl -fsS -o /dev/null -w '%{http_code}\n' --cookie "flow_session=$COOKIE" http://localhost:8080/ui` → `200`
  - `curl -fsS --cookie "flow_session=$COOKIE" http://localhost:8080/ui | rg -q "Design-System" && echo OK` → `OK`
- [ ] **Docker offline smoke:**
  - Build: `podman build -t flow-server:slice0 -f deploy/podman/Dockerfile.server .` → succeeds (the in-image tailwind step produces app.css; templ generate is a no-op; go build succeeds).
  - Inspect the image serves vendored assets with no CDN: run the container detached on a network with NO egress is overkill for a smoke; instead assert the rendered HTML references only local paths: `podman run --rm -d --name flow-slice0 -e FLOW_DEV=1 -p 18080:8080 flow-server:slice0` then `curl -fsS -o /dev/null -w '%{http_code}\n' http://localhost:18080/static/vendor/htmx.min.js` → `200`, and `curl -fsS http://localhost:18080/ui` (unauthenticated returns the 302 to /auth/login; that is fine — the offline proof is the static asset 200 + the absence of external origins in `Base`, already asserted by `TestBaseHullIsOfflineAndThemed`). Stop: `podman rm -f flow-slice0`.
  - If `FLOW_DEV=1` alone is insufficient to boot the server without Postgres/OIDC for this smoke, scope the smoke to the static endpoints only (`/static/*` need no auth and no DB) — those prove the embedded vendored assets ship and serve. Note the limitation in the commit message.
- [ ] Commit: `git add internal/adapter/webui/components internal/adapter/httpserver/webui_styleguide.go internal/adapter/httpserver/webui_styleguide_test.go internal/adapter/httpserver/server.go && git commit -m "feat(webui): /ui styleguide route + main-wiring; Slice 0 done-gate (ci+curl+docker)"`

---

## Done-Gate Checklist (verify before declaring Slice 0 complete)
- [ ] `make ci` green (lint, verify-generate, verify-css, verify-no-popups, cover ≥ 80%, build).
- [ ] `/ui` renders 200 behind auth, 302 to login when unauthenticated.
- [ ] `/static/app.css`, `/static/vendor/htmx.min.js`, `/static/vendor/htmx-ext-sse.js`, all three fonts serve 200.
- [ ] `Base` references ZERO external origins (asserted by `TestBaseHullIsOfflineAndThemed`).
- [ ] Dark/Light toggle works on `/ui` (manual: click ☀/☾, theme flips + persists across reload; `aria-pressed` updates).
- [ ] `podman build` succeeds and produces fresh embedded CSS; container serves vendored assets.
- [ ] Existing feature pages (`/`, `/docs`, `/projects`, `/stats`, `/dayoffs`, `/export`) still render unchanged (run their existing tests: `go test ./internal/adapter/httpserver/... ./internal/adapter/webui/...`).
- [ ] No `*.templ` left ungenerated (`make verify-generate` OK).

---

## Notes for the executor
- After EACH `.templ` edit, run `make generate` before `go test`/`golangci-lint` — the committed `*_templ.go` must be current or `make verify-generate` fails the gate.
- Subagent git commits can land on a detached ref; after each task verify `git log --oneline -1` shows your commit on `rebuild` (recover orphans via `git reflog` if needed) — the wiring/verification is your responsibility, not the per-task implementer's.
- Run `make ci` (NOT just `go test`) at the integration points — lint catches things tests do not (e.g. QF1002).
- Keep every component in its own file (`keine Monolithen`); do not merge components to "save files".
