# flow M1 Slice 6 — Cockpit (Node-Detail mit Tabs) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the read-only single-scroll node page `/nodes/{id}` into the **Cockpit**: a persistent glass head (per-node timer + subtree rollup) plus four htmx-swapped tabs (Worktime · Wissen · Struktur · Bindings).

**Architecture:** Server-rendered templ + htmx + SSE, hexagonal (`internal/domain` / `usecase` / `ports` / `adapter`). The head (`#cockpit-head`) and the tab area (`#cockpit-main`) are separate htmx containers so a live reload of one never resets the other. Timer mutations re-render the head and `Emit` an SSE event that the active panel reacts to. No new usecases, no migration — only thin WebUI handlers and view models on top of existing backend reads.

**Tech Stack:** Go 1.x, `a-h/templ` (`go tool templ generate`), Tailwind v4 (`make web` → `app.css`), htmx + SSE, Postgres (pgstore) + Dex (dev OIDC).

## Global Constraints

- Branch: `m1-slice6-cockpit` (off `rebuild`; merged back to `rebuild` when the slice is reviewed-complete, like m1-webui). Spec: `docs/superpowers/specs/2026-06-30-flow-m1-slice6-cockpit-design.md`.
- Events are published via **`s.Emitter.Emit(ctx, domain.Event{Type: ..., UserID: u.ID, Data: ...})`** — NOT `s.Bus.Publish`. Mirror `handleHomeStart`/`handleHomeStop` exactly.
- The running session is resolved via **`s.GetRunningSession.Execute(ctx, ownerID) (domain.WorkSession, bool, error)`** (cross-day correct), with a fallback scan of a session range only where the harness lacks it.
- Per-node rollup uses **`s.Stats.NodeStats(ctx, ownerID, nodeID) (domain.NodeRollup{Total,Week,Month}, error)`** (subtree). `StatsComputer` MUST be wired with `Nodes:` (the NodeStore) or `NodeStats` cannot walk the subtree.
- Inherited rate uses **`domain.ResolveRate(chain []domain.Node) *domain.Money`** over `[node]+ancestors` (leaf→root).
- After **every** `.templ` change: `make generate` (`go tool templ generate`) and commit the regenerated `*_templ.go`. `make ci` runs `verify-generate` and will fail otherwise.
- After adding **any** new Tailwind utility class used in templ: `make web` (rebuild `internal/adapter/webui/static/app.css`) and commit it. `make ci` runs `verify-css` and will fail otherwise.
- **No browser popups**: `make ci` runs `verify-no-popups` (greps for `window.alert/confirm/prompt`). The "Wechseln" confirm MUST be an inline DOM confirm (a two-step button / a revealed confirm row), never `window.confirm`.
- `make ci` (gate: **75%** line coverage, `-coverpkg=./internal/...`) green per task. Commit frequently. **Generated `*_templ.go` files are EXCLUDED from the gate** (`coverage-gate.sh` filters them) — so write REAL output-asserting handler/render tests (assert the produced HTML) to cover new code; NEVER add no-assertion render calls just to chase templ-generated lines (that padding was removed in Task 5).
- i18n parity: every new `cockpit.*` key exists in BOTH `internal/i18n/catalog_de.go` and `internal/i18n/catalog_en.go`.
- Copy stays terse/lowercase to match existing fragments. No emoji pictograms — monospace glyphs (▶ ■ ◆ ⬡ ›) only.

---

## File Structure

**Create:**
- `internal/adapter/webui/cockpit.templ` — cockpit head + tabstrip + per-tab panels (replaces the cockpit half of `nodes.templ`).
- `internal/adapter/webui/cockpit_vm.go` — `NodeCockpit` (moved/expanded), `CockpitTimer` + `NodeTimer(...)` pure state function, `NodeChild`, small formatters.
- `internal/adapter/httpserver/webui_cockpit.go` — `nodeCockpitData`, `handleWebNodeView` (page), `handleWebNodeHead`, `handleWebNodeTab`, `handleWebNodeStart/Stop/Switch`, `handleWebNodeAddSession`, `handleWebNodeBindRemote`, `handleWebNodeUnbind`.
- `internal/adapter/httpserver/webui_cockpit_test.go` — `newCockpitTestServer` harness + all handler tests.
- `internal/adapter/webui/cockpit_vm_test.go` — pure VM unit tests (`NodeTimer`, rollup/earnings, active-tab selection).

**Modify:**
- `internal/adapter/webui/nodes.templ` — DELETE the cockpit half (`NodeView`, `nodeViewBody`, `nodeViewOuter`, `nodeBreadcrumb`, `nodeCockpitBody`, `nodeMoveForm`); the list/form templ stays.
- `internal/adapter/webui/node_tree_vm.go` — REMOVE the old `NodeCockpit` struct (moves to `cockpit_vm.go`).
- `internal/adapter/httpserver/webui_nodes.go` — REMOVE `nodeWorktime`, `nodeCockpitData`, `handleWebNodeView` (move to `webui_cockpit.go`).
- `internal/adapter/httpserver/server.go` — register the new cockpit routes.
- `internal/adapter/httpserver/webui_editor.go` — `handleWebEditorNew` reads `?node=` to pre-scope a new doc (Task 6).
- `internal/adapter/httpserver/webui_nodes.go` — `handleWebNodeNew` reads `?parent=` / `?kind=` to pre-fill the child form (Task 7).
- `internal/i18n/catalog_de.go`, `internal/i18n/catalog_en.go` — `cockpit.*` keys (Task 1).

---

## Task 1: i18n `cockpit.*` keys (de + en parity)

**Files:**
- Modify: `internal/i18n/catalog_de.go`, `internal/i18n/catalog_en.go`
- Test: `internal/i18n/catalog_test.go` (a parity test almost certainly exists; if so this task makes it pass)

**Interfaces:**
- Produces: the `cockpit.*` key set consumed by all later templ tasks.

- [ ] **Step 1: Run the existing parity test to see it currently passes (baseline)**

Run: `go test ./internal/i18n/ -run Parity -v` (if no parity test exists, skip to Step 2; add keys and rely on templ compile + later handler tests).
Expected: PASS (baseline before we add keys).

- [ ] **Step 2: Add the keys to `catalog_de.go`** (next to the existing `// node cockpit` block, `catalog_de.go:85`)

```go
			// cockpit (Slice 6)
			"cockpit.tab.worktime":     "Worktime",
			"cockpit.tab.wissen":       "Wissen",
			"cockpit.tab.struktur":     "Struktur",
			"cockpit.tab.bindings":     "Bindings",
			"cockpit.timer.start":      "Start",
			"cockpit.timer.stop":       "Stopp",
			"cockpit.timer.switch":     "Wechseln",
			"cockpit.timer.switchHint": "Timer läuft woanders — hierher wechseln?",
			"cockpit.timer.runningOn":  "läuft auf",
			"cockpit.timer.unbound":    "Timer läuft ohne Projekt",
			"cockpit.timer.notBookable": "nicht buchbar",
			"cockpit.timer.startedAt":  "seit",
			"cockpit.rollup.total":     "Σ Gesamt",
			"cockpit.rollup.week":      "Woche",
			"cockpit.rollup.month":     "Monat",
			"cockpit.rollup.earnings":  "Erlös",
			"cockpit.rollup.inclChildren": "inkl. Unterknoten",
			"cockpit.rollup.rateInherited": "geerbt",
			"cockpit.worktime.title":   "Sessions",
			"cockpit.worktime.add":     "Nachbuchen",
			"cockpit.worktime.empty":   "Noch keine Buchungen auf diesem Knoten.",
			"cockpit.worktime.ownOnly": "eigene Buchungen dieses Knotens",
			"cockpit.worktime.running": "läuft",
			"cockpit.wissen.title":     "Dokumente",
			"cockpit.wissen.add":       "Neu",
			"cockpit.wissen.empty":     "Noch keine Dokumente zu diesem Knoten.",
			"cockpit.struktur.title":   "Unterknoten",
			"cockpit.struktur.add":     "Unterknoten",
			"cockpit.struktur.empty":   "Keine Unterknoten.",
			"cockpit.struktur.move":    "Verschieben",
			"cockpit.struktur.status":  "Status",
			"cockpit.bindings.title":   "Bindings",
			"cockpit.bindings.addRemote": "Git-Remote binden",
			"cockpit.bindings.remotePlaceholder": "github.com/owner/repo",
			"cockpit.bindings.pathHint": "Pfad-Bindings werden pro Maschine über die CLI (flow start) angelegt.",
			"cockpit.bindings.delete":  "Lösen",
			"cockpit.bindings.empty":   "Keine Bindings.",
			"cockpit.bindings.remoteOnlyRepo": "Remote-Bindings nur an Repo-Knoten möglich.",
```

- [ ] **Step 3: Add the SAME keys to `catalog_en.go`** (next to `catalog_en.go:78`)

```go
			// cockpit (Slice 6)
			"cockpit.tab.worktime":     "Worktime",
			"cockpit.tab.wissen":       "Knowledge",
			"cockpit.tab.struktur":     "Structure",
			"cockpit.tab.bindings":     "Bindings",
			"cockpit.timer.start":      "Start",
			"cockpit.timer.stop":       "Stop",
			"cockpit.timer.switch":     "Switch here",
			"cockpit.timer.switchHint": "A timer is running elsewhere — switch it here?",
			"cockpit.timer.runningOn":  "running on",
			"cockpit.timer.unbound":    "A timer is running without a project",
			"cockpit.timer.notBookable": "not bookable",
			"cockpit.timer.startedAt":  "since",
			"cockpit.rollup.total":     "Σ Total",
			"cockpit.rollup.week":      "Week",
			"cockpit.rollup.month":     "Month",
			"cockpit.rollup.earnings":  "Earnings",
			"cockpit.rollup.inclChildren": "incl. sub-nodes",
			"cockpit.rollup.rateInherited": "inherited",
			"cockpit.worktime.title":   "Sessions",
			"cockpit.worktime.add":     "Add session",
			"cockpit.worktime.empty":   "No sessions booked on this node yet.",
			"cockpit.worktime.ownOnly": "own bookings on this node",
			"cockpit.worktime.running": "running",
			"cockpit.wissen.title":     "Documents",
			"cockpit.wissen.add":       "New",
			"cockpit.wissen.empty":     "No documents for this node yet.",
			"cockpit.struktur.title":   "Sub-nodes",
			"cockpit.struktur.add":     "Sub-node",
			"cockpit.struktur.empty":   "No sub-nodes.",
			"cockpit.struktur.move":    "Move",
			"cockpit.struktur.status":  "Status",
			"cockpit.bindings.title":   "Bindings",
			"cockpit.bindings.addRemote": "Bind git remote",
			"cockpit.bindings.remotePlaceholder": "github.com/owner/repo",
			"cockpit.bindings.pathHint": "Path bindings are created per machine via the CLI (flow start).",
			"cockpit.bindings.delete":  "Unbind",
			"cockpit.bindings.empty":   "No bindings.",
			"cockpit.bindings.remoteOnlyRepo": "Remote bindings can only attach to repo nodes.",
```

