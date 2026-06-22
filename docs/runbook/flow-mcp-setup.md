# flow-mcp: Registration and Operations Runbook

This runbook covers building the flow-mcp binary, registering it with Claude Code, authenticating, and operating it day-to-day.

---

## 1. Build

From the repository root:

```bash
go build -o bin/flow-mcp ./cmd/flow-mcp
```

The output binary is `bin/flow-mcp`. The `command` field in `.mcp.json` must be an absolute path because Claude Code spawns stdio servers directly; a relative path will not resolve. Rebuild whenever you pull changes to `cmd/flow-mcp` or its dependencies.

---

## 2. Registration

### Option A: Hand-write `.mcp.json` (recommended for shared repos)

Create or update `.mcp.json` at the repository root. This file is project-scoped: it is committed to version control and loaded automatically by every team member who opens the project in Claude Code.

```json
{
  "mcpServers": {
    "flow": {
      "command": "/absolute/path/to/flow-rebuild/bin/flow-mcp",
      "env": {
        "FLOW_SERVER_URL": "https://flow.thebackend.org",
        "FLOW_OIDC_ISSUER": "https://id.thebackend.org/application/o/flow-cli/"
      }
    }
  }
}
```

Replace the `command` value with the absolute path on your machine. The working `.mcp.json` at the root of this repository already contains the correct values for the primary development machine; other contributors must update `command` to match their local checkout path.

`FLOW_SERVER_URL` is the base URL of the flow API server. `FLOW_OIDC_ISSUER` is the OIDC issuer URL used to validate tokens (the same value passed to `flow login`).

### Option B: `claude mcp add` command

The following command registers the server at project scope, writing the same entry to `.mcp.json`:

```bash
claude mcp add \
  --transport stdio \
  --scope project \
  --env FLOW_SERVER_URL=https://flow.thebackend.org \
  --env FLOW_OIDC_ISSUER=https://id.thebackend.org/application/o/flow-cli/ \
  flow \
  -- /absolute/path/to/flow-rebuild/bin/flow-mcp
```

Key points from the official docs (confirmed from `https://code.claude.com/docs/en/mcp.md`):
- `--scope project` writes the entry to `.mcp.json` (shared via version control).
- `--scope local` (the default) writes to `~/.claude.json` and is private to your machine.
- `--scope user` writes to `~/.claude.json` and is available across all your projects.
- `--env` accepts multiple `KEY=value` pairs. Place at least one other flag between `--env` and the server name to avoid the CLI misreading the name as another key-value pair.
- The `--` separator is required for stdio servers: everything after it is the command to run. Without `--`, Claude Code attempts to parse the server's own flags as its own options.

After adding a project-scoped server from `.mcp.json`, Claude Code prompts for one-time approval before loading it. Run `claude mcp reset-project-choices` if you need to reset approval state.

---

## 3. Authentication

flow-mcp authenticates by reading the token that `flow login` writes to the OS keyring. Authentication is a two-step process on first setup:

**Step 1 — Initial login**

In a terminal on this device, run:

```bash
flow login
```

This performs an OIDC device-flow: the CLI prints a URL and a code, you open the URL in a browser, authorize the device, and the CLI stores the resulting token in the OS keyring. No browser interaction happens inside Claude Code.

**Step 2 — One-time reconnect**

After the first `flow login`, run `/mcp` inside Claude Code to confirm the server has loaded its tools. If the server was already connected before you authenticated, use the reconnect option in the `/mcp` panel to reload it with the new credentials.

**Durable re-authentication (Säule A)**

When the OIDC access token expires, the tools will return an error. To restore them, run `flow login` again in a terminal. The server lazily rebuilds its authenticated client on the next tool call — no `/mcp` reconnect in Claude Code is required. This property holds for all subsequent re-authentications after the initial setup.

---

## 4. Tool Surface

flow-mcp exposes eleven tools:

- `flow_project_context` — returns the active project binding and contextual metadata for the current repository
- `flow_search_docs` — full-text and semantic search across all documents
- `flow_list_docs` — list documents, optionally filtered by project or tag
- `flow_get_doc` — fetch a single document by slug or ID, including its content
- `flow_list_tags` — list all tags in use across the document store
- `flow_backlinks` — return all documents that link to a given document
- `flow_create_doc` — create a new document with title, content, and optional tags
- `flow_update_doc` — update the content, title, or tags of an existing document
- `flow_delete_doc` — permanently delete a document by ID
- `flow_list_projects` — list all projects known to the flow server
- `flow_bind_project` — bind the current repository to a flow project

---

## 5. Binding a Repository to a Project

Before `flow_project_context` and the document tools can operate in project scope, the current repository must be bound to a flow project. Use `flow_bind_project` to do this.

The tool auto-detects the project identity in two ways:
- **Git origin URL** — if the repository has a remote named `origin`, the tool matches it against known projects by URL.
- **Path** — falls back to matching by the absolute filesystem path of the repository root.

Invoke the tool once per repository. The binding is persisted on the server.

Verify the binding with `flow_project_context`: it should return the project name and ID rather than an error or empty result.

---

## 6. Degraded Mode (Before Login)

If `flow login` has not been run, or if the stored token has been revoked, every tool call returns an error result with the following message:

```
Login required: run 'flow login' in a terminal on this device.
```

This is the expected behavior. The server starts successfully and registers its tools; it defers authentication to the first tool call. No configuration change is needed — run `flow login` in a terminal and the next tool call will succeed.

---

## 7. Clean Shutdown

flow-mcp exits with status 0 when the host (Claude Code) closes the stdio connection at the end of a session. Only abnormal termination — such as a crash or an OS-level kill — produces a non-zero exit code. No special teardown steps are required; the server holds no persistent state that needs flushing on exit.
