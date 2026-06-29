# flow Kontext cap+rank Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Budget + rank the always-tier (leaf/vorhaben memories) in `Compose` so the SessionStart bootstrap fits the cap, let pinned global memories bypass the D7 tag-gate, and report per-tier drops.

**Architecture:** All memories (leaf, vorhaben, engagement, global) enter one ranked pool sorted `(pinned desc, tierRank asc, updatedAt desc)` and fill greedily until `cap`; only instructions + activeContext stay uncapped. A pinned global memory bypasses the D7 gate. `DroppedCount` grows to per-tier + a pinned subset; the CLI footer surfaces each. Pure-function change + render + a config default; no new constructors.

**Tech Stack:** Go, standard `sort`/`testing`; cobra CLI; pgstore (untouched). Spec: `docs/superpowers/specs/2026-06-29-flow-kontext-cap-rank-design.md`.

## Global Constraints

- **Ranking sort key (exact):** `(pinned desc, tierRank asc, updatedAt desc)`, `tierRank{global:0, engagement:1, vorhaben:2, leaf:3}`. `updatedAt` is RFC3339 and sorts lexicographically (newest = greater string).
- **Uncapped, always loaded:** instructions + activeContext (counted in `Used`, never dropped — even if alone they exceed `cap`).
- **pinned bypasses D7:** a global memory crosses iff `globalAllowed[d.ID] || d.Pinned`. Unpinned global stays tag-gated.
- **Fill is greedy skip-not-break:** an item that doesn't fit is dropped + counted; the loop continues (a single over-size item must not block smaller items behind it).
- **Default cap 6000 → 12000** at both literals: `cmd/flow-server/main.go:250` and `internal/adapter/httpserver/context.go:56`. CLI `--cap` default stays `0` (= "use server default").
- **`DroppedCount` is additive** (new JSON fields, keyed) → backward-compatible for offline cache + MCP `flow_get_context`. No positional `DroppedCount{…}` literals exist in the tree (verified) — safe to add fields.
- **Footer text:** keep `+N engagement not shown` / `+N global not shown` verbatim; add `+N leaf not shown`, `+N vorhaben not shown`, and a loud emoji-free pinned marker `!! N pinned not shown — raise --cap or unpin`.
- **Out of scope (do NOT touch):** Querschnitt-A lifecycle/`veraltet`, D7 node-tagging for unpinned globals, the one 6 246-token oversized memory, the `globalAllowed`/`Execute`/store plumbing.

---

### Task 1: `Compose` — ranked pool, pinned-bypass, per-tier drops

**Files:**
- Modify: `internal/usecase/compose_context.go` (`DroppedCount` struct ~88-91; `Compose` func ~123-224)
- Test: `internal/usecase/compose_context_test.go` (add cases; existing cases stay green)

**Interfaces:**
- Consumes: `domain.Document{ID, NodeID, Type, Pinned, UpdatedAt, Body, Tags}`, `domain.Node{ID, Name, Kind}`, `domain.KindRepo/KindVorhaben/KindEngagement`, `domain.DocInstruction/DocMemory/DocActiveContext`, existing helpers `estTokens`, `itemOf`.
- Produces: `Compose(chain []domain.Node, docs []domain.Document, globalAllowed map[string]bool, cap int) ComposedContext` (signature **unchanged**). New `DroppedCount` fields `Leaf, Vorhaben, Pinned int` (plus existing `Engagement, Global`). `ComposedContext.Memories` still keyed `"leaf"|"vorhaben"|"engagement"|"global"`.

- [ ] **Step 1: Write the failing tests** — append to `internal/usecase/compose_context_test.go`:

```go
func TestCompose_PinnedGlobalBypassesGate(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		doc("gPinned", nil, domain.DocMemory, "g1", true, t0, "always"),  // pinned, NOT in globalAllowed
		doc("gPlain", nil, domain.DocMemory, "g2", false, t0, "topical"), // unpinned, NOT in globalAllowed
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 100000) // empty globalAllowed
	if len(got.Memories["global"]) != 1 || got.Memories["global"][0].ID != "gPinned" {
		t.Fatalf("pinned global must bypass D7; unpinned stays gated: %+v", got.Memories["global"])
	}
}

func TestCompose_TierRankFillOrder(t *testing.T) {
	t.Parallel()
	leaf, vor, eng := "L", "V", "E"
	chain := []domain.Node{
		node(leaf, "flow", domain.KindRepo),
		node(vor, "Vorhaben", domain.KindVorhaben),
		node(eng, "Privat", domain.KindEngagement),
	}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) } // EstTokens = ceil(n/4)
	docs := []domain.Document{
		doc("le", &leaf, domain.DocMemory, "l", false, t0, body(400)),
		doc("vo", &vor, domain.DocMemory, "v", false, t0, body(400)),
		doc("en", &eng, domain.DocMemory, "e", false, t0, body(400)),
		doc("gl", nil, domain.DocMemory, "g", false, t0, body(400)),
	}
	// 4×100-tok items, cap=250 → two highest tiers (global, engagement) fit; vorhaben+leaf drop.
	got := usecase.Compose(chain, docs, map[string]bool{"gl": true}, 250)
	if len(got.Memories["global"]) != 1 || len(got.Memories["engagement"]) != 1 {
		t.Fatalf("global+engagement should survive tight cap: %+v", got.Memories)
	}
	if len(got.Memories["vorhaben"]) != 0 || len(got.Memories["leaf"]) != 0 {
		t.Fatalf("vorhaben+leaf should drop: %+v", got.Memories)
	}
	if got.Budget.Dropped.Vorhaben != 1 || got.Budget.Dropped.Leaf != 1 {
		t.Errorf("want vorhaben=1 leaf=1 dropped, got %+v", got.Budget.Dropped)
	}
}

func TestCompose_PinnedBeatsTier(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) }
	docs := []domain.Document{
		doc("leafPinned", &leaf, domain.DocMemory, "l", true, t0, body(400)), // pinned leaf (rank 3)
		doc("globalPlain", nil, domain.DocMemory, "g", false, t0, body(400)),  // unpinned global (rank 0)
	}
	// cap=100 → exactly one 100-tok item fits; pinned beats tier → leafPinned wins.
	got := usecase.Compose(chain, docs, map[string]bool{"globalPlain": true}, 100)
	if len(got.Memories["leaf"]) != 1 || got.Memories["leaf"][0].ID != "leafPinned" {
		t.Fatalf("pinned leaf must win over unpinned higher-tier global: %+v", got.Memories)
	}
	if got.Budget.Dropped.Global != 1 {
		t.Errorf("unpinned global should drop, got %+v", got.Budget.Dropped)
	}
}

func TestCompose_DroppedPinnedSignaled(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) }
	docs := []domain.Document{
		doc("p1", &leaf, domain.DocMemory, "a", true, t0, body(400)), // pinned 100 tok
		doc("p2", &leaf, domain.DocMemory, "b", true, t0, body(400)), // pinned 100 tok
	}
	// cap=100 → one pinned fits, one drops → Dropped.Pinned=1 AND Dropped.Leaf=1.
	got := usecase.Compose(chain, docs, map[string]bool{}, 100)
	if got.Budget.Dropped.Pinned != 1 {
		t.Errorf("a dropped pin must set Dropped.Pinned, got %+v", got.Budget.Dropped)
	}
	if got.Budget.Dropped.Leaf != 1 {
		t.Errorf("the dropped pin is a leaf → Dropped.Leaf must also count it, got %+v", got.Budget.Dropped)
	}
}

func TestCompose_FloorExceedsCapKeepsAlways(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) }
	docs := []domain.Document{
		doc("instr", &leaf, domain.DocInstruction, "claude", false, t0, body(800)),                 // 200 tok
		doc("ac", &leaf, domain.DocActiveContext, usecase.ActiveContextPath, false, t0, body(400)), // 100 tok
		doc("m", &leaf, domain.DocMemory, "m", false, t0, body(400)),                               // 100 tok
	}
	// cap=50 < instructions(200)+activeContext(100): both always-tier kept, Used>cap, memory dropped.
	got := usecase.Compose(chain, docs, map[string]bool{}, 50)
	if len(got.Instructions) != 1 || got.ActiveContext == nil {
		t.Fatalf("instructions+activeContext must always load over cap: %+v / %+v", got.Instructions, got.ActiveContext)
	}
	if got.Budget.Used != 300 {
		t.Errorf("Used should be the always-tier sum 300, got %d", got.Budget.Used)
	}
	if len(got.Memories["leaf"]) != 0 || got.Budget.Dropped.Leaf != 1 {
		t.Errorf("the leaf memory must drop, got mem=%+v dropped=%+v", got.Memories["leaf"], got.Budget.Dropped)
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test ./internal/usecase/ -run 'TestCompose_(PinnedGlobalBypassesGate|TierRankFillOrder|PinnedBeatsTier|DroppedPinnedSignaled|FloorExceedsCapKeepsAlways)' -v`
Expected: COMPILE ERROR (`got.Budget.Dropped.Leaf`/`.Vorhaben`/`.Pinned` undefined) or FAIL.

