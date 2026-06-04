# flow Phase 1 — M5 flow-mcp Implementation Plan (Plan D)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `cmd/flow-mcp` — a stdio MCP server that lets Claude Code / Cursor / Codex read+write RepoNotes and drive Worktime without leaving its host editor. Shares the local sqliteclient cache and httpsync worker with the existing `flow` CLI/TUI, so MCP work syncs the same way TUI work does.

**Architecture:** New `cmd/flow-mcp/main.go` reuses the M4 use cases (`usecase.RepoNotes`, `usecase.ActiveSessions`, etc.) and the sqliteclient cache; only the transport differs (stdio JSON-RPC framing instead of cobra). MCP protocol implementation: the official `github.com/modelcontextprotocol/go-sdk` if that's stable enough; otherwise a minimal hand-rolled `internal/adapter/mcpstdio` that speaks the MCP 2024-11-05 protocol (init / list_tools / call_tool / list_resources / read_resource). Auth: MCP server boots with OIDC token from the keyring (same slot as `flow`); if no token, every tool returns a `"Login required"` error pointing the user to `flow login`.

**Tech Stack:** Go 1.24. Protocol library: `github.com/modelcontextprotocol/go-sdk` (check current state) — fallback to a minimal stdio JSON-RPC implementation inside the repo. No new runtime deps if we go hand-rolled.

**Prerequisite:** Plan C (M4 RepoNotes-Sync) must be merged onto `next` — flow-mcp's most valuable tool surface (`flow_get_repo_note` / `flow_save_repo_note`) depends on `usecase.RepoNotes`.

---

## File Structure

**Create (cmd):**
- `cmd/flow-mcp/main.go` — composition root. Opens cache.db, builds use cases, instantiates MCP server, runs `Serve(os.Stdin, os.Stdout)`.

**Create (mcp adapter):**
- `internal/adapter/mcpstdio/server.go` — stdio JSON-RPC loop with MCP 2024-11-05 dispatch. Decision-point: if `github.com/modelcontextprotocol/go-sdk` is solid, use it and this file is a thin wrapper.
- `internal/adapter/mcpstdio/server_test.go` — round-trip test (write init, read response).
- `internal/adapter/mcpstdio/protocol.go` — Go structs for `InitializeRequest`, `Tool`, `Resource`, `CallToolRequest/Result` etc. Skip if using the official SDK.

**Create (use case — MCP tool handlers):**
- `internal/usecase/mcp_tools.go` — `MCPTools{notes *RepoNotes, active *ActiveSessions, sessions *Sessions, worktimeReader *WorktimeReader, userID string}` with one method per tool. Each method takes the raw MCP `CallToolRequest.Arguments` map, validates, dispatches to the existing use case, formats the result for MCP.
- `internal/usecase/mcp_tools_test.go` — happy + auth-missing + invalid-args paths.

**Create (mcp tool router):**
- `internal/adapter/mcpstdio/router.go` — maps tool names to `MCPTools` methods. Lives in the adapter because it knows MCP error shape.

**Create (login check):**
- `cmd/flow-mcp/auth.go` — boot-time check that `keyringadapter.Get(slot)` returns a token; if not, runs the server in "auth-missing" mode where every tool returns `"Login required: run \`flow login\` in a terminal first."` per the spec.

