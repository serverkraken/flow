# M1 Slice 1b-d — Job/Privat-Soll (`countsTowardTarget` + `TargetTotal`) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Only worktime on engagements flagged `countsTowardTarget` counts toward the daily/weekly **Soll** (saldo / hits / streak / burndown). All worked time is still tracked and shown (raw `Total`); the Soll is driven by a new `TargetTotal`. Private projects are tracked but excluded from the Soll gauge.

**Architecture:** A per-node boolean `countsTowardTarget` (default `true`, so existing behavior is unchanged). `DayRecord` gains a `TargetTotal` (sum of elapsed for sessions whose node counts). `BuildDayRecords` takes a `countsToward func(*string) bool`. The saldo/hit/streak/burndown computations switch from `Total` to `TargetTotal`. `StatsComputer` builds the `countsToward` map from the node store.

**Tech Stack:** Go, hexagonal; pgstore (goose migration 0023); `internal/testutil` fakes; pgstore tests Docker-gated; TDD.

## Global Constraints
- `countsTowardTarget` defaults **`true`** everywhere (new column `NOT NULL DEFAULT true`; `NewNode` sets `true`; REST create with the field omitted → `true`). So an unmigrated/unset node behaves exactly as today.
- **Raw `Total` is unchanged** (all sessions, for display, Max/Min/Avg/ByTag). Only **saldo, hits, streak, burndown saldo** switch to `TargetTotal`.
- Staging keeps tests green at every task: Task 3 adds `TargetTotal` (== `Total` under the default), Task 4 switches the saldo math to `TargetTotal` (still == `Total` under the default → unchanged), Task 5 makes `StatsComputer` compute the real per-node flag.
- The flag is per-**engagement** but applied per-session via the session's node id directly (the `countsToward` map is keyed by node id; in 1b-d a session books to a node and we look up that node's flag; descendant nodes inherit nothing here — the flag lives on whatever node the session booked to; see Task 5 note).
- Go; `make ci`; all commands from `/Users/msoent/SourceCode/serverkraken/flow-m1`.

## Touchpoint reference
The full per-file touchpoint table is in `.superpowers/sdd/slice1b-touchpoints.md` (section "(d)"). Read it for any file not fully quoted below.

---

### Task 1: `countsTowardTarget` persistence (migration + domain + pgstore + fake)

**Files:** new `internal/adapter/pgstore/migrations/0023_nodes_counts_toward_target.sql`; `internal/domain/node.go`; `internal/adapter/pgstore/nodes.go`; `internal/testutil/fakes.go`; pgstore node test.

**Interfaces — Produces:** `domain.Node.CountsTowardTarget bool` (default true), persisted.

- [ ] **Step 1: Migration.** Create `internal/adapter/pgstore/migrations/0023_nodes_counts_toward_target.sql`:
```sql
-- +goose Up
ALTER TABLE nodes ADD COLUMN counts_toward_target BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE nodes DROP COLUMN counts_toward_target;
```

- [ ] **Step 2: pgstore round-trip test (RED).** In the pgstore node test file, add a Docker-gated test: `Create` a node WITHOUT setting `CountsTowardTarget` → `Get` returns `CountsTowardTarget == true` (the column default); then create one with `CountsTowardTarget: false`, `Get` returns `false`; `Update` it to `true`, `Get` returns `true`. (Match the existing pgstore node-test harness.)

