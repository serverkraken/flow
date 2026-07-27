# Kristall K3 — Sweep Worktime + IA-Enforcement — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce the single-mechanism IA on the worktime surfaces — Home becomes a pure dashboard and Heute a pure day-ledger (both lose their timer forms; the shell timer widget + the shared `SessionDialog` take over) — and stand every remaining worktime/admin page on the Kristall glass design system.

**Architecture:** The K1 shell timer widget (`/ui/timer/*`, `appshell.templ`) already owns start/stop/switch/unbound and already self-reloads on `sse:session.*`; the K2 `SessionDialog` component already provides the one add/edit session mechanism. K3 removes the now-duplicate Home/Heute timer forms + handlers, wires `SessionDialog` into Heute (add **and** the previously-missing edit), and swaps hand-rolled `bg-surface border border-line shadow-*` card chrome across Woche/Historie/Frei/Export + four shared components for the existing `.glass` K1 components. No new usecases, ports, migrations, or domain types.

**Tech Stack:** Go, templ (server-rendered HTML, `*_templ.go` generated via `make generate`), htmx + SSE, Tailwind v4 (`web/tailwind.css` → `internal/adapter/webui/static/app.css` via `make web`), i18n Go catalogs (`internal/i18n/catalog_{de,en}.go`).

**Model policy (subagent-driven execution):** implementers **sonnet** (haiku for pure transcription/tiny), task reviewers **haiku/sonnet** by risk, final whole-branch review **opus**. ALWAYS set an explicit model; NEVER inherit Fable/Opus. If a subagent dies mid-task at a session limit, the controller finishes + verifies inline (precedent: K1 T5, K2 T6).

**Branch:** `cockpit-story` (continues after K2 hygiene commit `d0a46b4`). Ledger: `.superpowers/sdd/progress.md`. Whole-branch review BASE for K3 = the first K3 commit's parent (record it in the ledger when Task 1 lands).

## Global Constraints

Every task's requirements implicitly include these (values verbatim from `AGENTS.md` / `CLAUDE.md` / the umbrella spec `docs/superpowers/specs/2026-07-02-kristall-redesign-design.md`):

- **`make ci` must be GREEN before a task is "done"** — it runs lint (`gofumpt`/`staticcheck`) + verify-generate + **verify-css** + **verify-no-popups** + cover (**75 % gate**, `*_templ.go` excluded) + build. Exit 0 required.
- **Never run `make fmt`** (toolchain skew reformats the whole repo).
- **templ:** after editing any `.templ`, run `make generate` and commit the resulting `*_templ.go`.
- **Tailwind:** `make web` is NOT part of `make ci`. Only run it when a task introduces a **genuinely new utility class combination**; then commit the regenerated `internal/adapter/webui/static/app.css` (else `scripts/verify-css.sh` fails on drift). Reusing existing `Card`/`StatTile`/`StatTileAccent`/`TabStrip`/`.glass`/`.seg` needs **no** CSS change. Tailwind v4 scans doc-comments in `.templ`/`.go` for class candidates — a new quoted class string in a comment can drift `app.css`.
- **i18n de+en parity** — every key exists in BOTH `catalog_de.go` and `catalog_en.go`; the catalog parity test gates this.
- **No emoji pictograms.** Monospace glyphs only (`▶ ■ ✚ ✗ ● ○ ◆ ⬡` etc.), matching existing usage.
- **Multi-tenant, owner-scoped** — every store/usecase call carries `u.ID`; a cross-tenant leak is Critical. "It's just one user" is an invalid justification.
- **SSE live-sync** — every session-mutating handler must `s.Emitter.Emit(... EventSessionStarted/Stopped/Updated/Deleted ...)` or the widget + page won't live-update.
- **`hx-boost="false"`** must stay on the Export download anchors and the logout form (real top-level navigation / file download, not an htmx fetch).
- **Kristall / Design-Änderbarkeit** — use tokens + named glass components; no new arbitrary one-off utility soup beyond the recipes in this plan (a new one-off is a review finding).
- **TDD, frequent commits.** Output-asserting tests (assert rendered strings / status codes, not mocks).

**Scope note — Heute stays today-scoped.** Umbrella §3 defines Heute as a ledger with no session-control; §7's word "Tagesnavigation" is NOT implemented in K3 — adding day navigation would duplicate Historie (past-day calendar + edit), the exact IA duplication K3 removes. Heute remains `heuteDataFor`'s today scope. Past-day editing lives in Historie. (Flagged to Soenne at handoff; veto is additive later.)

**Scope note — Home Puls.** §7 lists "Puls (Ziel-Pills)" for Home. K3 keeps Home's existing activity logstream (glass-restyled); porting the cockpit's richer subtree-pulse (avatars, target pills) to a global Home is a separate feature deferred past K3. Not in this plan.

---

## Task 1: `SessionDialog` — optional hidden `SessionID` field (Heute-edit reuse)

**Why:** Heute's edit endpoint `POST /ui/worktime/edit` (`webui_worktime.go:102 handleWebEdit`) reads the session id from the **form** (`r.FormValue("sessionId")`), but `SessionDialog` currently emits no such field (the cockpit edit endpoint takes the id from the URL path instead). Add an optional hidden `sessionId` so the one shared dialog can drive Heute's form-based edit. Additive + backward compatible — the cockpit passes `SessionID: ""` and is unaffected.

**Files:**
- Modify: `internal/adapter/webui/components/sessiondialog.templ` (VM struct + `sessionDialogBody`)
- Test: `internal/adapter/webui/components/primitives_test.go`

**Interfaces:**
- Produces: `components.SessionDialogVM.SessionID string` — when non-empty, `sessionDialogBody` renders `<input type="hidden" name="sessionId" value={SessionID}>`. Consumed by Task 3/4 (Heute edit dialog).

- [ ] **Step 1: Write the failing test** — append to `internal/adapter/webui/components/primitives_test.go`:

```go
func TestSessionDialog_EditMode_RendersHiddenSessionID(t *testing.T) {
	vm := components.SessionDialogVM{
		DialogID: "d", Mode: "edit", Action: "/ui/worktime/edit", Target: "#content",
		SessionID: "s-42", Date: "2026-07-03", From: "09:00", To: "10:00",
	}
	out := render(t, components.SessionDialog(vm))
	if !strings.Contains(out, `name="sessionId"`) || !strings.Contains(out, `value="s-42"`) {
		t.Errorf("edit dialog missing hidden sessionId: %s", out)
	}
}

func TestSessionDialog_AddMode_NoSessionID(t *testing.T) {
	vm := components.SessionDialogVM{DialogID: "d", Mode: "add", Action: "/x", Target: "#c"}
	out := render(t, components.SessionDialog(vm))
	if strings.Contains(out, `name="sessionId"`) {
		t.Errorf("add dialog must not render sessionId: %s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/webui/components/ -run TestSessionDialog_EditMode_RendersHiddenSessionID -v`
