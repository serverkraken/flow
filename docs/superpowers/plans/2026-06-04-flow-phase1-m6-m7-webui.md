# flow Phase 1 — M6+M7 WebUI Implementation Plan (Plan E)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the Phase-1 WebUI on top of the existing `flow-server` — Templ-rendered pages with HTMX partials, Tokyonight-themed Tailwind v4 styling, Alpine.js for tiny interaction state, ApexCharts on the worktime dashboard, CodeMirror 6 for note editing. M6 lands read-only navigation across all entities; M7 unlocks editing + start/stop buttons.

**Architecture:** The same `cmd/flow-server` binary that already serves `/api/v1/*` adds an HTML route group at `/`. Templates compile through `github.com/a-h/templ` to typed Go funcs and live in `internal/webui/templates/`. Static assets (Tailwind output, Alpine, ApexCharts, CodeMirror) ship via `//go:embed`, mirroring how `flow` already bundles cheatsheet markdown. Browser auth was wired in M1 (Authorization-Code-Flow with `HttpOnly`/`Secure`/`SameSite=Lax` cookies) — the new HTML routes reuse the existing `NewAuthMiddleware`. All write operations route through the same use cases the CLI/TUI/MCP use; HTMX sends `hx-post` to handlers that return partial HTML on success, full-page redirects on auth failure.

**Tech Stack:** Templ 0.x (latest stable), Tailwind v4 (esbuild-pipeline triggered by Make), Alpine.js 3.x (CDN-pinned + vendored), ApexCharts (vendored minified), CodeMirror 6 (vendored bundle built via esbuild). Go side: `github.com/a-h/templ` for codegen, no other new deps. Tailwind toolchain runs through Node — vendor `npx tailwindcss@^4` into a `tools/tailwind/` directory or use the standalone CLI to keep Go-only build path intact for CI.

**Prerequisite:** Plans C (M4 RepoNotes) and ideally D (flow-mcp) merged on `next`. The WebUI's repo-note page targets the same `usecase.RepoNotes` that Plan C builds; if Plan C is missing, scope M6/M7 to Worktime + Projects + Settings only and defer the notes pages.

---

## File Structure

**Create (build toolchain):**
- `tools/tailwind/package.json` — pins `tailwindcss@^4`, lists `@tailwindcss/typography`.
- `tools/tailwind/tailwind.config.js` — Tokyonight palette, JetBrains Mono font, scans `internal/webui/**/*.templ`.
- `tools/tailwind/input.css` — `@tailwind base; @tailwind components; @tailwind utilities;` plus custom layer for tokyonight tokens.
- `Makefile` — new targets `webui-css` (compile Tailwind → `internal/webui/static/styles.css`), `webui-templ` (`templ generate ./internal/webui/...`), and integration with `make build` (run both before `go build`).

**Create (webui adapter):**
- `internal/webui/assets.go` — `//go:embed static templates *.templ` (after codegen) for the production binary.
- `internal/webui/static/styles.css` — Tailwind output (generated, committed).
- `internal/webui/static/alpine.min.js` — vendored 3.x.
- `internal/webui/static/apexcharts.min.js` + `apexcharts.min.css` — vendored.
- `internal/webui/static/codemirror.bundle.js` + `.css` — esbuild bundle (build script in `tools/codemirror/`).
- `internal/webui/static/htmx.min.js` — vendored 2.x.
- `internal/webui/static/fonts/` — JetBrains Mono subsets.

**Create (templates — base + chrome):**
- `internal/webui/templates/layout/base.templ` — full HTML shell, nav, footer, HTMX boost.
- `internal/webui/templates/layout/nav.templ` — top nav with current-page indicator.
- `internal/webui/templates/layout/flash.templ` — flash message strip (sync errors, save confirmations).
- `internal/webui/templates/layout/icons.templ` — lucide-icon SVG snippets (stroke style per `feedback_no_icons`).

**Create (handlers + templates per route — M6 read-only):**
- `internal/webui/handlers/dashboard.go` + `templates/dashboard/index.templ` — `/`.
- `internal/webui/handlers/worktime.go` + `templates/worktime/{index,today,week,history,frei}.templ` — `/worktime`.
- `internal/webui/handlers/notes.go` + `templates/notes/{index,view}.templ` — `/notes`, `/notes/:id`.
- `internal/webui/handlers/repos.go` + `templates/repos/{index,note}.templ` — `/repos`, `/repos/:hash/note`.
- `internal/webui/handlers/projects.go` + `templates/projects/index.templ` — `/projects`.
- `internal/webui/handlers/settings.go` + `templates/settings/index.templ` — `/settings`.

