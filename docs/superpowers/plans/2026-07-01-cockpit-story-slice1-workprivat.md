# Cockpit-Story Slice 1 — Work/Privat-Modell (vererbt + Rollup-Split) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Den per-Node `countsTowardTarget`-Flag von einem *pro-Knoten unabhängigen* `bool` auf ein **vererbtes** Modell (effektiver Flag = nächster explizit gesetzter Vorfahr, wie die Rate) heben, den Node-Rollup **Work/Privat splitten**, das Soll konsistent halten, und einen **Tri-State-Toggle** ins WebUI-Node-Formular bringen.

**Architecture:** Hexagonal (`internal/domain` / `usecase` / `ports` / `adapter`). Domain-Feld `CountsTowardTarget bool` → `*bool` (nil = erbt); neue reine Domain-Funktion `ResolveCountsTowardTarget(chain)` spiegelt `ResolveRate`. `NodeStats` bucketet Subtree-Sessions per effektivem Flag (Basis via `Ancestors`, dann top-down über den Subtree). Das Soll (`StatsComputer.countsTowardFn`) löst künftig effektiv auf. Ein neuer `SetCountsTowardTarget`-Usecase (always-apply, spiegelt `SetNodeRate`) bedient den Formular-Toggle, damit die REST-„nil=preserve"-Semantik unangetastet bleibt.

**Tech Stack:** Go, pgx/pgstore + goose-Migrationen, templ + Tailwind (WebUI), `make ci`/`make web`/`make generate`.

