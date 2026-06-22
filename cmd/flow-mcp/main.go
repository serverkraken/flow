// Command flow-mcp is a stdio Model Context Protocol server exposing the flow
// Kompendium to AI clients. stdout carries the JSON-RPC stream; all logs go to
// stderr.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/domain"
)

func main() {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client, authed := bootClient(ctx, log)
	var proj domain.Project
	var matched bool
	if authed {
		proj, matched = resolveProject(ctx, client, log)
	}

	srv, h := newServerH(client, authed, proj, matched)
	if err := h.registerResources(ctx); err != nil {
		log.Warn("could not register document resources", "err", err)
	}
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Error("flow-mcp exited", "err", err)
		os.Exit(1)
	}
}
