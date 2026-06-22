# flow-mcp Slice 2d — Project Tools + Runbook + Live Done-Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the flow-mcp V1 surface — add `flow_list_projects` + `flow_bind_project` (→ 11 tools), ship the registration runbook, close the rolled-up MCP Minors, and run the deferred live `.mcp.json` done-gate.

**Architecture:** Two new stdio tools on the existing go-sdk MCP server in `cmd/flow-mcp`. `flow_list_projects` exposes the project list; `flow_bind_project` mirrors the CLI's non-interactive bind (detect git-origin vs path → find-or-create project → `BindRemote`/`BindPath`) via a new inline `bind.go`, then re-resolves the cwd→project and refreshes resources. Both run through the durable-auth `authManager` (`mgr.Do`).

**Tech Stack:** Go, `github.com/modelcontextprotocol/go-sdk/mcp`, the in-repo `internal/adapter/apiclient`, `internal/gitremote`, `internal/clientmachine`, `internal/projectresolve`.

## Global Constraints

- **Branch:** `rebuild`, worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. Do NOT merge to `main`. Slice BASE (final review) = the `rebuild` HEAD at the plan commit.
- **Spec:** `docs/superpowers/specs/2026-06-22-flow-mcp-slice2d-projecttools-design.md` (+ the v1 design `2026-06-21-flow-mcp-v1-design.md` §7.2 for the two tool definitions).
- **CI gate:** `make ci` = `golangci-lint run` + `verify-generate` + `cover` (≥ **80%**) + `build`. **NOT gofmt** (`make fmt` is manual/ungated). Run pgstore Docker tests via podman so coverage counts them: `export DOCKER_HOST=unix:///var/folders/3t/5169xft1491d9_vdw_mzw05h0000gn/T/podman/podman-machine-default-api.sock` and `export TESTCONTAINERS_RYUK_DISABLED=true`.
- **Lint (ST1005):** error strings lowercase, no trailing punctuation.
- **`unused` linter:** an unexported func/method with no caller (prod or test) fails lint. Land each helper together with a caller or a test that exercises it.
- **No migration, no new env vars.**
- **Commit trailers** — every commit message ends with:
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01CicEBhyR4hr3ci2gyoneiP
  ```
  Referenced below as "(+ standard trailers)".
- **As-built surface (do not re-derive):** tool handler shape `func (h *handlers) X(ctx context.Context, _ *mcp.CallToolRequest, in XIn) (*mcp.CallToolResult, any, error)`; apiclient work inside `h.mgr.Do(ctx, func(c *apiclient.Client) error { … })`; helpers `h.resolved()`, `h.resultErr(err)`, `textResult(s)`, `errorResult(s)`, `h.lookupProject(ctx, ref)`, `h.projectList(ctx, refresh)`, `errGuard{err}`. apiclient: `CreateProject(ctx,name)→(Project,error)`, `ListProjects(ctx)→([]Project,error)`, `BindRemote(ctx,projectID,remoteSlug)→(ProjectBinding,error)`, `BindPath(ctx,projectID,machineID,machineLabel,path)→(ProjectBinding,error)`. `gitremote.OriginSlug(dir)→(slug string,ok bool,err error)`. `clientmachine.Load()→(Machine,error)`, `Machine{ID,Label string}`. The 9 current tools register in `server.go` `newServerH` (lines ~53-88).

---

### Task 1: `flow_list_projects` tool

**Files:**
- Modify: `cmd/flow-mcp/tools_project.go` (add `listProjectsTool` handler — note the name: `h.listProjects` is already a field)
- Modify: `cmd/flow-mcp/format.go` (add `formatProjects`)
- Modify: `cmd/flow-mcp/server.go` (register the tool in `newServerH`)
- Test: `cmd/flow-mcp/format_test.go` (test `formatProjects`)

**Interfaces:**
- Produces: tool `flow_list_projects` (no params) returning concise `name (slug) — id` lines; `formatProjects([]domain.Project) string`.

- [ ] **Step 1: Write the failing test for `formatProjects`**

Add to `cmd/flow-mcp/format_test.go`:

```go
func TestFormatProjects(t *testing.T) {
	ps := []domain.Project{
		{ID: "p1", Name: "Alpha", Slug: "alpha"},
		{ID: "p2", Name: "Beta Project", Slug: "beta"},
	}
	got := formatProjects(ps)
	for _, want := range []string{"Alpha", "alpha", "p1", "Beta Project", "beta", "p2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatProjects missing %q in:\n%s", want, got)
		}
	}
	if formatProjects(nil) == "" {
		t.Fatal("formatProjects(nil) must return a non-empty 'no projects' message")
	}
}
```

Ensure `format_test.go` imports `strings` and `github.com/serverkraken/flow/internal/domain` (match the existing imports; add only what is missing).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/flow-mcp/ -run TestFormatProjects -v`
Expected: FAIL — `formatProjects` undefined.