**Create (docs):**
- `docs/runbook/flow-mcp-setup.md` — Claude Code MCP config snippet, troubleshooting (token expired, can't find cache.db, log location).
- `docs/runbook/claude-session-start-hook.md` — optional `.claude/hooks/load-repo-note.sh` example that auto-injects the current repo's RepoNote into the session context.

---

## Decision: official MCP SDK vs hand-rolled?

Before Task 1, run:

```bash
go list -m -versions github.com/modelcontextprotocol/go-sdk 2>&1
```

Acceptance criteria for using the SDK:
* Has a tagged release ≥ 0.1.0 (not a `v0.0.0-202…` master snapshot).
* Provides stdio transport out of the box.
* Allows custom error shapes for tool calls.

If the SDK is too young / pre-stable, fall back to a minimal `mcpstdio` package. The MCP wire format is small (newline-delimited JSON-RPC 2.0 + a couple of capability negotiation methods); ~300 lines of Go does it.

**Default assumption for this plan: hand-rolled `internal/adapter/mcpstdio`.** Re-evaluate at Task 1 — if the SDK is solid, Tasks 1+2 collapse into one wiring task.

---

## Task 1: mcpstdio protocol skeleton

**Files:**
- Create: `internal/adapter/mcpstdio/protocol.go`
- Create: `internal/adapter/mcpstdio/server.go`
- Create: `internal/adapter/mcpstdio/server_test.go`

- [ ] **Step 1: Protocol structs**

Cover the subset of the MCP 2024-11-05 spec we need: `initialize`, `initialized` notification, `tools/list`, `tools/call`, `resources/list`, `resources/read`, plus the JSON-RPC envelope.

```go
package mcpstdio

type Request struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      any             `json:"id,omitempty"`     // number, string, or null for notifications
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
    JSONRPC string `json:"jsonrpc"`
    ID      any    `json:"id"`
    Result  any    `json:"result,omitempty"`
    Error   *Error `json:"error,omitempty"`
}

type Error struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    any    `json:"data,omitempty"`
}

type Tool struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    InputSchema map[string]any `json:"inputSchema"`
}

type CallToolRequest struct {
    Name      string         `json:"name"`
    Arguments map[string]any `json:"arguments"`
}

type CallToolResult struct {
    Content []Content `json:"content"`
    IsError bool      `json:"isError,omitempty"`
}

type Content struct {
    Type string `json:"type"`
    Text string `json:"text,omitempty"`
}

type Resource struct {
    URI         string `json:"uri"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
}
```

- [ ] **Step 2: Server loop**

```go
type Server struct {
    impl Implementation
    log  *slog.Logger
}

type Implementation interface {
    // Tools returns the static tool catalog.
    Tools() []Tool
    // Resources returns the static resource catalog. Both lists may be empty.
    Resources() []Resource
    // CallTool dispatches a tool by name. Errors here become MCP "tool errors"
    // (IsError=true content), not JSON-RPC errors — Claude shows the message
    // to the user and continues. JSON-RPC errors signal protocol/transport
    // bugs.
    CallTool(name string, args map[string]any) CallToolResult
    // ReadResource returns the content of a resource URI.
    ReadResource(uri string) (Content, error)
}

func NewServer(impl Implementation, logger *slog.Logger) *Server {
    return &Server{impl: impl, log: logger}
}

// Serve reads newline-delimited JSON-RPC from r and writes responses to w.
// Returns when r returns io.EOF or an unrecoverable transport error. Each
// request is dispatched in-line — MCP servers are expected to be single-
// threaded per stdio pair.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
    dec := json.NewDecoder(r)
    enc := json.NewEncoder(w)
    for {
        var req Request
        if err := dec.Decode(&req); err != nil {
            if errors.Is(err, io.EOF) { return nil }
            return err
        }
        resp := s.dispatch(req)
        if req.ID == nil { continue } // notification, no response
        if err := enc.Encode(resp); err != nil { return err }
    }
}

func (s *Server) dispatch(req Request) Response {
    switch req.Method {
    case "initialize":      // negotiate capabilities, return server info
    case "tools/list":      // return s.impl.Tools()
    case "tools/call":      // unmarshal CallToolRequest, dispatch, wrap
    case "resources/list":  // return s.impl.Resources()
    case "resources/read":  // dispatch to s.impl.ReadResource
    default:                // -32601 Method not found
    }
}
```

- [ ] **Step 3: Test**

`server_test.go` uses two `io.Pipe`s — write a real `initialize` request, read the response, assert capabilities + server info.

- [ ] **Step 4: Commit**

```bash
go test ./internal/adapter/mcpstdio/... -v
git add internal/adapter/mcpstdio/
git commit -m "feat(mcpstdio): JSON-RPC server skeleton for MCP 2024-11-05"
```

---

## Task 2: MCPTools — use-case-shaped tool handlers

**Files:**
- Create: `internal/usecase/mcp_tools.go`
- Create: `internal/usecase/mcp_tools_test.go`

- [ ] **Step 1: MCPTools struct + tool catalog**

```go
type MCPTools struct {
    UserID         string
    Notes          *RepoNotes
    Active         *ActiveSessions
    Sessions       *Sessions
    WorktimeReader *WorktimeReader
    // Pwd is the working directory the MCP server boots from. RepoNote tools
    // use it to resolve the canonical key. Set once in main.go; tools that
    // need a different PWD (e.g. "what note for repo /other/path") take it
    // as an explicit argument.
    Pwd string
}