- [ ] **Step 4: Verify parity + build**

Run: `go build ./... && go test ./internal/i18n/ -v`
Expected: PASS (parity test green if present; build green).

- [ ] **Step 5: Commit**

```bash
git add internal/i18n/catalog_de.go internal/i18n/catalog_en.go
git commit -m "i18n(cockpit): add cockpit.* keys (de+en parity)"
```

---

## Task 2: Cockpit head + rollup correction (no timer buttons yet)

Rebuild the cockpit as a head + tab shell, switch the rollup to the **subtree** `NodeStats` + **inherited** `ResolveRate`, and delete the stale `nodeWorktime`. The timer is rendered read-only (state computed, buttons added in Task 3). This is the structural cut-over.

**Files:**
- Create: `internal/adapter/webui/cockpit_vm.go`
- Create: `internal/adapter/webui/cockpit.templ`
- Create: `internal/adapter/httpserver/webui_cockpit.go`
- Create: `internal/adapter/httpserver/webui_cockpit_test.go`
- Create: `internal/adapter/webui/cockpit_vm_test.go`
- Modify: `internal/adapter/webui/nodes.templ` (delete cockpit half), `internal/adapter/webui/node_tree_vm.go` (delete old `NodeCockpit`), `internal/adapter/httpserver/webui_nodes.go` (delete `nodeWorktime`, `nodeCockpitData`, `handleWebNodeView`), `internal/adapter/httpserver/server.go` (route stays `GET /nodes/{id}` → `handleWebNodeView` now in the new file).

**Interfaces:**
- Produces:
  - `webui.NodeCockpit` struct (head fields + active-tab data; see Step 3).
  - `webui.CockpitTimer{State CockpitTimerState; RunningID string; RunningBase int; OtherID, OtherName string}` and `webui.NodeTimer(running *domain.WorkSession, nodeID string, bookable bool, now time.Time, nameOf func(id string) string) CockpitTimer`.
  - `webui.NodeView(d NodeCockpit) templ.Component`, `webui.NodeHead(d NodeCockpit) templ.Component`.
  - httpserver `nodeCockpitData(r, u, id, activeTab string) (webui.NodeCockpit, error)`.
- Consumes: `domain.NodeRollup`, `domain.ResolveRate`, `s.Stats.NodeStats`, `s.NodeAncestors`, `s.GetNode`, `domain.IsBookable`.

- [ ] **Step 1: Write the failing VM unit test** — `internal/adapter/webui/cockpit_vm_test.go`

```go
package webui_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func nodeWithKind(id string, k domain.NodeKind) domain.Node {
	return domain.Node{ID: id, Kind: k, Status: domain.NodeActive}
}

func TestNodeTimer_States(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	nameOf := func(id string) string { return map[string]string{"y": "Vorhaben Y"}[id] }

	// idle (bookable, nothing running)
	if got := webui.NodeTimer(nil, "x", true, now, nameOf); got.State != webui.TimerIdle {
		t.Errorf("idle: state=%v", got.State)
	}
	// not bookable (branch)
	if got := webui.NodeTimer(nil, "x", false, now, nameOf); got.State != webui.TimerNotBookable {
		t.Errorf("notBookable: state=%v", got.State)
	}
	// running on THIS node
	yid := "x"
	run := domain.WorkSession{ID: "s1", NodeID: &yid, Start: now.Add(-90 * time.Second)}
	if got := webui.NodeTimer(&run, "x", true, now, nameOf); got.State != webui.TimerHere || got.RunningID != "s1" || got.RunningBase != 90 {
		t.Errorf("here: %+v", got)
	}
	// running on ANOTHER node (bound)
	other := "y"
	run2 := domain.WorkSession{ID: "s2", NodeID: &other, Start: now}
	g := webui.NodeTimer(&run2, "x", true, now, nameOf)
	if g.State != webui.TimerOtherBound || g.OtherID != "y" || g.OtherName != "Vorhaben Y" {
		t.Errorf("otherBound: %+v", g)
	}
	// running unbound (started from Home, no node)
	run3 := domain.WorkSession{ID: "s3", NodeID: nil, Start: now}
	if got := webui.NodeTimer(&run3, "x", true, now, nameOf); got.State != webui.TimerUnbound {
		t.Errorf("unbound: state=%v", got.State)
	}
}
```

- [ ] **Step 2: Run it — fails to compile** (`webui.NodeTimer` undefined)

Run: `go test ./internal/adapter/webui/ -run TestNodeTimer -v`
Expected: FAIL (undefined: NodeTimer / TimerIdle…).

- [ ] **Step 3: Write `internal/adapter/webui/cockpit_vm.go`**

```go
package webui

import (
	"html/template"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// CockpitTimerState is the per-node timer state shown in the cockpit head.
type CockpitTimerState int

const (
	TimerNotBookable CockpitTimerState = iota // branch (or non-bookable kind)
	TimerIdle                                 // bookable, nothing running → [Start]
	TimerHere                                 // running on THIS node → [Stop] + live clock
	TimerOtherBound                           // running on another node → [Switch]
	TimerUnbound                              // running but unbooked (no node) → link Home, no switch
)

// CockpitTimer is the resolved timer view state for the head.
type CockpitTimer struct {
	State       CockpitTimerState
	RunningID   string // running session id (hidden field for stop/switch)
	RunningBase int    // elapsed seconds at render (data-base for the live clock)
	OtherID     string // node id the timer runs on (OtherBound) — link target
	OtherName   string // node name the timer runs on (OtherBound)
}

// NodeTimer computes the timer state for nodeID given the owner's running
// session (nil = none). Pure: unit-tested in cockpit_vm_test.go.
func NodeTimer(running *domain.WorkSession, nodeID string, bookable bool, now time.Time, nameOf func(id string) string) CockpitTimer {
	if !bookable {
		return CockpitTimer{State: TimerNotBookable}
	}
	if running == nil {
		return CockpitTimer{State: TimerIdle}
	}
	base := int(running.Elapsed(now).Seconds())
	if running.NodeID == nil {
		return CockpitTimer{State: TimerUnbound, RunningID: running.ID, RunningBase: base}
	}
	if *running.NodeID == nodeID {
		return CockpitTimer{State: TimerHere, RunningID: running.ID, RunningBase: base}
	}
	return CockpitTimer{
		State: TimerOtherBound, RunningID: running.ID, RunningBase: base,
		OtherID: *running.NodeID, OtherName: nameOf(*running.NodeID),
	}
}

// NodeChild is one direct child row in the Struktur tab.
type NodeChild struct {
	N     domain.Node
	Total string // mini worktime label (e.g. "12:30 h"), "" if zero
}

// NodeCockpit drives the cockpit page, head fragment, and the active tab panel.
type NodeCockpit struct {
	User            string
	N               domain.Node
	Ancestors       []domain.Node // leaf→root (NodeStore.Ancestors order)
	DescriptionHTML template.HTML
	// head: subtree rollup + inherited rate
	Rollup   domain.NodeRollup // Total, Week, Month durations
	Earnings string            // ResolveRate(chain) × Total, "" if no rate in chain
	Rate     string            // inherited rate label, "" if none
	Timer    CockpitTimer
	// active tab + its data (only the active tab's slice is populated)
	ActiveTab string             // worktime|wissen|struktur|bindings
	Sessions  []domain.WorkSession // worktime: own sessions, newest first
	Docs      []domain.Document    // wissen
	Children  []NodeChild          // struktur
	MoveTargets []domain.Node      // struktur reparent
	Bindings  []domain.ProjectBinding // bindings
	BindErr   string                  // inline binding error
}

// CockpitTabs is the fixed tab order/keys for the strip.
var CockpitTabs = []struct{ Key, LabelKey string }{
	{"worktime", "cockpit.tab.worktime"},
	{"wissen", "cockpit.tab.wissen"},
	{"struktur", "cockpit.tab.struktur"},
	{"bindings", "cockpit.tab.bindings"},
}

// NormalizeTab returns a valid tab key, defaulting to "worktime".
func NormalizeTab(tab string) string {
	for _, t := range CockpitTabs {
		if t.Key == tab {
			return tab
		}
	}
	return "worktime"
}
```

- [ ] **Step 4: Run the VM test — passes**

Run: `go test ./internal/adapter/webui/ -run TestNodeTimer -v`
Expected: PASS.

- [ ] **Step 5: Add the active-tab + rollup VM tests**

Append to `cockpit_vm_test.go`:

```go
func TestNormalizeTab(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", "worktime"}, {"bogus", "worktime"}, {"wissen", "wissen"},
		{"struktur", "struktur"}, {"bindings", "bindings"},
	} {
		if got := webui.NormalizeTab(c.in); got != c.want {
			t.Errorf("NormalizeTab(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
```

Run: `go test ./internal/adapter/webui/ -run "TestNormalizeTab|TestNodeTimer" -v` → PASS.

- [ ] **Step 6: Delete the old cockpit half**

In `internal/adapter/webui/node_tree_vm.go`: delete the `NodeCockpit` struct (lines around `:129-142`) — it now lives in `cockpit_vm.go`. Keep `NodeMoveData`, `moveTargetsFor`, `MoveTargetsFor`.

In `internal/adapter/webui/nodes.templ`: delete `NodeView`, `nodeViewBody`, `nodeViewOuter`, `nodeBreadcrumb`, `nodeCockpitBody`, `nodeMoveForm` (`:284-395`). Keep everything above (`NodesPage`, list, form, badges).

In `internal/adapter/httpserver/webui_nodes.go`: delete `nodeWorktime` (`:372-405`), `nodeCockpitData` (`:407-432`), `handleWebNodeView` (`:435-448`). (They move to `webui_cockpit.go`.)

- [ ] **Step 7: Write `internal/adapter/webui/cockpit.templ`** (head only for now; tabs panels are stubs filled in later tasks)