- [ ] **Step 3: Implement `formatProjects`**

Add to `cmd/flow-mcp/format.go`:

```go
// formatProjects renders the project list as concise lines the model can read
// to pick an existing project before binding.
func formatProjects(ps []domain.Project) string {
	if len(ps) == 0 {
		return "No projects yet. Use flow_bind_project with create_name to make one."
	}
	var b strings.Builder
	for _, p := range ps {
		fmt.Fprintf(&b, "%s (%s) — %s\n", p.Name, p.Slug, p.ID)
	}
	return strings.TrimRight(b.String(), "\n")
}
```

Confirm `format.go` imports `fmt`, `strings`, and `github.com/serverkraken/flow/internal/domain` (add any missing).

- [ ] **Step 4: Add the tool handler**

Add to `cmd/flow-mcp/tools_project.go`:

```go
// listProjectsIn has no parameters.
type listProjectsIn struct{}

// listProjectsTool lists all projects (id/name/slug) so the model can pick an
// existing one before binding instead of duplicate-creating.
func (h *handlers) listProjectsTool(ctx context.Context, _ *mcp.CallToolRequest, _ listProjectsIn) (*mcp.CallToolResult, any, error) {
	var ps []domain.Project
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		got, e := c.ListProjects(ctx)
		if e != nil {
			return e
		}
		ps = got
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(formatProjects(ps)), nil, nil
}
```

Add `"github.com/serverkraken/flow/internal/domain"` to `tools_project.go` imports if not already present.

- [ ] **Step 5: Register the tool in `newServerH`**

In `cmd/flow-mcp/server.go`, immediately after the `flow_delete_doc` `mcp.AddTool` block (~line 88), add:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list_projects",
		Description: "List all flow projects (id, name, slug). Use this to find an existing project before flow_bind_project, to avoid creating a duplicate.",
	}, h.listProjectsTool)
```

- [ ] **Step 6: Run build + format test**

Run: `go build ./cmd/flow-mcp/ && go test ./cmd/flow-mcp/ -run TestFormatProjects -v`
Expected: build OK; PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/flow-mcp/tools_project.go cmd/flow-mcp/format.go cmd/flow-mcp/format_test.go cmd/flow-mcp/server.go
git commit -m "feat(flow-mcp): flow_list_projects tool (10/11)" # (+ standard trailers)
```

---

### Task 2: `flow_bind_project` tool (bind.go core + handler + re-resolve + register → 11)

**Files:**
- Create: `cmd/flow-mcp/bind.go` (`validateBindRef`, `decideBindKind`, `(*handlers).bindProjectCore`)
- Create: `cmd/flow-mcp/bind_test.go` (unit tests for the pure helpers)
- Modify: `cmd/flow-mcp/tools_project.go` (`bindProjectIn` + `bindProject` handler; fix the stale `flow_project_context` text)
- Modify: `cmd/flow-mcp/server.go` (extract `refreshResolved` from `postAuthInit`; register `flow_bind_project`)
- Modify: `cmd/flow-mcp/loopback_test.go` (11-tool surface + bind integration)

**Interfaces:**
- Consumes: `h.lookupProject` (Task source: existing scope.go), `h.refreshResolved` (defined here), apiclient `CreateProject`/`BindRemote`/`BindPath`, `gitremote.OriginSlug`, `clientmachine.Load`.
- Produces: tool `flow_bind_project` (params `project?` xor `create_name?`, `kind?`); `validateBindRef(in bindProjectIn) error`; `decideBindKind(kindOverride string, originOK bool) (string, error)`; `(*handlers).bindProjectCore(ctx, c, in, originSlug, originOK, machine, cwd) (domain.Project, string, error)`; `(*handlers).refreshResolved(ctx, c)`.