Expected: FAIL — `SessionID` is not a field of `SessionDialogVM` (compile error) or the hidden input is absent.

- [ ] **Step 3: Add the field + hidden input.** In `sessiondialog.templ`, add to the `SessionDialogVM` struct (after `NodeID`):

```go
	SessionID string // optional; when set, posted as a hidden "sessionId" field
	                  // (Heute's /ui/worktime/edit takes the id from the form, not
	                  // the URL path like the cockpit endpoints do).
```

In `sessionDialogBody`, immediately after the opening `<form ...>` tag, add:

```go
		if vm.SessionID != "" {
			<input type="hidden" name="sessionId" value={ vm.SessionID }/>
		}
```

- [ ] **Step 4: Regenerate templ + run tests**

Run: `make generate && go test ./internal/adapter/webui/components/ -run TestSessionDialog -v`
Expected: PASS (both new tests + the existing `TestSessionDialog_*` add/edit tests).

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/webui/components/sessiondialog.templ internal/adapter/webui/components/sessiondialog_templ.go internal/adapter/webui/components/primitives_test.go
git commit -m "feat(kristall): SessionDialog optional hidden sessionId for Heute edit"
```

---

## Task 2: Align Heute add/edit endpoints to the `SessionDialog` `node` contract

**Why:** `SessionDialog` posts the booking node as form field **`node`** (`sessiondialog.templ` — `<select name="node">` / hidden `name="node"`). Heute's `handleWebAdd`/`handleWebEdit` read the node via `resolveWebNode` which uses **`projectId`** + inline-create **`newProject`**. Per umbrella §3.4 the "neues Projekt" quick-create now lives ONLY in the timer widget picker; Nachbuchen/edit use the picker of existing bookable nodes. Align both handlers to read `node` (a plain id; `""` = unassigned) and drop the `projectId`/`newProject` path.

**Files:**
- Modify: `internal/adapter/httpserver/webui_worktime.go` (`handleWebAdd:63`, `handleWebEdit:102`, `resolveWebNode:49`)
- Test: `internal/adapter/httpserver/webui_worktime_handlers_test.go`

**Interfaces:**
- Consumes: `SessionDialog` form fields `date, from, to, node, tag, note` (+ `sessionId` for edit, Task 1).
- Produces: `handleWebAdd`/`handleWebEdit` book to the node in form field `node`; no inline node creation.

- [ ] **Step 1: Write the failing tests** — in `webui_worktime_handlers_test.go`, add (mirror the existing add/edit test harness in that file for server construction + request shape):

```go
func TestHandleWebAdd_BooksNodeFromNodeField(t *testing.T) {
	srv, u := newWorktimeTestServer(t) // reuse the file's existing helper
	form := url.Values{"date": {todayStr()}, "from": {"09:00"}, "to": {"10:00"}, "node": {"n1"}}
	rec := postForm(t, srv, u, "/ui/worktime/add", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("add: got %d", rec.Code)
	}
	// the booked session must reference n1
	assertSessionBookedTo(t, srv, u, "n1")
}

func TestHandleWebAdd_IgnoresLegacyNewProject(t *testing.T) {
	srv, u := newWorktimeTestServer(t)
	before := countNodes(t, srv, u)
	form := url.Values{"date": {todayStr()}, "from": {"09:00"}, "to": {"10:00"}, "newProject": {"ShouldNotExist"}}
	_ = postForm(t, srv, u, "/ui/worktime/add", form)
	if got := countNodes(t, srv, u); got != before {
		t.Errorf("newProject must no longer create a node: nodes %d→%d", before, got)
	}
}
```

> Adapt helper names (`newWorktimeTestServer`, `postForm`, `assertSessionBookedTo`, `countNodes`, `todayStr`) to whatever the existing tests in this file already use; if a helper is missing, add a minimal one next to the existing ones. Do NOT invent a new harness style.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapter/httpserver/ -run 'TestHandleWebAdd_(BooksNodeFromNodeField|IgnoresLegacyNewProject)' -v`
Expected: FAIL — `node` is not read; `newProject` still creates.

- [ ] **Step 3: Rewrite node resolution.** Replace `resolveWebNode` (`webui_worktime.go:49-61`) with a form-field reader (no create):

```go
// webNode reads the SessionDialog booking node ("node" form field). Empty → nil
// (unassigned). Inline project creation lives only in the timer widget picker
// now (umbrella spec §3.4), so there is no "newProject" path here.
func webNode(r *http.Request) *string {
	if v := r.FormValue("node"); v != "" {
		return &v
	}
	return nil
}
```

In `handleWebAdd` (line 77) change `nodeID := s.resolveWebNode(r, u)` → `nodeID := webNode(r)`.
In `handleWebEdit` (line 112) change `nodeID := s.resolveWebNode(r, u)` → `nodeID := webNode(r)`.
Remove the now-unused `usecase` import if `resolveWebNode` was its only user in this file (check: `go build ./...` will flag). `handleWebEdit` still uses `usecase.EditSessionInput`, so the import stays.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapter/httpserver/ -run 'TestHandleWeb(Add|Edit)' -v`
Expected: PASS. Update any pre-existing add/edit test that posted `projectId`/`newProject` to post `node` instead (they now assert the new contract).

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/httpserver/webui_worktime.go internal/adapter/httpserver/webui_worktime_handlers_test.go
git commit -m "refactor(kristall): Heute add/edit read SessionDialog 'node' field, drop inline-create"
```

---

## Task 3: Heute view model — per-row edit prefill + day summary

**Why:** The Heute ledger needs, per session, a pre-filled edit `SessionDialogVM` (so clicking a block opens the shared dialog with times/node/tags/note filled), and a compact day-summary (Heute gesamt / Ziel / Saldo) to preserve "Tages-Summen" after the hero is removed. Mirror the cockpit's `sessionDialogEditVM` (`cockpit_vm.go:128`) but target Heute's endpoint.

**Files:**
- Modify: `internal/adapter/webui/heute_vm.go` (add `Ledger []HeuteLedgerRow`; the day-summary reuses existing `LoggedDur`/`TargetDur`/`Balance`/`BalancePos`)
- Modify: `internal/adapter/httpserver/webui_heute.go` (`heuteDataFor` builds `Ledger`)
- Test: `internal/adapter/httpserver/webui_heute_test.go` (or `_internal_test.go`)

**Interfaces:**
- Produces:
  ```go
  // in package webui (heute_vm.go)
  type HeuteLedgerRow struct {
      Row  components.SessionRowVM   // display fields (existing sessionRowVM output)
      Edit components.SessionDialogVM // edit-mode dialog, per session (empty for a running session)
  }
  ```
  `HeuteVM.Ledger []HeuteLedgerRow`. Consumed by Task 4's template. `Edit.DialogID = "edit-" + sess.ID`.