```go
package webui

import (
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// NodeView is the full cockpit page.
templ NodeView(d NodeCockpit) {
	@components.Base("projekte", nodeCockpitShell(d))
}

templ nodeCockpitShell(d NodeCockpit) {
	@components.AppShell("projekte", nodeBreadcrumb(d), nil, cockpitBody(d))
}

templ nodeBreadcrumb(d NodeCockpit) {
	@components.Breadcrumb(nodeCrumbs(d))
}

templ cockpitBody(d NodeCockpit) {
	// head reloads on timer/struct events; never resets the active tab.
	<div
		id="cockpit-head"
		hx-get={ "/nodes/" + d.N.ID + "/head" }
		hx-trigger="sse:session.started, sse:session.stopped, sse:session.updated, sse:node.updated, sse:node.moved"
		hx-swap="innerHTML"
	>
		@NodeHead(d)
	</div>
	// main = tabstrip + panel; tab clicks swap this whole block (keeps active state).
	<div id="cockpit-main" class="mt-6">
		@cockpitTabsAndPanel(d)
	</div>
}

// NodeHead is the persistent glass head: identity, rate, rollup, timer.
templ NodeHead(d NodeCockpit) {
	<section class="relative overflow-hidden rounded-3xl bg-surface border border-line shadow-lift">
		<span class={ "absolute inset-y-0 left-0 w-1", cockpitAccent(d.N.Color) } aria-hidden="true"></span>
		<div class="relative p-6 sm:p-8">
			<div class="flex items-start justify-between gap-5 flex-wrap">
				<div class="flex items-center gap-4 min-w-0">
					@cockpitHex(d.N)
					<div class="min-w-0">
						<h1 class="font-display text-2xl sm:text-3xl font-semibold leading-tight truncate">{ d.N.Name }</h1>
						<div class="mt-2 flex items-center gap-2 flex-wrap text-sm">
							@nodeKindBadge(d.N.Kind)
							@nodeStatusBadge(d.N.Status)
							if d.Rate != "" {
								<span class="text-muted">· { d.Rate } <span class="text-faint">({ components.T(ctx, "cockpit.rollup.rateInherited") })</span></span>
							}
						</div>
					</div>
				</div>
				@cockpitTimer(d)
			</div>
			@cockpitRollup(d)
		</div>
	</section>
}

// cockpitTimer is replaced with full Start/Stop/Switch markup in Task 3.
// For Task 2 it renders the state read-only (no buttons).
templ cockpitTimer(d NodeCockpit) {
	switch d.Timer.State {
		case TimerHere:
			<div class="rounded-2xl bg-cyan/[.08] border border-cyan/20 px-4 py-3">
				<div class="font-mono tnum text-2xl text-ink" data-timer data-base={ secStr(d.Timer.RunningBase) }>{ fmtSecsClock(d.Timer.RunningBase) }</div>
			</div>
		case TimerOtherBound:
			<a href={ templ.SafeURL("/nodes/" + d.Timer.OtherID) } class="text-sm text-muted hover:text-ink">{ components.T(ctx, "cockpit.timer.runningOn") } { d.Timer.OtherName } →</a>
		case TimerNotBookable:
			<span class="text-sm text-faint">{ components.T(ctx, "cockpit.timer.notBookable") }</span>
		default:
	}
}

templ cockpitRollup(d NodeCockpit) {
	<div class="mt-6 flex gap-3 flex-wrap">
		@cockpitTile("cockpit.rollup.total", fmtDurHM(d.Rollup.Total), components.T(ctx, "cockpit.rollup.inclChildren"), false)
		@cockpitTile("cockpit.rollup.week", fmtDurHM(d.Rollup.Week), "", false)
		@cockpitTile("cockpit.rollup.month", fmtDurHM(d.Rollup.Month), "", false)
		if d.Earnings != "" {
			@cockpitTile("cockpit.rollup.earnings", d.Earnings, "", true)
		}
	</div>
}

templ cockpitTile(labelKey, value, sub string, earn bool) {
	<div class={ "flex-1 min-w-[8rem] rounded-2xl border px-4 py-3", templ.KV("bg-sunken/40 border-line", !earn), templ.KV("bg-green/[.10] border-green/25", earn) }>
		<div class="eyebrow uppercase text-[.66rem] tracking-wide text-faint">{ components.T(ctx, labelKey) }</div>
		<div class={ "mt-1 text-xl font-semibold tnum", templ.KV("text-ink", !earn), templ.KV("text-green", earn) }>{ value }</div>
		if sub != "" {
			<div class="mt-0.5 text-[.66rem] text-muted">{ sub }</div>
		}
	</div>
}

// cockpitTabsAndPanel is the swappable tab area. Panels are stubbed here and
// filled by Tasks 5–8; Task 4 wires the real tab navigation + SSE.
templ cockpitTabsAndPanel(d NodeCockpit) {
	<nav class="flex gap-1 border-b border-line2">
		for _, t := range CockpitTabs {
			@cockpitTabLink(d.N.ID, t.Key, t.LabelKey, d.ActiveTab == t.Key)
		}
	</nav>
	<div id="cockpit-panel" class="pt-6">
		@cockpitPanel(d)
	</div>
}

templ cockpitTabLink(id, key, labelKey string, active bool) {
	<a
		class={ "px-4 py-2.5 text-sm font-medium border-b-2 -mb-px",
			templ.KV("text-cyan border-cyan", active),
			templ.KV("text-muted border-transparent hover:text-body", !active) }
	>{ components.T(ctx, labelKey) }</a>
}

// cockpitPanel switches on the active tab. Stubs until Tasks 5–8.
templ cockpitPanel(d NodeCockpit) {
	switch d.ActiveTab {
		default:
			<p class="text-sm text-faint">{ components.T(ctx, "node.none") }</p>
	}
}
```

> NOTE: `cockpitAccent`, `cockpitHex`, `secStr`, `webui_fmtSecs`, `fmtDurHM`, `fmtDurHM`, `nodeCrumbs`, `nodeKindBadge`, `nodeStatusBadge` — `nodeCrumbs`/`nodeKindBadge`/`nodeStatusBadge` already exist in `nodes.templ`/helpers; `secStr` exists (used by home). Add the small new helpers in Step 8.

- [ ] **Step 8: Add small templ/Go helpers** — `internal/adapter/webui/cockpit_vm.go` (append)

```go
import "fmt" // add to the import block

// fmtDurHM renders a duration as "H:MM h" (e.g. 2h30m → "2:30 h").
func fmtDurHM(d time.Duration) string {
	m := int(d.Minutes())
	return fmt.Sprintf("%d:%02d h", m/60, m%60)
}

// fmtSecsClock renders integer seconds as the initial clock text (overwritten
// by the live-timer JS on bind). Format mirrors the [data-timer] hero output.
func fmtSecsClock(sec int) string {
	return fmt.Sprintf("%dh %02dm %02ds", sec/3600, (sec%3600)/60, sec%60)
}
```

For `cockpitAccent` and `cockpitHex`, reuse the existing node color/glyph helpers (`nodeGlyphSwatch`, the `heuteAccentBar`/`heuteTileClass` colour mapping). If a dedicated accent class helper does not exist, add to `cockpit_vm.go`:

```go
// cockpitAccent maps a node colour name to the left accent-bar class.
func cockpitAccent(color string) string {
	switch color {
	case "cyan":
		return "bg-cyan"
	case "purple":
		return "bg-purple"
	case "green":
		return "bg-green"
	case "blue":
		return "bg-blue"
	case "orange":
		return "bg-orange"
	default:
		return "bg-blue"
	}
}
```

And `cockpitHex` as a templ in `cockpit.templ` (small hexagon with the node glyph):

```go
templ cockpitHex(n domain.Node) {
	<span class={ "grid place-items-center h-11 w-11 rounded-xl text-lg shrink-0", cockpitTileClass(n.Color) } aria-hidden="true">{ glyphOr(n.Glyph) }</span>
}
```

> Reuse `glyphOr` (exists, used in `homeDataFor`) and add `cockpitTileClass(color string) string` mirroring `heuteTileClass` (a `bg-<hue>/[.12] text-<hue>` map) in `cockpit_vm.go`. If `heuteTileClass` is exported/reachable, call it directly instead of duplicating.

- [ ] **Step 9: Write `internal/adapter/httpserver/webui_cockpit.go`** (data assembly + page handler)

```go
package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// nodeCockpitData assembles the cockpit head + the active tab's panel data.
func (s *Server) nodeCockpitData(r *http.Request, u domain.User, id, activeTab string) (webui.NodeCockpit, error) {
	ctx := r.Context()
	now := s.Clock.Now()
	n, err := s.GetNode.Execute(ctx, u.ID, id)
	if err != nil {
		return webui.NodeCockpit{}, err
	}
	d := webui.NodeCockpit{User: u.Username, N: n, ActiveTab: webui.NormalizeTab(activeTab)}

	// Ancestor chain (leaf→root) for the breadcrumb + rate resolution.
	d.Ancestors, _ = s.NodeAncestors.Execute(ctx, u.ID, n.ID)
	if n.Description != "" {
		d.DescriptionHTML = webui.RenderDocument(n.Description, func(string) (string, string, bool) { return "", "", false })
	}

	// Subtree rollup (replaces the old in-process own-only sum).
	if roll, rerr := s.Stats.NodeStats(ctx, u.ID, n.ID); rerr == nil {
		d.Rollup = roll
		// Inherited rate over [node]+ancestors (leaf→root).
		chain := d.Ancestors
		if len(chain) == 0 || chain[0].ID != n.ID {
			chain = append([]domain.Node{n}, chain...)
		}
		if rate := domain.ResolveRate(chain); rate != nil {
			d.Rate = rateLabel(rate)
			d.Earnings = rate.Mul(roll.Total).String()
		}
	}

	// Timer state from the running session.
	var running *domain.WorkSession
	if s.GetRunningSession.Sessions != nil {
		if rs, ok, gerr := s.GetRunningSession.Execute(ctx, u.ID); gerr == nil && ok {
			r2 := rs
			running = &r2
		}
	}
	nameOf := s.nodeNameLookup(ctx, u.ID)
	d.Timer = webui.NodeTimer(running, n.ID, domain.IsBookable(n.Kind), now, nameOf)

	// Active-tab data (filled by Tasks 5–8; Task 2 leaves them empty).
	return d, nil
}

// nodeNameLookup returns a closure mapping node id → name (for "running on Y").
func (s *Server) nodeNameLookup(ctx context.Context, ownerID string) func(string) string {
	all, _ := s.ListNodes.Execute(ctx, ownerID)
	m := make(map[string]string, len(all))
	for _, n := range all {
		m[n.ID] = n.Name
	}
	return func(id string) string { return m[id] }
}

// handleWebNodeView serves GET /nodes/{id}?tab= : the cockpit page.
func (s *Server) handleWebNodeView(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"), r.URL.Query().Get("tab"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeView(d).Render(r.Context(), w)
}

// handleWebNodeHead serves GET /nodes/{id}/head : the head fragment (SSE reload).
func (s *Server) handleWebNodeHead(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"), "")
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeHead(d).Render(r.Context(), w)
}
```