- [ ] **Step 3: Implement.**
  - `internal/domain/node.go`: add field to the `Node` struct (after `Kind`/`Extra` area, grouped with the other metadata): `CountsTowardTarget bool \`json:"countsTowardTarget"\``. In `NewNode(...)`, add `CountsTowardTarget: true` to the returned `Node` literal.
  - `internal/adapter/pgstore/nodes.go`:
    - `nodeCols` (line 19): append `, counts_toward_target` (now 18 columns).
    - `Create` SQL: the `VALUES (...$17)` becomes `...$17,$18)`; add `n.CountsTowardTarget` as the 18th query param (in the same order as `nodeCols`).
    - `Update` SQL: add `, counts_toward_target=$10` to the SET list; renumber `updated_at=$10`→`$11`, and the WHERE `owner_id=$11 AND id=$12`→`$12 AND id=$13`; add `n.CountsTowardTarget` to the params before `n.UpdatedAt`, and shift `ownerID, n.ID` accordingly. (Read the current `Update` method and renumber carefully — the params slice order must match the `$n` order exactly.)
    - `scanNode`: add `&n.CountsTowardTarget` as the 18th scan target (after `&extra` / in `nodeCols` order — match the exact column order). Note `scanNode` scans `nodeCols` order; insert the new target at the same position `counts_toward_target` occupies in `nodeCols`.
  - `internal/testutil/fakes.go`: in `FakeNodeStore.Update`, add `existing.CountsTowardTarget = p.CountsTowardTarget` alongside the other field copies. (`Create`/`Get` store/return the whole struct, so the field round-trips automatically there.) **Important:** `FakeNodeStore.Create` must default the field to `true` when the caller passes the zero value? NO — the fake stores verbatim, and Go's zero value for bool is `false`. To mirror the DB default, callers that rely on the default must set it; but `NewNode` sets `true`, and the create usecase (Task 2) defaults it. Leave the fake verbatim; just ensure Task 2's create path sets `true` by default.

- [ ] **Step 4: Run** — `go test ./internal/domain/ ./internal/adapter/pgstore/...` → PASS (pgstore Docker).

- [ ] **Step 5: Commit**
```bash
git add internal/adapter/pgstore/migrations/0023_nodes_counts_toward_target.sql internal/domain/node.go internal/adapter/pgstore/nodes.go internal/testutil/fakes.go internal/adapter/pgstore/*_test.go
git commit -m "feat(nodes): counts_toward_target column (default true)"
```

---

### Task 2: write path (usecase inputs + REST + apiclient)

**Files:** `internal/usecase/create_node.go`, `internal/usecase/update_node.go`, `internal/adapter/httpserver/worktime.go` (createNodeReq/updateProjReq + handlers), `internal/adapter/apiclient/nodes.go` (`CreateNodeFields`), + tests.

- [ ] **Step 1: Tests (RED).** Usecase: `CreateNode` with `CountsTowardTarget: false` persists `false`; default path (true) — add to `create_node_test.go`. REST: `handleCreateNode` test posting `{"countsTowardTarget": false, ...}` persists false; omitting it → true. (Match existing create/update test harnesses.)

- [ ] **Step 2: Implement.**
  - `create_node.go`: add `CountsTowardTarget bool` to `CreateNodeInput`; in `Execute`, set `n.CountsTowardTarget = in.CountsTowardTarget` (after `n.Kind = in.Kind`). **Default-true at the boundary:** since `NewNode` already sets `true`, only OVERRIDE when the caller intends false — i.e. set `n.CountsTowardTarget = in.CountsTowardTarget` unconditionally is wrong if `CreateNodeInput` zero-value false would clobber the NewNode true. SAFER: keep `CreateNodeInput.CountsTowardTarget *bool` (pointer) and only set when non-nil: `if in.CountsTowardTarget != nil { n.CountsTowardTarget = *in.CountsTowardTarget }`. Use the pointer form so omission preserves the `NewNode` default true.
  - `update_node.go`: add `CountsTowardTarget *bool` to `UpdateNodeInput`; in `Execute`, `if in.CountsTowardTarget != nil { p.CountsTowardTarget = *in.CountsTowardTarget }` (update preserves the existing value when omitted).
  - `httpserver/worktime.go`: `createNodeReq` += `CountsTowardTarget *bool \`json:"countsTowardTarget"\``; `handleCreateNode` passes it straight through to `CreateNodeInput.CountsTowardTarget` (pointer → pointer). `updateProjReq` += same; `handleUpdateNode` → `UpdateNodeInput.CountsTowardTarget`.
  - `apiclient/nodes.go`: `CreateNodeFields` += `CountsTowardTarget *bool \`json:"countsTowardTarget"\``.

- [ ] **Step 3: Run** — `go test ./internal/usecase/ ./internal/adapter/httpserver/` → PASS.

- [ ] **Step 4: Commit** — `feat(nodes): create/update + REST/apiclient carry countsTowardTarget`.

---

### Task 3: `DayRecord.TargetTotal` + thread `countsToward` into `BuildDayRecords` (additive)

