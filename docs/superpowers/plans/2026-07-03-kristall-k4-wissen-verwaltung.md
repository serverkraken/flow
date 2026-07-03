# Kristall K4 — Sweep Wissen & Verwaltung + Wissen-Rollup + Login — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand every remaining Wissen + Verwaltung page on the Kristall glass design system, complete the §4 containment rule in the cockpit Wissen tab (subtree-default + a `?scope=self` toggle, no new port), and deliver the first flow-owned Kristall auth moment (a logout landing + Kristall error pages; `/auth/login` stays a bare redirect to Dex).

**Architecture:** WebUI-only. No new domain type, port, usecase, or migration. The Wissen-Rollup reuses the exact `Stats.Nodes.Subtree(u.ID, id)` + in-memory `subtreeIDs` filter the Übersicht "Zuletzt-Wissen" card already uses (`TopDocs`, `webui_cockpit.go`), and returns the effective scope so a `.seg` toggle can render. The glass sweep swaps hand-rolled `bg-surface border border-line shadow-soft` chrome for the existing K1 `.glass` class / `@components.Card` (already glass) / `@components.Button`, and introduces one named `.field` input class in `web/tailwind.css` as the single Kristall form-language source. Login adds two slim templates rendered inside `components.Base` (which already carries the facets canvas + no-flash theme), wired into `handleLogout` and the three `http.Error` sites in `webauth.go`.

**Tech Stack:** Go, templ (`*_templ.go` via `make generate`), htmx + SSE, Tailwind v4 (`web/tailwind.css` → `internal/adapter/webui/static/app.css` via `make web`), i18n Go catalogs (`internal/i18n/catalog_{de,en}.go`).

**Model policy (subagent-driven execution):** implementers **sonnet** (haiku for pure transcription/tiny), task reviewers **haiku/sonnet** by risk, final whole-branch review **opus**. ALWAYS set an explicit model; NEVER inherit Fable/Opus. If a subagent dies mid-task at a session limit, the controller finishes + verifies inline (precedent: K1 T5, K2 T6).

**Branch:** `cockpit-story` (continues after the K4 spec commit `ca2aae4`). Ledger: `.superpowers/sdd/progress.md`. **Whole-branch review BASE for K4 = `ca2aae4`** (parent of the first K4 code commit, Task 1).

## Global Constraints

Every task's requirements implicitly include these (verbatim from `AGENTS.md` / `CLAUDE.md` / the K4 spec `docs/superpowers/specs/2026-07-03-kristall-k4-wissen-verwaltung-design.md` + umbrella `2026-07-02-kristall-redesign-design.md`):

- **`make ci` must be GREEN before a task is "done"** — lint (`gofumpt`/`staticcheck`) + verify-generate + **verify-css** + **verify-no-popups** + cover (**75 % gate**, `*_templ.go` excluded) + build. Exit 0 required.
- **Never run `make fmt`** (toolchain skew reformats the whole repo). The `cockpit.*` i18n block has a KNOWN pre-existing gofmt reading from toolchain skew (K3 T7/T8 note) — leave it untouched; the pinned gofumpt does not flag it.
- **templ:** after editing any `.templ`, run `make generate` and commit the resulting `*_templ.go`.
- **Tailwind:** `make web` is NOT part of `make ci`. Run it ONLY when a task introduces a genuinely new utility/class combination (here: Task 5 adds `.field`); then commit the regenerated `internal/adapter/webui/static/app.css` (else `scripts/verify-css.sh` fails on drift). Reusing `.glass`/`.glass-strong`/`.seg`/`.pill-tabs`/`.cta-glow`/`@components.Card`/`@components.Button`/`@components.Chip` needs **no** CSS change. Tailwind v4 scans doc-comments in `.templ`/`.go` for class candidates — a new quoted class string in a comment can drift `app.css`.
- **i18n de+en parity** — every key exists in BOTH `catalog_de.go` and `catalog_en.go`; the parity test gates this.
- **No emoji pictograms.** Monospace glyphs only (`▶ ■ ✚ ✗ ● ○ ◆ ⬡ ⌕` etc.), matching existing usage.
- **Multi-tenant, owner-scoped** — every store/usecase call carries `u.ID`; a cross-tenant leak is Critical. "It's just one user" is invalid.
- **Design-Änderbarkeit** — use tokens + named classes + existing components only. A new arbitrary one-off utility (e.g. an ad-hoc `h-[26px]`, a raw hex) beyond the `.field` class this plan defines is a **review finding**.
- **`hx-boost="false"`** stays on real top-level navigations / downloads / the OIDC redirect anchors.
- **TDD, frequent commits.** Output-asserting tests (assert rendered strings / status codes, not mocks).