> Add `"context"` to the import block for `nodeNameLookup`'s signature.
> `rateLabel` exists (used in `homeDataFor`). `RenderDocument` exists (used by the old `nodeCockpitData`).

- [ ] **Step 10: Register routes** — `internal/adapter/httpserver/server.go`, in the `/nodes/{id}` block (`:253`)

Add after the `GET /nodes/{id}` line:

```go
	mux.Handle("GET /nodes/{id}/head", s.webAuth(http.HandlerFunc(s.handleWebNodeHead)))
```

(The `GET /nodes/{id}` registration stays; its handler now lives in `webui_cockpit.go`.)

- [ ] **Step 11: Write the cockpit test harness + a page test** — `internal/adapter/httpserver/webui_cockpit_test.go`

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

type cockpitTestServer struct {
	srv   *httpserver.Server
	ss    *testutil.FakeSessionStore
	ps    *testutil.FakeNodeStore
	bs    *testutil.FakeProjectBindingStore
	ds    *testutil.FakeDocumentStore
	ids   *testutil.FakeIDGen
	clk   testutil.FakeClock
	codec *websession.Codec
}

func newCockpitTestServer(t *testing.T) *cockpitTestServer {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 12, 0, 0, 0, time.Local)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	ds := testutil.NewFakeDocumentStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	bus := sse.NewBus()
	settings := testutil.NewFakeUserSettingsStore()
	srv := &httpserver.Server{
		Ensure:            usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:               bus,
		Emitter:           sse.NewEmitter(bus, &fakeActivityStore{}, &testutil.FakeIDGen{}, clk),
		Clock:             clk,
		Users:             users,
		Session:           codec,
		StartSession:      usecase.StartSession{Sessions: ss, Nodes: ps, IDs: ids, Clock: clk},
		StopSession:       usecase.StopSession{Sessions: ss, Nodes: ps, Clock: clk},
		AddSession:        usecase.AddSession{Sessions: ss, Nodes: ps, IDs: ids, Clock: clk},
		ListSessionsRange: usecase.ListSessionsRange{Sessions: ss},
		GetRunningSession: usecase.GetRunningSession{Sessions: ss},
		GetNode:           usecase.GetNode{Nodes: ps},
		ListNodes:         usecase.ListNodes{Nodes: ps},
		NodeAncestors:     usecase.NodeAncestors{Nodes: ps},
		CreateNode:        usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
		UpdateNode:        usecase.UpdateNode{Nodes: ps, Clock: clk},
		ListNodeBindings:  usecase.ListNodeBindings{Bindings: bs},
		BindNode:          usecase.BindNode{Bindings: bs, Nodes: ps, IDs: ids, Clock: clk},
		UnbindNode:        usecase.UnbindNode{Bindings: bs},
		ListDocuments:     usecase.ListDocuments{Docs: ds},
		Stats: usecase.StatsComputer{
			Sessions: ss,
			Nodes:    ps, // REQUIRED for NodeStats subtree walk
			Settings: settings,
			Clock:    clk,
			Loc:      time.Local,
		},
	}
	return &cockpitTestServer{srv: srv, ss: ss, ps: ps, bs: bs, ds: ds, ids: ids, clk: clk, codec: codec}
}

func (c *cockpitTestServer) do(t *testing.T, method, path string, form map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	req, _ := http.NewRequest(method, path, strings.NewReader(""))
	if form != nil {
		vals := make([]string, 0, len(form))
		for k, v := range form {
			vals = append(vals, k+"="+v)
		}
		body = strings.NewReader(strings.Join(vals, "&"))
		req, _ = http.NewRequest(method, path, body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	cookieVal, _ := c.codec.Issue("u1")
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rec := httptest.NewRecorder()
	c.srv.Routes().ServeHTTP(rec, req)
	return rec
}

// seedNode inserts a node directly into the fake store.
func (c *cockpitTestServer) seedNode(t *testing.T, n domain.Node) {
	t.Helper()
	if n.Status == "" {
		n.Status = domain.NodeActive
	}
	if _, err := c.ps.Create(context.Background(), n); err != nil {
		t.Fatalf("seedNode: %v", err)
	}
}

func TestCockpitView_RollupAndIdentity(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, Color: "cyan"})

	rec := c.do(t, "GET", "/nodes/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"flow", `id="cockpit-head"`, `id="cockpit-main"`, "Σ Gesamt"} {
		if !strings.Contains(body, want) {
			t.Errorf("cockpit missing %q", want)
		}
	}
	if rec2 := c.do(t, "GET", "/nodes/nope", nil); rec2.Code != http.StatusNotFound {
		t.Errorf("unknown id status=%d want 404", rec2.Code)
	}
}
```

> Verify the exact fake constructor + field names while writing: `testutil.NewFakeDocumentStore`, `FakeNodeStore.Create`, `usecase.ListDocuments{Docs: ...}`, `usecase.GetNode{Nodes: ...}`, `usecase.NodeAncestors{Nodes: ...}`, `usecase.BindNode{...}`/`UnbindNode{...}` field names, and whether `StartSession`/`AddSession` take a `Nodes` field. The `newWorktimeTestServer` harness (`webui_worktime_handlers_test.go:23`) is the authoritative template — copy field wiring from it; this task only ADDS `GetNode`, `NodeAncestors`, `BindNode`, `UnbindNode`, `ListDocuments`, and `Nodes: ps` inside `Stats`.

- [ ] **Step 12: Generate templ + run**

Run: `make generate && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/ -run "Cockpit|NodeTimer|NormalizeTab" -v`
Expected: PASS (page renders head + rollup; 404 path works).

- [ ] **Step 13: Full build + ci**

Run: `make generate && go build ./... && make ci`
Expected: green (gate ≥75%).

- [ ] **Step 14: Commit**

```bash
git add internal/adapter/webui/cockpit.templ internal/adapter/webui/cockpit_templ.go internal/adapter/webui/cockpit_vm.go internal/adapter/webui/cockpit_vm_test.go internal/adapter/webui/nodes.templ internal/adapter/webui/nodes_templ.go internal/adapter/webui/node_tree_vm.go internal/adapter/httpserver/webui_cockpit.go internal/adapter/httpserver/webui_cockpit_test.go internal/adapter/httpserver/webui_nodes.go internal/adapter/httpserver/server.go
git commit -m "feat(cockpit): head + subtree rollup (NodeStats + inherited rate); retire nodeWorktime"
```

---

## Task 3: Per-node timer (Start / Stop / Switch)

Add the three timer states' buttons + the handlers. Mirrors `handleHomeStart`/`handleHomeStop` (Emitter, GetRunningSession) but pre-books the node at start.

**Files:**
- Modify: `internal/adapter/webui/cockpit.templ` (`cockpitTimer` → full markup), `internal/adapter/httpserver/webui_cockpit.go` (handlers), `internal/adapter/httpserver/server.go` (routes), `internal/adapter/webui/static/js/` (no new JS — reuse `[data-timer]` rebind in `base.templ`).
- Test: `internal/adapter/httpserver/webui_cockpit_test.go`

**Interfaces:**
- Produces: `POST /nodes/{id}/start|stop|switch` → re-render `NodeHead` fragment + `Emit` session event.
- Consumes: `s.StartSession.Execute(ctx, ownerID, *nodeID, nil, "")`, `s.StopSession.Execute(ctx, ownerID, sessionID, *nodeID)`, `s.GetRunningSession`.

- [ ] **Step 1: Write the failing handler tests**

Append to `webui_cockpit_test.go`:

```go
func TestCockpitStart_BooksNode(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "POST", "/nodes/n1/start", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("start status %d body=%.300s", rec.Code, rec.Body.String())
	}
	// running session now exists, booked to n1
	rs, ok, _ := (usecase.GetRunningSession{Sessions: c.ss}).Execute(context.Background(), "u1")
	if !ok || rs.NodeID == nil || *rs.NodeID != "n1" {
		t.Fatalf("expected running session booked to n1, got ok=%v rs=%+v", ok, rs)
	}
	// head shows the live timer (data-timer) + stop button target
	if !strings.Contains(rec.Body.String(), "data-timer") || !strings.Contains(rec.Body.String(), "/nodes/n1/stop") {
		t.Errorf("head after start missing live timer / stop form")
	}
}

func TestCockpitStart_RejectsBranch(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "b1", OwnerID: "u1", Name: "feature/x", Kind: domain.KindBranch})
	rec := c.do(t, "POST", "/nodes/b1/start", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("start on branch status=%d want 400", rec.Code)
	}
}

func TestCockpitStop_EndsSession(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	nid := "n1"
	_, _ = (usecase.StartSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(context.Background(), "u1", &nid, nil, "")

	rec := c.do(t, "POST", "/nodes/n1/stop", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status %d body=%.300s", rec.Code, rec.Body.String())
	}
	if _, ok, _ := (usecase.GetRunningSession{Sessions: c.ss}).Execute(context.Background(), "u1"); ok {
		t.Errorf("session still running after stop")
	}
}

func TestCockpitSwitch_StopsOtherStartsHere(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	c.seedNode(t, domain.Node{ID: "n2", OwnerID: "u1", Name: "homelab", Kind: domain.KindRepo})
	other := "n2"
	_, _ = (usecase.StartSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(context.Background(), "u1", &other, nil, "")

	rec := c.do(t, "POST", "/nodes/n1/switch", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("switch status %d body=%.300s", rec.Code, rec.Body.String())
	}
	rs, ok, _ := (usecase.GetRunningSession{Sessions: c.ss}).Execute(context.Background(), "u1")
	if !ok || rs.NodeID == nil || *rs.NodeID != "n1" {
		t.Fatalf("after switch expected running on n1, got ok=%v rs=%+v", ok, rs)
	}
}
```

- [ ] **Step 2: Run — fail (routes 404 / handlers missing)**

Run: `go test ./internal/adapter/httpserver/ -run "TestCockpitStart|TestCockpitStop|TestCockpitSwitch" -v`
Expected: FAIL (404 / not registered).

- [ ] **Step 3: Implement the handlers** — append to `internal/adapter/httpserver/webui_cockpit.go`

```go
// handleWebNodeStart starts a timer pre-booked to {id}. Mirrors handleHomeStart
// but passes the node id at start (StartSession validates IsBookable → 400).
func (s *Server) handleWebNodeStart(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if _, err := s.StartSession.Execute(r.Context(), u.ID, &id, nil, ""); err != nil {
		if errors.Is(err, domain.ErrInvalidNode) || errors.Is(err, domain.ErrNotBookable) {
			http.Error(w, "node not bookable", http.StatusBadRequest)
			return
		}
		// already running, etc. — fall through and re-render current state.
	} else {
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID})
	}
	s.renderNodeHead(w, r, u, id)
}