## Global Constraints
- Branch: off `rebuild` (Feature-Branch, beim Execution-Handoff festgelegt). Spec: `docs/superpowers/specs/2026-07-01-cockpit-story-rework-design.md` §5.1/§7.
- **Vererbungs-Default:** ist die ganze Ahnenkette `nil` (erbt), gilt **`true` = Work/zählt** (Root ohne expliziten Flag zählt). Spiegelt das bisherige „alles zählt".
- **Migrationen** brauchen `-- +goose Up`/`-- +goose Down` (sonst „unexpected state 0" beim Apply; nur pgstore-Docker-Tests fangen das) — [[feedback_pgstore_goose_migrations]].
- `make ci` grün pro Task (Gate **75 %**, `*_templ.go` **ausgeschlossen** — echte Tests, kein Padding). Nach `.templ`: `make generate` + `_templ.go` committen; nach neuen Tailwind-Utilities: `make web` + `app.css` committen. **Kein `make fmt`** (Toolchain-Skew formatiert das ganze Repo um). Keine Emojis/Popups. Events via `s.Emitter.Emit` (nicht `Bus.Publish`).
- Nächste freie Migration-Nummer: **0025** (0023 = counts_toward_target, 0024 = activity).

---

## File Structure
**Modify:**
- `internal/domain/node.go` — `CountsTowardTarget` → `*bool`; `NewNode` Default entfernen; `ResolveCountsTowardTarget`.
- `internal/usecase/create_node.go`, `update_node.go` — Pointer-Zuweisung an das jetzt-`*bool`-Feld.
- `internal/adapter/pgstore/migrations/0025_nodes_counts_toward_target_nullable.sql` — **Create**.
- `internal/adapter/pgstore/nodes.go` — Create/Update binden `*bool`; `scanNode` scannt `*bool`. (nodeCols/CTEs listen die Spalte bereits.)
- `internal/testutil/fakes.go` — nur Feld-Typ (voller Struct-Copy trägt es).
- `internal/domain/node_rollup.go` — Work/Privat-Split-Felder.
- `internal/usecase/node_stats.go` — effektives Bucketing.
- `internal/usecase/stats_computer.go` — `countsTowardFn` löst effektiv auf.
- `internal/usecase/set_counts_toward_target.go` — **Create** (neuer Usecase).
- `internal/adapter/httpserver/server.go`, `cmd/flow-server/main.go` — `SetCountsTowardTarget` verdrahten.
- `internal/adapter/webui/node_tree_vm.go` — `NodeFormValues.CountsMode`.
- `internal/adapter/webui/nodes.templ` — Tri-State-Control.
- `internal/adapter/httpserver/webui_nodes.go` — `nodeFormValues` liest `countsMode`; Create/Update/Edit verdrahten.
- `internal/i18n/catalog_de.go`, `catalog_en.go` — neue Keys.
- `internal/adapter/httpserver/nodes_extra.go` (oder wo `handleNodeStats`/`nodeRollupDTO` lebt) — DTO um Work-Felder erweitern (optional, für REST-Vollständigkeit).

---

## Task 1: Domain-Feld `*bool` + `ResolveCountsTowardTarget` + Migration + Storage

Atomarer Type-Change (`bool`→`*bool`) end-to-end — kompiliert nur als Einheit.

**Files:** wie oben (node.go, create_node.go, update_node.go, 0025-Migration, pgstore/nodes.go, fakes.go). Test: `internal/domain/node_test.go`, `internal/adapter/pgstore/nodes_test.go` (oder das bestehende pgstore-Node-Test-File).

**Interfaces:**
- Produces: `domain.Node.CountsTowardTarget *bool` (nil = erbt); `domain.ResolveCountsTowardTarget(chain []domain.Node) bool` (leaf→root, erster non-nil gewinnt, sonst `true`).

- [ ] **Step 1: Failing domain test** — `internal/domain/node_test.go` (anhängen)

```go
func TestResolveCountsTowardTarget(t *testing.T) {
	b := func(v bool) *bool { return &v }
	// leaf→root chains (as NodeStore.Ancestors returns): [0]=self … [n]=root
	cases := []struct {
		name  string
		chain []domain.Node
		want  bool
	}{
		{"all nil → default true", []domain.Node{{}, {}}, true},
		{"self explicit privat wins", []domain.Node{{CountsTowardTarget: b(false)}, {CountsTowardTarget: b(true)}}, false},
		{"inherit from parent privat", []domain.Node{{CountsTowardTarget: nil}, {CountsTowardTarget: b(false)}}, false},
		{"nearest ancestor wins", []domain.Node{{CountsTowardTarget: nil}, {CountsTowardTarget: b(true)}, {CountsTowardTarget: b(false)}}, true},
		{"empty chain → true", nil, true},
	}
	for _, c := range cases {
		if got := domain.ResolveCountsTowardTarget(c.chain); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run — fails to compile** (`ResolveCountsTowardTarget` undefined; `CountsTowardTarget` is `bool` not `*bool`)

Run: `go test ./internal/domain/ -run TestResolveCountsTowardTarget`
Expected: FAIL (undefined / type mismatch).

- [ ] **Step 3: Domain change** — `internal/domain/node.go`

Change the field (line ~47):
```go
	// CountsTowardTarget: nil = erbt (nächster expliziter Vorfahr entscheidet);
	// *true = Work (zählt aufs Soll); *false = Privat (nur getrackt).
	CountsTowardTarget *bool `json:"countsTowardTarget,omitempty"`
```
In `NewNode` (line ~63-66) **remove** the `CountsTowardTarget: true,` line (leave nil = erbt):
```go
	return Node{
		ID: id, OwnerID: ownerID, Name: name, Slug: slug,
		Status: NodeActive, CreatedAt: now, UpdatedAt: now,
	}, nil
```
Add the resolver (mirror `ResolveRate`, near it):
```go
// ResolveCountsTowardTarget returns the effective Work/Privat flag for a node by
// walking its ancestor chain (leaf→root, as NodeStore.Ancestors returns): the
// nearest node with an explicit value wins. All-nil (or empty) → true (Work),
// so an unconfigured tree counts toward the Soll like before.
func ResolveCountsTowardTarget(chain []Node) bool {
	for _, n := range chain {
		if n.CountsTowardTarget != nil {
			return *n.CountsTowardTarget
		}
	}
	return true
}
```

- [ ] **Step 4: Fix the two usecases** (the field is now `*bool`)

`internal/usecase/create_node.go` — replace the `if in.CountsTowardTarget != nil { n.CountsTowardTarget = *in.CountsTowardTarget }` block with a direct pointer assign (nil = erbt, which is also NewNode's default):
```go
	n.CountsTowardTarget = in.CountsTowardTarget
```
`internal/usecase/update_node.go` — KEEP the nil=preserve convention for REST (do not break it), but the field is now `*bool`, so the assign is a pointer copy:
```go
	if in.CountsTowardTarget != nil {
		p.CountsTowardTarget = in.CountsTowardTarget
	}
```
(`in.CountsTowardTarget` is `*bool`; `p.CountsTowardTarget` is now `*bool` — assign the pointer. nil-input still means "preserve" here; the WebUI uses `SetCountsTowardTarget` (Task 4) for explicit set-to-inherit.)

- [ ] **Step 5: Migration 0025** — `internal/adapter/pgstore/migrations/0025_nodes_counts_toward_target_nullable.sql`

```sql
-- +goose Up
ALTER TABLE nodes ALTER COLUMN counts_toward_target DROP NOT NULL;
ALTER TABLE nodes ALTER COLUMN counts_toward_target SET DEFAULT NULL;
-- Existing rows are all the old default `true`; treat them as "inherit" so the
-- new inheritance model works out of the box. Explicit `false` (Privat) is kept.
UPDATE nodes SET counts_toward_target = NULL WHERE counts_toward_target = true;

-- +goose Down
UPDATE nodes SET counts_toward_target = true WHERE counts_toward_target IS NULL;
ALTER TABLE nodes ALTER COLUMN counts_toward_target SET DEFAULT true;
ALTER TABLE nodes ALTER COLUMN counts_toward_target SET NOT NULL;
```

- [ ] **Step 6: pgstore `*bool` binding** — `internal/adapter/pgstore/nodes.go`

`Create` and `Update` already pass `n.CountsTowardTarget` as the bind value; since the field is now `*bool`, pgx binds it as a nullable boolean — **no change to the arg lists needed** (they already reference `n.CountsTowardTarget`). In `scanNode`, the scan target `&n.CountsTowardTarget` is now `**bool`→ scans a nullable bool into `*bool` — **also unchanged** (pgx scans SQL NULL into a nil `*bool`). VERIFY by reading: `Create` binds `... n.CountsTowardTarget)`, `Update` binds `... n.CountsTowardTarget, n.UpdatedAt ...`, `scanNode` scans `... &n.CountsTowardTarget)`. If pgx rejects scanning NULL into the field, wrap with a local `var ctt *bool` + assign; otherwise leave as-is. (nodeCols + Ancestors/Subtree CTEs already list `counts_toward_target` — no CTE edits.)

- [ ] **Step 7: FakeNodeStore** — `internal/testutil/fakes.go`

`FakeNodeStore` stores the whole `domain.Node` by value (Create) and copies `existing.CountsTowardTarget = p.CountsTowardTarget` in `Update` — both now carry a `*bool` transparently. **No change** beyond the code compiling against the new field type. VERIFY the `Update` copy line still compiles.

- [ ] **Step 8: Add a pgstore round-trip test** — the node pgstore test file (find it: `rg -l "func Test" internal/adapter/pgstore | rg -i node`)

```go
func TestNodeStore_CountsTowardTargetNullable(t *testing.T) {
	// (use the existing pgstore test harness in this file: pool + NodeStore + a seeded owner)
	st := newTestNodeStore(t) // reuse whatever constructor the file already uses
	ctx := context.Background()
	mk := func(id string, ctt *bool) domain.Node {
		n, _ := domain.NewNode(id, "u1", id, id, time.Now())
		n.Kind = domain.KindEngagement
		n.CountsTowardTarget = ctt
		return n
	}
	tt := true
	got, err := st.Create(ctx, mk("n-inherit", nil))
	if err != nil { t.Fatal(err) }
	if got.CountsTowardTarget != nil { t.Errorf("nil must persist as NULL, got %v", *got.CountsTowardTarget) }
	got2, _ := st.Create(ctx, mk("n-work", &tt))
	if got2.CountsTowardTarget == nil || *got2.CountsTowardTarget != true { t.Errorf("explicit true lost") }
}
```
> Adapt the harness/constructor names to what the file already has (it has a working pgstore NodeStore test; mirror its setup). If pgstore tests are Docker-gated and unavailable, at minimum add the equivalent `FakeNodeStore` round-trip in `internal/testutil` and note the pgstore test for the live gate.

- [ ] **Step 9: Generate/build/test/ci**

Run: `go build ./... && go test ./internal/domain/ ./internal/usecase/ -run "CountsToward|Node" && make ci`
Expected: green (make ci runs pgstore Docker tests → migration applies cleanly).

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "feat(node): countsTowardTarget nullable *bool + ResolveCountsTowardTarget (inherited) + migration 0025"
```