- [ ] **Step 1: Write the failing unit tests (pure helpers)**

Create `cmd/flow-mcp/bind_test.go`:

```go
package main

import "testing"

func TestValidateBindRef(t *testing.T) {
	cases := []struct {
		name    string
		in      bindProjectIn
		wantErr bool
	}{
		{"project only", bindProjectIn{Project: "alpha"}, false},
		{"create only", bindProjectIn{CreateName: "New"}, false},
		{"both", bindProjectIn{Project: "alpha", CreateName: "New"}, true},
		{"neither", bindProjectIn{}, true},
		{"whitespace neither", bindProjectIn{Project: "  ", CreateName: " "}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateBindRef(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("validateBindRef(%#v) err=%v, wantErr=%v", c.in, err, c.wantErr)
			}
		})
	}
}

func TestDecideBindKind(t *testing.T) {
	cases := []struct {
		name     string
		override string
		originOK bool
		want     string
		wantErr  bool
	}{
		{"auto with origin", "", true, "remote", false},
		{"auto without origin", "", false, "path", false},
		{"force remote with origin", "remote", true, "remote", false},
		{"force remote without origin", "remote", false, "", true},
		{"force path", "path", false, "path", false},
		{"force path even with origin", "path", true, "path", false},
		{"invalid", "bogus", true, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := decideBindKind(c.override, c.originOK)
			if (err != nil) != c.wantErr {
				t.Fatalf("decideBindKind(%q,%v) err=%v wantErr=%v", c.override, c.originOK, err, c.wantErr)
			}
			if got != c.want {
				t.Fatalf("decideBindKind(%q,%v)=%q want %q", c.override, c.originOK, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/flow-mcp/ -run 'TestValidateBindRef|TestDecideBindKind' -v`
Expected: FAIL — `bindProjectIn`, `validateBindRef`, `decideBindKind` undefined.

- [ ] **Step 3: Create `bind.go`**

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/domain"
)

// validateBindRef enforces exactly one of project / create_name.
func validateBindRef(in bindProjectIn) error {
	hasRef := strings.TrimSpace(in.Project) != ""
	hasCreate := strings.TrimSpace(in.CreateName) != ""
	if hasRef == hasCreate {
		return errGuard{errors.New(`give either "project" (an existing project id/slug/name) or "create_name" (to create one), not both or neither`)}
	}
	return nil
}

// decideBindKind picks the binding kind. An explicit override wins ("remote"
// requires a git origin); otherwise a git origin → remote, else path.
func decideBindKind(kindOverride string, originOK bool) (string, error) {
	switch strings.TrimSpace(kindOverride) {
	case "remote":
		if !originOK {
			return "", errGuard{errors.New(`kind "remote" needs a git origin in this directory; use "path" or omit kind`)}
		}
		return "remote", nil
	case "path":
		return "path", nil
	case "":
		if originOK {
			return "remote", nil
		}
		return "path", nil
	default:
		return "", errGuard{fmt.Errorf(`invalid kind %q; use "remote" or "path", or omit to auto-detect`, kindOverride)}
	}
}