**Glass swap recipe (shared by the sweep tasks 3–7):** replace hand-rolled container chrome `... border border-line bg-surface shadow-soft ...` → `... glass shadow-soft ...` (the `.glass` class supplies background + border + blur; keep `shadow-soft`, `rounded-*`, padding, layout utilities). Replace raw `bg-ink px-5 py-2.5 ... text-canvas` action buttons → `@components.Button(components.BtnPrimary, label, glyph, attrs)`. Each sweep task adds a **positive** (glass present), **preservation** (function/link/attr intact), and **negative** (`bg-surface` gone from the swapped block) render assertion.

---

## Task 1: Wissen-Rollup — cockpit Wissen tab subtree-default + `?scope=self` toggle

**Why:** The cockpit Wissen tab is own-only today (`webui_cockpit.go:228-230` → `ListDocuments.Execute(u.ID, &n.ID, nil)`), violating the §4 containment rule that an Engagement/Vorhaben cockpit shows its whole subtree. Make Eng/Vorhaben default to subtree docs with a `?scope=self` toggle to own-only; Repo stays own-only with no toggle. Reuse the Übersicht card's Subtree + in-memory filter — no new port.

**Files:**
- Modify: `internal/adapter/httpserver/webui_cockpit.go` (`fillPanelData` `case "wissen":`, add helper `wissenTabDocs`)
- Modify: `internal/adapter/webui/cockpit_vm.go` (`NodeCockpit` — add `WissenScope string`)
- Modify: `internal/adapter/webui/cockpit.templ` (`case "wissen":` — add `.seg` toggle for Eng/Vorhaben, glass the doc list)
- Modify: `internal/i18n/catalog_de.go`, `internal/i18n/catalog_en.go` (toggle labels)
- Test: `internal/adapter/httpserver/webui_cockpit_test.go`

**Interfaces:**
- Produces: `Server.wissenTabDocs(r *http.Request, u domain.User, n domain.Node) ([]domain.Document, string)` — returns the tab's docs + effective scope `"subtree"` | `"self"`. `NodeCockpit.WissenScope string` carries it to the template.
- Consumes: `s.Stats.Nodes.Subtree(ctx, u.ID, id) ([]domain.Node, error)`, `s.ListDocuments.Execute(ctx, u.ID, *string, []string)`, `domain.Document.NodeID *string`, `domain.KindRepo`.

- [ ] **Step 1: Write the failing tests** — in `webui_cockpit_test.go`, mirror the existing cockpit test harness (server + seeded nodes/docs), add:

```go
// Engagement default: shows a doc booked on a CHILD node (subtree), no ?scope.
func TestCockpitWissen_EngagementDefaultShowsSubtreeDocs(t *testing.T) {
	// seed: engagement E, child repo R under E; a document on R.
	// GET /nodes/{E}/tab/wissen  (no scope param)
	body := ... // render the wissen tab fragment
	if !strings.Contains(body, "doc-on-repo-title") {
		t.Errorf("engagement wissen tab must include subtree (child) docs: %s", body)
	}
	if !strings.Contains(body, `data-scope="subtree"`) {
		t.Errorf("effective scope should be subtree: %s", body)
	}
}

// ?scope=self on an engagement: own-only, child doc dropped.
func TestCockpitWissen_ScopeSelfIsOwnOnly(t *testing.T) {
	// GET /nodes/{E}/tab/wissen?scope=self
	if strings.Contains(body, "doc-on-repo-title") {
		t.Errorf("scope=self must NOT include child docs: %s", body)
	}
	if !strings.Contains(body, `data-scope="self"`) { t.Errorf("want self: %s", body) }
}

// Repo: own-only, no toggle rendered.
func TestCockpitWissen_RepoOwnOnlyNoToggle(t *testing.T) {
	// GET /nodes/{R}/tab/wissen — R is a Repo
	if strings.Contains(body, `data-wissen-toggle`) {
		t.Errorf("repo wissen tab must not render the subtree toggle: %s", body)
	}
}

// Foreign owner's subtree doc never leaks.
func TestCockpitWissen_ForeignDocNotLeaked(t *testing.T) {
	// seed a doc on E owned by user B; request E as user A (A owns E path? — use
	// the existing 2-user harness pattern from webui_nodelogo_test.go): assert
	// the foreign doc title is absent.
}
```

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/adapter/httpserver/ -run TestCockpitWissen -v` → FAIL (`WissenScope`/`data-scope` absent; child docs missing on engagement).

- [ ] **Step 3: Add the VM field.** In `cockpit_vm.go`, in `NodeCockpit` after `Docs []domain.Document // wissen`:

```go
	WissenScope string // "subtree"|"self" — effective Wissen-tab scope (drives the .seg toggle)
```

- [ ] **Step 4: Add the helper + rewrite the `wissen` case.** In `webui_cockpit.go`, replace the `case "wissen":` body (lines 228-230) with:

```go
	case "wissen":
		d.Docs, d.WissenScope = s.wissenTabDocs(r, u, d.N)
```