- [ ] **Step 1: Write the failing test** — in `webui_heute_test.go`:

```go
func TestHeuteDataFor_LedgerCarriesEditPrefill(t *testing.T) {
	srv, u := newHeuteTestServer(t) // reuse existing helper
	seedCompletedSession(t, srv, u, "n1", "09:00", "11:00", []string{"deep"}, "note-x")
	vm, err := srv.heuteDataForExported(t, u) // or call heuteDataFor via the package-internal test
	if err != nil { t.Fatal(err) }
	if len(vm.Ledger) == 0 { t.Fatal("no ledger rows") }
	e := vm.Ledger[0].Edit
	if e.Mode != "edit" || e.SessionID == "" || e.From != "09:00" || e.To != "11:00" || e.NodeID != "n1" {
		t.Errorf("edit prefill wrong: %+v", e)
	}
	if e.Action != "/ui/worktime/edit" { t.Errorf("edit action = %q", e.Action) }
}
```

> If `heuteDataFor` is unexported and untested directly today, add this as an internal test in `webui_heute_internal_test.go` (same package `httpserver`) calling `s.heuteDataFor(ctx, u, "")` directly. Match the seeding style already used by `webui_heute_test.go`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapter/httpserver/ -run TestHeuteDataFor_LedgerCarriesEditPrefill -v`
Expected: FAIL — `Ledger` field / builder absent.

- [ ] **Step 3: Add the VM type + builder.** In `heute_vm.go` add the `HeuteLedgerRow` type above `HeuteWeekRow` and the field `Ledger []HeuteLedgerRow` to `HeuteVM`.

In `webui_heute.go`, replace the row loop (`heuteDataFor` lines 110-113) so it builds both the display row and the edit VM:

```go
	// Today's session rows + per-row edit dialog (the ledger).
	vm.Rows = make([]components.SessionRowVM, 0, len(sessions))
	vm.Ledger = make([]webui.HeuteLedgerRow, 0, len(sessions))
	for _, sess := range sessions {
		row := sessionRowVM(sess, projects, now)
		vm.Rows = append(vm.Rows, row) // kept for any existing consumers/tests
		vm.Ledger = append(vm.Ledger, webui.HeuteLedgerRow{
			Row:  row,
			Edit: heuteEditDialogVM(sess, vm.Nodes, vm.DayParam),
		})
	}
```

Add the builder in `webui_heute.go` (mirrors `sessionDialogEditVM`, but Heute endpoint + shown picker + form-based sessionId):

```go
// heuteEditDialogVM builds the per-row edit SessionDialogVM for the Heute
// ledger. A running session (no Stop) is not editable here — return the zero VM
// (Mode "" → the template skips its dialog). Nodes is the shown reassignment
// picker (preselected to the session's own node); SessionID + Date ride hidden
// so /ui/worktime/edit (form-based) resolves the target.
func heuteEditDialogVM(sess domain.WorkSession, nodes []components.NodePickerItem, dayParam string) components.SessionDialogVM {
	if sess.Stop == nil {
		return components.SessionDialogVM{}
	}
	nodeID := ""
	if sess.NodeID != nil {
		nodeID = *sess.NodeID
	}
	// convert picker items to domain.Node for the dialog's Nodes field
	picker := make([]domain.Node, 0, len(nodes))
	for _, n := range nodes {
		picker = append(picker, domain.Node{ID: n.ID, Name: n.Name})
	}
	return components.SessionDialogVM{
		DialogID:  "edit-" + sess.ID,
		Mode:      "edit",
		Action:    "/ui/worktime/edit",
		Target:    "#content",
		SessionID: sess.ID,
		Date:      sess.Start.Local().Format("2006-01-02"),
		From:      sess.Start.Local().Format("15:04"),
		To:        sess.Stop.Local().Format("15:04"),
		Tag:       strings.Join(sess.Tags, " "),
		Note:      sess.Note,
		Nodes:     picker,
		NodeID:    nodeID,
	}
}
```

(`domain` and `strings` are already imported in `webui_heute.go`.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapter/httpserver/ -run 'TestHeuteDataFor|TestHeute' -v`
Expected: PASS.

- [ ] **Step 5: Regenerate + commit**

```bash
make generate
git add internal/adapter/webui/heute_vm.go internal/adapter/webui/*_templ.go internal/adapter/httpserver/webui_heute.go internal/adapter/httpserver/webui_heute*_test.go
git commit -m "feat(kristall): Heute ledger row VM with per-session edit prefill + day summary reuse"
```

---

## Task 4: Heute template — Blöcke/Cards ledger + SessionDialog add/edit; drop timer forms

**Why:** Heute becomes a pure ledger. Remove the running-hero + idle start-card (the widget owns start/stop). Render each session as a clickable Kristall glass card that opens its edit `SessionDialog`; the "Nachbuchen" button opens an add `SessionDialog` (replacing the old `Dialog`+`heuteAddForm`); keep the delete `ConfirmDialog` and the week pace card; add a compact day-summary tile row.

**Files:**
- Modify: `internal/adapter/webui/heute.templ`
- Test: `internal/adapter/httpserver/webui_heute_test.go`

- [ ] **Step 1: Write the failing render test:**