// bindProjectCore validates the request, resolves or creates the target
// project, then binds the cwd to it (remote-slug or per-device path). It is a
// method so it can reuse the cached project-ref lookup; all IO that needs the
// environment (git origin, machine id, cwd) is passed in for testability.
func (h *handlers) bindProjectCore(ctx context.Context, c *apiclient.Client, in bindProjectIn, originSlug string, originOK bool, machine clientmachine.Machine, cwd string) (domain.Project, string, error) {
	if err := validateBindRef(in); err != nil {
		return domain.Project{}, "", err
	}
	kind, err := decideBindKind(in.Kind, originOK)
	if err != nil {
		return domain.Project{}, "", err
	}
	var proj domain.Project
	if name := strings.TrimSpace(in.CreateName); name != "" {
		proj, err = c.CreateProject(ctx, name)
	} else {
		proj, err = h.lookupProject(ctx, strings.TrimSpace(in.Project))
	}
	if err != nil {
		return domain.Project{}, "", err
	}
	switch kind {
	case "remote":
		if _, err := c.BindRemote(ctx, proj.ID, originSlug); err != nil {
			return domain.Project{}, "", err
		}
	case "path":
		if machine.ID == "" {
			return domain.Project{}, "", errGuard{errors.New("cannot determine this device's machine id for a path binding")}
		}
		if _, err := c.BindPath(ctx, proj.ID, machine.ID, machine.Label, filepath.Clean(cwd)); err != nil {
			return domain.Project{}, "", err
		}
	}
	return proj, kind, nil
}
```

- [ ] **Step 4: Run the unit tests (GREEN for the pure helpers)**

Run: `go test ./cmd/flow-mcp/ -run 'TestValidateBindRef|TestDecideBindKind' -v`
Expected: PASS. (`bindProjectCore` has no caller yet — Step 5 adds it before any lint run.)

- [ ] **Step 5: Add the handler + fix the stale context text**

In `cmd/flow-mcp/tools_project.go`, add (and add imports `os`, `github.com/serverkraken/flow/internal/clientmachine`, `github.com/serverkraken/flow/internal/gitremote` as needed):

```go
// bindProjectIn binds the current working directory to a project.
type bindProjectIn struct {
	Project    string `json:"project,omitempty" jsonschema:"an existing project to bind: id, slug, or name"`
	CreateName string `json:"create_name,omitempty" jsonschema:"create a new project with this name, then bind to it"`
	Kind       string `json:"kind,omitempty" jsonschema:"binding kind override: 'remote' (git origin) or 'path' (this directory); omit to auto-detect"`
}

// bindProject binds this directory to a project (remote-slug if a git origin is
// present, else a per-device path binding), creating the project first when
// create_name is given, then re-resolves so subsequent tools are scoped here.
func (h *handlers) bindProject(ctx context.Context, _ *mcp.CallToolRequest, in bindProjectIn) (*mcp.CallToolResult, any, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return errorResult("cannot determine the working directory: " + err.Error()), nil, nil
	}
	originSlug, originOK, _ := gitremote.OriginSlug(cwd)
	machine, _ := clientmachine.Load() // best-effort; the path branch validates machine.ID
	var bound domain.Project
	var kind string
	derr := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		p, k, e := h.bindProjectCore(ctx, c, in, originSlug, originOK, machine, cwd)
		if e != nil {
			return e
		}
		bound, kind = p, k
		h.refreshResolved(ctx, c)
		return nil
	})
	if derr != nil {
		return h.resultErr(derr), nil, nil
	}
	msg := fmt.Sprintf("Bound this directory to project %s (%s) via %s binding. flow_project_context now resolves here.", bound.Name, bound.Slug, kind)
	return textResult(msg), nil, nil
}
```

Also fix the now-stale hint in `projectContext` (tools_project.go ~line 25): replace
`"No flow project is bound to this directory. Set FLOW_PROJECT, or bind it (flow_bind_project, coming in a later version)."`
with
`"No flow project is bound to this directory. Bind it with flow_bind_project, or set FLOW_PROJECT."`

- [ ] **Step 6: Extract `refreshResolved` + register the tool**

In `cmd/flow-mcp/server.go`, replace `postAuthInit` with an extracted `refreshResolved` (behavior-preserving):

```go
// refreshResolved re-resolves the cwd→project and re-registers the project's
// documents as resources, overwriting the resolved state under projMu. Run once
// by postAuthInit and again by flow_bind_project after a successful bind.
func (h *handlers) refreshResolved(ctx context.Context, c *apiclient.Client) {
	proj, matched := resolveProject(ctx, c, mcpLog())
	h.projMu.Lock()
	h.proj, h.matched = proj, matched
	h.projMu.Unlock()
	if err := h.registerResources(ctx, c); err != nil {
		mcpLog().Warn("could not register document resources", "err", err)
	}
}

// postAuthInit runs once on the first successful auth (mgr.onAuth).
func (h *handlers) postAuthInit(ctx context.Context, c *apiclient.Client) {
	h.refreshResolved(ctx, c)
}
```

(Note: `refreshResolved` re-registers for the newly resolved project. The primary flow is unbound→bound, where the old resource set is empty. Re-binding an already-bound directory to a *different* project may leave the prior project's resources registered — accepted V1 limitation; tools are the reliable path, resources the bonus.)

Then register the tool — in `newServerH`, after the `flow_list_projects` block from Task 1:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_bind_project",
		Description: "Bind the current working directory to a flow project so other tools auto-scope here. Pass project (existing id/slug/name) or create_name (to create one). Auto-detects a git-origin (remote) vs per-device (path) binding; override with kind.",
	}, h.bindProject)
```