- [ ] **Step 3: Extend `DroppedCount`** — replace the struct at `compose_context.go` ~88-91 with:

```go
type DroppedCount struct {
	Leaf       int `json:"leaf"`
	Vorhaben   int `json:"vorhaben"`
	Engagement int `json:"engagement"`
	Global     int `json:"global"`
	Pinned     int `json:"pinned"`
}
```

- [ ] **Step 4: Rewrite `Compose`** — replace the whole `Compose` function (`compose_context.go` ~123-224) with:

```go
// Compose classifies docs, ranks all memories into one pool
// (pinned desc, tierRank asc, updatedAt desc), and fills until the token cap.
// instructions + activeContext are always-tier (counted, never dropped). A pinned
// global memory bypasses the D7 tag-gate. Pure: no I/O.
func Compose(chain []domain.Node, docs []domain.Document, globalAllowed map[string]bool, cap int) ComposedContext {
	out := ComposedContext{Memories: map[string][]ContextItem{}}
	out.Budget.Cap = cap
	if len(chain) > 0 {
		repo := chain[0]
		out.Resolution.Repo = &repo
		out.Resolution.Chain = chain
	} else {
		out.Resolution.Unresolved = true
	}

	// node-id → scope label + tier classification from the chain.
	label := map[string]string{}
	tier := map[string]string{} // "leaf" | "vorhaben" | "engagement"
	for i, n := range chain {
		label[n.ID] = string(n.Kind) + ":" + n.Name
		switch {
		case i == 0:
			tier[n.ID] = "leaf"
		case i == len(chain)-1 && n.Kind == domain.KindEngagement:
			tier[n.ID] = "engagement"
		default:
			tier[n.ID] = "vorhaben"
		}
	}

	// tierRank: lower fills first among equally-pinned items.
	rankOf := map[string]int{"global": 0, "engagement": 1, "vorhaben": 2, "leaf": 3}

	type ranked struct {
		item   ContextItem
		group  string
		pinned bool
		rank   int
		upd    string
	}
	var pool []ranked

	for _, d := range docs {
		switch d.Type {
		case domain.DocInstruction:
			lbl := "global"
			if d.NodeID != nil {
				lbl = label[*d.NodeID]
			}
			out.Instructions = append(out.Instructions, itemOf(d, lbl))
		case domain.DocActiveContext:
			if d.NodeID != nil && tier[*d.NodeID] == "leaf" {
				it := itemOf(d, label[*d.NodeID])
				out.ActiveContext = &it
			}
		case domain.DocMemory:
			if d.NodeID == nil {
				if globalAllowed[d.ID] || d.Pinned { // pinned bypasses the D7 tag-gate
					it := itemOf(d, "global")
					pool = append(pool, ranked{it, "global", d.Pinned, rankOf["global"], it.UpdatedAt})
				}
				continue
			}
			g := tier[*d.NodeID]
			if g == "" {
				continue // node not in chain (defensive)
			}
			it := itemOf(d, label[*d.NodeID])
			pool = append(pool, ranked{it, g, d.Pinned, rankOf[g], it.UpdatedAt})
		}
	}

	// Always-tier (uncapped): instructions + activeContext into Used.
	for _, it := range out.Instructions {
		out.Budget.Used += it.EstTokens
	}
	if out.ActiveContext != nil {
		out.Budget.Used += out.ActiveContext.EstTokens
	}

	// Rank: pinned first, then tierRank (global→engagement→vorhaben→leaf), then newest.
	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].pinned != pool[j].pinned {
			return pool[i].pinned
		}
		if pool[i].rank != pool[j].rank {
			return pool[i].rank < pool[j].rank
		}
		return pool[i].upd > pool[j].upd
	})
	for _, r := range pool {
		if out.Budget.Used+r.item.EstTokens <= cap {
			out.Budget.Used += r.item.EstTokens
			out.Memories[r.group] = append(out.Memories[r.group], r.item)
			continue
		}
		switch r.group {
		case "leaf":
			out.Budget.Dropped.Leaf++
		case "vorhaben":
			out.Budget.Dropped.Vorhaben++
		case "engagement":
			out.Budget.Dropped.Engagement++
		case "global":
			out.Budget.Dropped.Global++
		}
		if r.pinned {
			out.Budget.Dropped.Pinned++
		}
	}
	return out
}
```