// handleWebNodeStop stops the running session and books it to {id}.
func (s *Server) handleWebNodeStop(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if rs, ok, gerr := s.GetRunningSession.Execute(r.Context(), u.ID); gerr == nil && ok {
		nid := id
		if _, err := s.StopSession.Execute(r.Context(), u.ID, rs.ID, &nid); err == nil {
			s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID})
		}
	}
	s.renderNodeHead(w, r, u, id)
}

// handleWebNodeSwitch stops whatever is running, then starts a timer on {id}.
func (s *Server) handleWebNodeSwitch(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if rs, ok, gerr := s.GetRunningSession.Execute(r.Context(), u.ID); gerr == nil && ok {
		// Book the stopped session to its own node when bound; else to {id}.
		stopNode := id
		if rs.NodeID != nil {
			stopNode = *rs.NodeID
		}
		if _, err := s.StopSession.Execute(r.Context(), u.ID, rs.ID, &stopNode); err != nil {
			http.Error(w, "could not switch", http.StatusBadRequest)
			return
		}
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID})
	}
	nid := id
	if _, err := s.StartSession.Execute(r.Context(), u.ID, &nid, nil, ""); err != nil {
		if errors.Is(err, domain.ErrInvalidNode) || errors.Is(err, domain.ErrNotBookable) {
			http.Error(w, "node not bookable", http.StatusBadRequest)
			return
		}
	} else {
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID})
	}
	s.renderNodeHead(w, r, u, id)
}

// renderNodeHead re-renders the head fragment after a timer mutation.
func (s *Server) renderNodeHead(w http.ResponseWriter, r *http.Request, u domain.User, id string) {
	d, err := s.nodeCockpitData(r, u, id, "")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeHead(d).Render(r.Context(), w)
}
```

> Verify the exact bookable-guard sentinel names in `internal/domain` (grep `ErrNotBookable`, `ErrInvalidNode`, `requireBookable`) and in `stop_session.go`/`start_session.go`; use whichever `StartSession.Execute` actually returns for a non-bookable node. The test `TestCockpitStart_RejectsBranch` pins the 400 contract regardless of the sentinel name.

- [ ] **Step 4: Register routes** — `server.go`, after the `GET /nodes/{id}/head` line

```go
	mux.Handle("POST /nodes/{id}/start", s.webAuth(http.HandlerFunc(s.handleWebNodeStart)))
	mux.Handle("POST /nodes/{id}/stop", s.webAuth(http.HandlerFunc(s.handleWebNodeStop)))
	mux.Handle("POST /nodes/{id}/switch", s.webAuth(http.HandlerFunc(s.handleWebNodeSwitch)))
```

- [ ] **Step 5: Fill `cockpitTimer` with the real button markup** — replace the Task-2 stub in `cockpit.templ`

```go
templ cockpitTimer(d NodeCockpit) {
	switch d.Timer.State {
		case TimerHere:
			<form hx-post={ "/nodes/" + d.N.ID + "/stop" } hx-target="#cockpit-head" hx-swap="innerHTML" class="rounded-2xl bg-cyan/[.08] border border-cyan/20 px-4 py-3 flex items-center gap-4">
				<div>
					<div class="font-mono tnum text-2xl text-ink" data-timer data-base={ secStr(d.Timer.RunningBase) } role="timer">{ fmtSecsClock(d.Timer.RunningBase) }</div>
				</div>
				@components.Button(components.BtnDanger, components.T(ctx, "cockpit.timer.stop"), "■", templ.Attributes{"type": "submit"})
			</form>
		case TimerIdle:
			<form hx-post={ "/nodes/" + d.N.ID + "/start" } hx-target="#cockpit-head" hx-swap="innerHTML">
				@components.Button(components.BtnPrimary, components.T(ctx, "cockpit.timer.start"), "▶", templ.Attributes{"type": "submit"})
			</form>
		case TimerOtherBound:
			// inline confirm (NO window.confirm): a details/summary reveals the switch button.
			<details class="rounded-2xl bg-sunken/40 border border-line px-4 py-3">
				<summary class="cursor-pointer text-sm text-muted list-none">{ components.T(ctx, "cockpit.timer.runningOn") } <a href={ templ.SafeURL("/nodes/" + d.Timer.OtherID) } class="text-body hover:text-ink underline">{ d.Timer.OtherName }</a> →</summary>
				<form hx-post={ "/nodes/" + d.N.ID + "/switch" } hx-target="#cockpit-head" hx-swap="innerHTML" class="mt-2 flex items-center gap-2">
					<span class="text-[.8rem] text-muted">{ components.T(ctx, "cockpit.timer.switchHint") }</span>
					@components.Button(components.BtnSecondary, components.T(ctx, "cockpit.timer.switch"), "⇄", templ.Attributes{"type": "submit"})
				</form>
			</details>
		case TimerUnbound:
			<a href="/" class="text-sm text-muted hover:text-ink">{ components.T(ctx, "cockpit.timer.unbound") } →</a>
		case TimerNotBookable:
			<span class="text-sm text-faint">{ components.T(ctx, "cockpit.timer.notBookable") }</span>
	}
}
```

> Confirm `components.BtnPrimary/BtnDanger/BtnSecondary` exist (used by home). The live-timer JS in `base.templ` rebinds `[data-timer]` on `htmx:afterSwap`, so swapping `#cockpit-head` re-arms the clock with no new JS.

- [ ] **Step 6: Generate + test**

Run: `make generate && go test ./internal/adapter/httpserver/ -run "TestCockpitStart|TestCockpitStop|TestCockpitSwitch" -v`
Expected: PASS.

- [ ] **Step 7: ci + commit**

```bash
make generate && make web && make ci
git add -A
git commit -m "feat(cockpit): per-node timer start/stop/switch (3-state head, inline switch confirm)"
```

---

## Task 4: Tab mechanic (fragment swap + SSE per tab)

Make the tabs navigate via htmx, push `?tab=`, and reload the active panel on its own events — without resetting the head or the active tab.

**Files:**
- Modify: `internal/adapter/webui/cockpit.templ` (`cockpitTabLink` href + hx attrs, `cockpitTabsAndPanel` SSE on the panel), `internal/adapter/httpserver/webui_cockpit.go` (`handleWebNodeTab` + panel data fill dispatcher), `server.go` (route).
- Test: `webui_cockpit_test.go`

**Interfaces:**
- Produces: `GET /nodes/{id}/tab/{name}` → `cockpitTabsAndPanel` fragment (tabstrip + panel) for `#cockpit-main`.
- Consumes: `webui.NormalizeTab`.

- [ ] **Step 1: Failing test**

```go
func TestCockpitTab_SwapsPanel(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/nodes/n1/tab/wissen", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("tab status %d", rec.Code)
	}
	body := rec.Body.String()
	// active tab marker present and panel content container present
	if !strings.Contains(body, `id="cockpit-panel"`) {
		t.Errorf("tab fragment missing panel container")
	}
	// unknown tab normalizes to worktime (no 404)
	if rec2 := c.do(t, "GET", "/nodes/n1/tab/bogus", nil); rec2.Code != http.StatusOK {
		t.Errorf("bogus tab status=%d want 200 (normalized)", rec2.Code)
	}
}
```

Run → FAIL (route missing).

- [ ] **Step 2: Handler** — append to `webui_cockpit.go`

```go
// handleWebNodeTab serves GET /nodes/{id}/tab/{name}: the tabstrip+panel fragment.
func (s *Server) handleWebNodeTab(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"), r.PathValue("name"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.fillPanelData(r, u, &d) // Tasks 5–8 populate the active tab's slice
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.CockpitTabsAndPanel(d).Render(r.Context(), w)
}
```

Add a dispatcher stub now (filled by Tasks 5–8):

```go
// fillPanelData loads the active tab's data into d.
func (s *Server) fillPanelData(r *http.Request, u domain.User, d *webui.NodeCockpit) {
	switch d.ActiveTab {
	// case "worktime": Task 5
	// case "wissen":   Task 6
	// case "struktur": Task 7
	// case "bindings": Task 8
	}
}
```

Wire `fillPanelData` into the page handler too: in `handleWebNodeView`, after building `d`, call `s.fillPanelData(r, u, &d)` before render. Export the templ: rename `cockpitTabsAndPanel` → `CockpitTabsAndPanel` (exported) in `cockpit.templ`.

- [ ] **Step 3: Tab link htmx attrs** — `cockpit.templ` `cockpitTabLink`

```go
templ cockpitTabLink(id, key, labelKey string, active bool) {
	<a
		hx-get={ "/nodes/" + id + "/tab/" + key }
		hx-target="#cockpit-main"
		hx-swap="innerHTML"
		hx-push-url={ "/nodes/" + id + "?tab=" + key }
		class={ "px-4 py-2.5 text-sm font-medium border-b-2 -mb-px cursor-pointer",
			templ.KV("text-cyan border-cyan", active),
			templ.KV("text-muted border-transparent hover:text-body", !active) }
		if active {
			aria-current="page"
		}
	>{ components.T(ctx, labelKey) }</a>
}
```

- [ ] **Step 4: Panel SSE reload** — wrap the panel so it reloads the ACTIVE tab on its events. In `CockpitTabsAndPanel`, give the panel container an SSE trigger keyed to the active tab:

```go
	<div
		id="cockpit-panel"
		class="pt-6"
		if cockpitPanelSSE(d.ActiveTab) != "" {
			hx-get={ "/nodes/" + d.N.ID + "/tab/" + d.ActiveTab }
			hx-trigger={ cockpitPanelSSE(d.ActiveTab) }
			hx-target="#cockpit-main"
			hx-swap="innerHTML"
		}
	>
		@cockpitPanel(d)
	</div>
```

> CRITICAL: the panel's SSE reload MUST `hx-target="#cockpit-main"` (the outer strip+panel container), NOT itself — `/tab/{name}` returns the full `CockpitTabsAndPanel`, so self-targeting nests a second tab strip + a duplicate `id="cockpit-panel"` on every SSE event. Omit the reload attrs entirely when `cockpitPanelSSE` returns `""` (bindings).

Add to `cockpit_vm.go`:

```go
// cockpitPanelSSE returns the hx-trigger SSE event list for a tab's live reload.
func cockpitPanelSSE(tab string) string {
	switch tab {
	case "worktime":
		return "sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted"
	case "wissen":
		return "sse:document.created, sse:document.updated, sse:document.deleted"
	case "struktur":
		return "sse:node.created, sse:node.updated, sse:node.moved, sse:node.deleted"
	default:
		return "" // bindings: reload only after own mutation
	}
}
```