**Files:** `internal/domain/dayrecord.go`, `internal/domain/records.go`, all `BuildDayRecords` callers (`internal/usecase/stats_computer.go` × the methods that call it, plus any test that calls it directly), `internal/domain/records_test.go`.

**Interfaces — Produces:** `DayRecord.TargetTotal time.Duration`; `BuildDayRecords(sessions, now, targetFor, countsToward func(*string) bool)`.

- [ ] **Step 1: Test (RED).** In `records_test.go`, add `TestBuildDayRecords_TargetTotal`: two sessions on the same day, one with `NodeID` "job" and one "priv"; pass `countsToward := func(id *string) bool { return id != nil && *id == "job" }`; assert the day's `Total` == both, but `TargetTotal` == only the "job" session.

- [ ] **Step 2: Run → FAIL** (signature/field).

- [ ] **Step 3: Implement.**
  - `internal/domain/dayrecord.go`: add `TargetTotal time.Duration` to the `DayRecord` struct, right after `Total`.
  - `internal/domain/records.go`: change the signature to
    ```go
    func BuildDayRecords(sessions []WorkSession, now time.Time, targetFor func(time.Time) time.Duration, countsToward func(*string) bool) []DayRecord {
    ```
    and inside the loop, right after `rec.Total += el`, add:
    ```go
    		if countsToward(s.NodeID) {
    			rec.TargetTotal += el
    		}
    ```
  - **Update every caller** with a nil-safe default so behavior is unchanged this task. Find them: `rg -n "BuildDayRecords(" internal`. For each StatsComputer method (`Today`, `Week`, `RangeStats`, `Burndown`) and any direct test caller, add a fourth arg `func(*string) bool { return true }`. (Do NOT compute the real flag yet — that's Task 5.)

- [ ] **Step 4: Run** — `go test ./internal/domain/ ./internal/usecase/` → PASS (the nil-safe default makes `TargetTotal == Total`, so existing saldo tests are unaffected).

- [ ] **Step 5: Commit** — `feat(stats): DayRecord.TargetTotal + countsToward in BuildDayRecords (additive)`.

---

### Task 4: saldo/hits/streak/burndown use `TargetTotal`

**Files:** `internal/domain/stats.go`, `internal/domain/burndown.go`, `internal/domain/aggregate.go`, `internal/usecase/stats_computer.go` (the `Today` saldo line), + `internal/domain/aggregate_test.go`.

**The exact edits in `aggregate.go`** (raw `Total` stays for display; saldo/hit/streak use `TargetTotal`):
- `Aggregate` loop: keep `st.Total += r.Total`; ADD `st.TargetTotal += r.TargetTotal`. Change `isHit(r.Total, r.Target)` → `isHit(r.TargetTotal, r.Target)`. Change `st.Overtime += r.Total - r.Target` → `st.Overtime += r.TargetTotal - r.Target`.
- `bestStreak`: `r.Total >= r.Target` → `r.TargetTotal >= r.Target`.
- `currentStreak`: `r.Total >= r.Target` → `r.TargetTotal >= r.Target`.
- `tallyRecordsInto`: keep `st.Total += r.Total`; ADD `st.TargetTotal += r.TargetTotal`. (Max/Min stay on `r.Total`.)
- `walkWorkdaysForSaldo`: `isHit(rec.Total, rec.Target)` → `isHit(rec.TargetTotal, rec.Target)`; `st.Overtime += rec.Total - rec.Target` → `st.Overtime += rec.TargetTotal - rec.Target`. (`st.Overtime -= targetFor(d)` for unworked days stays.)
- `MonthBurndownCompute`: in the records loop keep `rep.Total += r.Total`; ADD `rep.TargetTotal += r.TargetTotal`. Change `rep.Saldo = rep.Total - expected` → `rep.Saldo = rep.TargetTotal - expected`. (The `active` branch is always called with `nil` from `StatsComputer.Burndown` — leave it; it only touches `Total`.)

- [ ] **Step 1: Tests (RED).** In `aggregate_test.go`, build `DayRecord`s with `TargetTotal < Total` (e.g. `DayRecord{Date: d, Total: 8h, TargetTotal: 5h, Target: 6h}`) and assert: `Aggregate`/`AggregateRange` `Overtime` uses `TargetTotal-Target` (here `5h-6h=-1h` per day, NOT `8h-6h`); `Hits` counts only when `TargetTotal>=Target`; `Stats.Total` still reports `8h`; `Stats.TargetTotal` reports `5h`. Add a `MonthBurndownCompute` case asserting `Saldo` uses `TargetTotal`. (The existing `rec(...)` test helper builds `DayRecord` with `TargetTotal==0`; either extend it to take a targetTotal arg or construct literals in the new tests — and where existing tests rely on `rec(...)` producing a hit/saldo, set `TargetTotal == Total` in the helper so they keep passing. SIMPLEST: change the `rec` helper to set `TargetTotal: total` by default — then existing tests are unchanged, and the new tests build literals with a distinct `TargetTotal`.)

- [ ] **Step 2: Run → FAIL** (Stats/MonthBurndownReport lack `TargetTotal`; saldo still uses Total).

- [ ] **Step 3: Implement.**
  - `internal/domain/stats.go`: add `TargetTotal time.Duration` to `Stats` (after `Total`).
  - `internal/domain/burndown.go`: add `TargetTotal time.Duration` to `MonthBurndownReport`.
  - Apply the `aggregate.go` edits listed above.
  - `internal/usecase/stats_computer.go` `Today`: the saldo (`sum.Saldo = sum.Logged - sum.Target` or equivalent) must use the day's **TargetTotal**, not its raw total. Read the `Today` method; wherever it derives today's logged duration that feeds `Saldo`, switch that to the day record's `TargetTotal` (keep any separately-displayed "logged today" as raw `Total`). State precisely in the report what you changed.
  - If the `rec(...)` test helper was changed to default `TargetTotal: total`, confirm existing aggregate tests still pass.

- [ ] **Step 4: Run** — `go test ./internal/domain/ ./internal/usecase/` → PASS. (Under the still-default `countsToward==true`, `TargetTotal==Total`, so end-to-end behavior is unchanged; the new unit tests prove the saldo now keys off `TargetTotal`.)

- [ ] **Step 5: Commit** — `feat(stats): saldo/hits/streak/burndown key off TargetTotal`.

---

### Task 5: `StatsComputer` computes the real per-node `countsToward`

**Files:** `internal/usecase/stats_computer.go` (resolver + the 4 methods), `internal/usecase/stats_computer_test.go`.

**Note on the flag's scope:** the flag lives on the node a session booked to. The `countsToward` map maps **every node id → its own `CountsTowardTarget`**; a session counts iff `countsToward(s.NodeID)` is true. (A session booked to a child node uses the child's flag; in practice the "private vs job" split is set at the engagement and children created under it are expected to carry the same flag — keeping the map a flat per-node lookup is correct for 1b-d and avoids an ancestor walk per session. If later you want descendants to inherit the engagement's flag, that's a follow-up.) `nil`/unknown node id → counts (true), preserving unbooked/legacy sessions in the Soll as today.