```go
func TestHeutePage_LedgerNoTimerForms(t *testing.T) {
	srv, u := newHeuteTestServer(t)
	seedCompletedSession(t, srv, u, "n1", "09:00", "11:00", nil, "")
	body := getBody(t, srv, u, "/zeit")
	// the timer control forms are gone
	if strings.Contains(body, "/ui/worktime/start") || strings.Contains(body, "/ui/worktime/stop") {
		t.Errorf("Heute must not render start/stop forms")
	}
	// add dialog is the shared SessionDialog (session.dialog.date key rendered)
	if !strings.Contains(body, "/ui/worktime/add") { t.Errorf("add SessionDialog missing") }
	// per-row edit dialog present + delete confirm present
	if !strings.Contains(body, `id="edit-`) { t.Errorf("per-row edit dialog missing") }
	if !strings.Contains(body, "/ui/worktime/delete") { t.Errorf("delete confirm missing") }
	// glass ledger cards
	if !strings.Contains(body, "glass") { t.Errorf("ledger not on glass") }
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapter/httpserver/ -run TestHeutePage_LedgerNoTimerForms -v`
Expected: FAIL (start/stop forms still present; no edit dialogs).

- [ ] **Step 3: Rewrite the Heute template.** In `heute.templ`:

Replace `HeuteFragment` (lines 36-51) with:

```go
templ HeuteFragment(vm HeuteVM) {
	if vm.Err != "" {
		<p class="mb-5 rounded-2xl bg-red/10 px-4 py-3 text-[.9rem] font-medium text-red" role="alert">{ vm.Err }</p>
	}
	<section class="grid grid-cols-1 sm:grid-cols-3 gap-4 md:gap-5">
		@heuteDaySummary(vm)
	</section>
	<section class="mt-5 md:mt-6 grid grid-cols-1 lg:grid-cols-3 gap-5 md:gap-6">
		@heuteLedgerCard(vm)
		@heuteWeekCard(vm)
	</section>
}
```

Add the day-summary (3 glass tiles reusing existing VM fields):

```go
// heuteDaySummary is the Tages-Summen tile row (replaces the removed hero stats).
templ heuteDaySummary(vm HeuteVM) {
	@components.StatTileAccent("heute.todayTotal", vm.LoggedDur, "", "")
	@components.StatTileAccent("heute.target", vm.TargetDur, "", "")
	@components.StatTileAccent("heute.balance", vm.Balance, "", heuteBalanceHue(vm.BalancePos))
}
```

Replace `heuteSessionsCard` (176-197) so it renders the glass ledger + the shared add dialog:

```go
// heuteLedgerCard lists today's sessions as clickable Kristall blocks + the
// Nachbuchen affordance; each block opens its edit SessionDialog, plus a delete
// ConfirmDialog for completed sessions.
templ heuteLedgerCard(vm HeuteVM) {
	@components.Card("lg:col-span-2 p-6", heuteLedgerBody(vm))
	@components.SessionDialog(components.SessionDialogVM{
		DialogID: "nachbuchen-dialog", Mode: "add",
		Action: "/ui/worktime/add", Target: "#content",
		Date: vm.DayParam, Nodes: heutePickerNodes(vm.Nodes),
	})
}

templ heuteLedgerBody(vm HeuteVM) {
	<div class="flex items-center justify-between mb-4">
		<h2 class="font-display text-lg font-semibold">{ components.T(ctx, "heute.sessions") }</h2>
		<span class="text-[.8rem] text-muted">{ components.Tn(ctx, "list.entries", len(vm.Ledger)) }</span>
	</div>
	if len(vm.Ledger) == 0 && vm.Running == nil {
		@components.EmptyState("○", "heute.emptyTitle", "heute.empty")
	} else {
		<ul class="space-y-2.5">
			for _, lr := range vm.Ledger {
				@heuteLedgerRow(vm, lr)
			}
		</ul>
	}
	<button type="button" data-dialog-open="nachbuchen-dialog"
		class="mt-4 w-full rounded-2xl border border-dashed border-line py-3 text-[.86rem] font-medium text-muted hover:border-blue/40 hover:text-blue transition-colors">
		<span aria-hidden="true">✚</span> { components.T(ctx, "heute.nachbuchen") }
	</button>
}

// heuteLedgerRow is one glass session block: the whole card opens the edit
// dialog (completed sessions); a running session is not editable. Delete lives
// in the mounted per-row ConfirmDialog.
templ heuteLedgerRow(vm HeuteVM, lr HeuteLedgerRow) {
	<li class="flex items-center gap-3">
		if lr.Edit.Mode == "edit" {
			<button type="button" data-dialog-open={ lr.Edit.DialogID }
				class="flex-1 min-w-0 text-left rounded-2xl glass shadow-soft px-3.5 py-3 hover:border-blue/40 transition-colors">
				@heuteRowInner(lr.Row)
			</button>
			@components.SessionDialog(lr.Edit)
		} else {
			<div class="flex-1 min-w-0 rounded-2xl glass shadow-soft px-3.5 py-3">
				@heuteRowInner(lr.Row)
			</div>
		}
		if !lr.Row.Running {
			<button type="button" aria-label={ components.T(ctx, "common.delete") }
				data-dialog-open={ "delete-" + lr.Row.ID }
				class="grid place-items-center h-8 w-8 rounded-lg text-faint hover:text-red hover:bg-red/10 transition-colors">
				<span aria-hidden="true">✗</span>
			</button>
			@components.ConfirmDialog(components.ConfirmSpec{
				ID: "delete-" + lr.Row.ID,
				ConfirmAttrs: templ.Attributes{
					"hx-post":   "/ui/worktime/delete",
					"hx-target": "#content",
					"hx-swap":   "innerHTML",
					"hx-vals":   heuteDeleteVals(vm.DayParam, lr.Row.ID),
					"type":      "button",
				},
			})
		}
	</li>
}