Add the helper (near `railContributors` / `uebersichtData` siblings), owner-scoped, degrade-safe:

```go
// wissenTabDocs returns the Wissen-tab documents honouring the §4 containment
// rule: an Engagement/Vorhaben shows its whole subtree's docs by default and
// own-only when ?scope=self is set; a Repo always shows own-only. It returns the
// docs plus the effective scope ("subtree"|"self") that drives the toggle.
// Owner-scoped throughout; a Subtree/List failure degrades to own-only (never a
// 500) — mirrors uebersichtData's TopDocs source, no new port.
func (s *Server) wissenTabDocs(r *http.Request, u domain.User, n domain.Node) ([]domain.Document, string) {
	ctx := r.Context()
	self := n.Kind == domain.KindRepo || r.URL.Query().Get("scope") == "self"
	if self || s.Stats.Nodes == nil {
		docs, _ := s.ListDocuments.Execute(ctx, u.ID, &n.ID, nil)
		return docs, "self"
	}
	subtree, serr := s.Stats.Nodes.Subtree(ctx, u.ID, n.ID)
	if serr != nil || len(subtree) == 0 {
		if serr != nil {
			slog.WarnContext(ctx, "cockpit wissen: subtree failed", "nodeID", n.ID, "err", serr)
		}
		docs, _ := s.ListDocuments.Execute(ctx, u.ID, &n.ID, nil)
		return docs, "self"
	}
	ids := make(map[string]bool, len(subtree))
	for _, sn := range subtree {
		ids[sn.ID] = true
	}
	all, err := s.ListDocuments.Execute(ctx, u.ID, nil, nil)
	if err != nil {
		return nil, "subtree"
	}
	out := make([]domain.Document, 0, len(all))
	for _, doc := range all {
		if doc.NodeID != nil && ids[*doc.NodeID] {
			out = append(out, doc)
		}
	}
	return out, "subtree"
}
```

- [ ] **Step 5: Render the toggle + glass the list.** In `cockpit.templ`, `case "wissen":` (lines 324-337). Keep the header + "Neues Wissen" link. After the header, for Eng/Vorhaben render a `.seg` toggle (reuses the existing segmented-toggle class — no new CSS); mark the container `data-scope` for tests:

```go
		case "wissen":
			<div id="wissen-panel" data-scope={ d.WissenScope } class="flex items-center justify-between mb-4">
				<h2 class="text-sm font-semibold text-ink">{ components.T(ctx, "cockpit.wissen.title") }</h2>
				<a href={ templ.SafeURL("/wissen/neu?node=" + d.N.ID) } hx-boost="false" class="text-[.8rem] font-semibold rounded-xl bg-cyan/[.12] text-cyan border border-cyan/25 px-3 py-1.5">+ { components.T(ctx, "cockpit.wissen.add") }</a>
			</div>
			if d.N.Kind != domain.KindRepo {
				<div data-wissen-toggle class="seg mb-4 inline-flex text-[.8rem]">
					<a href={ templ.SafeURL("/nodes/" + d.N.ID + "/tab/wissen") }
						hx-get={ "/nodes/" + d.N.ID + "/tab/wissen" } hx-target="#cockpit-main" hx-swap="innerHTML"
						aria-pressed={ boolAttr(d.WissenScope == "subtree") } class="px-3 py-1.5">{ components.T(ctx, "cockpit.wissen.scopeSubtree") }</a>
					<a href={ templ.SafeURL("/nodes/" + d.N.ID + "/tab/wissen?scope=self") }
						hx-get={ "/nodes/" + d.N.ID + "/tab/wissen?scope=self" } hx-target="#cockpit-main" hx-swap="innerHTML"
						aria-pressed={ boolAttr(d.WissenScope == "self") } class="px-3 py-1.5">{ components.T(ctx, "cockpit.wissen.scopeSelf") }</a>
				</div>
			}
			if len(d.Docs) == 0 {
				<p class="text-sm text-faint">{ components.T(ctx, "cockpit.wissen.empty") }</p>
			} else {
				<ul class="divide-y divide-line2 rounded-2xl glass">
					for _, doc := range d.Docs {
						<li class="px-4 py-2.5 text-sm"><a href={ templ.SafeURL("/wissen/" + doc.ID) } class="hover:text-cyan">{ doc.Title }</a></li>
					}
				</ul>
			}
```

Verify `boolAttr` exists in the webui package (used by other toggles); if not, use `if d.WissenScope == "subtree" { aria-pressed="true" }`-style conditional attributes as elsewhere in `cockpit.templ`. **htmx contract:** the toggle links target `#cockpit-main` (the canonical tab-fragment target) — do NOT change the target.

- [ ] **Step 6: i18n keys.** Add to BOTH catalogs: `cockpit.wissen.scopeSubtree` = de "Ganzer Baum" / en "Whole tree"; `cockpit.wissen.scopeSelf` = de "Nur dieser Knoten" / en "This node only".

