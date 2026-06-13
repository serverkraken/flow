# SessionStart Hook for RepoNotes

A Claude Code [SessionStart hook](https://docs.claude.com/en/docs/claude-code/hooks) that auto-injects the current repo's RepoNote into Claude's context every time a session starts. Use this **instead of** flow-mcp's `flow://repos/<key>/note` MCP Resource if your client doesn't auto-attach Resources, or as an additional safety net.

## Drop-in script

Save as `.claude/hooks/load-repo-note.sh`, chmod +x:

```bash
#!/usr/bin/env bash
# Auto-inject the RepoNote for the current working directory into the
# new session's context. Silent on missing repo / missing note.
set -euo pipefail

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

Then register it in `.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      ".claude/hooks/load-repo-note.sh"
    ]
  }
}
```

## When to prefer the hook over the MCP Resource

| Use the hook                                 | Use the MCP Resource                          |
| -------------------------------------------- | --------------------------------------------- |
| Client doesn't auto-attach MCP Resources     | Client supports auto-attach by URI            |
| You want the note in **every** session       | You want Claude to fetch on demand            |
| Latency-sensitive (one shell call < 50 ms)   | Latency-tolerant (MCP roundtrip)              |
| You're not running flow-mcp                  | You're already running flow-mcp               |

Running both is redundant but harmless — the Resource and the hook produce the same content; Claude deduplicates verbatim duplicates in context.

## Caveats

- Requires the `flow` CLI on `$PATH`. If Claude Code can't find `flow`, the hook silently emits nothing.
- The hook reads from `~/.local/share/flow/cache.db`. If you ran `flow login` from a different user account, the cache won't carry the right data.
- The `<system-reminder>` tag is a Claude Code convention; Cursor / other clients ignore it but still consume the text.