**Create (handlers — M7 write):**
- HTMX-partial templates under `templates/<area>/_partials/*.templ` (e.g. `_session_row.templ` for the worktime session table).
- `internal/webui/handlers/session_actions.go` — PUT/DELETE for sessions, POST start/stop.
- `internal/webui/handlers/note_actions.go` — PUT for kompendium notes + repo notes.
- `internal/webui/handlers/project_actions.go` — POST create project.

**Create (SSE):**
- `internal/webui/handlers/events.go` — `/api/v1/events?stream=ui` SSE endpoint.
- `internal/webui/sse/broadcaster.go` — fan-out broadcaster, used by sync worker + active-session events.

**Modify:**
- `internal/adapter/httpserver/server.go` — register `webui.Handlers(...)` on the cookie-authed route group (not the bearer group; the WebUI uses cookies).
- `cmd/flow-server/main.go` — instantiate `webui.New(deps)` and pass to `httpserver.AuthDeps.WebUI`.

**Docs:**
- `docs/runbook/webui-dev.md` — `make webui-css watch`, dev-mode tooling (Air?), how to add a new page.

---

## Phase A: Toolchain + base layout (M6 scaffold)

### Task 1: Tailwind v4 toolchain

**Files:**
- Create: `tools/tailwind/package.json`, `tailwind.config.js`, `input.css`
- Modify: `Makefile`

- [ ] **Step 1: Vendor Tailwind**

```json
// tools/tailwind/package.json
{
  "name": "flow-tailwind",
  "private": true,
  "devDependencies": {
    "tailwindcss": "^4.0.0",
    "@tailwindcss/typography": "^0.5.0"
  }
}
```

```bash
cd tools/tailwind && npm install
```

Commit the lockfile but **not** `node_modules` (gitignore).

- [ ] **Step 2: Tokyonight config**

```js
// tools/tailwind/tailwind.config.js
module.exports = {
  content: ['../../internal/webui/templates/**/*.templ'],
  theme: {
    extend: {
      fontFamily: { mono: ['JetBrains Mono', 'monospace'] },
      colors: {
        // tokyonight-night palette — match internal/frontend/tui/theme
        bg:       '#1a1b26',
        bgDark:   '#16161e',
        fg:       '#c0caf5',
        fgDim:    '#9aa5ce',
        accent:   '#7aa2f7',
        active:   '#9ece6a',
        warn:     '#e0af68',
        err:      '#f7768e',
        muted:    '#414868',
      },
    },
  },
  plugins: [require('@tailwindcss/typography')],
};
```

- [ ] **Step 3: input.css + Makefile target**

```css
/* tools/tailwind/input.css */
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  body { @apply bg-bg text-fg font-mono; }
}
```

```makefile
.PHONY: webui-css webui-css-watch webui-templ webui

webui-css:
	cd tools/tailwind && npx tailwindcss \
	  -c tailwind.config.js \
	  -i input.css \
	  -o ../../internal/webui/static/styles.css \
	  --minify

webui-css-watch:
	cd tools/tailwind && npx tailwindcss \
	  -c tailwind.config.js \
	  -i input.css \
	  -o ../../internal/webui/static/styles.css \
	  --watch

webui-templ:
	go run github.com/a-h/templ/cmd/templ@latest generate ./internal/webui/...

webui: webui-templ webui-css

build: webui
	go build -o bin/flow ./cmd/flow
	go build -o bin/flow-server ./cmd/flow-server
```

- [ ] **Step 4: Smoke**

```bash
make webui-css
ls -l internal/webui/static/styles.css
```

- [ ] **Step 5: Commit**

```bash
git add tools/tailwind/ Makefile
git commit -m "build(webui): Tailwind v4 toolchain + webui-css/webui-templ targets"
```

### Task 2: Base layout + nav

**Files:**
- Create: `internal/webui/templates/layout/{base,nav,flash,icons}.templ`
- Create: `internal/webui/assets.go`

- [ ] **Step 1: Base shell**