> Note: the panel's `hx-get` points at `/tab/{active}`, so an SSE reload re-fetches the SAME tab (not a reset to worktime). The head and panel are independent containers.

- [ ] **Step 5: Route**

```go
	mux.Handle("GET /nodes/{id}/tab/{name}", s.webAuth(http.HandlerFunc(s.handleWebNodeTab)))
```

- [ ] **Step 6: Generate, test, ci, commit**

```bash
make generate && go test ./internal/adapter/httpserver/ -run TestCockpitTab -v && make ci
git add -A
git commit -m "feat(cockpit): htmx tab swap (?tab= deep-link) + per-tab SSE reload"
```

---

## Task 5: Worktime tab (own sessions list + Nachbuchen)

**Files:** Modify `cockpit.templ` (worktime panel), `webui_cockpit.go` (`fillPanelData` worktime case + `handleWebNodeAddSession`), `server.go` (route). Test `webui_cockpit_test.go`.

**Interfaces:**
- Consumes: `s.ListSessionsRange`, `s.AddSession.Execute(ctx, ownerID, *nodeID, start, stop, tags, note)`.
- Produces: `POST /nodes/{id}/sessions` (Nachbuchen) → re-render the worktime panel; `Emit(EventSessionUpdated)`.

- [ ] **Step 1: Failing test**

```go
func TestCockpitWorktime_ListsOwnSessions(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	start := time.Date(2026, 6, 27, 14, 0, 0, 0, time.Local)
	stop := start.Add(2 * time.Hour)
	nid := "n1"
	_, _ = (usecase.AddSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(context.Background(), "u1", &nid, start, stop, []string{"slice6"}, "")

	rec := c.do(t, "GET", "/nodes/n1/tab/worktime", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "slice6") || !strings.Contains(body, "/nodes/n1/sessions") {
		t.Errorf("worktime panel missing session row / add form: %.400s", body)
	}
}

func TestCockpitAddSession_Books(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	rec := c.do(t, "POST", "/nodes/n1/sessions", map[string]string{
		"date": "2026-06-28", "from": "09:00", "to": "11:00", "tag": "x",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("add status %d body=%.300s", rec.Code, rec.Body.String())
	}
	// session exists booked to n1
	all, _ := c.ss.ListRange(context.Background(), "u1",
		time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local), time.Date(2026, 6, 29, 0, 0, 0, 0, time.Local))
	if len(all) != 1 || all[0].NodeID == nil || *all[0].NodeID != "n1" {
		t.Fatalf("expected 1 session booked to n1, got %+v", all)
	}
}
```

> Adjust `c.ss.ListRange` to the real FakeSessionStore method (likely `ListRange` or use `usecase.ListSessionsRange{Sessions: c.ss}.Execute`). Use the usecase to avoid coupling to the fake's internal method.

- [ ] **Step 2: `fillPanelData` worktime case** — `webui_cockpit.go`

```go
	case "worktime":
		now := s.Clock.Now()
		since := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		all, _ := s.ListSessionsRange.Execute(r.Context(), u.ID, since, now.AddDate(0, 0, 1))
		out := make([]domain.WorkSession, 0, 25)
		for i := len(all) - 1; i >= 0 && len(out) < 25; i-- { // newest first
			if all[i].NodeID != nil && *all[i].NodeID == d.N.ID {
				out = append(out, all[i])
			}
		}
		d.Sessions = out
```

(Add `"time"` import.)

- [ ] **Step 3: `handleWebNodeAddSession`** — mirrors `handleWebAdd` (`webui_worktime.go:63`)

```go
// handleWebNodeAddSession books a manual session on {id} (Nachbuchen).
func (s *Server) handleWebNodeAddSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	day := parseDayParam(s, r.FormValue("date"))
	start, err1 := dayTime(day, r.FormValue("from"))
	stop, err2 := dayTime(day, r.FormValue("to"))
	if err1 != nil || err2 != nil || !stop.After(start) {
		s.renderNodePanel(w, r, u, id, "worktime", "ungültige Zeit — HH:MM, bis > von")
		return
	}
	nid := id
	if _, err := s.AddSession.Execute(r.Context(), u.ID, &nid, start, stop,
		strings.Fields(r.FormValue("tag")), r.FormValue("note")); err != nil {
		s.renderNodePanel(w, r, u, id, "worktime", "konnte nicht buchen: "+err.Error())
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID})
	s.renderNodePanel(w, r, u, id, "worktime", "")
}

// renderNodePanel re-renders one tab's panel fragment (with an optional inline error).
func (s *Server) renderNodePanel(w http.ResponseWriter, r *http.Request, u domain.User, id, tab, errMsg string) {
	d, err := s.nodeCockpitData(r, u, id, tab)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.fillPanelData(r, u, &d)
	d.BindErr = errMsg // reuse the inline-error field for any panel error
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.CockpitTabsAndPanel(d).Render(r.Context(), w)
}
```

> `parseDayParam`, `dayTime` exist (used by `handleWebAdd`). Reuse them. (`d.BindErr` is a generic inline-error carrier; rename to `d.PanelErr` in `cockpit_vm.go` for clarity and update Task 8 accordingly.)

- [ ] **Step 4: Worktime panel templ** — `cockpit.templ`, add to the `cockpitPanel` switch

```go
		case "worktime":
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-sm font-semibold text-ink">{ components.T(ctx, "cockpit.worktime.title") }</h2>
				<button class="text-[.8rem] font-semibold rounded-xl bg-cyan/[.12] text-cyan border border-cyan/25 px-3 py-1.5" onclick="document.getElementById('nb-form').classList.toggle('hidden')">+ { components.T(ctx, "cockpit.worktime.add") }</button>
			</div>
			if d.PanelErr != "" {
				<p class="mb-3 rounded-xl bg-red/10 text-red px-3 py-2 text-[.82rem]" role="alert">{ d.PanelErr }</p>
			}
			<form id="nb-form" class="hidden mb-4 flex flex-wrap items-end gap-2 rounded-2xl border border-line bg-sunken/30 p-3"
				hx-post={ "/nodes/" + d.N.ID + "/sessions" } hx-target="#cockpit-panel" hx-swap="innerHTML">
				<input type="date" name="date" class="rounded-lg border border-line bg-surface px-2 py-1.5 text-sm"/>
				<input name="from" placeholder="09:00" class="w-20 rounded-lg border border-line bg-surface px-2 py-1.5 text-sm"/>
				<input name="to" placeholder="11:00" class="w-20 rounded-lg border border-line bg-surface px-2 py-1.5 text-sm"/>
				<input name="tag" placeholder="#tag" class="w-28 rounded-lg border border-line bg-surface px-2 py-1.5 text-sm"/>
				@components.Button(components.BtnPrimary, components.T(ctx, "cockpit.worktime.add"), "+", templ.Attributes{"type": "submit"})
			</form>
			if len(d.Sessions) == 0 {
				<p class="text-sm text-faint">{ components.T(ctx, "cockpit.worktime.empty") }</p>
			} else {
				<ul class="divide-y divide-line2 rounded-2xl border border-line bg-surface">
					for _, sess := range d.Sessions {
						@cockpitSessionRow(sess)
					}
				</ul>
				<p class="mt-2 text-[.72rem] text-faint">{ components.T(ctx, "cockpit.worktime.ownOnly") }</p>
			}
```

Add `cockpitSessionRow` (date · span · tag · dur) — reuse `fmtClock`/`fmtDur` helpers from the worktime templ (grep their names; e.g. `fmtHM`). If unsure of the exact helper names, format inline with the session's `Start.Format("Mon 02.01.")` and a duration via `fmtDurHM(sess.Elapsed(now))` — pass `now` into the row or precompute a label slice in `fillPanelData`. Simplest: precompute display strings in `fillPanelData` into a `[]CockpitSessionRow{Date, Span, Tag, Dur, Running bool}` to keep the templ logic-free.

> RECOMMENDED: define `CockpitSessionRow` in `cockpit_vm.go` and build it in `fillPanelData` (so the templ has no time math). Mirror the v1 mockup row.

- [ ] **Step 5: Route + generate + test + ci + commit**

```go
	mux.Handle("POST /nodes/{id}/sessions", s.webAuth(http.HandlerFunc(s.handleWebNodeAddSession)))
```

```bash
make generate && go test ./internal/adapter/httpserver/ -run "TestCockpitWorktime|TestCockpitAddSession" -v && make web && make ci
git add -A && git commit -m "feat(cockpit): worktime tab — own-session list + nachbuchen"
```

---

## Task 6: Wissen tab (node-scoped docs + Neu pre-scoped)

**Files:** Modify `cockpit.templ` (wissen panel), `webui_cockpit.go` (`fillPanelData` wissen case), `webui_editor.go` (`handleWebEditorNew` reads `?node=`). Test `webui_cockpit_test.go` + `webui_editor` test.

**Interfaces:** Consumes `s.ListDocuments.Execute(ctx, ownerID, &nodeID, nil)`.

- [ ] **Step 1: Failing test**

```go
func TestCockpitWissen_ListsNodeDocs(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	nid := "n1"
	doc, _ := domain.NewDocument("d1", "u1", &nid, "Architektur", "# A", c.clk.Now())
	_, _ = c.ds.Create(context.Background(), doc)

	rec := c.do(t, "GET", "/nodes/n1/tab/wissen", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "Architektur") || !strings.Contains(body, "/wissen/neu?node=n1") {
		t.Errorf("wissen panel missing doc / scoped-new link: %.300s", body)
	}
}
```

> Verify `domain.NewDocument` signature + `FakeDocumentStore.Create` while writing; adapt the seed accordingly.

- [ ] **Step 2: `fillPanelData` wissen case**

```go
	case "wissen":
		nid := d.N.ID
		d.Docs, _ = s.ListDocuments.Execute(r.Context(), u.ID, &nid, nil)
```

- [ ] **Step 3: Wissen panel templ** — add to `cockpitPanel`

```go
		case "wissen":
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-sm font-semibold text-ink">{ components.T(ctx, "cockpit.wissen.title") }</h2>
				<a href={ templ.SafeURL("/wissen/neu?node=" + d.N.ID) } hx-boost="false" class="text-[.8rem] font-semibold rounded-xl bg-cyan/[.12] text-cyan border border-cyan/25 px-3 py-1.5">+ { components.T(ctx, "cockpit.wissen.add") }</a>
			</div>
			if len(d.Docs) == 0 {
				<p class="text-sm text-faint">{ components.T(ctx, "cockpit.wissen.empty") }</p>
			} else {
				<ul class="divide-y divide-line2 rounded-2xl border border-line bg-surface">
					for _, doc := range d.Docs {
						<li class="px-4 py-2.5 text-sm"><a href={ templ.SafeURL("/wissen/" + doc.ID) } class="hover:text-cyan">{ doc.Title }</a></li>
					}
				</ul>
			}
```

