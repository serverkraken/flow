# M1 Slice 1b-c — Subtree-Rollup + NodeStats — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** A node's worktime can be rolled up over its whole subtree (own sessions + all descendants'), exposed as `domain.NodeRollup{Total,Week,Month}` via a `NodeStats` use case, a `GET /api/v1/nodes/{id}/stats` route, and an apiclient method.

**Architecture:** Add a downward `NodeStore.Subtree` (recursive CTE, mirror of `Ancestors` with the join inverted) + fake impl. `StatsComputer` gains a `Nodes` field and a `NodeStats` method that filters all sessions to the subtree's node-IDs and buckets elapsed into Total/Week/Month. Additive — the existing Today/Week/Stats/Burndown are untouched (the `countsTowardTarget` Soll-scoping is the separate Plan 1b-d).

**Tech Stack:** Go, hexagonal (domain/usecase/ports/adapter); pgstore (Postgres + goose); `internal/testutil` fakes; pgstore tests gated on Docker; TDD.

## Global Constraints
- `Subtree(node)` = the node itself + ALL descendants. The pgstore CTE **mirrors `Ancestors`** (same anchor `id=$2`, same columns, same scanNode) with the recursive join inverted: `JOIN sub s ON n.parent_id = s.id` (down) instead of `JOIN chain c ON n.id = c.parent_id` (up).
- This slice is **additive**: do NOT change `BuildDayRecords`, `AggregateRange`, or the existing `Today/Week/RangeStats/Burndown` methods. `StatsComputer.Nodes` is used ONLY by the new `NodeStats`.
- Durations only (no earnings in this slice). Money/rate rollup is out of scope here.
- Go; `errors` via `errors.Is`; all commands from repo root `/Users/msoent/SourceCode/serverkraken/flow-m1`.
- Each task ends with its focused tests green; final task: `go build/vet`, `go test ./...` (incl. pgstore Docker), curl-smoke note.

## File Structure
- `internal/ports/ports.go` — +`Subtree` on `NodeStore`.
- `internal/adapter/pgstore/nodes.go` — +`Subtree` (downward CTE).
- `internal/testutil/fakes.go` — +`FakeNodeStore.Subtree`.
- `internal/adapter/pgstore/nodes_test.go` (or the existing pgstore node test file) — +Subtree Docker test.
- `internal/testutil/fakes_test.go` OR a usecase test — +Subtree fake unit test.
- `internal/domain/node_rollup.go` (new) — `NodeRollup` struct.
- `internal/usecase/node_stats.go` (new) — `StatsComputer.NodeStats` (method on existing `StatsComputer`; add the `Nodes` field in `stats_computer.go`).
- `internal/usecase/node_stats_test.go` (new).
- `internal/adapter/httpserver/stats.go` — +`handleNodeStats` + `nodeRollupDTO`.
- `internal/adapter/httpserver/server.go` — +route.
- `internal/adapter/httpserver/*_test.go` — +handler test.
- `internal/adapter/apiclient/nodes.go` — +`NodeStats`.
- `cmd/flow/main.go` (or composition root) — wire `Nodes` into `StatsComputer`.

---

### Task 1: `NodeStore.Subtree` (port + pgstore + fake)

**Files:** `internal/ports/ports.go`, `internal/adapter/pgstore/nodes.go`, `internal/testutil/fakes.go`, + a fake unit test + a pgstore Docker test.

**Interfaces — Produces:** `Subtree(ctx context.Context, ownerID, nodeID string) ([]domain.Node, error)` (node + all descendants).

- [ ] **Step 1: Fake unit test (RED).** Add to the test file that exercises `FakeNodeStore` (search for an existing `FakeNodeStore` Ancestors test; if none, create `internal/testutil/fakes_subtree_test.go` in package `testutil_test`). Seed eng→vorhaben→repo (+ a sibling engagement) and assert `Subtree(eng)` returns the 3-node chain (by ID set) and `Subtree(repo)` returns just the repo:

```go
func TestFakeNodeStore_Subtree(t *testing.T) {
	ns := testutil.NewFakeNodeStore()
	ctx := context.Background()
	mk := func(id string, parent *string, k domain.NodeKind) {
		if _, err := ns.Create(ctx, domain.Node{ID: id, OwnerID: "u1", ParentID: parent, Kind: k, Name: id, Slug: id, Status: domain.NodeActive}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	eng := "eng"; vor := "vor"
	mk("eng", nil, domain.KindEngagement)
	mk("vor", &eng, domain.KindVorhaben)
	mk("repo", &vor, domain.KindRepo)
	mk("other", nil, domain.KindEngagement)
	got, err := ns.Subtree(ctx, "u1", "eng")
	if err != nil { t.Fatalf("subtree: %v", err) }
	ids := map[string]bool{}
	for _, n := range got { ids[n.ID] = true }
	if len(ids) != 3 || !ids["eng"] || !ids["vor"] || !ids["repo"] || ids["other"] {
		t.Fatalf("want {eng,vor,repo}, got %v", ids)
	}
	leaf, _ := ns.Subtree(ctx, "u1", "repo")
	if len(leaf) != 1 || leaf[0].ID != "repo" {
		t.Fatalf("want just repo, got %v", leaf)
	}
}
```

- [ ] **Step 2: Run → FAIL** — `go test ./internal/testutil/...` → `Subtree` undefined on the interface/fake.

- [ ] **Step 3: Implement (3 spots, must change together to compile).**
  - **Port** — in `internal/ports/ports.go`, add to the `NodeStore` interface after `Reparent`:
    ```go
    // Subtree returns the node itself and all its descendants (root→leaf order).
    Subtree(ctx context.Context, ownerID, nodeID string) ([]domain.Node, error)
    ```
  - **pgstore** — in `internal/adapter/pgstore/nodes.go`, add a `Subtree` method by **copying the existing `Ancestors` method verbatim** and changing exactly two things: rename the CTE from `chain` to `sub`, and change the recursive join `FROM nodes n JOIN chain c ON n.id = c.parent_id` to `FROM nodes n JOIN sub s ON n.parent_id = s.id` (and `c.depth+1` → `s.depth+1`). Keep the identical column lists, the `WHERE n.owner_id=$1`, the final `ORDER BY depth`, and the same `rows`/`scanNode` loop. Method name `Subtree`, same signature as `Ancestors`.
  - **fake** — in `internal/testutil/fakes.go`, add after `Ancestors`:
    ```go
    // Subtree returns nodeID and all transitive descendants (BFS, children sorted by name).
    func (s *FakeNodeStore) Subtree(_ context.Context, ownerID, nodeID string) ([]domain.Node, error) {
    	s.mu.Lock()
    	defer s.mu.Unlock()
    	root, ok := s.m[nodeID]
    	if !ok || root.OwnerID != ownerID {
    		return nil, nil
    	}
    	out := []domain.Node{root}
    	frontier := []string{nodeID}
    	for len(frontier) > 0 {
    		cur := frontier[0]
    		frontier = frontier[1:]
    		var kids []domain.Node
    		for _, n := range s.m {
    			if n.OwnerID == ownerID && n.ParentID != nil && *n.ParentID == cur {
    				kids = append(kids, n)
    			}
    		}
    		sort.Slice(kids, func(i, j int) bool { return kids[i].Name < kids[j].Name })
    		for _, k := range kids {
    			out = append(out, k)
    			frontier = append(frontier, k.ID)
    		}
    	}
    	return out, nil
    }
    ```
    (Add `"sort"` to fakes.go imports if missing.)

- [ ] **Step 4: pgstore Docker test (RED→GREEN together).** In the existing pgstore node test file (find it: `rg -l "func Test.*Node" internal/adapter/pgstore`), add a Docker-gated test mirroring the existing `Ancestors` pgstore test: seed eng→vorhaben→repo, assert `Subtree(eng)` returns 3 nodes incl. the repo, `Subtree(repo)` returns 1. Use the same store/setup harness the neighbouring pgstore tests use.

- [ ] **Step 5: Run all** — `go test ./internal/testutil/... ./internal/adapter/pgstore/... ./internal/ports/...` → PASS.

- [ ] **Step 6: Commit**
```bash
git add internal/ports/ports.go internal/adapter/pgstore/nodes.go internal/testutil/fakes.go internal/adapter/pgstore/*_test.go internal/testutil/*_test.go
git commit -m "feat(nodes): NodeStore.Subtree (downward recursive CTE + fake)"
```