> Note: the `cap int` parameter shadows the builtin `cap()` — this matches the existing signature; do **not** rename it. `sort` is already imported.

- [ ] **Step 5: Run the full usecase suite to verify pass**

Run: `go test ./internal/usecase/ -run TestCompose -v`
Expected: PASS — all 5 new tests **and** all 6 pre-existing (`TiersAndActiveContext`, `BudgetDropsRelevanceByRank`, `GlobalGatedByTag`, `UnresolvedNotHandledHere`, `ActiveContextByType`, `SingleEngagementChainLeafTier`).

- [ ] **Step 6: Commit**

```bash
git add internal/usecase/compose_context.go internal/usecase/compose_context_test.go
git commit -m "feat(kontext): rank+cap all memory tiers; pinned global bypasses D7 (cap+rank 1)

Compose now feeds leaf/vorhaben memories through one ranked pool
(pinned desc, tierRank asc, updatedAt desc) instead of the uncapped
always-tier; a pinned global memory bypasses the D7 tag-gate; per-tier
DroppedCount{Leaf,Vorhaben,Engagement,Global,Pinned}.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: CLI footer — per-tier drops + loud pinned marker

**Files:**
- Modify: `cmd/flow/context.go` (`renderContext` footer block ~113-124)
- Test: `cmd/flow/context_test.go` (`TestRenderContext_BodiesAndFooter` ~10-28)

**Interfaces:**
- Consumes: `usecase.ComposedContext.Budget.Dropped.{Leaf,Vorhaben,Engagement,Global,Pinned}` (from Task 1).
- Produces: footer string with per-tier `+N <tier> not shown` lines and `!! N pinned not shown — raise --cap or unpin`.

- [ ] **Step 1: Update the failing test** — replace the body of `TestRenderContext_BodiesAndFooter` in `cmd/flow/context_test.go` (lines ~18-27) with:

```go
	cc.Budget.Used = 1200
	cc.Budget.Cap = 12000
	cc.Budget.Dropped.Leaf = 65
	cc.Budget.Dropped.Vorhaben = 1
	cc.Budget.Dropped.Engagement = 2
	cc.Budget.Dropped.Global = 3
	cc.Budget.Dropped.Pinned = 1
	out := renderContext(cc, false, "")
	for _, want := range []string{
		"RULE A", "where I was", "leaf mem", "1200/12000",
		"+65 leaf not shown", "+1 vorhaben not shown", "+2 engagement not shown", "+3 global not shown",
		"!! 1 pinned not shown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\n---\n%s", want, out)
		}
	}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/flow/ -run TestRenderContext_BodiesAndFooter -v`
Expected: FAIL — `+65 leaf not shown` / `+1 vorhaben not shown` / `!! 1 pinned not shown` absent (renderContext emits only engagement+global).

- [ ] **Step 3: Rewrite the footer block** — replace `renderContext`'s footer (`cmd/flow/context.go` lines ~113-124, from `b.WriteString("\n---\n")` to the `if offline {…}` block inclusive) with:

```go
	b.WriteString("\n---\n")
	fmt.Fprintf(&b, "%d/%d tokens", cc.Budget.Used, cc.Budget.Cap)
	for _, d := range []struct {
		n     int
		label string
	}{
		{cc.Budget.Dropped.Leaf, "leaf"},
		{cc.Budget.Dropped.Vorhaben, "vorhaben"},
		{cc.Budget.Dropped.Engagement, "engagement"},
		{cc.Budget.Dropped.Global, "global"},
	} {
		if d.n > 0 {
			fmt.Fprintf(&b, " · +%d %s not shown", d.n, d.label)
		}
	}
	if cc.Budget.Dropped.Pinned > 0 {
		fmt.Fprintf(&b, " · !! %d pinned not shown — raise --cap or unpin", cc.Budget.Dropped.Pinned)
	}
	if offline {
		fmt.Fprintf(&b, " · offline — Stand %s", stamp)
	}
	b.WriteString("\n")
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/flow/ -run TestRenderContext -v`
Expected: PASS (both `BodiesAndFooter` and the untouched `UnboundHintAndOffline`).

- [ ] **Step 5: Commit**

```bash
git add cmd/flow/context.go cmd/flow/context_test.go
git commit -m "feat(kontext): per-tier drop footer + loud pinned-drop marker (cap+rank 2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Default cap 6000 → 12000