- [ ] **Step 7: Generate + test.** `make generate && go test ./internal/adapter/httpserver/ -run TestCockpitWissen -v` → PASS; `go test ./internal/adapter/webui/... ./internal/i18n/...` green; `bash scripts/verify-css.sh` (no new class → no drift).

- [ ] **Step 8: Commit**

```bash
git add internal/adapter/httpserver/webui_cockpit.go internal/adapter/webui/cockpit_vm.go internal/adapter/webui/cockpit.templ internal/adapter/webui/cockpit_templ.go internal/i18n/catalog_*.go internal/adapter/httpserver/webui_cockpit_test.go
git commit -m "feat(kristall): Wissen-Rollup — cockpit tab subtree-default + scope=self toggle"
```

---

## Task 2: Login — Kristall logout landing + error pages

**Why:** `/auth/login` is a bare 302 to Dex (no own UI). The only flow-owned auth moments are plaintext `http.Error` (`webauth.go`: `bad state` 400, `auth failed` 401, `forbidden` 403) and `handleLogout` redirecting to `/` — which loops straight back to Dex. Give logout a Kristall landing and turn the three errors into Kristall pages. No new auth logic, no new dependency. (Spec §5.)

**Files:**
- Create: `internal/adapter/webui/auth.templ` (`AuthPage` — one parametrized template for landing + errors)
- Modify: `internal/adapter/httpserver/webauth.go` (`handleLogout`; the 3 `http.Error` sites in `handleCallback`)
- Modify: `internal/i18n/catalog_de.go`, `catalog_en.go`
- Test: `internal/adapter/httpserver/webauth_test.go`, `internal/adapter/webui/auth_render_test.go` (new)

**Interfaces:**
- Produces: `webui.AuthPage(vm webui.AuthVM) templ.Component` where `AuthVM{ TitleKey, MsgKey string; ShowLogin bool }`. Renders inside `components.Base`: a centered `.glass` card with the wordmark, `T(ctx, TitleKey)`, `T(ctx, MsgKey)`, and — when `ShowLogin` — an `<a href="/auth/login" hx-boost="false">` CTA (`.cta-glow` or `@components.Button`). Status code is set by the HANDLER, not the template.
- Consumes: `components.Base`, `components.T`.

- [ ] **Step 1: Write the failing render tests** — `internal/adapter/webui/auth_render_test.go`:

```go
package webui_test

func TestAuthPage_LandingShowsLoginCTA(t *testing.T) {
	out := render(t, webui.AuthPage(webui.AuthVM{TitleKey: "auth.loggedOut.title", MsgKey: "auth.loggedOut.msg", ShowLogin: true}))
	if !strings.Contains(out, `href="/auth/login"`) { t.Errorf("landing missing login CTA: %s", out) }
	if !strings.Contains(out, "glass") { t.Errorf("auth page not on glass: %s", out) }
}

func TestAuthPage_ForbiddenHasNoLoginCTA(t *testing.T) {
	out := render(t, webui.AuthPage(webui.AuthVM{TitleKey: "auth.forbidden.title", MsgKey: "auth.forbidden.msg", ShowLogin: false}))
	if strings.Contains(out, `href="/auth/login"`) { t.Errorf("forbidden page must not offer re-login: %s", out) }
}
```

And handler tests in `webauth_test.go` (mirror existing auth test harness):

```go
func TestHandleLogout_RendersKristallLanding(t *testing.T) {
	// POST /auth/logout → 200, clears cookie, body contains the "Abgemeldet" title + login CTA.
	if rec.Code != http.StatusOK { t.Fatalf("want 200, got %d", rec.Code) }
	if !strings.Contains(rec.Body.String(), `href="/auth/login"`) { t.Error("logout landing missing login CTA") }
	// cookie cleared: Set-Cookie flow_session with MaxAge<0
}

func TestHandleCallback_ForbiddenRendersKristallPage(t *testing.T) {
	// Ensure.Execute → usecase.ErrNotAllowed ⇒ 403 + Kristall forbidden page (no login CTA).
	if rec.Code != http.StatusForbidden { t.Fatalf("want 403, got %d", rec.Code) }
	if !strings.Contains(rec.Body.String(), "glass") { t.Error("forbidden not Kristall") }
}
```

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/adapter/webui/ -run TestAuthPage -v` + `go test ./internal/adapter/httpserver/ -run 'TestHandleLogout_RendersKristallLanding|TestHandleCallback_ForbiddenRendersKristallPage' -v` → FAIL.

- [ ] **Step 3: Create `auth.templ`.**

```go
package webui

import "github.com/serverkraken/flow/internal/adapter/webui/components"

type AuthVM struct {
	TitleKey string
	MsgKey   string
	ShowLogin bool
}

templ AuthPage(vm AuthVM) {
	@components.Base("auth", authBody(vm))
}