- [ ] **Step 7: Extend the loopback integration test**

In `cmd/flow-mcp/loopback_test.go`, extend the existing httptest fixture backend to also serve the project + binding endpoints the new tools hit, mirroring how apiclient calls them (check `internal/adapter/apiclient/client.go` `CreateProject`/`ListProjects` and `internal/adapter/apiclient/projectbindings.go` `BindRemote`/`BindPath` for the exact method + path + JSON shapes). Add these assertions (follow the existing loopback test style — call tools by name over the in-memory transport against the real httptest backend):

1. **Tool surface = 11**: the advertised tool list now includes `flow_list_projects` and `flow_bind_project` (assert `len == 11` and both names present).
2. **`flow_list_projects`** returns the fixture projects (assert a known project name/slug appears in the result text).
3. **`flow_bind_project` create-then-bind**: call with `create_name:"Scratch"`; assert success text names the project and the backend received a `CreateProject` then a bind call.
4. **`flow_bind_project` error**: call with neither `project` nor `create_name`; assert `IsError` with the "either project or create_name" message.
5. **Re-resolve after bind**: after a successful bind to a project, `flow_project_context` reports that project (the binding fixture should make `resolveProject` return it). If wiring `resolveProject` through the fixture is impractical in-loopback, assert instead that the bind handler called `refreshResolved` by observing the resolved project via a follow-up scoped read; document whichever you used.

Set the loopback fixture's resolution inputs (env `FLOW_PROJECT` or the `ResolveProject` endpoint) so the bind+re-resolve path is exercised; if the test process has a git origin/cwd that interferes, force `kind` explicitly in the test calls to keep them deterministic.

- [ ] **Step 8: Run flow-mcp tests + build**

Run:
```bash
go build ./cmd/flow-mcp/
go test ./cmd/flow-mcp/ -v
```
Expected: PASS (pure unit tests + extended loopback). If `unused` lint later flags anything, ensure every new symbol has a caller or test.

- [ ] **Step 9: Commit**

```bash
git add cmd/flow-mcp/bind.go cmd/flow-mcp/bind_test.go cmd/flow-mcp/tools_project.go cmd/flow-mcp/server.go cmd/flow-mcp/loopback_test.go
git commit -m "feat(flow-mcp): flow_bind_project tool + re-resolve after bind (11/11)" # (+ standard trailers)
```

---

### Task 3: Close the rolled-up MCP Minors

**Files (touch-ups across):**
- `cmd/flow-mcp/write.go` (or wherever `requireType` lives) — share `typeList()`
- `cmd/flow-mcp/tools_write.go` — align create/update success-message format
- `cmd/flow-mcp/server.go` — `mcpLog()` → package-level logger var
- `cmd/flow-mcp/resources.go` — `removeResource`-on-absent comment
- `cmd/flow-mcp/scope_test.go` — err-guards on global/none cases
- `cmd/flow-mcp/auth_manager_test.go` — add `builds==1` assertion
- `cmd/flow-mcp/loopback_write_test.go` — check `ListResources` errors after create/delete
- `internal/clientauth/clientauth_errors_test.go` — drop the inert `wrapped:=…;_=wrapped` lines
- `cmd/flow-mcp/loopback_test.go` (and any test starting a server goroutine) — add a cancel path

**Interfaces:** none new — behavior-preserving cleanups + test hardening.

**Do NOT touch (intentional keeps — flag to the reviewer):** `document_types_test.go` hardcoded `8` (drift guard); `searchDocsIn.Limit` negative→default clamp; `format.go` `TrimRight`. The oauth2 coverage and the scope-error rewording were already fixed in `3aa4ba4` — confirm they are present, do not redo them.

- [ ] **Step 1: Survey the exact sites**