```go
// internal/webui/templates/layout/base.templ
package layout

templ Base(title string, currentPath string, body templ.Component) {
  <!DOCTYPE html>
  <html lang="de" class="dark">
    <head>
      <meta charset="utf-8"/>
      <meta name="viewport" content="width=device-width, initial-scale=1"/>
      <title>{ title } — flow</title>
      <link rel="stylesheet" href="/static/styles.css"/>
      <script src="/static/htmx.min.js"></script>
      <script src="/static/alpine.min.js" defer></script>
    </head>
    <body hx-boost="true">
      @Nav(currentPath)
      <main class="max-w-5xl mx-auto px-6 py-8">
        @Flash()
        @body
      </main>
    </body>
  </html>
}
```

- [ ] **Step 2: Nav + Flash + Icons**

Nav lists `Dashboard / Worktime / Projekte / Notes / Repos / Settings` with the active page in `bg-muted text-accent`. Flash reads a session-cookie value set by handlers (`flash=success:saved` → green pill, `flash=err:conflict` → red pill, cleared on read).

- [ ] **Step 3: assets.go**

```go
package webui

import "embed"

//go:embed static templates
var assets embed.FS

func StaticFS() embed.FS { return assets }
```

- [ ] **Step 4: Run codegen + commit**

```bash
make webui-templ
go build ./...
git add internal/webui/templates/layout/ internal/webui/assets.go
git commit -m "feat(webui): base layout + nav + flash + icons"
```

### Task 3: Static asset vendoring

**Files:**
- `internal/webui/static/{alpine.min.js, apexcharts.min.js, apexcharts.min.css, htmx.min.js, codemirror.bundle.js, codemirror.bundle.css, fonts/}`

- [ ] **Step 1: Vendor CDN assets**

```bash
curl -L https://unpkg.com/alpinejs@3/dist/cdn.min.js > internal/webui/static/alpine.min.js
curl -L https://unpkg.com/htmx.org@2 > internal/webui/static/htmx.min.js
curl -L https://cdn.jsdelivr.net/npm/apexcharts/dist/apexcharts.min.js > internal/webui/static/apexcharts.min.js
curl -L https://cdn.jsdelivr.net/npm/apexcharts/dist/apexcharts.css > internal/webui/static/apexcharts.min.css
```

Each download: pin to a specific version in the URL, commit the version next to the file in a `VERSIONS.md`.

- [ ] **Step 2: CodeMirror 6 bundle**

CM6 doesn't ship a single-file CDN bundle. Build one:

```bash
mkdir -p tools/codemirror
cat > tools/codemirror/build.mjs <<'EOF'
import { build } from 'esbuild';
build({
  entryPoints: ['entry.mjs'],
  bundle: true,
  minify: true,
  format: 'iife',
  globalName: 'CM',
  outfile: '../../internal/webui/static/codemirror.bundle.js',
}).catch(() => process.exit(1));
EOF
cat > tools/codemirror/entry.mjs <<'EOF'
import { EditorView, basicSetup } from 'codemirror';
import { markdown } from '@codemirror/lang-markdown';
import { keymap } from '@codemirror/view';
import { defaultKeymap, indentWithTab } from '@codemirror/commands';
import { vim } from '@replit/codemirror-vim';
export { EditorView, basicSetup, markdown, keymap, defaultKeymap, indentWithTab, vim };
EOF
cat > tools/codemirror/package.json <<'EOF'
{ "private": true, "devDependencies": { "esbuild": "*", "codemirror": "*",
  "@codemirror/lang-markdown": "*", "@codemirror/view": "*",
  "@codemirror/commands": "*", "@replit/codemirror-vim": "*" } }
EOF
cd tools/codemirror && npm install && node build.mjs
```

- [ ] **Step 3: Fonts**

JetBrains Mono subset. Either reuse the local TUI font file or download from JetBrains' GitHub release.

- [ ] **Step 4: Commit**

```bash
git add internal/webui/static/ tools/codemirror/
echo "node_modules/" >> tools/codemirror/.gitignore
git commit -m "feat(webui): vendor static assets (alpine/htmx/apexcharts/codemirror)"
```

### Task 4: Login page wiring

**Files:**
- Create: `internal/webui/templates/auth/login.templ`
- Modify: `internal/adapter/httpserver/auth_browser.go`