// heuteRowInner is the block's visual content (glyph tile, title+tags, range, duration).
templ heuteRowInner(row components.SessionRowVM) {
	<div class="flex items-center gap-3">
		<span class={ "grid place-items-center h-9 w-9 rounded-xl text-[.95rem] flex-none", heuteRowTile(row) } aria-hidden="true">{ row.Glyph }</span>
		<div class="min-w-0 flex-1">
			<p class={ "text-[.9rem] font-semibold truncate", templ.KV("text-muted", row.Unassigned) }>
				{ row.Title }
				for _, t := range row.Tags {
					<span class="ml-1">@components.Tag(t)</span>
				}
			</p>
			<p class="text-[.78rem] text-muted font-mono tnum">{ row.TimeRange }</p>
		</div>
		<span class={ "font-mono text-[.85rem] font-semibold tnum flex-none", templ.KV("text-muted", row.Unassigned) }>{ row.Duration }</span>
	</div>
}
```

Add the two small helpers in `webui_heute.go` (or a heute helpers file):

```go
// heutePickerNodes converts the NodePickerItem list to the domain.Node slice
// the SessionDialog picker expects (id + name only).
func heutePickerNodes(items []components.NodePickerItem) []domain.Node {
	out := make([]domain.Node, 0, len(items))
	for _, it := range items {
		out = append(out, domain.Node{ID: it.ID, Name: it.Name})
	}
	return out
}
```

Add `heuteRowTile` in the heute templ helpers (a whitelist hue→wash mirroring `rowTileClass`):

```go
// heuteRowTile picks the glyph-tile wash for a ledger block (whitelisted hues).
func heuteRowTile(row components.SessionRowVM) string {
	if row.Unassigned {
		return "bg-orange/10 text-orange"
	}
	switch row.Hue {
	case "blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal":
		return "bg-" + row.Hue + "/10 text-" + row.Hue
	default:
		return "bg-sunken text-body"
	}
}
```

Delete the now-dead templ funcs: `heuteHero` (53-120), `heuteStat` (122-128), `heuteStartCard` (130-142), `heuteSessionsCard` (old, replaced), `heuteSessionRow` (199-226), `heuteNachbuchenDialog` (228-231), `heuteAddForm` (233-270). Keep `heuteWeekCard`, `worktimeSubnav`, `HeutePage`, `heuteBody`, `heuteOuter`.

- [ ] **Step 4: Regenerate + run tests**

Run: `make generate && go test ./internal/adapter/httpserver/ -run 'TestHeute' -v`
Expected: PASS. Update/replace any old Heute render test that asserted the hero/start-card markup (those templates are gone) to assert the ledger instead.

- [ ] **Step 5: verify-no-popups + commit**

Run: `bash scripts/verify-no-popups.sh` (dialogs must be the in-design `<dialog>`, no `alert()`/`confirm()`). Expected: exit 0.

```bash
git add internal/adapter/webui/heute.templ internal/adapter/webui/*_templ.go internal/adapter/httpserver/webui_heute.go internal/adapter/httpserver/webui_heute*_test.go
git commit -m "feat(kristall): Heute is a glass ledger — SessionDialog add/edit, no timer forms"
```

---

## Task 5: Home template — remove timer block, glass-restyle tiles + lists

**Why:** Home becomes a pure dashboard. Remove the running-hero + idle start-card; the shell widget (always visible in the sidebar) owns the timer. Retire the hand-rolled `statsSaldoTile` in favour of glass `components.StatTileAccent`; put the newest-docs + logstream lists on glass.

**Files:**
- Modify: `internal/adapter/webui/home.templ`, `internal/adapter/webui/saldo.templ`
- Test: `internal/adapter/httpserver/webui_home_test.go`

- [ ] **Step 1: Write the failing test:**

```go
func TestHomePage_DashboardNoTimerForms(t *testing.T) {
	srv, u := newHomeTestServer(t)
	body := getBody(t, srv, u, "/")
	if strings.Contains(body, "/ui/home/start") || strings.Contains(body, "/ui/home/stop") {
		t.Errorf("Home must not render start/stop forms")
	}
	if !strings.Contains(body, "glass") { t.Errorf("saldo tiles not on glass") }
	// dashboard content still present
	for _, want := range []string{"home.greeting", "home.activity"} { // rendered German text; adapt to the harness's i18n
		_ = want
	}
}
```

> Assert on the rendered German strings the harness produces (as the existing `TestHomeHome_*` tests do), not the raw i18n keys. Keep the existing `TestHomeHome_ShowsSaldoTilesAndBurndown` / `TestHomeHome_RendersLanding` green.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapter/httpserver/ -run TestHomePage_DashboardNoTimerForms -v`
Expected: FAIL (start/stop forms present; tiles not glass).

- [ ] **Step 3: Edit the templates.**

In `home.templ` `HomeFragment` (29-63): delete the timer `<section>` (lines 37-43, the `if vm.Running … @homeHero … else … @homeStartCard`). Keep greeting, saldo tiles, burndown, logstream, newest docs.

Retire `statsSaldoTile`: change the saldo section (44-48) to call the glass component:

```go
	<section class="mt-6 grid grid-cols-1 sm:grid-cols-3 gap-4 md:gap-5">
		@components.StatTileAccent("stats.tileToday", vm.TodaySaldo, vm.TodaySub, statsSaldoHue(vm.TodayPos))
		@components.StatTileAccent("stats.tileWeek", vm.WeekSaldo, vm.WeekSub, statsSaldoHue(vm.WeekPos))
		@components.StatTileAccent("stats.tileMonth", vm.MonthSaldo, vm.MonthSub, statsSaldoHue(vm.MonthPos))
	</section>
```

Delete the `statsSaldoTile` templ from `saldo.templ` (only caller was `home.templ`). Keep `statsSaldoHue`. Verify no other caller: `rg -n "statsSaldoTile" internal --glob '!*_templ.go'` → must be empty after edit.

Glassify the newest-docs list (56) and logstream list (216): swap `rounded-2xl border border-line bg-surface shadow-soft` → `rounded-2xl glass shadow-soft` in both `<ul>`s.

Delete the dead templ funcs `homeHero` (70-134) and `homeStartCard` (139-150).

- [ ] **Step 4: Regenerate + run tests**

Run: `make generate && go test ./internal/adapter/httpserver/ -run 'TestHome' -v`
Expected: PASS. Update `TestHomeHome_ShowsSaldoTilesAndBurndown` only if it asserted the old `bg-surface` tile markup.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/webui/home.templ internal/adapter/webui/saldo.templ internal/adapter/webui/*_templ.go internal/adapter/httpserver/webui_home_test.go
git commit -m "feat(kristall): Home is a pure dashboard on glass — no timer block"
```

---

## Task 6: Remove the retired timer handlers, routes, and dead VM fields

**Why:** With Home/Heute no longer posting to them, `handleHomeStart/Stop`, `handleWebStart/Stop`, the `renderFragment` wrapper, their routes, and the hero-only VM fields are dead. Remove them so `staticcheck` stays green and the surface is honestly gone.

**Files:**
- Modify: `internal/adapter/httpserver/webui_home.go`, `internal/adapter/httpserver/webui.go`, `internal/adapter/httpserver/server.go`, `internal/adapter/webui/home_vm.go`, `internal/adapter/webui/heute_vm.go`, `internal/adapter/httpserver/webui_home.go` (homeDataFor pruning)
- Test: existing home/heute tests; a route-404 assertion.

- [ ] **Step 1: Write the failing test** (route is gone):

```go
func TestRetiredTimerRoutes_Return404(t *testing.T) {
	srv, u := newHomeTestServer(t)
	for _, path := range []string{"/ui/home/start", "/ui/home/stop", "/ui/worktime/start", "/ui/worktime/stop"} {
		rec := postForm(t, srv, u, path, url.Values{})
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", path, rec.Code)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapter/httpserver/ -run TestRetiredTimerRoutes_Return404 -v`
Expected: FAIL (routes still registered → 200).

- [ ] **Step 3: Delete handlers + routes.**
- `server.go`: delete lines registering `POST /ui/home/start` (197), `POST /ui/home/stop` (198), `POST /ui/worktime/start` (213), `POST /ui/worktime/stop` (214).
- `webui_home.go`: delete `handleHomeStart` (48-57) and `handleHomeStop` (59-86).
- `webui.go`: delete `handleWebStart` (15-22), `handleWebStop` (24-49), and `renderFragment` (11-13) **iff** no other caller remains (`rg -n "renderFragment\b" internal/adapter/httpserver` — after deleting the two handlers it should be empty; `handleWebAdd/Edit/Delete` use `renderDay`, not `renderFragment`). Remove now-unused imports (`errors`, `usecase`) as the compiler flags.

- [ ] **Step 4: Prune dead VM fields + homeDataFor.**
- `home_vm.go`: delete the "Timer-hero fields" block (Running, RunningBase, RunningName, RunningHue, RunningTag, StartedAt, LoggedDur, TargetDur, TargetPct, TargetVar, Balance, BalancePos) and the `Nodes`/`HasProj` picker fields — none are read after Task 5. Keep TodaySaldo…MonthSub, Burndown, NewestDocs, Log*.
- `webui_home.go` `homeDataFor`: delete the running-session resolution (116-131), the Nodes picker loop (138-155), and the `if running != nil { vm.RunningBase … }` block (157-164), and the `vm.LoggedDur/TargetDur/TargetPct/TargetVar/Balance/BalancePos` assignments (170-177) — keep the saldo-tile + burndown + docs + activity assignments. Remove now-unused locals/imports (`strings`, `time` may still be used by the week-saldo loop — keep if so).
- `heute_vm.go`: delete the hero-only fields `RunningBase, RunningName, RunningHue, RunningTag, StartedAt` (unused after Task 4). **Keep** `Running *domain.WorkSession` (the week card + empty-state use it), `Rows` (kept for compatibility/tests), `Nodes`, `HasProj`, `DayParam`, `LoggedDur/TargetDur/TargetPct/TargetVar/Balance/BalancePos` (day summary), `Week*`. In `webui_heute.go` remove the `if running != nil { vm.RunningBase … }` assignment block (115-122) since those fields are gone; keep resolving `running` (week rows need it).

- [ ] **Step 5: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./internal/adapter/httpserver/ ./internal/adapter/webui/... -v 2>&1 | tail -30`
Expected: no unused-symbol errors; PASS. Fix any test that referenced a removed field/handler.

- [ ] **Step 6: Regenerate + commit**

```bash
make generate
git add internal/adapter/httpserver/webui_home.go internal/adapter/httpserver/webui.go internal/adapter/httpserver/server.go internal/adapter/webui/home_vm.go internal/adapter/webui/heute_vm.go internal/adapter/webui/*_templ.go internal/adapter/httpserver/*_test.go
git commit -m "refactor(kristall): remove retired Home/Heute timer handlers, routes, dead VM fields"
```

---

## Task 7: Purge the now-dead worktime i18n keys (de + en)

**Why:** Removing the hero/start-card/nachbuchen-inline templates orphans a set of `heute.*` keys. Remove exactly the orphans from BOTH catalogs (parity), keeping keys still used by the ledger, day-summary, week card, and SessionDialog.

**Files:**
- Modify: `internal/i18n/catalog_de.go`, `internal/i18n/catalog_en.go`
- Test: `internal/i18n/catalog_test.go` (parity test — must stay green)

**Candidate orphans** (verify each before deleting): `heute.start`, `heute.stop`, `heute.bookProject`, `heute.orNewProject`, `heute.bookEngagement`, `heute.orNewEngagement`, `heute.newSession`, `heute.capture`, `heute.captureHint`, `heute.startedAt`, `heute.dayProgress`, `heute.elapsedAria`, `heute.running`, `heute.from`, `heute.to`, `heute.add`.
**Keep** (still referenced): `heute.target`, `heute.balance`, `heute.todayTotal` (day summary); `heute.sessions`, `heute.empty`, `heute.emptyTitle`, `heute.nachbuchen` (ledger); `heute.thisWeek`, `heute.met`, `heute.missed`, `heute.todayPace`, `heute.legendMet/Miss/Today` (week card); all `session.dialog.*`, all `timer.*`.

- [ ] **Step 1: Verify each candidate is orphaned.** For every candidate key run:

```bash
for k in heute.start heute.stop heute.bookProject heute.orNewProject heute.bookEngagement heute.orNewEngagement heute.newSession heute.capture heute.captureHint heute.startedAt heute.dayProgress heute.elapsedAria heute.running heute.from heute.to heute.add; do
  n=$(rg -n "\"$k\"" internal --glob '!internal/i18n/catalog_*.go' | wc -l | tr -d ' ')
  echo "$k -> $n remaining refs"
done
```

Expected: `0 remaining refs` for a truly-dead key. **Any key with >0 stays** (e.g. the cockpit rail timer may still use `heute.running`/`heute.stop` — if so, keep it). Adjust the delete list to only the confirmed-zero keys.

- [ ] **Step 2: Delete the confirmed-orphan keys** from `catalog_de.go` AND `catalog_en.go` (identical key set — the parity test enforces it).

- [ ] **Step 3: Run the catalog + build**

Run: `go test ./internal/i18n/ -v && go build ./...`
Expected: PASS (parity holds; no missing-key lookups at build — templ refs are compile-time strings, so a wrongly-deleted key surfaces as a still-present `T(ctx,"…")` call returning the key literal at runtime, which Step 1 prevents).

- [ ] **Step 4: Commit**

```bash
git add internal/i18n/catalog_de.go internal/i18n/catalog_en.go
git commit -m "chore(i18n): drop worktime keys orphaned by the timer-form removal"
```

---

## Task 8: Restyle the four shared non-glass components to Kristall glass

**Why:** `KennzahlenPanel`, `WeekTotalBanner`, `SelectionActionBar`, `BurndownBanner` still hand-roll `bg-surface border border-line shadow-soft|shadow-lift`. Glassing them centrally Kristall-ifies Woche (Kennzahlen + WeekTotal), Historie (SelectionBar), and Home/Stats (Burndown) in one place.

**Files:**
- Modify: `internal/adapter/webui/components/kennzahlen.templ:36`, `weektotal.templ:7`, `selectionbar.templ:11`, `burndownbanner.templ:9`
- Test: the components' render tests (or the page tests that assert on them).

- [ ] **Step 1: Write/extend a failing render test** for each — assert the glass class is present, e.g. in `components/primitives_test.go`:

```go
func TestSharedBanners_OnGlass(t *testing.T) {
	cases := []string{
		render(t, components.BurndownBanner(components.BurndownVM{})),
		render(t, components.WeekTotalBanner(components.WeekTotalVM{})),
		render(t, components.SelectionActionBar(components.SelectionBarVM{})),
		render(t, components.KennzahlenPanel(components.KennzahlenVM{})),
	}
	for i, out := range cases {
		if !strings.Contains(out, "glass") {
			t.Errorf("shared banner %d not on glass: %s", i, out)
		}
	}
}
```

> Fill the VM zero-values as each type requires (check the `*VM` structs; pass minimal valid values). If a component panics on a zero VM, pass a minimal populated one.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapter/webui/components/ -run TestSharedBanners_OnGlass -v`
Expected: FAIL.

- [ ] **Step 3: Swap the chrome.** In each file change the outer container class:
`bg-surface border border-line shadow-soft` → `glass shadow-soft`; `bg-surface border border-line shadow-lift` → `glass-strong shadow-lift`. Keep rounding/padding/layout classes untouched. (`glass`/`glass-strong` are existing tokens — `web/tailwind.css`.)

- [ ] **Step 4: Regenerate, test, verify-css**

Run: `make generate && go test ./internal/adapter/webui/... -v 2>&1 | tail -20 && bash scripts/verify-css.sh`
Expected: PASS; verify-css exit 0 (no new utilities — `glass`/`glass-strong` already compiled). If verify-css reports drift, run `make web` and add `internal/adapter/webui/static/app.css`.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/webui/components/kennzahlen.templ internal/adapter/webui/components/weektotal.templ internal/adapter/webui/components/selectionbar.templ internal/adapter/webui/components/burndownbanner.templ internal/adapter/webui/components/*_templ.go internal/adapter/webui/components/primitives_test.go
git commit -m "feat(kristall): shared banners (Kennzahlen/WeekTotal/SelectionBar/Burndown) on glass"
```

---

## Task 9: Woche — glass restyle

**Why:** Swap the hand-rolled day-list card + KW-nav pills for glass; the Kennzahlen/WeekTotal banners are already handled in Task 8.

**Files:** `internal/adapter/webui/woche.templ` (`wocheDaysCard:83`, `wocheNav:51,74`); test `internal/adapter/httpserver/webui_woche_test.go`.

- [ ] **Step 1: Extend the existing render test** `TestWocheHome_RendersDayBarsAndTotal` (or add a sibling) to assert `strings.Contains(body, "glass")` and that the KW `‹ ›` nav labels + day bars are still present (preserve `TestWocheFragment_KWNavClampsForward`).
- [ ] **Step 2: Run — expect FAIL** on the glass assertion. `go test ./internal/adapter/httpserver/ -run TestWoche -v`
- [ ] **Step 3: Restyle.** `wocheDaysCard` (83): swap the `<section>` chrome to `components.Card("...", wocheDaysBody(vm))` OR change `bg-surface border border-line shadow-soft` → `glass shadow-soft` in place. `wocheNav` pills (51, 74): `border border-line bg-surface shadow-soft` → `glass shadow-soft`. Preserve the KW stepper forward-clamp (`vm.CanForward`/`vm.NextWeek`), the "Diese Woche" jump, per-day bar variants, day-off chips, and the `#content` SSE trigger list (`woche.templ:22`) verbatim.
- [ ] **Step 4:** `make generate && go test ./internal/adapter/httpserver/ -run TestWoche -v` → PASS; `bash scripts/verify-css.sh` (make web + commit app.css only if drift).
- [ ] **Step 5: Commit** `git add internal/adapter/webui/woche.templ internal/adapter/webui/*_templ.go internal/adapter/httpserver/webui_woche_test.go && git commit -m "feat(kristall): Woche on glass"`

---

## Task 10: Historie — glass restyle (bulk-select + edit-dialog preserved)

**Why:** Biggest surface — 3 card wrappers + a toolbar pill group to glass, WITHOUT touching the bulk-select machinery or the edit-dialog swap semantics.

**Files:** `internal/adapter/webui/historie.templ` (`historieWeek:105`, `historieMonth:245`, list container `:349`, toolbar pill `:69`); tests `internal/adapter/httpserver/webui_historie_test.go` (10 tests must stay green).

- [ ] **Step 1: Add a glass assertion** to an existing calendar + list render test; add an explicit guard test that the bulk-select attrs survive:

```go
func TestHistorie_GlassAndBulkAttrsPreserved(t *testing.T) {
	srv, u := newHistorieTestServer(t)
	body := getBody(t, srv, u, "/historie")
	if !strings.Contains(body, "glass") { t.Errorf("historie not on glass") }
	for _, want := range []string{"data-select-toggle", "data-block-wrap", "/ui/worktime/edit"} {
		if !strings.Contains(body, want) { t.Errorf("historie lost %q", want) }
	}
}
```

- [ ] **Step 2: Run — expect FAIL** on glass. `go test ./internal/adapter/httpserver/ -run TestHistorie -v`
- [ ] **Step 3: Restyle** the four wrappers (`bg-surface border border-line shadow-soft` → `glass shadow-soft`). **Preserve verbatim:** the `data-select-toggle`/`data-day-select`/`data-block-wrap`/`data-edit-*` attrs feeding `historie-select.js`; `HistorieSelectionBarC`→`SelectionActionBar` (glassed in Task 8); the single-edit dialog `historieEditDialog`/`historieEditForm` posting `/ui/worktime/edit` + `/ui/worktime/delete` with **`hx-swap="none"`** (do NOT change swap semantics — it relies on the page SSE listener); both `SegToggle`s; `historieMonthBars`; the unassigned-count orange banner (`:89-98`).
  - **Contract check:** Task 2 changed `/ui/worktime/edit` to read the `node` field. `historieEditForm` posts the reassign node — confirm it uses `name="node"` after Task 2, or update it here so Historie edit still books correctly. If it currently posts `projectId`, change it to `node` (and add a render-test assertion). This is the one cross-task coupling — verify it.
- [ ] **Step 4:** `make generate && go test ./internal/adapter/httpserver/ -run TestHistorie -v` → all 10+ PASS; `bash scripts/verify-no-popups.sh` (edit/confirm dialogs) + `bash scripts/verify-css.sh`.
- [ ] **Step 5: Commit** `git add internal/adapter/webui/historie.templ internal/adapter/webui/*_templ.go internal/adapter/httpserver/webui_historie_test.go && git commit -m "feat(kristall): Historie on glass (bulk-select + edit intact)"`

---

## Task 11: Frei — glass restyle

**Files:** `internal/adapter/webui/frei.templ` (3 article cards `:49,86,135`); test `internal/adapter/httpserver/webui_dayoffs_test.go`.

- [ ] **Step 1:** Add glass assertion to `TestWebDayOffPageAndMutations` (or a sibling): `strings.Contains(body,"glass")`, and that the Bundesland select + ICS copy button + add-form remain.
- [ ] **Step 2:** Run — expect FAIL. `go test ./internal/adapter/httpserver/ -run TestWebDayOff -v`
- [ ] **Step 3:** Swap `freiAddCard`/`freiListCard`/`freiSettingsCard` (`rounded-3xl bg-surface border border-line shadow-soft p-6`) → `components.Card("p-6", …)` or in-place `rounded-3xl glass shadow-soft p-6`. **Preserve:** add-form posting `/ui/dayoffs/add`; per-row delete `ConfirmDialog` `frei-del-*` → `/ui/dayoffs/delete`; holiday rows dimmed (`opacity-60`); Bundesland `<select>` auto-submit (`hx-post="/ui/dayoffs/bundesland" hx-trigger="change"`, emits `EventSettingsChanged`); ICS token copy + regenerate confirm; SSE `dayoff.changed, settings.changed` trigger.
- [ ] **Step 4:** `make generate && go test ./internal/adapter/httpserver/ -run TestWebDayOff -v` → PASS; `bash scripts/verify-no-popups.sh` + `bash scripts/verify-css.sh`.
- [ ] **Step 5: Commit** `git add internal/adapter/webui/frei.templ internal/adapter/webui/*_templ.go internal/adapter/httpserver/webui_dayoffs_test.go && git commit -m "feat(kristall): Frei on glass"`

---

## Task 12: Export — glass table + `components.Button` (downloads preserved)

**Files:** `internal/adapter/webui/export.templ` (inner `<table>` `:97-130`, bare buttons `:66,76,82,88`, inputs `:57,64`); test `internal/adapter/httpserver/webui_export_test.go`.

- [ ] **Step 1:** Extend `TestWebExportHome`/`TestWebExportPreview` to assert the 3 download anchors STILL carry `hx-boost="false"` and the table is present, plus a glass/`components.Button` marker.
- [ ] **Step 2:** Run — expect FAIL (buttons not yet `components.Button`). `go test ./internal/adapter/httpserver/ -run TestWebExport -v`
- [ ] **Step 3:** Replace the bare `rounded bg-ink px-3 py-1 text-canvas` format/submit buttons with `@components.Button(components.BtnPrimary, …, "", templ.Attributes{...})` (keep the CSV/JSON/MD download anchors as `<a hx-boost="false">` — Button is for the non-download actions; the download anchors keep their `hx-boost="false"` verbatim). Give `exportSummaryTable`'s wrapping `<section class="rounded bg-sunken p-3 …">` a glass treatment (`rounded-2xl glass shadow-soft p-4`) and give the `<thead>`/`<tfoot>` `border-line` separators. Keep the `#ep` preview target id and the per-project Σ aggregation untouched.
- [ ] **Step 4:** `make generate && go test ./internal/adapter/httpserver/ -run TestWebExport -v` → PASS; `bash scripts/verify-css.sh` (likely a new utility combo on the table → run `make web` + commit `app.css`).
- [ ] **Step 5: Commit** `git add internal/adapter/webui/export.templ internal/adapter/webui/*_templ.go internal/adapter/webui/static/app.css internal/adapter/httpserver/webui_export_test.go && git commit -m "feat(kristall): Export table + buttons on glass (downloads intact)"`

---

## Task 13: Main-wiring / route audit + full gate + live dogfood checklist

**Why:** Standing rule (`feedback_plan_main_wiring_task`): a slice ends with an explicit audit that nothing dangles, the whole `make ci` is green, and the change is exercised end-to-end against the dev stack. No new code unless the audit finds a gap.

- [ ] **Step 1: Dangling-reference audit.** All must be empty:

```bash
rg -n "handleHomeStart|handleHomeStop|handleWebStart|handleWebStop|renderFragment\b" internal/adapter/httpserver --glob '!*_test.go'
rg -n "/ui/home/start|/ui/home/stop|/ui/worktime/start|/ui/worktime/stop" internal --glob '!*_test.go'
rg -n "statsSaldoTile|heuteHero|heuteStartCard|heuteAddForm|heuteNachbuchenDialog|homeHero|homeStartCard" internal --glob '!*_templ.go'
```

Expected: no hits. Confirm the retained routes still resolve: `rg -n "/ui/worktime/(add|edit|delete)|/ui/timer|/ui/home\b|/zeit|/ui/worktime\b" internal/adapter/httpserver/server.go`.

- [ ] **Step 2: SSE double-reload verification.** Confirm the widget still self-updates when a session mutates from a page and vice-versa: `rg -n "sse:session" internal/adapter/webui/components/appshell.templ internal/adapter/webui/home.templ internal/adapter/webui/heute.templ` — the `#timer-widget` + `#timer-chip` mounts AND both `#content` containers must carry the `sse:session.*` triggers (they do today; assert unchanged). No code change expected — this is a guard.

- [ ] **Step 3: Full CI gate.**

Run: `make ci`
Expected: exit 0, coverage ≥ 75 %. If `verify-css` fails → `make web && git add internal/adapter/webui/static/app.css`. If coverage dipped below 75 %, add the missing output-asserting test for the surface that dropped.

- [ ] **Step 4: Live dogfood checklist (dev stack).** `make dev-up && make dev-run` (server on `https://localhost:8080`), scripted Dex login. Verify:
  - **Timer widget owns start/stop** — from the sidebar widget: start on a node → Home AND Heute update live (no page timer form anywhere); stop → both update; unbound start → stop demands a node in the widget.
  - **Heute ledger** — sessions render as glass blocks; click a block → SessionDialog opens prefilled (times/node/tags/note); save → row updates live; "Nachbuchen" → add dialog → new session appears; delete → confirm → gone.
  - **Home dashboard** — no timer block; saldo tiles + burndown + activity + newest-docs all on glass.
  - **Sweep pages** — Woche/Historie/Frei/Export all on glass; Historie bulk-select still works (select → reassign/delete); Export CSV/JSON/MD downloads still download (not an htmx swap); Frei Bundesland change still re-tints Woche day-off chips.
  - Clean up any seeded test sessions/day-offs.

- [ ] **Step 5: Update the ledger + whole-branch review.** Append K3 status to `.superpowers/sdd/progress.md` (commits, `make ci` %, live-gate result). Generate the review package `git diff <K3-base>..HEAD > .superpowers/sdd/review-<base>..<head>.diff` and dispatch the **opus** whole-branch reviewer (focus: the `node`-field contract change across Heute+Historie edit, the handler/route/VM removals, i18n orphan correctness, SSE double-reload, glass-only restyle with no behavior drift). Then Soenne dogfood. No commit for this step beyond the ledger update.

---

## Self-Review (completed against umbrella §3 + §7)

- **§7 Home "ohne Timer-Block", Glas-Karten** → Tasks 5 (+6 removal, +8 burndown glass). ✓
- **§7 Heute Ledger, SessionDialog Add/Edit, Start/Stop-Form+Handler entfernt, tote i18n raus** → Tasks 1–4, 6, 7. ✓ (Blöcke/Cards decision → glass ledger cards, Task 4.)
- **§7 Woche/Historie/Stats/Frei/Export Kristall-Feinschliff** → Tasks 8 (Kennzahlen/WeekTotal/SelectionBar/Burndown = Woche+Historie+Stats shared), 9 (Woche), 10 (Historie), 11 (Frei), 12 (Export). "Stats" = Home saldo tiles (Task 5) + BurndownBanner (Task 8) — no separate page exists. ✓
- **§3 one mechanism** → SessionDialog is the sole add/edit path (Task 4); timer widget the sole start/stop (Task 6 removes the duplicates); `node`-field contract unified (Task 2). ✓
- **Main-wiring task** → Task 13. ✓
- **Placeholder scan** → none (real code/commands in every step). **Type consistency** → `SessionDialogVM.SessionID` (T1) used by T3/T4; `HeuteLedgerRow` (T3) used by T4; `webNode` (T2) used by add/edit; `heuteEditDialogVM` action `/ui/worktime/edit` matches T2's handler. ✓
- **Cross-task coupling flagged** → Historie's edit form must post `node` after Task 2 (called out in Task 10 Step 3).