Run, and read each hit before editing:
```bash
rg -n "requireType|typeList|mcpLog\(\)|removeResource|builds|wrapped\s*:?=|ListResources" cmd/flow-mcp internal/clientauth
rg -n "g, _ :=|, _ =" cmd/flow-mcp/scope_test.go
```
Note which sites still apply (some may have shifted after durable-auth). Skip any that no longer exist and record that in the report.

- [ ] **Step 2: Code cleanups (each behavior-preserving)**

- `requireType`'s invalid-type branch: build its error via the shared `typeList()` (scope.go) instead of an inline duplicated type loop.
- `mcpLog()` in `server.go`: replace the per-call `slog.New(...)` with a package-level `var mcpLogger = slog.New(slog.NewTextHandler(os.Stderr, nil))` and have `mcpLog()` return it (or replace `mcpLog()` calls with `mcpLogger`). Keep stderr (never stdout).
- `tools_write.go`: make the create and update success messages use one consistent format (e.g. both show `path` and `type`, or factor a shared `formatWriteResult`); pick the create message's shape as canonical.
- `resources.go`: one-line comment on `removeResource` noting it is a no-op when the id is absent (SDK-safe).
- `internal/clientauth/clientauth_errors_test.go`: delete the inert `wrapped := errors.New("x"); _ = wrapped` scaffold lines.
- If the `&h.proj.ID` address-of-field note still applies after the durable-auth refactor, add a one-line comment; if it no longer exists, note that.

- [ ] **Step 3: Test hardening**

- `scope_test.go`: the global/none sentinel cases that do `g, _ := h.resolveScope(...)` → capture and `t.Fatal(err)` on a non-nil error.
- `auth_manager_test.go` `TestAuthManager_Do_NonAuthErrorNotRetried`: add an assertion that `builds == 1` (no rebuild on a non-auth error), alongside the existing `calls == 1`.
- `loopback_write_test.go`: where `ListResources` is called after create/delete and the error is discarded, check it (`if err != nil { t.Fatal(err) }`).
- Any test that launches the server in a goroutine without a cancel path: wrap with `ctx, cancel := context.WithCancel(...)` + `t.Cleanup(cancel)`.

- [ ] **Step 4: Verify**

Run:
```bash
go build ./...
go test ./cmd/flow-mcp/ ./internal/clientauth/ -v
make lint
```
Expected: PASS; `make lint` → 0 issues.

- [ ] **Step 5: Commit**

```bash
git add cmd/flow-mcp/ internal/clientauth/
git commit -m "chore(flow-mcp): close rolled-up MCP minors (DRY + test gaps + cleanups)" # (+ standard trailers)
```

---

### Task 4: Registration runbook

**Files:**
- Create: `docs/runbook/flow-mcp-setup.md`

**Interfaces:** none (docs).

- [ ] **Step 1: Confirm the `.mcp.json` / `claude mcp add` format**

Use the `claude-docs-consultant` skill to fetch the current, exact schema for registering a project-scoped stdio MCP server (the `.mcp.json` `mcpServers` entry shape and the `claude mcp add` invocation, including how `env` is passed). Do not hand-wave the format — quote the confirmed shape.

- [ ] **Step 2: Write the runbook**

Create `docs/runbook/flow-mcp-setup.md` covering, in order:
1. Build: `go build -o bin/flow-mcp ./cmd/flow-mcp`.
2. The `.mcp.json` entry (confirmed format): `command` = the built binary's absolute path; `env` = `FLOW_SERVER_URL` (e.g. `https://flow.thebackend.org`) and `FLOW_OIDC_ISSUER` (the flow-cli issuer URL). Show a complete example block.
3. Auth: `flow login` (device-flow) in a terminal on this device, then `/mcp` reconnect in Claude Code. Note the durable-reauth behavior (Säule A): after the token expires, `flow login` again restores tools **without** a reconnect.
4. The 11-tool surface (one line each: project_context, search_docs, list_docs, get_doc, list_tags, backlinks, create_doc, update_doc, delete_doc, list_projects, bind_project).
5. Binding a repo: `flow_bind_project` (auto-detect git-origin vs path); verify with `flow_project_context`.
6. Degraded mode: before login every tool returns "Login required"; this is expected.
7. Clean shutdown: the process exits 0 when the host closes the connection.

- [ ] **Step 3: Commit**

```bash
git add docs/runbook/flow-mcp-setup.md
git commit -m "docs(flow-mcp): registration + operations runbook" # (+ standard trailers)
```