templ authBody(vm AuthVM) {
	<main class="min-h-screen grid place-items-center px-6">
		<div class="glass shadow-soft rounded-2xl p-8 max-w-md w-full text-center">
			<p class="eyebrow mb-2 text-[.72rem] font-semibold uppercase text-cyan">flow</p>
			<h1 class="font-display text-2xl font-semibold text-ink mb-2">{ components.T(ctx, vm.TitleKey) }</h1>
			<p class="text-[.92rem] text-muted mb-6">{ components.T(ctx, vm.MsgKey) }</p>
			if vm.ShowLogin {
				<a href="/auth/login" hx-boost="false" class="cta-glow inline-flex items-center justify-center rounded-xl bg-gradient-to-r from-green to-cyan px-5 py-2.5 text-[.92rem] font-semibold text-canvas">
					{ components.T(ctx, "auth.login") } ›
				</a>
			}
		</div>
	</main>
}
```

(Match the existing Kristall CTA recipe used by the K1 timer widget / `.cta-glow`; if a `components.Button` CTA is the cleaner reuse, use it — no new class either way.)

- [ ] **Step 4: Wire the handlers.** In `webauth.go`, replace `handleLogout`'s redirect:

```go
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.AuthPage(webui.AuthVM{TitleKey: "auth.loggedOut.title", MsgKey: "auth.loggedOut.msg", ShowLogin: true}).Render(r.Context(), w)
}
```

Replace the three `http.Error` sites in `handleCallback` with a small helper that sets the status THEN renders (order matters — write header before body):

```go
func (s *Server) authErrorPage(w http.ResponseWriter, r *http.Request, status int, titleKey, msgKey string, showLogin bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = webui.AuthPage(webui.AuthVM{TitleKey: titleKey, MsgKey: msgKey, ShowLogin: showLogin}).Render(r.Context(), w)
}
```

- `bad state` (400): `s.authErrorPage(w, r, http.StatusBadRequest, "auth.badState.title", "auth.badState.msg", true)`
- `auth failed` (401): `s.authErrorPage(w, r, http.StatusUnauthorized, "auth.failed.title", "auth.failed.msg", true)`
- `forbidden` (403): `s.authErrorPage(w, r, http.StatusForbidden, "auth.forbidden.title", "auth.forbidden.msg", false)`

Leave the two 500 `server error` sites as plaintext `http.Error` (genuine internal errors, not a user-facing Kristall moment). Add `"github.com/serverkraken/flow/internal/adapter/webui"` to `webauth.go` imports.

- [ ] **Step 5: i18n keys** (BOTH catalogs): `auth.login` (de "Anmelden"/en "Sign in"), `auth.loggedOut.title` (de "Abgemeldet"/en "Signed out"), `auth.loggedOut.msg` (de "Du bist abgemeldet."/en "You are signed out."), `auth.forbidden.title` (de "Kein Zugriff"/en "No access"), `auth.forbidden.msg` (de "Dieser Account ist nicht freigeschaltet. Wende dich an den Administrator."/en "This account is not allow-listed. Contact your administrator."), `auth.failed.title` (de "Anmeldung fehlgeschlagen"/en "Sign-in failed"), `auth.failed.msg` (de "Bitte erneut anmelden."/en "Please sign in again."), `auth.badState.title` (de "Sitzung abgelaufen"/en "Session expired"), `auth.badState.msg` (de "Die Anmeldung ist abgelaufen. Bitte erneut versuchen."/en "The sign-in flow expired. Please try again.").

- [ ] **Step 6: Generate + test.** `make generate && go test ./internal/adapter/webui/ -run TestAuthPage -v && go test ./internal/adapter/httpserver/ -run 'TestHandleLogout|TestHandleCallback' -v` → PASS. Note: `AuthPage` uses `components.Base`, which emits `sse-connect="/api/v1/events"`; on an unauthenticated error page that endpoint 401s and EventSource retries quietly — harmless, no user-visible effect (flag for K5 if a no-SSE base is ever wanted). `bash scripts/verify-css.sh` + `verify-no-popups.sh`.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/webui/auth.templ internal/adapter/webui/auth_templ.go internal/adapter/httpserver/webauth.go internal/i18n/catalog_*.go internal/adapter/webui/auth_render_test.go internal/adapter/httpserver/webauth_test.go
git commit -m "feat(kristall): Kristall logout landing + auth error pages"
```

---

## Task 3: Wissen — overview + category on glass

**Files:** `internal/adapter/webui/wissen.templ`; test `internal/adapter/httpserver/webui_document_test.go` (or the Wissen page test — find with `rg -n "WissenPage|handleWebWissenHome|wissenOverview" internal/adapter/httpserver/*_test.go`).