---

### Task 2: `domain.NodeRollup` + `StatsComputer.NodeStats`

**Files:** `internal/domain/node_rollup.go` (new), `internal/usecase/stats_computer.go` (add `Nodes` field), `internal/usecase/node_stats.go` (new), `internal/usecase/node_stats_test.go` (new).

**Interfaces:**
- Consumes: `NodeStore.Subtree` (Task 1).
- Produces: `domain.NodeRollup{Total, Week, Month time.Duration}`; `func (c StatsComputer) NodeStats(ctx, ownerID, nodeID string) (domain.NodeRollup, error)`.

- [ ] **Step 1: Test (RED).** New `internal/usecase/node_stats_test.go`: seed eng→repo, two sessions on the repo (one this week, one older this month) + one session on an unrelated engagement; assert the rollup counts only the eng's subtree and buckets Total/Week/Month correctly. Use a fixed clock. Build StatsComputer with the local test fakes plus `Nodes: ns` (a `*testutil.FakeNodeStore`).

```go
func TestNodeStats_RollsUpSubtree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	eng := "eng"
	_, _ = ns.Create(ctx, domain.Node{ID: "eng", OwnerID: "u1", Kind: domain.KindEngagement, Name: "eng", Slug: "eng", Status: domain.NodeActive})
	_, _ = ns.Create(ctx, domain.Node{ID: "repo", OwnerID: "u1", ParentID: &eng, Kind: domain.KindRepo, Name: "repo", Slug: "repo", Status: domain.NodeActive})
	_, _ = ns.Create(ctx, domain.Node{ID: "other", OwnerID: "u1", Kind: domain.KindEngagement, Name: "other", Slug: "other", Status: domain.NodeActive})
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC) // Wednesday
	ss := <local fakeSessionStore with>:
	  - repo session 2026-06-17 09:00–11:00 (2h, this week+month)
	  - repo session 2026-06-03 09:00–10:00 (1h, this month, not this week)
	  - other session 2026-06-17 09:00–13:00 (4h, must NOT count)
	c := <StatsComputer with Sessions: ss, Clock: fixedClock{now}, Loc: time.UTC, Nodes: ns>
	r, err := c.NodeStats(ctx, "u1", "eng")
	if err != nil { t.Fatalf("nodestats: %v", err) }
	if r.Total != 3*time.Hour { t.Errorf("Total = %v, want 3h", r.Total) }
	if r.Week != 2*time.Hour { t.Errorf("Week = %v, want 2h", r.Week) }
	if r.Month != 3*time.Hour { t.Errorf("Month = %v, want 3h", r.Month) }
}
```
(Match the exact local-fake construction style of `stats_computer_test.go` — `newComputer` there shows the field names; add a `Nodes` field.)

- [ ] **Step 2: Run → FAIL** (`NodeStats` / `NodeRollup` undefined).

- [ ] **Step 3: Implement.**
  - `internal/domain/node_rollup.go`:
    ```go
    package domain

    import "time"

    // NodeRollup is a node's worktime summed over its whole subtree.
    type NodeRollup struct {
    	Total time.Duration `json:"total"`
    	Week  time.Duration `json:"week"`
    	Month time.Duration `json:"month"`
    }
    ```
  - `internal/usecase/stats_computer.go`: add field `Nodes ports.NodeStore` to the `StatsComputer` struct.
  - `internal/usecase/node_stats.go`:
    ```go
    package usecase

    import (
    	"context"
    	"time"

    	"github.com/serverkraken/flow/internal/domain"
    )

    // NodeStats rolls a node's worktime up over its subtree (own sessions + all
    // descendants'), bucketed into Total / current-ISO-week / current-month.
    func (c StatsComputer) NodeStats(ctx context.Context, ownerID, nodeID string) (domain.NodeRollup, error) {
    	sub, err := c.Nodes.Subtree(ctx, ownerID, nodeID)
    	if err != nil {
    		return domain.NodeRollup{}, err
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
    	weekStart := startOfISOWeek(now)
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
    		r.Total += el
    		st := s.Start.In(loc)
    		if !st.Before(weekStart) {
    			r.Week += el
    		}
    		if !st.Before(monthStart) {
    			r.Month += el
    		}
    	}
    	return r, nil
    }

    // startOfISOWeek returns Monday 00:00 of t's week, in t's location.
    func startOfISOWeek(t time.Time) time.Time {
    	wd := (int(t.Weekday()) + 6) % 7 // Mon=0 … Sun=6
    	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
    	return d.AddDate(0, 0, -wd)
    }
    ```
    (If `startOfISOWeek` or an equivalent already exists in the usecase package, reuse it instead of redefining — check with `rg "func startOf" internal/usecase`.)