---

## Task 2: NodeRollup Work/Privat-Split + NodeStats effektives Bucketing

**Files:** Modify `internal/domain/node_rollup.go`, `internal/usecase/node_stats.go`. Test: `internal/usecase/node_stats_test.go` (find/create).

**Interfaces:**
- Consumes: `domain.ResolveCountsTowardTarget`, `Nodes.Subtree` (root→leaf, depth-ordered), `Nodes.Ancestors`.
- Produces: `domain.NodeRollup` gains `WorkTotal, WorkWeek, WorkMonth time.Duration` (Privat = Total − Work, derived by callers).

- [ ] **Step 1: Failing test** — `internal/usecase/node_stats_test.go`

```go
func TestNodeStats_WorkPrivatSplit(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ss := testutil.NewFakeSessionStore()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 1, 12, 0, 0, 0, time.Local)}
	b := func(v bool) *bool { return &v }
	mk := func(id string, parent *string, ctt *bool) {
		n, _ := domain.NewNode(id, "u1", id, id, clk.Now())
		n.ParentID = parent; n.CountsTowardTarget = ctt; n.Kind = domain.KindRepo
		_, _ = ns.Create(ctx, n)
	}
	eng := "eng"; mk(eng, nil, b(false))      // engagement explicitly Privat
	rp := "repo"; mk(rp, &eng, nil)           // repo inherits → Privat
	rp2 := "repo2"; mk(rp2, &eng, b(true))    // repo explicit Work (override)
	// 2h on the inherited-privat repo, 1h on the work-override repo
	add := func(node string, h int) {
		st := clk.Now().Add(time.Duration(-h) * time.Hour)
		_, _ = usecase.AddSession{Sessions: ss, Nodes: ns, IDs: &testutil.FakeIDGen{}, Clock: clk}.
			Execute(ctx, "u1", &node, st, clk.Now(), nil, "")
	}
	add(rp, 2); add(rp2, 1)

	sc := usecase.StatsComputer{Sessions: ss, Nodes: ns, Clock: clk, Loc: time.Local}
	r, err := sc.NodeStats(ctx, "u1", eng)
	if err != nil { t.Fatal(err) }
	if r.Total != 3*time.Hour { t.Errorf("Total=%v want 3h", r.Total) }
	if r.WorkTotal != 1*time.Hour { t.Errorf("WorkTotal=%v want 1h (only repo2 counts)", r.WorkTotal) }
	// Privat = Total - Work = 2h
}
```