---

### Task 5: Wiring verification + live done-gate (CONTROLLER)

**Files:** verify only (no new code unless a gap is found).

- [ ] **Step 1: Full CI**

Run (with the podman env exported, per Global Constraints):
```bash
make ci
```
Expected: PASS — lint 0, verify-generate clean, coverage ≥ 80%, build ok, pgstore Docker tests RAN (not skipped). If coverage dipped, add a focused test for the lowest-covered new code (`formatProjects`, `decideBindKind` branches).

- [ ] **Step 2: Main-wiring audit (per `feedback_plan_main_wiring_task`)**

Run:
```bash
go build -o bin/flow-mcp ./cmd/flow-mcp && echo build-ok
rg -n 'mcp.AddTool' cmd/flow-mcp/server.go | wc -l   # expect 11
```
Then start the built binary with a throwaway in-process `CommandTransport` client (mirror the durable-auth main-wiring smoke) and assert exactly **11** tools are advertised: flow_project_context, flow_search_docs, flow_list_docs, flow_get_doc, flow_list_tags, flow_backlinks, flow_create_doc, flow_update_doc, flow_delete_doc, **flow_list_projects, flow_bind_project**. Confirm stdout carries only the JSON-RPC stream (no stray writes from `bind.go`).

- [ ] **Step 3: Assert clean-disconnect exit 0**

Drive the built binary over a stdio pipe; on host-initiated close, confirm the process exits 0 (only abnormal termination is non-zero). Record the observation (closes the 2a os.Exit Minor).

- [ ] **Step 4: Live done-gate vs PROD**

The repo `.mcp.json` points at `flow.thebackend.org`. Rebuild `bin/flow-mcp` at the `.mcp.json` path, `/mcp` reconnect in a real Claude Code session (after `flow login` if the token is stale), then:
1. `flow_list_projects` lists the real projects.
2. **`flow_bind_project` binds THIS repo** (flow-rebuild) → `flow_project_context` now resolves it → `flow://doc` resources appear.
3. A scoped `flow_search_docs` finds a doc in the bound project; `project:"global"` widens it.
4. A guarded `flow_update_doc` on a human-owned doc is refused without `confirm`, accepted with.
5. Clean up any test docs/bindings created during the gate.

Document the outcome in the final commit message or a follow-up note.

- [ ] **Step 5: Final commit / branch note**

```bash
git commit --allow-empty -m "chore(flow-mcp): slice 2d done-gate (11 tools, CI green, wiring audited, live PROD dogfood)" # (+ standard trailers)
```

---

## Self-Review

**Spec coverage:** §1 scope → all tasks. `flow_list_projects` → Task 1. `flow_bind_project` (bind.go, kind-detect, find-or-create, BindRemote/BindPath, re-resolve, errors) → Task 2. §5 Minors → Task 3. §6 runbook → Task 4. §7 done-gate (CI, main-wiring, exit-0, live PROD) → Task 5. §8 testing → folded into Tasks 1-2 (unit + loopback) and Task 5 (CI). §2 integration surface used verbatim (mgr.Do, resultErr, lookupProject, projectList, resolved).

**Placeholder scan:** No TBD/TODO. Task 2 Step 7 (loopback fixture extension) and Task 4 Step 1 (`.mcp.json` format) defer to the existing fixture pattern / `claude-docs-consultant` rather than inventing a backend or a config shape that may not match — each lists exact assertions / exact content to produce, the same way the codebase's existing loopback tests are written; intentional, not a placeholder.

**Type consistency:** `bindProjectIn{Project,CreateName,Kind}` is identical in `bind.go` (validateBindRef), `bind_test.go`, and the handler. `decideBindKind(string,bool)→(string,error)` and `validateBindRef(bindProjectIn)→error` match between definition, tests, and `bindProjectCore`. `bindProjectCore(ctx,c,in,originSlug,originOK,machine,cwd)→(domain.Project,string,error)` matches its handler call site. `refreshResolved(ctx,c)` defined in server.go, called by `postAuthInit` and `bindProject`. Tool handler signatures match the as-built shape. `formatProjects([]domain.Project)→string` matches its test and caller. Tool count 9→10 (Task 1)→11 (Task 2) is consistent.