- [ ] **Step 4: `handleWebEditorNew` reads `?node=`** — `webui_editor.go:35`

Read the existing handler; add: parse `r.URL.Query().Get("node")` and pre-select it in the editor's node picker view model (set the form's preselected node id). Add a test asserting the rendered new-editor contains the node preselected when `?node=n1`.

```go
func TestEditorNew_PrescopesNode(t *testing.T) {
	c := newCockpitTestServer(t) // editor handler also needs CreateDocument/ListNodes wiring; reuse + extend harness
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	rec := c.do(t, "GET", "/wissen/neu?node=n1", nil)
	if rec.Code == http.StatusOK && !strings.Contains(rec.Body.String(), "n1") {
		t.Errorf("new editor did not pre-scope node n1")
	}
}
```

> If `newCockpitTestServer` lacks the editor's usecases (`CreateDocument`, etc.), add them to the harness or place this test in the editor's existing `_test.go` with its own harness. Keep the assertion lenient (the editor markup may encode the node id in a hidden field/select option).

- [ ] **Step 5: generate + test + ci + commit**

```bash
make generate && go test ./internal/adapter/httpserver/ -run "TestCockpitWissen|TestEditorNew" -v && make ci
git add -A && git commit -m "feat(cockpit): wissen tab — node-scoped docs + pre-scoped new editor"
```

---

## Task 7: Struktur tab (children + add-child + move + status)

**Files:** Modify `cockpit.templ` (struktur panel + move form ported from old `nodeMoveForm`), `webui_cockpit.go` (`fillPanelData` struktur case), `webui_nodes.go` (`handleWebNodeNew` reads `?parent=`/`?kind=`). Test `webui_cockpit_test.go`.

**Interfaces:** Consumes `s.ListNodes`, `webui.MoveTargetsFor`; reuses existing `POST /nodes/{id}/move` and `POST /nodes/{id}/status`.

- [ ] **Step 1: Failing test**

```go
func TestCockpitStruktur_ListsChildrenAndMove(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "p1", OwnerID: "u1", Name: "Plattform", Kind: domain.KindVorhaben})
	pp := "p1"
	c.seedNode(t, domain.Node{ID: "c1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, ParentID: &pp})

	rec := c.do(t, "GET", "/nodes/p1/tab/struktur", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "flow") || !strings.Contains(body, "/nodes/p1/move") {
		t.Errorf("struktur panel missing child / move form: %.300s", body)
	}
	if !strings.Contains(body, "/nodes/new?parent=p1") {
		t.Errorf("struktur panel missing add-child link")
	}
}
```

- [ ] **Step 2: `fillPanelData` struktur case**

```go
	case "struktur":
		all, _ := s.ListNodes.Execute(r.Context(), u.ID)
		now := s.Clock.Now()
		for _, n := range all {
			if n.ParentID != nil && *n.ParentID == d.N.ID {
				label := ""
				if roll, err := s.Stats.NodeStats(r.Context(), u.ID, n.ID); err == nil && roll.Total > 0 {
					label = webui.FmtDurHMExport(roll.Total) // exported fmtDurHM
				}
				d.Children = append(d.Children, webui.NodeChild{N: n, Total: label})
			}
		}
		d.MoveTargets = webui.MoveTargetsFor(all, d.N)
```

> Export `fmtDurHM` as `FmtDurHMExport` (or just inline the same `m/60:m%60` math) so the handler can format the child total. Keep one implementation — call the exported one from both templ and handler.

- [ ] **Step 3: Struktur panel templ** (children list + add-child + move + status) — add to `cockpitPanel`

```go
		case "struktur":
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-sm font-semibold text-ink">{ components.T(ctx, "cockpit.struktur.title") }</h2>
				<a href={ templ.SafeURL("/nodes/new?parent=" + d.N.ID) } hx-boost="false" class="text-[.8rem] font-semibold rounded-xl bg-cyan/[.12] text-cyan border border-cyan/25 px-3 py-1.5">+ { components.T(ctx, "cockpit.struktur.add") }</a>
			</div>
			if len(d.Children) == 0 {
				<p class="text-sm text-faint mb-6">{ components.T(ctx, "cockpit.struktur.empty") }</p>
			} else {
				<ul class="divide-y divide-line2 rounded-2xl border border-line bg-surface mb-6">
					for _, ch := range d.Children {
						<li class="px-4 py-2.5 text-sm flex items-center justify-between">
							<a href={ templ.SafeURL("/nodes/" + ch.N.ID) } class="flex items-center gap-2 hover:text-cyan">
								@nodeKindBadge(ch.N.Kind)
								{ ch.N.Name }
							</a>
							if ch.Total != "" {
								<span class="text-[.72rem] text-muted tnum">{ ch.Total }</span>
							}
						</li>
					}
				</ul>
			}
			// reparent (ported from old nodeMoveForm)
			@cockpitMoveForm(d)
			// status toggle
			@cockpitStatusForm(d)
```

Port `cockpitMoveForm` from the deleted `nodeMoveForm` (`nodes.templ:376`) — same `<form method="post" action="/nodes/{id}/move" hx-boost="false">` with the `parentId` select over `d.MoveTargets`. Add `cockpitStatusForm` posting to the existing `POST /nodes/{id}/status` with a status `<select>` (active/paused/archived) defaulting to `d.N.Status`.

- [ ] **Step 4: `handleWebNodeNew` reads `?parent=`/`?kind=`** — `webui_nodes.go:133`