- [ ] **Step 1: Test (RED).** In `stats_computer_test.go`: seed two engagements — "job" (`CountsTowardTarget: true`) and "priv" (`CountsTowardTarget: false`) — in a `*testutil.FakeNodeStore`; sessions on each on a workday; build the `StatsComputer` with `Nodes: ns`; assert the **week saldo** counts only the "job" session's time against the target (the "priv" time is excluded), while `Stats.Total` includes both. (Use the `newComputer` harness extended with a `Nodes` arg.)

- [ ] **Step 2: Run → FAIL** (resolver still passes the all-true default).

- [ ] **Step 3: Implement.** In `stats_computer.go`:
  - Add a helper that loads nodes once and returns the closure:
    ```go
    func (c StatsComputer) countsTowardFn(ctx context.Context, ownerID string) (func(*string) bool, error) {
    	nodes, err := c.Nodes.List(ctx, ownerID)
    	if err != nil {
    		return nil, err
    	}
    	flag := make(map[string]bool, len(nodes))
    	for _, n := range nodes {
    		flag[n.ID] = n.CountsTowardTarget
    	}
    	return func(id *string) bool {
    		if id == nil {
    			return true // unbooked time still counts toward the Soll
    		}
    		v, ok := flag[*id]
    		if !ok {
    			return true // unknown node → count (legacy-safe)
    		}
    		return v
    	}, nil
    }
    ```
  - In each of `Today`, `Week`, `RangeStats`, `Burndown`: call `countsToward, err := c.countsTowardFn(ctx, ownerID)` (handle err) and pass `countsToward` to `BuildDayRecords` in place of the `func(*string) bool { return true }` default from Task 3.

