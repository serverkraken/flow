# flow-mcp Setup

`flow-mcp` is a stdio MCP server that exposes flow's RepoNote and Worktime use cases to MCP-aware clients (Claude Code, Cursor, Codex). It shares the local sqliteclient cache + httpsync worker with the `flow` CLI/TUI binaries, so notes written via MCP sync identically to notes written via the TUI.

## Build

```bash
go build -o ~/bin/flow-mcp ./cmd/flow-mcp
```

## Authenticate

`flow-mcp` reads OIDC tokens from the OS keychain (same slot as `flow`). Log in once via the CLI:

```bash
flow login --server=https://flow.example.com
```

Without a valid token, every MCP tool returns `Login required: run `flow login` in a terminal first.` and the resource catalog is empty — boot still succeeds so the MCP client can surface a helpful error to the user.

## Claude Code config

Add to `~/.claude/claude_desktop_config.json` (or your platform's equivalent):

```json
{
  "mcpServers": {
    "flow": {
      "command": "/Users/you/bin/flow-mcp",
      "env": {
        "FLOW_SERVER_URL": "https://flow.example.com"
      }
    }
  }
}
```

Restart Claude Code. The seven `flow_*` tools should appear in the tool list.

## Tools shipped in M5

| Tool                   | Purpose                                                   |
| ---------------------- | --------------------------------------------------------- |
| `flow_get_repo_note`   | Read the RepoNote for a path (defaults to PWD).           |
| `flow_save_repo_note`  | Overwrite the RepoNote for a path. Syncs to the server.   |
| `flow_list_repo_notes` | Discover every known repo + note size.                    |
| `flow_search_notes`    | Substring search across every RepoNote.                   |
| `flow_worktime_status` | Today's logged time, target, active sessions.             |
| `flow_start_session`   | Start a worktime session for a project.                   |
| `flow_stop_session`    | Stop the active worktime session for a project.           |

Two additional tools listed in the original plan — `flow_get_note` / `flow_save_note` for Kompendium notes — are deferred to a follow-up plan. They need a new note-IO port that the strict depguard layering blocks from M5; the workaround does not fit M5's minimal scope.

## Resources

`flow-mcp` advertises one resource per known repo at `flow://repos/<canonical-key>/note` with `text/markdown` mime type. MCP clients that auto-attach resources can show the right RepoNote when the user opens the matching repo folder.

## Environment

| Variable                | Default                                | Purpose                                          |
| ----------------------- | -------------------------------------- | ------------------------------------------------ |
| `FLOW_SERVER_URL`       | `http://localhost:8080`                | flow-server base URL for sync.                   |
| `FLOW_CACHE_DB`         | `$XDG_DATA_HOME/flow/cache.db`         | Shared cache with `flow` CLI/TUI.                |
| `FLOW_LOCAL_USER_SUB`   | `local`                                | Local OIDC sub for the cache `users` row.        |

## Troubleshooting

### Every tool says "Login required"

The keyring has no valid token for `tokens:$FLOW_SERVER_URL`. Run `flow login --server=$FLOW_SERVER_URL` in a normal terminal. The keychain entry expires when the OIDC refresh token does (Authentik default: 30 days).

### "cache open" error in stderr

The XDG path is wrong or unwritable. Set `FLOW_CACHE_DB` to a writable path and ensure the parent dir exists (`flow-mcp` mkdirs the parent but not the grandparent).

### Logs end up on the wrong stream

Diagnostic logging is **always** stderr. Stdout carries newline-delimited JSON-RPC. If your MCP client shows raw log lines as messages, the client is reading stderr — file a bug against the client, not against `flow-mcp`.

### MCP client doesn't pick up new repo notes

The resource catalog is regenerated on every `resources/list`. If the client caches it aggressively, restart the MCP server (Claude Code: toggle the server off + on in the tool palette).

## Manual smoke

```bash
# 1. Initialize:
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize"}' | flow-mcp 2>/dev/null

# 2. List tools:
printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | flow-mcp 2>/dev/null

# 3. Read the current RepoNote (cwd must be a git repo or a known repo):
printf '%s\n' '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"flow_get_repo_note","arguments":{}}}' | flow-mcp 2>/dev/null
```

## See also

- [mcp-session-start-hook.md](mcp-session-start-hook.md) — optional alternative to MCP Resources for auto-injecting RepoNotes (Claude Code SessionStart hook).
- [Plan D](../superpowers/plans/2026-06-04-flow-phase1-m5-mcp-server.md) — the implementation plan this runbook covers.