Pre-fill `NodeFormValues` from query: if `?parent=` present, set the parent select; if `?kind=` present (else infer the allowed child kind from the parent's kind), set Kind. Add a test:

```go
func TestNodeNew_PrefillsParent(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "p1", OwnerID: "u1", Name: "Plattform", Kind: domain.KindVorhaben})
	rec := c.do(t, "GET", "/nodes/new?parent=p1", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "p1") {
		t.Errorf("new-node form did not prefill parent p1 (status %d)", rec.Code)
	}
}
```

- [ ] **Step 5: generate + test + ci + commit**

```bash
make generate && go test ./internal/adapter/httpserver/ -run "TestCockpitStruktur|TestNodeNew" -v && make web && make ci
git add -A && git commit -m "feat(cockpit): struktur tab — children + add-child prefill + move + status"
```

---

## Task 8: Bindings tab (list + remote add + delete + path hint)

**Files:** Modify `cockpit.templ` (bindings panel), `webui_cockpit.go` (`fillPanelData` bindings case + `handleWebNodeBindRemote` + `handleWebNodeUnbind`), `server.go` (routes). Test `webui_cockpit_test.go`.

**Interfaces:** Consumes `s.ListNodeBindings.ExecuteByProject`, `s.BindNode.Execute(ctx, ownerID, nodeID, usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: ...})`, `s.UnbindNode.Execute(ctx, ownerID, usecase.BindKey{...})`.

- [ ] **Step 1: Failing tests**

```go
func TestCockpitBindings_AddRemote(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	rec := c.do(t, "POST", "/nodes/n1/bindings", map[string]string{"remoteSlug": "github.com/serverkraken/flow"})
	if rec.Code != http.StatusOK {
		t.Fatalf("bind status %d body=%.300s", rec.Code, rec.Body.String())
	}
	bs, _ := (usecase.ListNodeBindings{Bindings: c.bs}).ExecuteByProject(context.Background(), "u1", "n1")
	if len(bs) != 1 || bs[0].Kind != domain.BindingRemote {
		t.Fatalf("expected 1 remote binding, got %+v", bs)
	}
	if !strings.Contains(rec.Body.String(), "github.com/serverkraken/flow") {
		t.Errorf("bindings panel did not list the new remote")
	}
}

func TestCockpitBindings_DeleteRemote(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	_, _ = (usecase.BindNode{Bindings: c.bs, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(
		context.Background(), "u1", "n1", usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: "github.com/x/y"})

	rec := c.do(t, "POST", "/nodes/n1/bindings/delete", map[string]string{"kind": "remote", "slug": "github.com/x/y"})
	if rec.Code != http.StatusOK {
		t.Fatalf("unbind status %d", rec.Code)
	}
	bs, _ := (usecase.ListNodeBindings{Bindings: c.bs}).ExecuteByProject(context.Background(), "u1", "n1")
	if len(bs) != 0 {
		t.Errorf("expected 0 bindings after delete, got %+v", bs)
	}
}
```

- [ ] **Step 2: `fillPanelData` bindings case**

```go
	case "bindings":
		d.Bindings, _ = s.ListNodeBindings.ExecuteByProject(r.Context(), u.ID, d.N.ID)
```

- [ ] **Step 3: Handlers** — append to `webui_cockpit.go`

```go
// handleWebNodeBindRemote adds a remote binding (form field remoteSlug) to {id}.
func (s *Server) handleWebNodeBindRemote(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	slug := strings.TrimSpace(r.FormValue("remoteSlug"))
	if slug == "" {
		s.renderNodePanel(w, r, u, id, "bindings", "")
		return
	}
	key := usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: slug}
	if _, err := s.BindNode.Execute(r.Context(), u.ID, id, key); err != nil {
		msg := "konnte nicht binden"
		if errors.Is(err, usecase.ErrInvalidBindTarget) {
			msg = i18nT(r, "cockpit.bindings.remoteOnlyRepo")
		}
		s.renderNodePanel(w, r, u, id, "bindings", msg)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	s.renderNodePanel(w, r, u, id, "bindings", "")
}

// handleWebNodeUnbind removes a binding (form: kind + slug | machine + path).
func (s *Server) handleWebNodeUnbind(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	key := usecase.BindKey{
		Kind:       domain.BindingKind(r.FormValue("kind")),
		RemoteSlug: r.FormValue("slug"),
		MachineID:  r.FormValue("machine"),
		Path:       r.FormValue("path"),
	}
	if err := s.UnbindNode.Execute(r.Context(), u.ID, key); err != nil {
		s.renderNodePanel(w, r, u, id, "bindings", "konnte nicht lösen")
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	s.renderNodePanel(w, r, u, id, "bindings", "")
}
```

> `i18nT(r, key)` — use the same ctx-bound `i18n.T(r.Context(), key)` the handlers already use for server-side strings (grep an existing handler that emits a localized string; if none, hardcode the German fallback as elsewhere in these handlers). `usecase.ErrInvalidBindTarget` is the real sentinel (seen in `handleBindNode`).

- [ ] **Step 4: Bindings panel templ** — add to `cockpitPanel`

```go
		case "bindings":
			<h2 class="text-sm font-semibold text-ink mb-4">{ components.T(ctx, "cockpit.bindings.title") }</h2>
			if d.PanelErr != "" {
				<p class="mb-3 rounded-xl bg-red/10 text-red px-3 py-2 text-[.82rem]" role="alert">{ d.PanelErr }</p>
			}
			if len(d.Bindings) == 0 {
				<p class="text-sm text-faint mb-4">{ components.T(ctx, "cockpit.bindings.empty") }</p>
			} else {
				<ul class="divide-y divide-line2 rounded-2xl border border-line bg-surface mb-4">
					for _, b := range d.Bindings {
						<li class="px-4 py-2.5 text-sm flex items-center justify-between gap-3">
							<span class="font-mono text-[.8rem] text-body truncate">{ string(b.Kind) }: { bindingTarget(b) }</span>
							<form hx-post={ "/nodes/" + d.N.ID + "/bindings/delete" } hx-target="#cockpit-panel" hx-swap="innerHTML">
								<input type="hidden" name="kind" value={ string(b.Kind) }/>
								<input type="hidden" name="slug" value={ b.RemoteSlug }/>
								<input type="hidden" name="machine" value={ b.MachineID }/>
								<input type="hidden" name="path" value={ b.Path }/>
								<button class="text-[.72rem] text-faint hover:text-red">{ components.T(ctx, "cockpit.bindings.delete") }</button>
							</form>
						</li>
					}
				</ul>
			}
			if d.N.Kind == domain.KindRepo {
				<form hx-post={ "/nodes/" + d.N.ID + "/bindings" } hx-target="#cockpit-panel" hx-swap="innerHTML" class="flex items-end gap-2">
					<input name="remoteSlug" placeholder={ components.T(ctx, "cockpit.bindings.remotePlaceholder") } class="flex-1 rounded-lg border border-line bg-surface px-3 py-2 text-sm font-mono"/>
					@components.Button(components.BtnSecondary, components.T(ctx, "cockpit.bindings.addRemote"), "+", templ.Attributes{"type": "submit"})
				</form>
			}
			<p class="mt-3 text-[.72rem] text-faint">{ components.T(ctx, "cockpit.bindings.pathHint") }</p>
```

> `bindingTarget(b)` helper existed in the old `nodes.templ`; if it was deleted with the cockpit half, re-add it to `cockpit.templ` (returns `b.RemoteSlug` for remote, `b.MachineLabel + ":" + b.Path` for path).

- [ ] **Step 5: Routes + generate + test + ci + commit**

```go
	mux.Handle("POST /nodes/{id}/bindings", s.webAuth(http.HandlerFunc(s.handleWebNodeBindRemote)))
	mux.Handle("POST /nodes/{id}/bindings/delete", s.webAuth(http.HandlerFunc(s.handleWebNodeUnbind)))
```

```bash
make generate && go test ./internal/adapter/httpserver/ -run TestCockpitBindings -v && make web && make ci
git add -A && git commit -m "feat(cockpit): bindings tab — remote add/delete + path read-only hint"
```

---

## Task 9: Responsive polish + styleguide check

**Files:** Modify `cockpit.templ` (Tailwind responsive classes), `internal/adapter/webui/static/app.css` (via `make web`).

- [ ] **Step 1: Apply responsive classes** (no behaviour change; purely Tailwind):
  - Head `idrow`: `flex-col gap-4 sm:flex-row sm:items-start sm:justify-between` so identity stacks above the timer on mobile; timer block `w-full sm:w-auto`.
  - Rollup: `grid grid-cols-2 gap-3 sm:flex sm:flex-wrap` → 2×2 on phone, row on desktop.
  - Tabstrip: add `overflow-x-auto whitespace-nowrap` on the `<nav>` so tabs scroll horizontally on narrow screens; keep the active tab first-visible.
  - Worktime/Children rows: ensure they read as two-line cards on phone (`flex-col items-start gap-1 sm:flex-row sm:items-center sm:justify-between`).
  - Sidebar→bottom-nav + app-bar are already provided by `components.AppShell` (Slice 3). Confirm the cockpit renders inside `AppShell` (it does, via `nodeCockpitShell`) — no extra work; just verify in the browser gate.

- [ ] **Step 2: Rebuild CSS + verify**

Run: `make web && make ci`
Expected: green (verify-css passes because app.css now matches; verify-no-popups passes — the switch confirm uses `<details>`, not `window.confirm`).

- [ ] **Step 3: Manual styleguide eyeball (light theme is derived)**

Run the dev stack (`make dev-up && make dev-run`), open `/ui` and a cockpit in light + dark; confirm glass head, accent bar, rollup tiles, and tabstrip read well in both. Fix any contrast issues via existing tokens only.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "style(cockpit): responsive reflow (stacked head, 2x2 rollup, scrollable tabs)"
```

---

## Task 10: Wiring verification + done-gate

**Files:** none (verification) — or small fixes surfaced here.

- [ ] **Step 1: Route audit** — confirm every new route is registered and reachable:

```bash
grep -n "nodes/{id}/\(head\|start\|stop\|switch\|tab\|sessions\|bindings\)" internal/adapter/httpserver/server.go
```
Expected: 8 lines (head, start, stop, switch, tab/{name}, sessions, bindings, bindings/delete).

- [ ] **Step 2: Publish-path audit** — every mutating cockpit handler calls `s.Emitter.Emit(...)`:

```bash
rg -n "Emitter.Emit" internal/adapter/httpserver/webui_cockpit.go
```
Expected: start, stop, switch, addSession, bindRemote, unbind (≥6).

- [ ] **Step 3: Full ci (incl. web)**

Run: `make generate && make web && make ci`
Expected: green; record the coverage % (must be ≥75).

- [ ] **Step 4: Live done-gate (dev stack: Postgres + Dex)**

```bash
make dev-up && make dev-run   # second terminal: make dev-token / flow login
```
Then in the browser (logged in), on a repo node's cockpit verify each:
1. **Idle → Start** books the node; head shows the live ticking clock + Stop.
2. **Stop** ends the session; rollup Σ increases; worktime tab shows the new row (SSE, no manual reload).
3. **Switch**: start a timer on node A, open node B's cockpit → "läuft auf A"; expand → Switch → B now runs, A stopped.
4. **Branch** node shows "nicht buchbar", no Start.
5. **Rollup correctness**: a parent's Σ Gesamt = its own + all descendants' sessions; earnings uses the inherited rate (set a rate only on an ancestor).
6. **Tabs**: switch Worktime↔Wissen↔Struktur↔Bindings; URL gains `?tab=`; reload keeps the tab; a session start in another tab/window does NOT reset the active tab.
7. **Nachbuchen** adds a row + bumps the rollup.
8. **Wissen**: "+ Neu" opens the editor pre-scoped to the node; saved doc appears in the tab.
9. **Struktur**: children listed; "+ Unterknoten" opens the form with parent prefilled; move + status work.
10. **Bindings**: add a remote (repo node) → appears; delete → gone; path hint visible; adding a remote on a non-repo node shows the inline error (or the add form is hidden).
11. **Mobile**: narrow the window — head stacks, rollup 2×2, tabstrip scrolls, rows become cards; bottom-nav reachable.

- [ ] **Step 5: Holistic review (Opus)** — request a whole-branch review of the cockpit slice (the `feedback_plan_main_wiring_task` lesson: per-task reviews miss composition-root gaps). Address any Critical/Major before declaring done.

- [ ] **Step 6: Final commit (only if Step 4/5 required fixes)**

```bash
git add -A
git commit -m "fix(cockpit): done-gate + holistic-review follow-ups"
```

---

## Self-Review (completed by plan author)

**Spec coverage (§-by-§):**
- §4 IA (head + 4 tabs, worktime default) → Tasks 2 (shell), 4 (tab nav). ✓
- §5 head (hexagon, breadcrumb, kind/status, inherited rate, rollup, 3 timer states + notBookable) → Tasks 2 (head/rollup/rate), 3 (timer states). ✓
- §6.1 worktime (own list + nachbuchen) → Task 5. ✓
- §6.2 wissen (node docs + Neu pre-scoped, `?node=` to add) → Task 6. ✓
- §6.3 struktur (children + add-child `?parent=` + move + status) → Task 7. ✓
- §6.4 bindings (remote add/delete, path read-only + hint, repo-only guard) → Task 8. ✓
- §7 rollup correction (NodeStats + ResolveRate, nodeWorktime removed) → Task 2. ✓
- §8 htmx tab swap + separate SSE containers + `?tab=` → Tasks 2 (containers), 4 (swap+SSE). ✓
- §9 new routes (start/stop/switch/tab/sessions/bindings/delete + head) → Tasks 2,3,4,5,8 + audit in 10. ✓
- §10 responsive → Task 9. ✓
- §11 errors (404, 400 branch, idempotent, 409→inline, to≤from) → Tasks 2 (404), 3 (400), 5 (range), 8 (inline). ✓
- §12 i18n cockpit.* → Task 1. ✓
- §13 testing & done-gate (TDD per handler, pure VM tests, make ci+web, live, opus review, wiring audit) → every task + Task 10. ✓

**Placeholder scan:** Two intentional, explicitly-flagged reads remain (NOT silent placeholders): the bookable-guard sentinel name in Task 3 (test pins the 400 contract), and fake-store constructor/usecase-field names in Task 2's harness (the authoritative template `newWorktimeTestServer` is cited). No "TBD"/"add error handling"/"similar to Task N" for code. (Pre-flight fixes: non-idiomatic `webui_fmtSecs`→`fmtSecsClock`; `context.Context` typo corrected.)

**Type consistency:** `NodeCockpit` fields, `CockpitTimer`/`CockpitTimerState`, `NodeTimer(...)`, `NormalizeTab`, `cockpitPanelSSE`, `fillPanelData`, `renderNodeHead`/`renderNodePanel`, `CockpitTabsAndPanel` (exported) used consistently across tasks. One rename is flagged in-place: the inline-error field `BindErr` → `PanelErr` (Task 5 introduces the generic name; Tasks 5 and 8 both use `d.PanelErr`). `fmtDurHM` is exported once (`FmtDurHMExport`) and reused by handler + templ.

**Reuse check:** Timer handlers mirror `handleHomeStart/Stop` (Emitter, GetRunningSession); Nachbuchen mirrors `handleWebAdd` (`parseDayParam`/`dayTime`); move form ported from the deleted `nodeMoveForm`; live timer reuses `base.templ`'s `[data-timer]` rebind (no new JS); test harness extends `newWorktimeTestServer`. No new usecases.