- [ ] **Step 1:** Add a glass assertion to the Wissen-home render/handler test: body contains `glass`, the category cards still link to their `Href`, the search form still posts to its action, and the `bg-ink` "Neu" button is gone (`!strings.Contains(body, "bg-ink")`).
- [ ] **Step 2:** Run — expect FAIL.
- [ ] **Step 3:** Apply the glass swap recipe to `wissen.templ`:
  - Overview cards `:64` `rounded-2xl border border-line bg-surface p-4 shadow-soft` → `rounded-2xl glass p-4 shadow-soft` (keep hover utilities).
  - "Neu" buttons `:46` + `:102` (`bg-ink px-5 py-2.5 text-canvas`) → `@components.Button(components.BtnPrimary, components.T(ctx,"common.new"), "✚", templ.Attributes{"href":"/wissen/neu"})` (check `Button`'s href support in `button.templ`; if it emits `<button>` only, keep an `<a>` wrapper with the CTA classes `.cta-glow` + gradient like Task 2 — no new class).
  - Category doc-list `:137` + results `:219` + flat-section `:261` + project-group article `:148` `border border-line bg-surface shadow-soft` → `glass shadow-soft`.
  - Category nav pill `:122` + search input `:182` `border border-line bg-surface` → `.glass` (or keep `bg-sunken`-style field; prefer `.glass` for containers, the search input can adopt the `.field` class once Task 5 lands — for now swap to `glass`).
  - Tag chips `:194`/`:200`: active → keep highlight; inactive `border border-line bg-surface` → `glass`.
  - **Preserve:** all `href`/`hx-get`/`hx-trigger` (`sse:document.*`) attrs, `swatchStyle(group.Color)`, `Pagination`, `EmptyState`, search form action, `itoa` counts.
- [ ] **Step 4:** `make generate && go test ./internal/adapter/httpserver/ -run 'Wissen' -v` → PASS; `verify-css.sh` (no new class) + `verify-no-popups.sh`.
- [ ] **Step 5: Commit** `... -m "feat(kristall): Wissen overview + category on glass"`

---

## Task 4: Dokument — prose + meta on glass

**Files:** `internal/adapter/webui/document.templ`; test `internal/adapter/httpserver/webui_document_test.go`.

- [ ] **Step 1:** Glass assertion on the document-view test: body contains `glass`; the node link `:51`, tag links `:66`, edit/delete actions `:73`/`:77`, and reembed submit `:127` all preserved; `markdownprose` render + backlinks + TOC intact; `!strings.Contains(swappedBlock, "bg-surface")`.
- [ ] **Step 2:** Run — expect FAIL.
- [ ] **Step 3:** Swap the meta/action chrome to glass: node pill `:51`, tag pills `:66`, edit action `:73`, delete icon-button `:77` (keep the `ConfirmDialog`/delete semantics + `hover:text-danger`), reembed submit `:127` — all `border border-line bg-surface` → `glass`. Wrap the prose column in a `@components.Card("prose-glass-utilities", ...)` OR a `glass rounded-2xl p-6` container if not already carded. **Preserve:** `DocumentFragment` structure, TOC (`components.toc`), backlinks (`components.backlinks`), `documentBreadcrumb`, embed badge, all links + the reembed `POST /wissen/{id}/reembed`.
- [ ] **Step 4:** `make generate && go test ./internal/adapter/httpserver/ -run 'Document' -v` → PASS; `verify-css.sh` + `verify-no-popups.sh`.
- [ ] **Step 5: Commit** `... -m "feat(kristall): Dokument on glass"`

---

## Task 5: Editor — Kristall form language (`.field` class) + glass container

**Why:** Introduces the single named `.field` input class (Kristall form-language source), then applies it to the editor. Task 7 reuses `.field`. **This is the ONLY task that runs `make web`.**

**Files:** `web/tailwind.css` (add `.field` in `@layer components`), `internal/adapter/webui/editor.templ`, `internal/adapter/webui/static/app.css` (regenerated); test `internal/adapter/httpserver/webui_editor_test.go`.

- [ ] **Step 1:** Glass/field assertion on the editor test: form container has `glass`; text/select/textarea inputs carry `field`; `!strings.Contains(container, "bg-surface")`; the form still posts to `vm.Action()`, the type/project selects + path + title + body fields + save button preserved.
- [ ] **Step 2:** Run — expect FAIL.
- [ ] **Step 3:** Add to `web/tailwind.css` `@layer components` (sibling of `.seg`/`.glass`, ~line 359):

```css
  /* Kristall form field — single source for glass inputs across editor + node form */
  .field { width: 100%; background: rgb(var(--sunken) / .6); border: 1px solid rgb(var(--line)); border-radius: 12px; padding: 8px 12px; color: rgb(var(--ink)); transition: border-color .13s ease, box-shadow .13s ease; }
  .field:focus { outline: none; border-color: rgb(var(--blue) / .5); box-shadow: 0 0 0 3px rgb(var(--blue) / .12); }
  .field:disabled { color: rgb(var(--muted)); background: rgb(var(--sunken)); }
  .field::placeholder { color: rgb(var(--faint)); }
```

- [ ] **Step 4:** In `editor.templ`: form container `:49` `border border-line bg-surface p-5 shadow-soft` → `glass p-5 shadow-soft`; every input/select/textarea (`:54,56,66,78,80,85,91,96`) → replace `rounded-xl border border-line bg-sunken/60 px-3 py-2 ...` with `class="field ..."` keeping only type-specific utilities (`font-mono`, `h-[60vh] min-h-96` on the body textarea). Preview section `:106` `border border-line bg-surface` → `glass`. Save/cancel → `@components.Button`. **Preserve:** `vm.Action()`, field `name`s (`type`, `projectId`, `path`, `title`, body), disabled states, preview hx wiring.
- [ ] **Step 5:** `make generate && make web` (new `.field` class) → commit `app.css`. `go test ./internal/adapter/httpserver/ -run 'Editor' -v` → PASS; `bash scripts/verify-css.sh` → PASS (app.css now contains `.field`); `verify-no-popups.sh`.
- [ ] **Step 6: Commit** `git add web/tailwind.css internal/adapter/webui/editor.templ internal/adapter/webui/editor_templ.go internal/adapter/webui/static/app.css internal/adapter/httpserver/webui_editor_test.go && git commit -m "feat(kristall): Editor Kristall form + .field class"`

---

## Task 6: Projektliste — tree-rows on glass

**Note:** Logos are ALREADY rendered (`nodeGlyphSwatch:88` does Logo>Icon>Glyph). This task is the container glass + "Neu" button only.

**Files:** `internal/adapter/webui/nodes.templ` (`NodesFragment:32`, list `:54`, "Neu" `:41`); test `internal/adapter/httpserver/webui_nodes_test.go`.

- [ ] **Step 1:** Glass assertion on the nodes-list test: body has `glass`; tree rows still link to `/nodes/{id}`, `nodeGlyphSwatch` (logo `<img>`), kind/status badges + `gitDisplay` preserved; `bg-ink` "Neu" gone.
- [ ] **Step 2:** Run — expect FAIL.
- [ ] **Step 3:** List container `:54` `border border-line bg-surface shadow-soft` → `glass shadow-soft`; "Neu" button `:41` `bg-ink` → `@components.Button` CTA (or `.cta-glow` anchor). **Preserve:** `nodeTreeRow` indent (`nodeIndentStyle`), `nodeGlyphSwatch` priority, badges, `sse` triggers on `nodesOuter`, `Pagination` if present.
- [ ] **Step 4:** `make generate && go test ./internal/adapter/httpserver/ -run 'Nodes' -v` → PASS; `verify-css.sh` + `verify-no-popups.sh`.
- [ ] **Step 5: Commit** `... -m "feat(kristall): Projektliste on glass"`

---

## Task 7: Node-Formular — Kristall form language (`.field` reuse)

**Files:** `internal/adapter/webui/nodes.templ` (`NodeForm:118`, `nodeFormInner:126`, inputs `:140-265`, radios `:302,317,335`); test `internal/adapter/httpserver/webui_nodes_test.go` (node-form render).

- [ ] **Step 1:** Field assertion on the node-form test: name/slug/kind/parent/description/upstreamGit/status/countsMode/logo/rate inputs carry `field`; `!strings.Contains(formBlock, "bg-surface")`; the color/glyph/icon radio grids (Slice-2 artifacts) preserved; cancel `:265` + submit → `@components.Button`; the form still posts create/update with all `name`s intact.
- [ ] **Step 2:** Run — expect FAIL.
- [ ] **Step 3:** Replace every `rounded-lg border border-line bg-surface px-3 py-2 ...` input/select/textarea (`:140,144,149,158,170,185,189,193,201,244,250,255,259`) with `class="field ..."` (keep `font-mono`, `w-32`/`w-20` width utilities, the disabled parent-select's `opacity-50 cursor-not-allowed`). Cancel anchor `:265` + submit → `@components.Button` (BtnSecondary/BtnPrimary). Radio grids (`nodeColorRadio`/`nodeGlyphRadio`/`nodeIconRadio`) — glass the option tiles if they use `bg-surface`, else leave. **Preserve:** all field `name`s, `id`s (`node-kind`, `node-parent` — `nodeform.js` hooks), the file input `accept`, the icon-radio grid, `orDefault` rate defaults.
- [ ] **Step 4:** `make generate && go test ./internal/adapter/httpserver/ -run 'Nodes|NodeForm' -v` → PASS; `verify-css.sh` (`.field` already in app.css from Task 5 — no new class → no drift) + `verify-no-popups.sh`.
- [ ] **Step 5: Commit** `... -m "feat(kristall): Node-Formular Kristall form language"`

---

## Task 8: Main-wiring, gate & live-verification

**Why:** No new routes/usecases were added, but the sweep touched many templates and the rollup added a query-param branch. This task verifies the composition end-to-end, confirms Einstellungen (already `@Card`-glass) needs nothing, and runs the full gate + live checklist before the Opus whole-branch review. (No production code beyond fixes surfaced here.)

**Files:** verification only (+ any fix commits).

- [ ] **Step 1: Einstellungen verify.** `rg -n "bg-surface|border border-line" internal/adapter/webui/einstellungen.templ` → expect empty (it uses `@components.Card`, already glass). If any hand-rolled chrome exists, apply the glass recipe + a render-test assertion and commit `feat(kristall): Einstellungen on glass`. Otherwise record "no-op (Card already glass)" in the ledger.
- [ ] **Step 2: Dangling-ref / consistency audit.**
  - `rg -n "bg-ink px-5" internal/adapter/webui/*.templ` → expect empty (all action buttons swept).
  - `rg -n "\.field" internal/adapter/webui/static/app.css` → present (Task 5 landed it).
  - `rg -n "WissenScope|wissenTabDocs" internal/adapter/{httpserver,webui}` → the field is produced (handler) and consumed (template) — no orphan.
  - Confirm the Wissen toggle targets `#cockpit-main` and the tab routes `GET /nodes/{id}/tab/{name}` + `GET /nodes/{id}?tab=` both honour `?scope=self` (both call `nodeCockpitData`→`fillPanelData` which reads `r.URL.Query().Get("scope")`).
- [ ] **Step 3: Full gate.** `make ci` → exit 0, coverage ≥ 75 %. Record the % in the ledger.
- [ ] **Step 4: Live gate** (scripted Dex login vs `make dev-up && make dev-run`, https://localhost:8080). Verify: (a) Wissen overview + a category + a document + the editor + the projektliste + a node form + Einstellungen all render on **glass** (no `bg-surface` panels); (b) on an **Engagement** cockpit the Wissen tab shows a child-repo's doc by default, the "Nur dieser Knoten" `.seg` toggle flips to own-only and back (URL gains/loses `?scope=self`), a **Repo** cockpit's Wissen tab shows **no** toggle; (c) **logout** lands on the Kristall "Abgemeldet" page with a working "Anmelden" CTA (not a bounce to Dex); (d) hitting `/auth/callback` with a bad state renders the Kristall 400 page (curl the plaintext-free body); a non-allowlisted login shows the Kristall **forbidden** page. Clean up any test docs/nodes created.
- [ ] **Step 5:** Update `.superpowers/sdd/progress.md` with the K4 ledger (commits, `make ci` %, live-gate result, BASE `ca2aae4`, HEAD). Then hand to the Opus whole-branch review: `scripts/review-package ca2aae4 <HEAD>`, dispatch opus. Focus: rollup owner-scoping (foreign-doc leak = Critical) + degrade-safety, `?scope` honoured on both cockpit entry points, auth pages set status BEFORE render + clear cookie, `.field` is the sole new class (no arbitrary one-offs), i18n de+en parity, downloads/hx-boost/SSE untouched, the 3 controller-inline-risk areas. Carry the accepted tradeoff (Task 1 in-memory full-doc-load, mirrors K2 T5) + the SSE-on-401 auth-page note for triage.

---

## Self-Review

**Spec coverage:** §3 sweep — Wissen (T3), Dokument (T4), Editor (T5), Projektliste (T6), Node-Formular (T7), Einstellungen (T8 verify). §4 Wissen-Rollup — T1. §5 Login — T2. §6 architecture (no new port/usecase) — honoured (T1 helper is compositional; T2 templates only). §7 testing — per-task positive/preservation/negative + rollup handler cases + auth render/status tests. §8 process — T8 gate + Opus review + BASE `ca2aae4`. §11 open details resolved: `scope=self` param + `.seg` toggle labels (T1), one parametrized `AuthPage` (T2), shared `Base` for landing+errors (T2).

**Placeholder scan:** No TBD/TODO. Sweep tasks follow the accepted K3 recipe granularity (concrete files/anchors/preserve-lists/assertions) rather than full line-by-line code — matches the shipped-clean K3 precedent; the two logic tasks (T1/T2) carry full code.

**Type consistency:** `wissenTabDocs(r, u, n) ([]domain.Document, string)` produced in T1 Step 4, field `NodeCockpit.WissenScope string` T1 Step 3, consumed T1 Step 5. `AuthVM{TitleKey, MsgKey string; ShowLogin bool}` + `webui.AuthPage` produced T2 Step 3, consumed T2 Step 4. `.field` class defined T5 Step 3, reused T7. `Document.NodeID *string` (verified). `domain.KindRepo`, `Stats.Nodes.Subtree(ctx,u.ID,id)` (verified against `webui_cockpit.go` worktime case).