- [ ] **Step 1: Pretty login page**

The current `/login` 302-redirects to Authentik. M6 adds a landing template that's shown when an unauthenticated user hits `/` — it explains the OIDC flow and offers a "Mit Authentik anmelden" button that submits to `/login`.

```go
templ Login() {
  @layout.Base("Anmelden", "/login") {
    <div class="max-w-md mx-auto text-center py-24">
      <h1 class="text-2xl mb-4">flow</h1>
      <p class="text-fgDim mb-8">Multi-device worktime + repo notes.</p>
      <a href="/login" class="px-6 py-3 bg-accent text-bg rounded-sm">
        Mit Authentik anmelden
      </a>
    </div>
  }
}
```

- [ ] **Step 2: Cookie middleware passthrough**

`NewAuthMiddleware` (from M1) already handles `/login`/`/auth/callback`/`/logout`; the new HTML routes mount inside an outer group that 302's to `/auth/landing` (showing the template above) when the cookie is missing.

- [ ] **Step 3: Commit**

```bash
make webui-templ
go build ./...
git add internal/webui/templates/auth/ internal/adapter/httpserver/auth_browser.go
git commit -m "feat(webui): auth landing template + unauthenticated redirect"
```

---

## Phase B: M6 read-only routes

### Task 5: Dashboard `/`

**Files:**
- Create: `internal/webui/handlers/dashboard.go`
- Create: `internal/webui/templates/dashboard/index.templ`

- [ ] **Step 1: Handler**

```go
type DashboardDeps struct {
    UserID         string
    WorktimeReader *usecase.WorktimeReader
    Active         *usecase.ActiveSessions
    StatusComposer *usecase.StatusComposer
}

func NewDashboard(d DashboardDeps) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        active, _ := d.Active.ListActive(d.UserID)
        today, _ := d.WorktimeReader.Today(d.UserID, time.Now())
        comp, _ := d.StatusComposer.Compose(d.UserID, time.Now())
        _ = layout.Base("Dashboard", "/",
            dashboard.Index(active, today, comp)).
            Render(r.Context(), w)
    })
}
```

- [ ] **Step 2: Template**

Shows: active sessions (one card per running project), today's saldo, week-saldo, streak. Uses ApexCharts for a small sparkline of the last 7 days.

- [ ] **Step 3: Commit**

```bash
make webui
go build ./...
git add internal/webui/handlers/dashboard.go internal/webui/templates/dashboard/
git commit -m "feat(webui): dashboard page"
```

### Task 6: Worktime `/worktime` + sub-tabs

**Files:**
- Create: `internal/webui/handlers/worktime.go`
- Create: `internal/webui/templates/worktime/{index,today,week,history,frei}.templ`

- [ ] **Step 1: Handler dispatches by ?tab=...**

```go
func NewWorktime(d WorktimeDeps) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        tab := cmp.Or(r.URL.Query().Get("tab"), "today")
        switch tab {
        case "today":   /* call reader.Today, render today.templ */
        case "week":    /* week.templ */
        case "history": /* history.templ */
        case "frei":    /* frei.templ */
        }
    })
}
```

- [ ] **Step 2: Templates**

Each sub-tab mirrors the TUI shape. `today.templ` shows a session table with edit/delete buttons (M7 enables them; M6 renders disabled). Week/history use ApexCharts for the saldo chart. Frei lists DayOff entries.

- [ ] **Step 3: Commit**

```bash
make webui
git add internal/webui/handlers/worktime.go internal/webui/templates/worktime/
git commit -m "feat(webui): /worktime with today/week/history/frei sub-tabs"
```

### Task 7: Notes `/notes`, `/notes/:id`

**Files:**
- Create: `internal/webui/handlers/notes.go`
- Create: `internal/webui/templates/notes/{index,view}.templ`

- [ ] **Step 1: Index lists kompendium notes**

Kompendium-Notes are still file-backed at this point (Plan C deferred their DB-sync). Read them through the existing `usecase.NoteLister` / `usecase.NoteReader` from cmd/flow. The list view shows title + first-paragraph excerpt, search box uses simple substring on file-cache (FTS5 wants its own plan).

- [ ] **Step 2: View renders Markdown**

Reuse the same markdown renderer the TUI uses (`internal/frontend/tui/markdown`). Wrap output in `prose prose-invert` for Tokyonight tone.