func (m *MCPTools) Catalog() []mcpstdio.Tool {
    return []mcpstdio.Tool{
        {
            Name: "flow_get_repo_note",
            Description: "Return the RepoNote for the given repo path (defaults to MCP-server PWD).",
            InputSchema: schema.Object().
                Optional("repo_path", schema.String()).
                Build(),
        },
        // ... eight more tools per the spec
    }
}
```

(The `schema` builder is a tiny helper to keep the JSON-Schema objects readable — add it inline if not worth its own file.)

- [ ] **Step 2: Tool implementations**

Each tool method returns `mcpstdio.CallToolResult`. Sample:

```go
func (m *MCPTools) flowGetRepoNote(args map[string]any) mcpstdio.CallToolResult {
    pwd, _ := args["repo_path"].(string)
    if pwd == "" { pwd = m.Pwd }
    note, repo, err := m.Notes.GetForPwd(m.UserID, pwd)
    if err != nil {
        return errorResult("flow_get_repo_note: " + err.Error())
    }
    return mcpstdio.CallToolResult{Content: []mcpstdio.Content{{
        Type: "text",
        Text: fmt.Sprintf("repo=%s content_bytes=%d\n%s",
            repo.CanonicalKey, len(note.Content), note.Content),
    }}}
}
```

Implement all nine tools from the spec:
* `flow_get_repo_note`, `flow_save_repo_note`, `flow_list_repo_notes`
* `flow_search_notes` (simple substring search via `notes.LoadAll` then filter — FTS5 is out of M5 scope)
* `flow_get_note`, `flow_save_note` — these target Kompendium notes; since Plan C deferred Kompendium sync, these tools point at the **local Kompendium store** directly (not the sqlite cache). Document that they're offline-only until a follow-up plan.
* `flow_worktime_status`, `flow_start_session`, `flow_stop_session`

- [ ] **Step 3: Tests**

Drive each tool against fake use cases. Verify the result shape + error path (invalid args → IsError=true, not JSON-RPC error).

- [ ] **Step 4: Commit**

```bash
go test ./internal/usecase/... -run TestMCPTools -v
git add internal/usecase/mcp_tools.go internal/usecase/mcp_tools_test.go
git commit -m "feat(usecase): MCPTools with nine tool handlers"
```

---

## Task 3: Resources — flow://repos/<key>/note

**Files:**
- Modify: `internal/usecase/mcp_tools.go`

- [ ] **Step 1: Implement Resources() + ReadResource**

The MCP server advertises one resource per known repo so Claude can auto-attach the right RepoNote when the user opens that repo's folder.

```go
func (m *MCPTools) ResourceCatalog() []mcpstdio.Resource {
    // List all repos for UserID. For each, build "flow://repos/<canonical-key>/note".
    repos, _ := m.Notes.ListRepos(m.UserID)
    out := make([]mcpstdio.Resource, 0, len(repos))
    for _, r := range repos {
        out = append(out, mcpstdio.Resource{
            URI: "flow://repos/" + url.PathEscape(r.CanonicalKey) + "/note",
            Name: r.DisplayName,
            Description: "RepoNote for " + r.CanonicalKey,
            MimeType: "text/markdown",
        })
    }
    return out
}