- [ ] **Step 2: Run — fails** (`WorkTotal` undefined). `go test ./internal/usecase/ -run TestNodeStats_WorkPrivatSplit` → FAIL.

- [ ] **Step 3: NodeRollup split** — `internal/domain/node_rollup.go`

```go
type NodeRollup struct {
	Total time.Duration
	Week  time.Duration
	Month time.Duration
	// Work* = subset that counts toward the Soll (effective flag = Work).
	// Privat is derived as Total-Work / Week-WorkWeek / Month-WorkMonth.
	WorkTotal time.Duration
	WorkWeek  time.Duration
	WorkMonth time.Duration
}
```

- [ ] **Step 4: NodeStats effektiv bucketen** — `internal/usecase/node_stats.go`

Replace the body so it computes the effective flag per subtree node (base from ancestors, then top-down over the depth-ordered subtree), then buckets Work separately:

```go
func (c StatsComputer) NodeStats(ctx context.Context, ownerID, nodeID string) (domain.NodeRollup, error) {
	sub, err := c.Nodes.Subtree(ctx, ownerID, nodeID)
	if err != nil {
		return domain.NodeRollup{}, err
	}
	if len(sub) == 0 {
		return domain.NodeRollup{}, ports.ErrNodeNotFound
	}
	// Effective Work/Privat flag per subtree node. Base = resolved from the
	// cockpit node's ancestors (covers "all-nil inherits from above the subtree").
	anc, _ := c.Nodes.Ancestors(ctx, ownerID, nodeID)
	base := domain.ResolveCountsTowardTarget(anc)
	eff := make(map[string]bool, len(sub))
	for _, n := range sub { // Subtree is depth-ordered root→leaf: parents precede children
		if n.CountsTowardTarget != nil {
			eff[n.ID] = *n.CountsTowardTarget
		} else if n.ParentID != nil {
			if pv, ok := eff[*n.ParentID]; ok {
				eff[n.ID] = pv
			} else {
				eff[n.ID] = base
			}
		} else {
			eff[n.ID] = base
		}
	}
	ids := make(map[string]bool, len(sub))
	for _, n := range sub {
		ids[n.ID] = true
	}
	sessions, err := c.Sessions.List(ctx, ownerID, time.Time{})
	if err != nil {
		return domain.NodeRollup{}, err
	}
	loc := c.Loc
	if loc == nil {
		loc = time.Local
	}
	now := c.Clock.Now().In(loc)
	weekStart := isoMondayLocal(now)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	var r domain.NodeRollup
	for _, s := range sessions {
		if s.NodeID == nil || !ids[*s.NodeID] {
			continue
		}
		el := s.Elapsed(now)
		if el < 0 {
			el = 0
		}
		work := eff[*s.NodeID]
		st := s.Start.In(loc)
		inWeek := !st.Before(weekStart)
		inMonth := !st.Before(monthStart)
		r.Total += el
		if inWeek {
			r.Week += el
		}
		if inMonth {
			r.Month += el
		}
		if work {
			r.WorkTotal += el
			if inWeek {
				r.WorkWeek += el
			}
			if inMonth {
				r.WorkMonth += el
			}
		}
	}
	return r, nil
}
```