- [ ] **Step 4: Run → PASS** — `go test ./internal/usecase/ -run TestNodeStats` then `go test ./internal/usecase/ ./internal/domain/`.

- [ ] **Step 5: Commit**
```bash
git add internal/domain/node_rollup.go internal/usecase/stats_computer.go internal/usecase/node_stats.go internal/usecase/node_stats_test.go
git commit -m "feat(stats): NodeStats rolls worktime up over a node subtree"
```

---

### Task 3: REST `GET /api/v1/nodes/{id}/stats`

**Files:** `internal/adapter/httpserver/stats.go` (+`handleNodeStats` + `nodeRollupDTO`), `internal/adapter/httpserver/server.go` (+route), a handler test.

**Interfaces:** Consumes `StatsComputer.NodeStats`. The `Server` already holds a `StatsComputer` (field used by `handleStats`).

- [ ] **Step 1: Handler test (RED).** Mirror an existing stats-handler test (find one: `rg -l "handleStats|/api/v1/stats" internal/adapter/httpserver`). Seed eng→repo + a repo session via the server's stores, GET `/api/v1/nodes/eng/stats` with auth, assert 200 and the JSON `totalMin`/`weekMin`/`monthMin`. (Match the existing handler-test harness exactly — auth helper, store seeding, owner id.)

- [ ] **Step 2: Run → FAIL** (404/no route).

- [ ] **Step 3: Implement.**
  - In `internal/adapter/httpserver/stats.go`:
    ```go
    type nodeRollupDTO struct {
    	TotalMin int `json:"totalMin"`
    	WeekMin  int `json:"weekMin"`
    	MonthMin int `json:"monthMin"`
    }

    func (s *Server) handleNodeStats(w http.ResponseWriter, r *http.Request) {
    	u := userFrom(r.Context()) // use the same auth-user accessor the other stats handlers use
    	id := r.PathValue("id")
    	roll, err := s.Stats.NodeStats(r.Context(), u.ID, id)
    	if err != nil {
    		// map ErrNodeNotFound→404 like the other node handlers; else 500
    		writeNodeErr(w, err) // or the existing error-mapping helper used by node handlers
    		return
    	}
    	writeJSON(w, http.StatusOK, nodeRollupDTO{
    		TotalMin: int(roll.Total.Minutes()),
    		WeekMin:  int(roll.Week.Minutes()),
    		MonthMin: int(roll.Month.Minutes()),
    	})
    }
    ```
    Match this file's existing helpers: the auth-user accessor, the JSON writer (`writeJSON`/`respondJSON` — whatever `handleStats` uses), and the error mapping used by the node handlers (`handleNodeView`/`handleAncestors`) for `ErrNodeNotFound`→404. Read `handleStats` and a node handler in this package first and copy their exact helper calls.
  - In `internal/adapter/httpserver/server.go`, register the route **before** the existing `GET /api/v1/nodes/{id}/ancestors` line (so the static `/stats` sub-path is matched):
    ```go
    mux.Handle("GET /api/v1/nodes/{id}/stats", s.auth(http.HandlerFunc(s.handleNodeStats)))
    ```

- [ ] **Step 4: Run → PASS** — focused handler test, then `go test ./internal/adapter/httpserver/`.

- [ ] **Step 5: Commit**
```bash
git add internal/adapter/httpserver/stats.go internal/adapter/httpserver/server.go internal/adapter/httpserver/*_test.go
git commit -m "feat(api): GET /api/v1/nodes/{id}/stats (subtree rollup)"
```

---

### Task 4: apiclient `NodeStats`