func (m *MCPTools) ReadResource(uri string) (mcpstdio.Content, error) {
    // Parse "flow://repos/<escaped-key>/note", look up the repo by canonical key,
    // return the RepoNote content as text/markdown.
}
```

`Notes.ListRepos(userID)` is a new method on `usecase.RepoNotes` — Task 1 here adds it via Plan C's `cacheRepos.PullSince` (passing since=0 returns everything).

- [ ] **Step 2: Test + commit**

```bash
go test ./internal/usecase/... -run "TestMCPTools_Resources" -v
git add internal/usecase/mcp_tools.go internal/usecase/mcp_tools_test.go
git commit -m "feat(usecase): expose RepoNotes via MCP Resource catalog"
```

---

## Task 4: cmd/flow-mcp main + boot-time auth check

**Files:**
- Create: `cmd/flow-mcp/main.go`
- Create: `cmd/flow-mcp/auth.go`

- [ ] **Step 1: Wiring**

```go
func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    logger := slog.New(slog.NewTextHandler(os.Stderr, nil)) // stdout reserved for MCP frames

    cacheStore, err := sqliteclient.Open(cacheDBPath())
    if err != nil { logger.Error("cache open", slog.Any("err", err)); os.Exit(1) }
    defer cacheStore.Close()

    // ... same wiring as cmd/flow/main.go: localUser, cacheRepos, cacheRepoNotes,
    // syncClient, syncWorker, repoNotesUC, activeSessionsUC, sessionsUC, reader.

    // Auth gate: if no token, every tool returns the same error.
    authed := hasValidToken(keyring, keyringSlot)

    tools := &usecase.MCPTools{
        UserID: localUser.ID,
        Notes:  repoNotesUC,
        Active: activeSessionsUC,
        Sessions: sessionsUC,
        WorktimeReader: reader,
        Pwd: pwdOrDot(),
    }
    impl := mcpstdio.Adapter(tools, authed)
    server := mcpstdio.NewServer(impl, logger)
    if err := server.Serve(os.Stdin, os.Stdout); err != nil {
        logger.Error("mcp serve", slog.Any("err", err))
        os.Exit(1)
    }
}
```

- [ ] **Step 2: auth.go**

```go
func hasValidToken(kr ports.TokenStore, slot string) bool {
    t, err := kr.Get(slot)
    if err != nil { return false }
    return !t.IsExpired()
}
```

`mcpstdio.Adapter(tools, false)` wraps a no-auth impl that replaces every tool with one that returns `"Login required: run \`flow login\` in a terminal first."` and an empty Resources catalog.

- [ ] **Step 3: Build verification**

```bash
go build ./cmd/flow-mcp
ls -lh ./flow-mcp # smoke that the binary exists
```

- [ ] **Step 4: Commit**

```bash
git add cmd/flow-mcp/
git commit -m "feat(flow-mcp): composition root + boot-time auth gate"
```

---

## Task 5: Loopback integration test

**Files:**
- Create: `cmd/flow-mcp/loopback_test.go`

- [ ] **Step 1: Test**

Spawn the binary via `exec.Command("./flow-mcp")`, write a real MCP init request, read the response, write a `tools/call` for `flow_get_repo_note` against a seeded sqlite cache + dex token, verify the result.

The test is `// +build integration` so it only runs when explicitly enabled (`go test -tags integration ./cmd/flow-mcp/...`).

- [ ] **Step 2: Commit**

```bash
go test -tags integration ./cmd/flow-mcp/... -v
git add cmd/flow-mcp/loopback_test.go
git commit -m "test(flow-mcp): stdio loopback integration test"
```

---

## Task 6: Docs

**Files:**
- Create: `docs/runbook/flow-mcp-setup.md`
- Create: `docs/runbook/claude-session-start-hook.md`

- [ ] **Step 1: flow-mcp setup runbook**

Cover:
* Build: `go build -o ~/bin/flow-mcp ./cmd/flow-mcp`
* Claude Code MCP config (`~/.claude/claude_desktop_config.json` snippet)
* Troubleshooting: token expired (`flow login`), can't open cache.db (XDG path explanation), MCP server logs to stderr.

- [ ] **Step 2: SessionStart hook example**

```bash
#!/usr/bin/env bash
# .claude/hooks/load-repo-note.sh
# Auto-inject the RepoNote for the current working directory.
note=$(flow repo note get 2>/dev/null || true)
if [[ -n "$note" ]]; then
  cat <<EOF
<system-reminder>
RepoNote for $(basename "$PWD"):
$note
</system-reminder>
EOF
fi
```

(Notes: this hook is optional — flow-mcp's `flow://repos/<key>/note` Resource is the canonical way; the hook is for users who prefer hooks over MCP Resources.)

- [ ] **Step 3: Commit**

```bash
git add docs/runbook/flow-mcp-setup.md docs/runbook/claude-session-start-hook.md
git commit -m "docs(flow-mcp): setup runbook + SessionStart-hook example"
```

---

## Task 7: Verification

- [ ] **Step 1: Build all binaries**

```bash
go build ./...
ls -lh ./flow ./flow-server ./flow-mcp
```

- [ ] **Step 2: Manual MCP-Inspector smoke**

If `npx @modelcontextprotocol/inspector` is available, point it at the local `flow-mcp` binary and verify:
* Server initializes cleanly
* Nine tools listed
* Resources list (one per known repo)
* `flow_get_repo_note` against the test repo returns content

- [ ] **Step 3: Claude Code integration smoke**

Add `flow-mcp` to `~/.claude/claude_desktop_config.json` server list, restart Claude Code, ask it `"What's in the current RepoNote?"`. Expected: Claude calls `flow_get_repo_note` and shows the content.

- [ ] **Step 4: make ci**

```bash
make ci
```

Expected: PASS, coverage ≥ 85%. The new MCP code is unit-tested via the loopback test + MCPTools fakes.

- [ ] **Step 5: Memory update**

Note M5 completion in `project_plan_b_progress.md` + MEMORY.md, including the commit range and the decision (SDK vs hand-rolled).