- [ ] **Step 5: Run — passes.** `go test ./internal/usecase/ -run TestNodeStats -v` → PASS.

- [ ] **Step 6: (Optional but recommended) expose Work in the REST DTO** — the `nodeRollupDTO` + `handleNodeStats` (grep `nodeRollupDTO`): add `WorkTotalMin/WorkWeekMin/WorkMonthMin`. If the cockpit will consume via `s.Stats.NodeStats` directly (Slice 4), this is only for API completeness — keep it if trivial, else note as deferred. Add a test asserting the DTO carries the work minutes.

- [ ] **Step 7: ci + commit**

```bash
make ci && git add -A
git commit -m "feat(stats): NodeStats splits subtree rollup into Work/Privat by effective flag"
```

---

## Task 3: Soll-Konsistenz — `countsTowardFn` löst effektiv auf

Der tägliche Soll/Saldo muss dasselbe effektive Vererbungsmodell nutzen (sonst zählt ein geerbt-Privat-Knoten fälschlich).

**Files:** Modify `internal/usecase/stats_computer.go` (`countsTowardFn`). Test: `internal/usecase/stats_computer_test.go`.

- [ ] **Step 1: Failing test** — assert a node that INHERITS Privat from its engagement does NOT count toward the Soll.

```go
func TestCountsTowardFn_Inherited(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 1, 12, 0, 0, 0, time.Local)}
	b := func(v bool) *bool { return &v }
	mkN := func(id string, parent *string, ctt *bool) {
		n, _ := domain.NewNode(id, "u1", id, id, clk.Now())
		n.ParentID = parent; n.CountsTowardTarget = ctt; n.Kind = domain.KindRepo
		_, _ = ns.Create(ctx, n)
	}
	eng := "eng"; mkN(eng, nil, b(false)) // Privat engagement
	rp := "repo"; mkN(rp, &eng, nil)      // inherits Privat
	sc := usecase.StatsComputer{Nodes: ns, Clock: clk, Loc: time.Local}
	fn, err := sc.CountsTowardFnForTest(ctx, "u1") // see note on exporting for test
	if err != nil { t.Fatal(err) }
	if fn(&rp) { t.Errorf("repo inheriting Privat must NOT count toward Soll") }
	if !fn(nil) { t.Errorf("unbooked time must count") }
}
```
> `countsTowardFn` is unexported. Either (a) add a tiny exported test seam `func (c StatsComputer) CountsTowardFnForTest(...)` that calls it, or (b) test it indirectly through `Today`/`RangeStats` by asserting `Saldo`. Prefer (b) if the file already has a `Today`/`RangeStats` test harness (assert the Privat-inherited session is excluded from `TargetTotal`); use (a) only if no such harness exists. Pick one and make the assertion real.