**Files:** `internal/adapter/apiclient/nodes.go` (+method + DTO), a client test if the package has client tests (else skip the test step and note it).

**Interfaces:** Produces `func (c *Client) NodeStats(ctx context.Context, id string) (NodeRollup, error)`.

- [ ] **Step 1: Implement** (this layer is a thin HTTP shim; follow the exact style of the neighbouring `Ancestors`/`GetStats` client methods — same `c.do`/decode helper):
```go
// NodeRollup mirrors the server's nodeRollupDTO.
type NodeRollup struct {
	TotalMin int `json:"totalMin"`
	WeekMin  int `json:"weekMin"`
	MonthMin int `json:"monthMin"`
}

// NodeStats fetches a node's worktime rolled up over its subtree.
func (c *Client) NodeStats(ctx context.Context, id string) (NodeRollup, error) {
	var out NodeRollup
	err := c.get(ctx, "/api/v1/nodes/"+id+"/stats", &out) // use the package's actual GET helper
	return out, err
}
```
Read `apiclient/stats.go` `GetStats` and `apiclient/nodes.go` `Ancestors` first to use the exact request helper this client uses (`c.get`, `c.do`, etc.).

- [ ] **Step 2: If the apiclient package has tests** (`rg -l "func Test" internal/adapter/apiclient`), add a httptest-server round-trip test for `NodeStats` matching their style; run it. Otherwise note "apiclient has no unit tests; covered by the live smoke + handler test."

- [ ] **Step 3: Build + commit**
```bash
go build ./...
git add internal/adapter/apiclient/nodes.go internal/adapter/apiclient/*_test.go
git commit -m "feat(apiclient): NodeStats client method"
```

---

### Task 5: Wiring + verification

**Files:** `cmd/flow/main.go` (or wherever `StatsComputer` is constructed).

- [ ] **Step 1: Wire `Nodes`.** Find where `StatsComputer{...}` is assembled (`rg -n "StatsComputer{" cmd internal`). Add `Nodes: <the node store>` to that struct literal, using the same node-store variable already passed to other use cases (e.g. the one given to `BuildExport`/`SetNodeRate`). Without this, `NodeStats` nil-panics.

- [ ] **Step 2: Build + vet + full tests**
Run: `go build ./... && go vet ./... && go test ./...`
Expected: all green (incl. pgstore Docker). Fix any wiring/compile issues.

- [ ] **Step 3: gofmt** — `gofmt -l internal/ cmd/ | grep -E 'node_stats|node_rollup|nodes.go|stats.go|server.go|fakes.go|ports.go|main.go'` should be empty; `gofmt -w` any listed file you touched.

- [ ] **Step 4: Commit (only if wiring/format changes)**
```bash
git add cmd/flow/main.go
git commit -m "wire(stats): pass NodeStore into StatsComputer for NodeStats"
```

- [ ] **Step 5: Curl-smoke note (manual done-gate).** Record for the human: with the dev stack up, `GET /api/v1/nodes/<ENG_ID>/stats` returns `{totalMin,weekMin,monthMin}` summing sessions booked to that engagement AND any descendant vorhaben/repo.

---

## Self-Review (done)
1. **Coverage:** (c) Subtree (Task 1) + NodeStats rollup (Task 2) + REST (Task 3) + apiclient (Task 4) + wiring (Task 5). ✔
2. **Placeholders:** New production code is exact. Three "read the neighbouring X and use its exact helper" steps (pgstore Ancestors mirror, httpserver auth/json/err helpers, apiclient request helper) name the precise source function to copy — concrete, not vague. The handler test harness is delegated to the existing pattern because handler-test scaffolding is package-specific.
3. **Type consistency:** `domain.NodeRollup{Total,Week,Month}` (durations) ↔ `nodeRollupDTO{TotalMin,WeekMin,MonthMin}` (ints) ↔ apiclient `NodeRollup{…Min}`. `NodeStats(ctx, ownerID, nodeID)` signature consistent across usecase→handler.

## Notes for the executor
- Additive slice — if any existing stats test breaks, that's a regression to investigate, NOT expected.
- The pgstore Subtree CTE is the one spot to get exactly right: it is `Ancestors` with the join direction flipped. Diff the two methods to confirm only the join predicate + CTE name differ.