**Files:**
- Modify: `cmd/flow-server/main.go:250` (`contextBudget` fallback)
- Modify: `internal/adapter/httpserver/context.go:56` (handler fallback)
- Test: `cmd/flow-server/context_budget_test.go` (new)

**Interfaces:**
- Consumes: existing `contextBudget(getenv func(string) string) int` (`cmd/flow-server/main.go:244`).
- Produces: default budget `12000` end-to-end (env `FLOW_CONTEXT_BUDGET` still overrides).

- [ ] **Step 1: Write the failing test** — create `cmd/flow-server/context_budget_test.go`:

```go
package main

import "testing"

func TestContextBudget_DefaultAndEnvOverride(t *testing.T) {
	if got := contextBudget(func(string) string { return "" }); got != 12000 {
		t.Errorf("default budget = %d, want 12000", got)
	}
	getenv := func(k string) string {
		if k == "FLOW_CONTEXT_BUDGET" {
			return "5000"
		}
		return ""
	}
	if got := contextBudget(getenv); got != 5000 {
		t.Errorf("env override = %d, want 5000", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/flow-server/ -run TestContextBudget_DefaultAndEnvOverride -v`
Expected: FAIL — `default budget = 6000, want 12000`.

- [ ] **Step 3: Change both default literals**

In `cmd/flow-server/main.go:250`, change `return 6000` → `return 12000`.
In `internal/adapter/httpserver/context.go:56`, change `budget = 6000` → `budget = 12000` (keep the two defaults in sync; also update the comment on `server.go:99` `0 → fall back to 6000` → `0 → fall back to 12000`).

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cmd/flow-server/ -run TestContextBudget_DefaultAndEnvOverride -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/flow-server/main.go cmd/flow-server/context_budget_test.go internal/adapter/httpserver/context.go internal/adapter/httpserver/server.go
git commit -m "feat(kontext): default context budget 6000 -> 12000 (cap+rank 3)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Done-Gate — `make ci` + dev-stack wiring smoke