- [ ] **Step 2: Run — fails** (current `countsTowardFn` reads the raw `*bool` and would treat nil as… — actually `flag[n.ID] = n.CountsTowardTarget` no longer compiles since the field is `*bool` not `bool`; the test also fails on the inheritance). `go test ./internal/usecase/ -run TestCountsTowardFn_Inherited` → FAIL/compile error.

- [ ] **Step 3: Resolve effectively** — `internal/usecase/stats_computer.go`, rewrite `countsTowardFn`

```go
func (c StatsComputer) countsTowardFn(ctx context.Context, ownerID string) (func(*string) bool, error) {
	if c.Nodes == nil {
		return func(*string) bool { return true }, nil
	}
	nodes, err := c.Nodes.List(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	// effective flag per node = ResolveCountsTowardTarget over its ancestor chain,
	// walked in-memory (leaf→root); memoized.
	eff := make(map[string]bool, len(nodes))
	var resolve func(id string) bool
	resolve = func(id string) bool {
		if v, ok := eff[id]; ok {
			return v
		}
		n, ok := byID[id]
		if !ok {
			return true // unknown → count (legacy-safe)
		}
		var v bool
		if n.CountsTowardTarget != nil {
			v = *n.CountsTowardTarget
		} else if n.ParentID != nil {
			v = resolve(*n.ParentID)
		} else {
			v = true // root, all-nil → Work
		}
		eff[id] = v
		return v
	}
	return func(id *string) bool {
		if id == nil {
			return true // unbooked time counts
		}
		return resolve(*id)
	}, nil
}
```

- [ ] **Step 4: Run — passes.** `go test ./internal/usecase/ -run "CountsToward|Today|Range|Burndown" -v` → PASS (verify existing stats tests still green — behavior unchanged for all-Work trees).

- [ ] **Step 5: ci + commit**

```bash
make ci && git add -A
git commit -m "fix(stats): Soll countsToward resolves the effective (inherited) Work/Privat flag"
```

---

## Task 4: WebUI Node-Formular Tri-State-Toggle + `SetCountsTowardTarget`-Usecase

Das Formular bekommt „Zählt zum Soll: erbt / Work / Privat". Weil `UpdateNodeInput` nil=preserve bedeutet (REST-Semantik), setzt ein **neuer always-apply-Usecase** den Flag beim Speichern (spiegelt `SetNodeRate`).

**Files:** Create `internal/usecase/set_counts_toward_target.go`; Modify `server.go` + `cmd/flow-server/main.go` (verdrahten), `node_tree_vm.go`, `nodes.templ`, `webui_nodes.go`, `catalog_de.go`/`catalog_en.go`. Tests: `internal/usecase/set_counts_toward_target_test.go`, `internal/adapter/httpserver/webui_nodes_test.go` (o.ä.).

**Interfaces:**
- Produces: `usecase.SetCountsTowardTarget{Nodes ports.NodeStore, Clock ports.Clock}` `.Execute(ctx, ownerID, id string, mode *bool) (domain.Node, error)` — always applies `mode` (nil/true/false).

- [ ] **Step 1: Failing usecase test** — `set_counts_toward_target_test.go`

```go
func TestSetCountsTowardTarget(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	clk := testutil.FakeClock{T: time.Now()}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindRepo
	tt := true; n.CountsTowardTarget = &tt // start explicit Work
	_, _ = ns.Create(ctx, n)
	uc := usecase.SetCountsTowardTarget{Nodes: ns, Clock: clk}
	got, err := uc.Execute(ctx, "u1", "n1", nil) // set to inherit
	if err != nil { t.Fatal(err) }
	if got.CountsTowardTarget != nil { t.Errorf("expected nil (inherit), got %v", *got.CountsTowardTarget) }
	pv := false
	got2, _ := uc.Execute(ctx, "u1", "n1", &pv) // set Privat
	if got2.CountsTowardTarget == nil || *got2.CountsTowardTarget != false { t.Errorf("expected explicit false") }
}
```

