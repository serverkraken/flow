# Project Resolution V0 — Slice 2 (path tier) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add the **per-device path** tier to the resolution chain so a project resolves for remote-less git repos and bare directories (where there's no git origin): `flow project bind` in such a dir records a `kind=path` binding for this machine, and `resolve` matches the longest path-prefix of cwd.

**Architecture:** Slice 1 already shipped the full chain *shape* and most of the path plumbing (the discriminated table, `pgstore.Upsert`/`DeletePath` for `kind=path`, kind-agnostic REST handlers + usecases, and the `BindKey`/`ProjectBinding` path fields). Slice 2 fills the **one inert server-side piece** — `domain.ResolveBinding`'s path branch (longest-prefix) — plus the client side: a machine identity, apiclient `BindPath`/`UnbindPath`, the `projectresolve` machine-id wiring, and the CLI `bind`/`unbind` auto-detect fallback.

**Tech Stack:** Go, `github.com/google/uuid`, pgx (Docker pgstore test), cobra.

## Global Constraints

- Module `github.com/serverkraken/flow`. Hexagonal; one responsibility per file. Owner-scoped everywhere.
- Spec: `docs/superpowers/specs/2026-06-21-flow-project-resolution-design.md`. Slice-1 plan (context): `…/plans/2026-06-21-flow-project-resolution-slice1.md`.
- **Path-match rule (verbatim from spec):** for `kind=path` bindings of the given `machineID`, `cwd` matches binding path `p` iff `cwd == p` OR `strings.HasPrefix(cwd, p + string(os.PathSeparator))` (segment boundary — `/a/b` matches `/a/b/c`, never `/a/bc`); the **longest** matching `p` wins. **A remote match always beats a path match** (remote tier is checked first; already true in `ResolveBinding`).
- Paths are stored/compared **cleaned + absolute** (`filepath.Clean`); the client cleans cwd before sending. (Symlink resolution via `EvalSymlinks` is a deferred robustness nicety — note the macOS `/tmp`→`/private/tmp` caveat but do NOT add it now; consistent `Clean` on both bind and resolve is enough.)
- Machine identity: a uuid + `os.Hostname()` label, read-or-created once at `<os.UserConfigDir>/flow/machine.json` (mirror `internal/adapter/tokenstore/store.go`'s `os.UserConfigDir()/flow/…` path pattern).
- `make ci` green (~80% gate) at the end. No `cmd/flow-server/main.go` change is needed (the path kind flows through the already-wired usecases).

## What Slice 1 already left in place (do NOT rebuild — consume it)

- `domain.ProjectBinding{… Kind, RemoteSlug, MachineID, MachineLabel, Path …}` and `usecase.BindKey{Kind, RemoteSlug, MachineID, MachineLabel, Path}` — path fields exist.
- `domain.ResolveBinding(bs []ProjectBinding, remoteSlug, machineID, cwd string) (ProjectBinding, bool)` — remote branch done; **path branch is an inert `return ProjectBinding{}, false`** (the line commented `// Slice 2: else longest-prefix path match…`). Task 1 fills it.
- `pgstore.ProjectBindingStore.Upsert` already has the `kind=path` `ON CONFLICT (owner_id, machine_id, path) WHERE kind='path'` branch; `DeletePath(ctx, ownerID, machineID, path)` exists. **Untested** — Task 6 tests them.
- REST handlers (`internal/adapter/httpserver/projectbindings.go`) are kind-agnostic: the `PUT …/bindings` body carries `{kind, remoteSlug, machineId, machineLabel, path}`; `DELETE …/bindings?kind=path&machine=&path=`; `GET …/resolve?slug=&machine=&path=` reads all three and passes `machineID`+`cwd` into `ResolveProject`→`ResolveBinding`. No handler change needed.
- `apiclient.ResolveProject(ctx, remoteSlug, machineID, cwd)` already takes `machineID`.
- The WebUI panel (Slice 1 Task 12) already renders `kind=path` rows as `label: path`.

---

### Task 1: `domain.ResolveBinding` — path branch (longest-prefix)

**Files:** Modify `internal/domain/projectbinding.go`; Test `internal/domain/projectbinding_test.go` (extend).

**Interfaces:** Same signature; the path branch now matches `kind=path` bindings for `machineID` by longest segment-boundary prefix of `cwd`.

- [ ] **Step 1: Write the failing tests** (add to the existing test file):

```go
func TestResolveBinding_Path(t *testing.T) {
	bs := []ProjectBinding{
		{ProjectID: "pa", Kind: BindingPath, MachineID: "m1", Path: "/home/u/code"},
		{ProjectID: "pb", Kind: BindingPath, MachineID: "m1", Path: "/home/u/code/flow"},
		{ProjectID: "pc", Kind: BindingPath, MachineID: "m2", Path: "/home/u/code/flow"}, // other machine
	}
	// longest-prefix wins, machine-scoped
	if got, ok := ResolveBinding(bs, "", "m1", "/home/u/code/flow/sub"); !ok || got.ProjectID != "pb" {
		t.Fatalf("longest prefix m1: %+v %v (want pb)", got, ok)
	}
	// shorter prefix when not under the longer one
	if got, ok := ResolveBinding(bs, "", "m1", "/home/u/code/other"); !ok || got.ProjectID != "pa" {
		t.Fatalf("shorter prefix: %+v %v (want pa)", got, ok)
	}
	// exact match
	if got, ok := ResolveBinding(bs, "", "m1", "/home/u/code/flow"); !ok || got.ProjectID != "pb" {
		t.Fatalf("exact: %+v %v (want pb)", got, ok)
	}
	// segment boundary: /home/u/codex must NOT match /home/u/code
	if _, ok := ResolveBinding(bs, "", "m1", "/home/u/codex"); ok {
		t.Fatal("/home/u/codex must not match /home/u/code")
	}
	// machine isolation: m2's binding not used for m1
	if got, ok := ResolveBinding(bs, "", "m1", "/home/u/code/flow"); ok && got.ProjectID == "pc" {
		t.Fatal("must not cross machines")
	}
	// no match
	if _, ok := ResolveBinding(bs, "", "m1", "/elsewhere"); ok {
		t.Fatal("no path match expected")
	}
}

func TestResolveBinding_RemoteBeatsPath(t *testing.T) {
	bs := []ProjectBinding{
		{ProjectID: "pp", Kind: BindingPath, MachineID: "m1", Path: "/home/u/code/flow"},
		{ProjectID: "rr", Kind: BindingRemote, RemoteSlug: "github.com/a/flow"},
	}
	if got, ok := ResolveBinding(bs, "github.com/a/flow", "m1", "/home/u/code/flow"); !ok || got.ProjectID != "rr" {
		t.Fatalf("remote must beat path: %+v (want rr)", got)
	}
}
```

- [ ] **Step 2: Run → FAIL** `go test ./internal/domain/ -run 'TestResolveBinding_Path|RemoteBeatsPath' -v` (path branch returns false).

- [ ] **Step 3: Implement** the path branch in `ResolveBinding` (replace the `// Slice 2:` line + `return …,false`):

```go
import (
	"strings"
	"time"
)

// pathSep is the separator for client paths. flow clients are macOS/Linux, so
// paths are "/"-separated; hardcoding it keeps the domain pure (no os import)
// and avoids comparing with the SERVER's separator (the path is the client's).
const pathSep = "/"

// ... inside ResolveBinding, after the remote loop:
	// Path tier: longest segment-boundary prefix of cwd among this machine's path bindings.
	var best ProjectBinding
	bestLen := -1
	for _, b := range bs {
		if b.Kind != BindingPath || b.MachineID != machineID || b.Path == "" {
			continue
		}
		if cwd == b.Path || strings.HasPrefix(cwd, b.Path+pathSep) {
			if len(b.Path) > bestLen {
				best, bestLen = b, len(b.Path)
			}
		}
	}
	if bestLen >= 0 {
		return best, true
	}
	return ProjectBinding{}, false
```

Add the `strings` import (the file currently imports only `time`). Domain stays pure — no `os` import; `pathSep="/"` matches the unix paths the macOS/Linux clients send.

- [ ] **Step 4: Run → PASS** + the existing remote tests still pass (`go test ./internal/domain/ -run TestResolveBinding -v`).
- [ ] **Step 5: Commit** `feat(domain): ResolveBinding path tier (longest-prefix, machine-scoped)`.

---

### Task 2: `internal/clientmachine` — machine identity

**Files:** Create `internal/clientmachine/machine.go`; Test `internal/clientmachine/machine_test.go`.

**Interfaces:** Produces `type Machine struct{ ID, Label string }` and `func Load() (Machine, error)` — reads `<UserConfigDir>/flow/machine.json`, or creates it (uuid + `os.Hostname()`) on first call and persists it.

- [ ] **Step 1: Failing test** — create-then-reload returns the same id; the label is non-empty.

```go
func TestLoad_StableAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir) // os.UserConfigDir honors this on Linux; on macOS see note
	m1, err := clientmachine.Load()
	if err != nil { t.Fatal(err) }
	if m1.ID == "" || m1.Label == "" { t.Fatalf("empty machine: %+v", m1) }
	m2, err := clientmachine.Load()
	if err != nil { t.Fatal(err) }
	if m2.ID != m1.ID { t.Fatalf("id not stable: %q vs %q", m1.ID, m2.ID) }
}
```

> **macOS note for the implementer:** `os.UserConfigDir()` ignores `XDG_CONFIG_HOME` on macOS (it returns `~/Library/Application Support`). To keep `Load()` testable without writing to the real home, structure it as `Load()` calling an unexported `loadFrom(dir string)`, and have the test call `loadFrom(t.TempDir())` directly. Make `Load()` = `loadFrom(filepath.Join(userConfigDir(), "flow"))`. Write the test against `loadFrom` and drop the env-var trick if it's unreliable.

- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** `machine.go`: `loadFrom(dir)` reads `dir/machine.json` (`{"id":..,"label":..}`); on missing/corrupt, generate `id := uuid.NewString()`, `label, _ := os.Hostname()` (fallback `"unknown"` if empty/err), `os.MkdirAll(dir, 0o700)`, write `0o600` JSON, return it. `Load()` resolves the dir from `os.UserConfigDir()`+`/flow`. Mirror `internal/adapter/tokenstore/store.go` for the path + file-perms style.
- [ ] **Step 4: Run → PASS.**
- [ ] **Step 5: Commit** `feat(clientmachine): per-device machine id (uuid+hostname) in UserConfigDir`.

---

### Task 3: `apiclient.BindPath` + `UnbindPath`

**Files:** Modify `internal/adapter/apiclient/projectbindings.go`; Test `internal/adapter/apiclient/projectbindings_test.go` (extend).

**Interfaces (Produces):**
```go
func (c *Client) BindPath(ctx, projectID, machineID, machineLabel, path string) (domain.ProjectBinding, error)  // PUT {id}/bindings {kind:path,...}
func (c *Client) UnbindPath(ctx, machineID, path string) error                                                   // DELETE bindings?kind=path&machine=&path=
```

- [ ] **Step 1: Failing test** (httptest, mirror the existing `BindRemote`/`UnbindRemote` tests): assert `BindPath` PUTs to `/api/v1/projects/{id}/bindings` with body `{"kind":"path","machineId":..,"machineLabel":..,"path":..}`; `UnbindPath` DELETEs `…/bindings?kind=path&machine=<m>&path=<p>` (url-escaped).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** both, mirroring `BindRemote`/`UnbindRemote` (use `c.do`; the body map keys must be exactly `kind`,`machineId`,`machineLabel`,`path`; `url.QueryEscape` the query params).
- [ ] **Step 4: Run → PASS** `go test ./internal/adapter/apiclient/ -run Binding -v`.
- [ ] **Step 5: Commit** `feat(apiclient): BindPath/UnbindPath (kind=path bindings)`.

---

### Task 4: `projectresolve.Resolve` — machine id + path tier

**Files:** Modify `internal/projectresolve/resolve.go`; Test `internal/projectresolve/resolve_test.go` (extend).

**Interfaces:** Same `Resolve(ctx, c, getenv, cwd)` signature. Now: after the FLOW_PROJECT + git-remote steps, it loads the machine id and passes it (and a cleaned cwd) to `ResolveProject`, so the server's path tier can match.

- [ ] **Step 1: Failing test** — `FLOW_PROJECT` unset, `cwd` is a bare (non-git) temp dir, and the httptest server's `/resolve` returns a project ONLY when it receives a non-empty `machine` param and the expected `path`. Assert `Resolve` calls `/resolve` with the machine id + cleaned cwd (and empty slug). Keep the existing override + git-remote tests passing.

```go
func TestResolve_PathTier(t *testing.T) {
	var gotMachine, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/projects/resolve":
			gotMachine = r.URL.Query().Get("machine")
			gotPath = r.URL.Query().Get("path")
			if gotMachine != "" {
				_ = json.NewEncoder(w).Encode(domain.Project{ID: "p1", Slug: "x"})
				return
			}
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	dir := t.TempDir() // bare, non-git
	p, ok, err := projectresolve.Resolve(context.Background(), c, func(string) string { return "" }, dir)
	if err != nil || !ok || p.ID != "p1" { t.Fatalf("%+v %v %v", p, ok, err) }
	if gotMachine == "" { t.Fatal("machine id not sent") }
	if gotPath != filepath.Clean(dir) { t.Fatalf("path %q not cleaned-cwd %q", gotPath, filepath.Clean(dir)) }
}
```

- [ ] **Step 2: Run → FAIL** (machine empty today).
- [ ] **Step 3: Implement** — in the git-remote/else branch, load `m, err := clientmachine.Load()` (on error, fall back to `m.ID=""` — resolution still works for the remote tier; don't fail the whole resolve), and call `c.ResolveProject(ctx, remoteSlug, m.ID, filepath.Clean(cwd))`. Keep the FLOW_PROJECT branch unchanged (it returns before this).
- [ ] **Step 4: Run → PASS** + existing `projectresolve` tests green.
- [ ] **Step 5: Commit** `feat(projectresolve): pass machine id + cleaned cwd → path-tier resolution`.

---

### Task 5: CLI `bind`/`unbind` — auto-detect path fallback

**Files:** Modify `cmd/flow/projectbind.go`; Test `cmd/flow/projectbind_test.go` (extend).

**Interfaces:** `flow project bind [<slug>]` and `unbind` now: **if cwd has a git origin → remote (unchanged); else → path** (this machine + cleaned cwd). The CLI reports which (`bound repo … → …` vs `bound path /… on <machine> → …`). `bindings` `*` already routes through `projectresolve.Resolve` (Slice-1 fix), which now includes the path tier — no change there.

- [ ] **Step 1: Failing tests** (httptest-backed, mirror the Slice-1 bind tests; inject the origin-slug + machine so no real repo is needed):
  - a `runBindPath(ctx, c, machine clientmachine.Machine, cwd, projectID, projectName string) (string, error)` helper that calls `c.BindPath` and returns `bound path <cwd> on <machine.Label> → <name>`; assert the PUT happened with kind=path + the machine + cleaned path.
  - the cobra `bind` `RunE` branch: when `gitremote.OriginSlug(cwd)` is NOT ok → it takes the path branch (resolve the project the same pick-or-create / `<slug>` way, then `runBindPath`). Test the helper; the RunE git-detection is a thin shell.
  - `unbind`: when no git origin → `c.UnbindPath(machine.ID, cleaned cwd)`; with git origin → `UnbindRemote` (unchanged). Test a `runUnbindPath` helper.

- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement.** Add `runBindPath`/`runUnbindPath` helpers (take the resolved/cleaned inputs, call the apiclient). In `projectBindCmd`'s `RunE` and `projectUnbindCmd`'s `RunE`: compute `cwd, _ := os.Getwd()`; `slug, ok, _ := gitremote.OriginSlug(cwd)`; **if ok → existing remote path; else → load `clientmachine.Load()` and take the path branch** (`filepath.Clean(cwd)`). For the no-slug interactive `bind` in a non-git dir, reuse the existing picker to choose/create the project, then `runBindPath`. Keep the messages distinct ("bound repo …" vs "bound path …"). The "not in a git repo with an 'origin' remote" hard-error is REMOVED (a non-git dir is now valid → path binding).
- [ ] **Step 4: Run → PASS** `go test ./cmd/flow/ -run 'Bind|Unbind|Path' -v`.
- [ ] **Step 5: Commit** `feat(cli): flow project bind/unbind path fallback for non-git dirs`.

---

### Task 6: pgstore path-binding Docker test (cover the existing path code)

**Files:** Modify `internal/adapter/pgstore/projectbindings_test.go` (extend the existing Docker test).

**Interfaces:** none new — this tests the already-shipped `Upsert`(kind=path) + `DeletePath`.

- [ ] **Step 1: Failing test** (mirror the existing `projectbindings_test.go` `startPG(t)` harness): upsert a `kind=path` binding `(owner, machine, /a/b)→p1`; re-upsert same `(owner, machine, /a/b)` to `p2` → still ONE row, reassigned (the path partial-unique conflict target); a different `(machine,path)` is a separate row; `DeletePath` removes by `(owner, machine, path)`; `List`/`ListByProject` return the path rows; deleting the project cascades the path binding. Also assert a remote and a path binding for the SAME owner+project coexist (different partial uniques).
- [ ] **Step 2: Run → FAIL** only if the existing code were wrong; if the path `Upsert`/`DeletePath` are already correct (they should be — Slice 1 shipped them), the test passes immediately on first run. **In that case, still keep the test** (it closes the Slice-1 coverage gap). Run `go test ./internal/adapter/pgstore/ -run Binding -v` (Docker).
- [ ] **Step 3:** If the test reveals a bug in the existing path `Upsert`/`DeletePath`, fix it in `projectbindings.go`; otherwise no production change.
- [ ] **Step 4: Run → PASS.**
- [ ] **Step 5: Commit** `test(pgstore): cover kind=path Upsert/DeletePath (partial-unique reassign + cascade)`.

---

### Task 7: full gate + live done-gate

**Files:** none (verification only). No `main.go` change is needed (usecases already wired in Slice 1).

- [ ] **Step 1: `make ci`** green (lint + tests + coverage gate). Fix anything it flags (run `make ci`, not just `go test`).
- [ ] **Step 2: Live done-gate (controller).** Against a temp dev server (migrations already at the project-delete version; no new migration this slice):
  - In a **bare non-git** dir (e.g. `/tmp/flow-pathtest`): `flow project bind <slug>` → reports `bound path /tmp/flow-pathtest on <machine> → <slug>`.
  - From a **subdir** of it (`/tmp/flow-pathtest/sub`): `flow project bindings` marks `*` on that project; `resolve` returns it (longest-prefix path tier).
  - In a **git repo with origin** (e.g. `…/flow-rebuild`): `bind` still does a **remote** binding (remote tier unchanged), and remote still wins resolution.
  - `FLOW_PROJECT=<other> flow project bindings` from the bare dir → override still wins over the path tier.
  - `flow project unbind` in the bare dir removes the path binding.
- [ ] **Step 3: Commit** any gate fixups (`chore(project-resolution): slice 2 gate`), else nothing.

---

## Self-review (done)

- **Spec coverage (path tier):** longest-prefix segment-boundary match ✓ (T1), machine identity ✓ (T2), apiclient path methods ✓ (T3), client resolution passes machine+cleaned-cwd ✓ (T4), CLI auto-detect path fallback ✓ (T5), pgstore path code covered ✓ (T6), remote-beats-path + FLOW_PROJECT-override preserved ✓ (T1 test + T7 gate). No new REST/usecase/migration/main.go work — Slice 1 shipped those for `kind=path` already (documented above).
- **Placeholders:** none; the `os.PathSeparator`/`filepath.Clean` choices are explicit; the macOS `UserConfigDir` testability note is a real instruction, not a TODO.
- **Type consistency:** `Machine{ID,Label}`, `clientmachine.Load()`, `BindPath(ctx,projectID,machineID,machineLabel,path)`, `ResolveBinding(bs,remoteSlug,machineID,cwd)` used identically across tasks; the apiclient `kind=path` body keys (`kind/machineId/machineLabel/path`) match the Slice-1 REST handler's decode struct.