- [ ] **Step 3: Commit**

```bash
make webui
git add internal/webui/handlers/notes.go internal/webui/templates/notes/
git commit -m "feat(webui): /notes index + /notes/:id view"
```

### Task 8: Repos `/repos`, `/repos/:hash/note`

**Files:**
- Create: `internal/webui/handlers/repos.go`
- Create: `internal/webui/templates/repos/{index,note}.templ`

- [ ] **Step 1: Index lists known repos**

Calls `cacheRepos.PullSince(userID, 0, 1000)` to enumerate. Each row shows CanonicalKey, DisplayName, last note update.

- [ ] **Step 2: Note view**

`/repos/:hash/note` parses the URL-escaped CanonicalKey, looks up the Repo + RepoNote, renders markdown. M6 = read-only; the "Bearbeiten" button is rendered but inert (or links to the future M7 edit route).

- [ ] **Step 3: Commit**

```bash
make webui
git add internal/webui/handlers/repos.go internal/webui/templates/repos/
git commit -m "feat(webui): /repos index + /repos/:hash/note view"
```

### Task 9: Projects + Settings

**Files:**
- Create: `internal/webui/handlers/projects.go` + `templates/projects/index.templ`
- Create: `internal/webui/handlers/settings.go` + `templates/settings/index.templ`

- [ ] **Step 1: Projects index**

