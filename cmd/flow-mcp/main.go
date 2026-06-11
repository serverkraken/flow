// flow-mcp is the stdio MCP server that exposes flow's RepoNotes +
// Worktime use cases to MCP clients (Claude Code, Cursor, Codex). See
// docs/runbook/flow-mcp-setup.md for the Claude Code config snippet.
//
// Architecture: reads and writes go directly to the flow server via the
// httpapi REST client (bearer-token auth). No local SQLite cache, no
// httpsync worker. The MCP transport runs on stdin/stdout; diagnostic
// logging goes exclusively to stderr because stdout is reserved for
// newline-delimited JSON-RPC frames.
//
// Token refresh: not wired here — flow-mcp uses the device-flow token
// stored in the keyring, which is long-lived. If it expires, the next
// tool call surfaces a server 401 as an error message. The user can
// run `flow login` in a terminal to refresh.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/serverkraken/flow/internal/adapter/gitremote"
	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/adapter/mcpstdio"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// version is overridden at build time via -ldflags "-X main.version=…".
// Reported back in the MCP initialize handshake so clients can log what
// they're talking to.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "flow-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverURL := os.Getenv("FLOW_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	keyring := keyringadapter.New()
	keyringSlot := "tokens:" + serverURL

	// httpapi client — no Refresher (device-flow token, long-lived).
	// Token refresh is out-of-band: `flow login` in a terminal.
	client := httpapi.New(httpapi.Config{
		BaseURL: serverURL,
		Tokens:  keyring,
		Slot:    keyringSlot,
	})

	// httpapi resource adapters
	projects := httpapi.NewProjects(client)
	sessions := httpapi.NewSessions(client)
	active := httpapi.NewActiveSessions(client)
	machine := httpapi.NewMachine(client, active, sessions)
	documents := httpapi.NewDocuments(client)
	identity := httpapi.NewIdentity(client)

	// Resolve identity — failure means not authed (no token or server unreachable).
	identityUC := usecase.NewIdentity(identity)
	localUser, identErr := identityUC.ResolveActiveUser(ctx)
	authed := identErr == nil
	if !authed {
		if !errors.Is(identErr, ports.ErrTokenNotFound) {
			logger.Warn("identity resolve failed — every tool will return 'Login required'",
				slog.String("slot", keyringSlot),
				slog.Any("err", identErr))
		} else {
			logger.Warn("no valid token in keyring — every tool will return 'Login required'",
				slog.String("slot", keyringSlot))
		}
		localUser = domain.User{ID: "anonymous"}
	}

	// Use cases
	activeSessionsUC := usecase.NewActiveSessions(nil, projects, active, machine)
	sessionsUC := usecase.NewSessions(nil, projects, sessions, nil)

	// WorktimeReader: uses the httpapi sessions adapter.
	// flow-mcp only exposes today's logged-time + target, so the cheapest
	// satisfying setup is enough: fixed 8h target, system clock.
	reader := newMinimalWorktimeReader(sessions)

	pwd, _ := os.Getwd()
	tools := &usecase.MCPTools{
		UserID:       localUser.ID,
		Pwd:          pwd,
		Authed:       authed,
		Documents:    documents,
		Resolver:     gitremote.New(),
		Active:       activeSessionsUC,
		Sessions:     sessionsUC,
		Reader:       reader,
		ProjectStore: projects,
	}

	impl := newImpl(tools)
	server := mcpstdio.NewServer(impl, mcpstdio.ServerInfo{Name: "flow-mcp", Version: version}, logger)

	// Run on stdin/stdout. Serve returns on EOF (client disconnect)
	// or a transport error. Ctrl+C / SIGTERM cancels ctx which is passed
	// to the identity resolve above; the stdio loop unwinds when the MCP
	// client closes its end. We deliberately don't force-close stdin on
	// ctx cancellation because the client may still be flushing its last
	// response.
	if err := server.Serve(os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("mcp serve: %w", err)
	}
	return nil
}
