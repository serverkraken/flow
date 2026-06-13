// Package mcpstdio implements a minimal MCP (Model Context Protocol)
// server over stdio (newline-delimited JSON-RPC 2.0). It covers the
// subset of the 2024-11-05 spec that cmd/flow-mcp needs:
//
//   - initialize          — capability negotiation
//   - initialized         — client-sent notification (no response)
//   - tools/list          — return the static tool catalog
//   - tools/call          — dispatch a tool by name
//   - resources/list      — return the dynamic resource catalog
//   - resources/read      — fetch a single resource by URI
//
// The package is transport+protocol only; tool dispatch is delegated to
// the Implementation interface so the usecase layer keeps the business
// logic and stays free of any MCP wire types.
package mcpstdio