- [ ] **Step 4: Run** — `go test ./internal/usecase/` → PASS (new test proves private time is excluded from the saldo; existing tests, whose seeded nodes default `CountsTowardTarget`… note: existing stats tests may seed sessions with node ids that don't exist in a node store → `countsToward` returns true (unknown→true), so they're unaffected. If a stats test now wires a `FakeNodeStore`, ensure its job nodes are `CountsTowardTarget: true`.)

- [ ] **Step 5: Commit** — `feat(stats): Soll counts only countsTowardTarget engagements`.

---

### Task 6: expose `TargetTotal` over REST + apiclient

**Files:** `internal/adapter/httpserver/stats.go` (`statsDTO`, `burndownDTO` + handlers), `internal/adapter/apiclient/stats.go` (`Stats`, `Burndown`), + handler test.

- [ ] **Step 1: Test (RED).** Extend an existing `handleStats`/`handleBurndown` test (or add one) asserting the response JSON includes `targetTotalMin` reflecting the scoped total.
- [ ] **Step 2: Implement.** Add `TargetTotalMin int \`json:"targetTotalMin"\`` to `statsDTO` and `burndownDTO`; in the handlers set `TargetTotalMin: minutes(st.TargetTotal)` / `minutes(rep.TargetTotal)` (use the same `minutes` helper). Add `TargetTotalMin int \`json:"targetTotalMin"\`` to apiclient `Stats` and `Burndown`.
- [ ] **Step 3: Run** — `go test ./internal/adapter/httpserver/` → PASS.
- [ ] **Step 4: Commit** — `feat(api): expose targetTotalMin (job-Soll total)`.

---

### Task 7: Verification

- [ ] **Step 1:** `go build ./... && go vet ./...` → clean.
- [ ] **Step 2:** `go test ./...` → all green (incl. pgstore Docker + migration 0023 applied). If a package fails only on infra, name it distinctly.
- [ ] **Step 3:** `gofmt -l` the touched files → empty; `gofmt -w` any.
- [ ] **Step 4:** Commit any wiring/format fixups (none expected — `StatsComputer.Nodes` was wired in 1b-c).
- [ ] **Step 5: Done-gate note (human):** with the dev stack, create a "Privat" engagement with `countsTowardTarget=false`, book time on it, and confirm `GET /api/v1/week` / `/api/v1/stats` saldo does NOT include that time (raw total does), while a `countsTowardTarget=true` engagement's time does.

---

## Self-Review (done)
1. **Coverage:** persistence (T1), write path (T2), TargetTotal plumbing (T3), saldo-uses-TargetTotal (T4), real per-node flag (T5), REST/apiclient exposure (T6), verify (T7). The (d) touchpoint table is fully covered.
2. **Placeholders:** the saldo-critical edits (T3/T4) are exact (I read `records.go`/`aggregate.go`). The pgstore param-renumbering (T1) and the `Today` saldo line (T4) are "read the method and apply this precise change" — concrete, file+change named.
3. **Type consistency:** `DayRecord.TargetTotal`, `Stats.TargetTotal`, `MonthBurndownReport.TargetTotal` (durations) → `targetTotalMin` (ints). `countsToward func(*string) bool` signature identical at the `BuildDayRecords` def and all call sites. `CountsTowardTarget` (`*bool` at the input/REST boundary for omission-default; `bool` on the domain struct).

## Notes for the executor
- The default-true invariant is the safety net: at every task before T5, `TargetTotal == Total` so nothing changes; T5 flips on the real flag. If any pre-T5 test changes a saldo number, that's a bug.
- Watch the pgstore `Update` `$n` renumbering in T1 — it's the one easy place to misalign a param.