- [ ] **Step 2: Run — fails** (undefined). → FAIL.

- [ ] **Step 3: Implement** — `internal/usecase/set_counts_toward_target.go`

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SetCountsTowardTarget always sets a node's Work/Privat override (nil = inherit,
// *true = Work, *false = Privat). Unlike UpdateNodeInput (nil = preserve), this
// applies the value verbatim — the WebUI tri-state control uses it so "set to
// inherit" is expressible. Mirrors SetNodeRate's always-apply shape.
type SetCountsTowardTarget struct {
	Nodes ports.NodeStore
	Clock ports.Clock
}

func (uc SetCountsTowardTarget) Execute(ctx context.Context, ownerID, id string, mode *bool) (domain.Node, error) {
	n, err := uc.Nodes.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Node{}, err
	}
	n.CountsTowardTarget = mode
	n.UpdatedAt = uc.Clock.Now()
	return uc.Nodes.Update(ctx, ownerID, n)
}
```

- [ ] **Step 4: Wire the usecase** — `server.go` (add field near `SetNodeRate`): `SetCountsTowardTarget usecase.SetCountsTowardTarget` ; `cmd/flow-server/main.go` (near `SetNodeRate: usecase.SetNodeRate{...}`): `SetCountsTowardTarget: usecase.SetCountsTowardTarget{Nodes: nodeStore, Clock: clock},` (use the same store/clock identifiers as `SetNodeRate`).

- [ ] **Step 5: i18n keys** (de+en parity) — add to both catalogs:
```
"node.counts.label":   "Zählt zum Soll" / "Counts toward target"
"node.counts.inherit": "erbt (vom Eltern-Knoten)" / "inherit (from parent)"
"node.counts.work":    "Work — zählt" / "Work — counts"
"node.counts.privat":  "Privat — nur tracken" / "Private — track only"
```

- [ ] **Step 6: Form VM + reader** — `node_tree_vm.go`: add `CountsMode string` to `NodeFormValues` (`""`/`"inherit"` = erbt, `"work"`, `"privat"`). `webui_nodes.go` `nodeFormValues`: add `CountsMode: r.FormValue("countsMode"),`. `handleWebNodeEdit`: set `vals.CountsMode = countsModeOf(n.CountsTowardTarget)` where a small helper maps `nil→"inherit"`, `*true→"work"`, `*false→"privat"` (add the helper in `webui_nodes.go`).

- [ ] **Step 7: templ control** — `nodes.templ`, mirror the Status `<select>` (add near it):
```templ
		<div>
			<label class="block text-muted">{ components.T(ctx, "node.counts.label") }</label>
			<select name="countsMode" class="rounded-lg border border-line bg-surface px-3 py-2">
				@nodeCountsOption("inherit", "node.counts.inherit", d.Vals.CountsMode)
				@nodeCountsOption("work", "node.counts.work", d.Vals.CountsMode)
				@nodeCountsOption("privat", "node.counts.privat", d.Vals.CountsMode)
			</select>
		</div>
