// Command flow-mcp is a stdio Model Context Protocol server exposing the flow
// Kompendium to AI clients. stdout carries the JSON-RPC stream; all logs go to
// stderr.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	mgr := newBootManager()
	srv, h := newServerH(mgr) // wires mgr.onAuth = h.postAuthInit
	_ = h

	// Eager warm: if a valid token is stored, this builds the client, resolves the
	// project, reconciles resources, and logs
	// who we are. Failures are expected when logged out — the server still starts
	// and recovers on the first authed tool call.
	if c, err := mgr.client(ctx); err != nil {
		log.Warn("not authenticated at boot; tools will require login until `flow login`", "err", err)
	} else if u, err := c.Whoami(ctx); err != nil {
		log.Warn("token present but server rejected it; will retry lazily", "err", err)
	} else {
		log.Info("flow-mcp authenticated", "user", u.Email)
	}
	go h.runResourceReconciler(ctx, 30*time.Second)

	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Error("flow-mcp exited", "err", err)
		os.Exit(1)
	}
}