Lists active + archived projects (mirrors the TUI projects screen's Worktime-Projekte sub-tab). M6 = read-only; create/rename/archive come in M7.

- [ ] **Step 2: Settings**

Shows: logged-in user info (`sub`, email, display name), server URL, cache.db path, last sync timestamp, "Abmelden" button → posts to `/logout`.

- [ ] **Step 3: Commit**

```bash
make webui
git add internal/webui/handlers/projects.go internal/webui/templates/projects/ \
        internal/webui/handlers/settings.go internal/webui/templates/settings/
git commit -m "feat(webui): /projects + /settings read-only views"
```

### Task 10: Route registration + M6 smoke

**Files:**
- Modify: `internal/adapter/httpserver/server.go`
- Modify: `cmd/flow-server/main.go`
- Create: `scripts/smoke-m6-webui.sh`

- [ ] **Step 1: Register routes**

Inside `NewWithAuth`, mount a cookie-authed group:

```go
r.Group(func(rr chi.Router) {
    rr.Use(NewBrowserAuthMiddleware(d.Session, d.Cookie.Name))
    rr.Handle("/static/*", http.StripPrefix("/static/",
        http.FileServer(http.FS(d.WebUI.StaticFS()))))
    rr.Get("/",                  d.WebUI.Dashboard().ServeHTTP)
    rr.Get("/worktime",          d.WebUI.Worktime().ServeHTTP)
    rr.Get("/notes",             d.WebUI.NotesIndex().ServeHTTP)
    rr.Get("/notes/{id}",        d.WebUI.NotesView().ServeHTTP)
    rr.Get("/repos",             d.WebUI.ReposIndex().ServeHTTP)
    rr.Get("/repos/{hash}/note", d.WebUI.RepoNote().ServeHTTP)
    rr.Get("/projects",          d.WebUI.Projects().ServeHTTP)
    rr.Get("/settings",          d.WebUI.Settings().ServeHTTP)
})
```

- [ ] **Step 2: Wiring**

`cmd/flow-server/main.go` builds a `webui.Handlers` struct that bundles all the per-page handlers (constructed from the existing use cases). Passes it via `AuthDeps.WebUI`.

- [ ] **Step 3: Smoke**

```bash
#!/usr/bin/env bash
# scripts/smoke-m6-webui.sh
set -euo pipefail
./bin/flow-server &
SERVER=$!
trap "kill $SERVER" EXIT
sleep 1
# Anonymous → landing
curl -fsS http://localhost:8080/ | grep "Mit Authentik anmelden"
# With a dex token (assumes dex is running per M2/M3 smoke)
TOKEN=$(./scripts/get-dex-token.sh)
COOKIE=$(curl -fsS -c - "http://localhost:8080/auth/callback?code=$TOKEN" | awk '/flow_session/{print $7}')
curl -fsS -H "Cookie: flow_session=$COOKIE" http://localhost:8080/worktime | grep "Heute"
echo "M6 read-only routes OK"
```

- [ ] **Step 4: Commit**

```bash
chmod +x scripts/smoke-m6-webui.sh
git add internal/adapter/httpserver/server.go cmd/flow-server/main.go scripts/smoke-m6-webui.sh
git commit -m "feat(webui): register M6 read-only routes + smoke script"
```

---

## Phase C: M7 write surfaces

### Task 11: Session actions (start/stop/edit/delete)

**Files:**
- Create: `internal/webui/handlers/session_actions.go`
- Create: `internal/webui/templates/worktime/_partials/session_row.templ`
- Modify: `internal/webui/templates/worktime/today.templ`

- [ ] **Step 1: HTMX-friendly partials**

`_session_row.templ` renders a single `<tr>`. On edit, the row swaps with `_session_form.templ` via `hx-get="/worktime/sessions/{id}/edit"`. Save posts to `/worktime/sessions/{id}` with `hx-put`, returning the read-only row again.

- [ ] **Step 2: Handlers**

```go
func NewSessionPut(d) http.Handler // PUT /worktime/sessions/{id}
func NewSessionDelete(d) http.Handler // DELETE /worktime/sessions/{id}
func NewActiveStart(d) http.Handler // POST /worktime/active/{project-id}/start
func NewActiveStop(d) http.Handler // POST /worktime/active/{project-id}/stop
```

Each calls the corresponding use case (`SessionWriter.Edit`, `SessionWriter.Delete`, `ActiveSessions.Start`, `ActiveSessions.Stop`). 409 conflict → render conflict-overlay partial; 200 → render updated row.

- [ ] **Step 3: Conflict overlay partial**

Reuse the conflict shape from the TUI's `conflict_overlay` component — same fields, HTML instead of lipgloss. On "Server übernehmen" the partial reloads the row; on "Lokal überschreiben" it re-PUTs with the higher version.

- [ ] **Step 4: Commit**

```bash
make webui
git add internal/webui/handlers/session_actions.go internal/webui/templates/worktime/
git commit -m "feat(webui): M7 — session edit/delete + start/stop via HTMX"
```

### Task 12: Note + Repo-Note editing with CodeMirror

**Files:**
- Create: `internal/webui/handlers/note_actions.go`
- Create: `internal/webui/templates/notes/edit.templ`
- Create: `internal/webui/templates/repos/note_edit.templ`
- Create: `internal/webui/static/codemirror-init.js`

- [ ] **Step 1: CodeMirror init script**

```js
// internal/webui/static/codemirror-init.js
window.initEditor = (textareaId, initialValue) => {
  const ta = document.getElementById(textareaId);
  const view = new CM.EditorView({
    doc: initialValue,
    extensions: [
      CM.basicSetup,
      CM.markdown(),
      CM.keymap.of([...CM.defaultKeymap, CM.indentWithTab]),
    ],
    parent: ta.parentNode,
  });
  ta.style.display = 'none';
  ta.form.addEventListener('submit', () => {
    ta.value = view.state.doc.toString();
  });
};
```

- [ ] **Step 2: Edit templates**

```go
templ Edit(note domain.RepoNote, repo domain.Repo) {
  @layout.Base("Edit RepoNote — " + repo.DisplayName, "/repos") {
    <form hx-put={ "/repos/" + url.PathEscape(repo.CanonicalKey) + "/note" }
          hx-headers={ fmt.Sprintf(`{"If-Match":"%d"}`, note.Version) }>
      <textarea id="content" name="content">{ note.Content }</textarea>
      <div class="mt-4 flex gap-2">
        <button class="px-4 py-2 bg-active text-bg">Speichern</button>
        <a href={ "/repos/" + url.PathEscape(repo.CanonicalKey) + "/note" }
           class="px-4 py-2 bg-muted">Abbrechen</a>
      </div>
    </form>
    <script>initEditor('content', { note.Content })</script>
  }
}
```

- [ ] **Step 3: PUT handlers**

`NoteActions.RepoNotePut` calls `usecase.RepoNotes.Save(userID, pwd, content)` via a path-by-canonical-key resolver lookup. 409 conflict → re-render with both versions + diff highlight.

- [ ] **Step 4: Commit**

```bash
make webui
git add internal/webui/handlers/note_actions.go internal/webui/templates/notes/edit.templ \
        internal/webui/templates/repos/note_edit.templ internal/webui/static/codemirror-init.js
git commit -m "feat(webui): M7 — note + repo-note editing with CodeMirror 6"
```

### Task 13: Project create/rename/archive

**Files:**
- Create: `internal/webui/handlers/project_actions.go`
- Modify: `internal/webui/templates/projects/index.templ`

- [ ] **Step 1: HTMX inline form**

A "Neues Projekt" button on `/projects` swaps a row into an inline `<input>` + submit. POST creates via `usecase.Projects.Create` and returns the new row partial. Rename + archive use `hx-put` / `hx-post` against the same row.

- [ ] **Step 2: Commit**

```bash
make webui
git add internal/webui/handlers/project_actions.go internal/webui/templates/projects/index.templ
git commit -m "feat(webui): M7 — project create/rename/archive"
```

### Task 14: SSE for live updates

**Files:**
- Create: `internal/webui/sse/broadcaster.go`
- Create: `internal/webui/handlers/events.go`
- Modify: existing handlers — broadcast on mutation.

- [ ] **Step 1: Broadcaster**

```go
package sse

type Broadcaster struct {
    mu   sync.Mutex
    subs map[chan Event]struct{}
}

type Event struct {
    Type string // "session.started", "session.stopped", "note.updated", "tick"
    Data any
}

func (b *Broadcaster) Subscribe() (<-chan Event, func()) { /* ... */ }
func (b *Broadcaster) Publish(e Event)                   { /* ... */ }
```

- [ ] **Step 2: SSE handler**

```go
func NewEvents(b *sse.Broadcaster) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        w.Header().Set("Cache-Control", "no-cache")
        ch, cancel := b.Subscribe()
        defer cancel()
        for {
            select {
            case <-r.Context().Done(): return
            case e := <-ch:
                fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, mustJSON(e.Data))
                w.(http.Flusher).Flush()
            }
        }
    })
}
```

- [ ] **Step 3: HTMX SSE consumers**

On the dashboard:

```html
<div hx-ext="sse" sse-connect="/api/v1/events?stream=ui"
     sse-swap="session.started,session.stopped,tick">
  <!-- replaced on every event -->
</div>
```

- [ ] **Step 4: Hook mutations**

`SessionActions.Stop` and friends call `broadcaster.Publish({...})` after successful use-case calls. A 1-second ticker publishes `tick` events so the dashboard's "läuft seit MM:SS" updates without polling.

- [ ] **Step 5: Commit**

```bash
git add internal/webui/sse/ internal/webui/handlers/events.go
git commit -m "feat(webui): SSE broadcaster + live updates for dashboard"
```

### Task 15: Tests + E2E smoke

**Files:**
- Create: `internal/webui/handlers/*_test.go` per handler
- Create: `scripts/smoke-m7-webui-write.sh`

- [ ] **Step 1: Handler unit tests**

Drive each handler with `httptest.NewRecorder` + a fake use case. Assert response status, partial HTML content, conflict path.

- [ ] **Step 2: Playwright E2E**

Optional but recommended. `tools/playwright/` directory with a node-based test suite that:
1. Spawns `flow-server` against an in-memory dex
2. Drives login flow
3. Edits a repo note via CodeMirror
4. Starts/stops a session, asserts dashboard updates via SSE

Add to CI via a `make smoke-webui` target that's manual-trigger (not part of `make ci` — Playwright is heavy).

- [ ] **Step 3: Commit**

```bash
make webui
go test ./internal/webui/...
git add internal/webui/handlers/*_test.go tools/playwright/ scripts/smoke-m7-webui-write.sh
git commit -m "test(webui): handler unit tests + Playwright E2E scaffold"
```

### Task 16: Verification

- [ ] **Step 1: make ci**

```bash
make ci
```

Templ-generated `.go` files commit alongside the `.templ` sources. The lint pipeline should accept generated code (golangci-lint's `// Code generated by templ` recognition).

- [ ] **Step 2: Manual round-trip**

Soenne logs into the WebUI from a browser, starts a session, sees it on the TUI on the same laptop, edits the repo note, sees the change in `flow repo note get` on the other laptop after a sync tick.

- [ ] **Step 3: Memory update**

Promote M6+M7 to "DONE" in `project_plan_b_progress.md` + MEMORY.md.