```
```templ
templ nodeCountsOption(val, labelKey, current string) {
	if val == current || (current == "" && val == "inherit") {
		<option value={ val } selected>{ components.T(ctx, labelKey) }</option>
	} else {
		<option value={ val }>{ components.T(ctx, labelKey) }</option>
	}
}
```

- [ ] **Step 8: Handlers apply it** — `webui_nodes.go`

Add a helper mapping the form string → `*bool`:
```go
// countsModeToPtr maps the tri-state form value to the *bool override.
func countsModeToPtr(mode string) *bool {
	switch mode {
	case "work":
		t := true
		return &t
	case "privat":
		f := false
		return &f
	default: // "inherit" / ""
		return nil
	}
}
```
`handleWebNodeCreate`: pass it into `CreateNodeInput{... CountsTowardTarget: countsModeToPtr(vals.CountsMode)}`.
`handleWebNodeUpdate`: after a successful `UpdateNode.Execute`, call `_, _ = s.SetCountsTowardTarget.Execute(r.Context(), u.ID, id, countsModeToPtr(vals.CountsMode))` (always-apply, like the `SetNodeRate` call just below it).

- [ ] **Step 9: Handler test** — `webui_nodes_test.go` (mirror the harness there): POST create with `countsMode=privat` → the created node has `*false`; POST edit `countsMode=inherit` on a Work node → node ends `nil`. Assert via `GetNode`/store. Also assert the edit form GET renders the select with the current value selected.

- [ ] **Step 10: generate + web + ci + commit**

```bash
make generate && make web && make ci
git add -A
git commit -m "feat(webui): node form Work/Privat tri-state toggle + SetCountsTowardTarget usecase"
```

---

## Task 5: Wiring-Verifikation + Done-Gate

- [ ] **Step 1: Wiring audit** — `SetCountsTowardTarget` reachable from the composition root:
```bash
rg -n "SetCountsTowardTarget" internal/adapter/httpserver/server.go cmd/flow-server/main.go internal/adapter/httpserver/webui_nodes.go
```
Expected: struct field + main wiring + the handler call (≥3).

- [ ] **Step 2: Full ci (incl. web + pgstore migration)** — `make generate && make web && make ci`. Expected green; record coverage % (≥75, `*_templ.go` excluded).

- [ ] **Step 3: Live done-gate (dev stack: Postgres + Dex)** — `make dev-up && make dev-run` (+ `make dev-token`/login), then via the API/WebUI:
  1. Migration applied: existing nodes now have `counts_toward_target IS NULL` (inherit); an explicit-Privat node keeps `false`.
  2. Create a node with `countsMode=privat` → its subtree time shows under **Privat**; `NodeStats` `WorkTotal` excludes it.
  3. Mark an engagement Privat → a child repo with "inherit" rolls up as Privat (cockpit rollup + Soll both agree).
  4. Daily Soll/Saldo unchanged for an all-Work tree; drops the Privat subtree's time.
  5. Edit a node Work→inherit→Privat via the form; the select round-trips.

- [ ] **Step 4: Holistic review (Opus)** — whole-slice review (BASE = branch start). Focus: the `bool→*bool` blast radius (every scan/bind site), the effective-resolution correctness (base-from-ancestors + top-down), Soll behavior parity for all-Work trees, migration reversibility.

- [ ] **Step 5: Final commit (only if review required fixes)** — `git commit -m "fix(workprivat): done-gate + holistic-review follow-ups"`.

---

## Self-Review (plan author)
**Spec coverage (§5.1/§7 Slice 1):** inherited+override flag → Task 1 (`ResolveCountsTowardTarget`, `*bool`, migration); rollup Work/Privat split → Task 2; Soll consistency → Task 3; form toggle (#9) → Task 4; wiring/gate → Task 5. ✓
**Placeholder scan:** the two "VERIFY … unchanged" steps (Task 1 Steps 6/7) are flagged reads of pgx behavior with an explicit fallback, not silent TODOs; the countsTowardFn-test seam (Task 3 Step 1) offers two concrete strategies and says "pick one, make it real". No "TBD"/"add validation"/"similar to Task N". ✓
**Type consistency:** `CountsTowardTarget *bool` used consistently (domain, usecases, pgstore, fake, SetCountsTowardTarget); `ResolveCountsTowardTarget(chain) bool`; `NodeRollup.WorkTotal/WorkWeek/WorkMonth`; `SetCountsTowardTarget.Execute(ctx, owner, id, *bool)`; form value `countsMode` + `countsModeToPtr`/`countsModeOf`. Migration 0025. ✓
**Reuse:** `ResolveCountsTowardTarget` mirrors `ResolveRate`; `SetCountsTowardTarget` mirrors `SetNodeRate`'s always-apply + WebUI call-after-update; form control mirrors the Status `<select>`; `CreateNodeInput`/REST `*bool` already exist (only the domain field type + WebUI were missing). No new store interface methods (reuses `Get`/`Update`/`Subtree`/`Ancestors`/`List`).