**Files:** none (verification only).

**Interfaces:** Consumes everything above end-to-end.

- [ ] **Step 1: Full CI**

Run: `make ci`
Expected: PASS — `lint verify-generate verify-css verify-no-popups cover build` all green, coverage gate held. This is the hard gate: all 5 new + 6 existing Compose cases, the footer test, and the contextBudget test run here.

- [ ] **Step 2: Bring up the dev stack** (per `reference_flow_dev_env`; needs Postgres + Dex)

Run: `make dev-up` then `make dev-run` (leave the server running in a second shell or background).
Expected: flow-server up against dev Postgres+Dex; logs show migrations applied, no error.

- [ ] **Step 3: Wiring smoke — default cap is 12000 over the real CLI→server path**

Run (sources the dev CLI env so `flow` points at the dev server, logs in, then composes):
```bash
make dev-login
set -a; . deploy/dev/flow-cli.env; set +a
go run ./cmd/flow context --json | jq '{cap: .budget.cap, dropped: .budget.dropped, unresolved: .resolution.unresolved}'
```
Expected: `cap` is `12000` (proves main.go default → `Server.ContextBudget` → handler), and `dropped` serializes the new keys `{leaf,vorhaben,engagement,global,pinned}` (additive struct reaches JSON). On an empty/unbound dev repo `unresolved` is `true` — that is fine; this step verifies wiring + serialization, not corpus behavior (corpus behavior is unit-tested in Task 1 and PROD-dogfooded below).

- [ ] **Step 4: Tear down**

Run: `make dev-down`
Expected: clean stop.

- [ ] **Step 5: Record deferred PROD acceptance** (Soenne, after deploy — NOT part of this branch)

The live ~12k bootstrap against the real 74-memory corpus only appears once the new `flow-server` image is deployed to PROD. After deploy, `flow context` should show: instructions + activeContext + 8 global-pins + 4 leaf-pins + 1 engagement + ~5 newest leaf, `Used` ≈ 12k, footer `+~65 leaf not shown`. Calibrate the cap at that footer (D8). Note this in the handoff activeContext; do not block this branch on it.

---

## Self-Review

**1. Spec coverage:**
- Beschluss 1 (uncapped instructions+activeContext) → Task 1 (`FloorExceedsCapKeepsAlways` test + always-tier `Used` loop).
- Beschluss 2 (one ranked pool, sort key) → Task 1 (`TierRankFillOrder`, `PinnedBeatsTier`, sort in `Compose`).
- Beschluss 3 (pinned bypasses D7) → Task 1 (`PinnedGlobalBypassesGate`, `|| d.Pinned`).
- Beschluss 4 (pins respect cap + loud marker) → Task 1 (`DroppedPinnedSignaled`) + Task 2 (footer `!! N pinned`).
- Beschluss 5 (DroppedCount per tier + footer) → Task 1 (struct + counting) + Task 2 (footer lines).
- Beschluss 6 (cap 12000) → Task 3.
- Done-Gate (make ci + live smoke) → Task 4.
All spec sections map to a task. No gaps.

**2. Placeholder scan:** No TBD/TODO; every code step carries complete code; every run-step has an exact command + expected output. None of the forbidden patterns present.

**3. Type consistency:** `DroppedCount.{Leaf,Vorhaben,Engagement,Global,Pinned}` defined in Task 1 are exactly the names read in Task 2's footer and test. `Compose` signature unchanged. `contextBudget` name matches the existing function. Memory group keys `"leaf"|"vorhaben"|"engagement"|"global"` consistent between `Compose`, the footer loop, and tests.
