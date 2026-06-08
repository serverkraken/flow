// flow-mcp is the stdio MCP server that exposes flow's RepoNotes +
// Worktime use cases to MCP clients (Claude Code, Cursor, Codex). See
// docs/runbook/flow-mcp-setup.md for the Claude Code config snippet.
//
// Architecture: shares the local sqliteclient cache and httpsync worker
// with the `flow` CLI/TUI binaries, so MCP-driven work syncs through
// the same path as TUI-driven work. The MCP transport runs on
// stdin/stdout; diagnostic logging goes exclusively to stderr because
// stdout is reserved for newline-delimited JSON-RPC frames.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/serverkraken/flow/internal/adapter/gitremote"
	"github.com/serverkraken/flow/internal/adapter/httpsync"
	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/adapter/mcpstdio"
	"github.com/serverkraken/flow/internal/adapter/oidcclient"
	"github.com/serverkraken/flow/internal/adapter/sqliteclient"
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

	paths, err := resolvePaths()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(paths.CacheDB), 0o755); err != nil {
		return fmt.Errorf("cache db dir: %w", err)
	}
	cacheStore, err := sqliteclient.Open(paths.CacheDB)
	if err != nil {
		return fmt.Errorf("open cache db: %w", err)
	}
	defer func() { _ = cacheStore.Close() }()

	// Resolve the active identity: if a token is present for this server, run as
	// the OIDC user (sub from the token); otherwise the offline `local` placeholder.
	localSub := envOrDefault("FLOW_LOCAL_USER_SUB", "local")
	serverURL := envOrDefault("FLOW_SERVER_URL", "http://localhost:8080")
	cacheUsers := sqliteclient.NewUsers(cacheStore)
	tokenSub := ""
	if toks, terr := keyringadapter.New().Get("tokens:" + serverURL); terr == nil {
		src := toks.IDToken
		if src == "" {
			src = toks.AccessToken
		}
		if c, cerr := oidcclient.ClaimsFromToken(src); cerr == nil {
			tokenSub = c.Sub
		}
	}
	identityUC := usecase.NewIdentity(cacheUsers, localSub)
	localUser, err := identityUC.ResolveActiveUser(tokenSub)
	if err != nil {
		return fmt.Errorf("resolve active user: %w", err)
	}

	cacheProjects := sqliteclient.NewProjects(cacheStore)
	cacheSessions := sqliteclient.NewSessions(cacheStore)
	cacheActive := sqliteclient.NewActiveSessions(cacheStore)
	cacheRepos := sqliteclient.NewRepos(cacheStore)
	cacheRepoNotes := sqliteclient.NewRepoNotes(cacheStore)
	cacheQueue := sqliteclient.NewWriteQueue(cacheStore)
	cacheSyncState := sqliteclient.NewSyncState(cacheStore)

	// HTTP sync — same wiring as cmd/flow/main.go so writes from MCP
	// flow into the same write_queue that the CLI/TUI drain. Two flow
	// processes running concurrently share one sqlite cache; both may
	// run a worker without conflict (the queue table is uniquely
	// seq-keyed and modernc/sqlite serialises through WAL).
	keyring := keyringadapter.New()
	keyringSlot := "tokens:" + serverURL
	syncClient := httpsync.NewClient(serverURL, keyring, keyringSlot)
	syncQueueAdapter := httpsync.NewQueue(cacheQueue)
	syncWorker := httpsync.NewWorker(
		syncClient,
		cacheSessions,
		cacheProjects,
		cacheActive,
		cacheSyncState,
		syncQueueAdapter,
		localUser.ID,
	)
	syncWorker.SetRepoStores(cacheRepos, cacheRepoNotes)
	syncWorker.Start(ctx)
	defer syncWorker.Stop()

	// Use cases — same shape as cmd/flow/main.go but trimmed (no
	// TUI/kompendium/dayoff/links machinery).
	repoNotesUC := usecase.NewRepoNotes(cacheRepos, cacheRepoNotes, cacheQueue, gitremote.New())
	repoNotesUC.SetPushSignal(syncWorker.SignalPush)
	activeSessionsUC := usecase.NewActiveSessions(cacheUsers, cacheProjects, cacheActive, cacheSessions, cacheQueue)
	activeSessionsUC.SetPushSignal(syncWorker.SignalPush)
	sessionsUC := usecase.NewSessions(cacheUsers, cacheProjects, cacheSessions, nil)

	// WorktimeReader needs Sessions + State + Targets + Clock. flow-mcp
	// only exposes today's logged-time + target, so the cheapest
	// satisfying setup is enough: empty active marker, fixed 8h
	// target, system clock. A future plan can wire the full config
	// stack if MCP starts driving full worktime reports.
	reader := newMinimalWorktimeReader(cacheSessions)

	authed := hasValidToken(keyring, keyringSlot)
	if !authed {
		logger.Warn("no valid token in keyring — every tool will return 'Login required'", slog.String("slot", keyringSlot))
	}

	pwd, _ := os.Getwd()
	tools := &usecase.MCPTools{
		UserID:        localUser.ID,
		Pwd:           pwd,
		Authed:        authed,
		Notes:         repoNotesUC,
		Active:        activeSessionsUC,
		Sessions:      sessionsUC,
		Reader:        reader,
		RepoNoteStore: cacheRepoNotes,
		ProjectStore:  cacheProjects,
	}

	impl := newImpl(tools)
	server := mcpstdio.NewServer(impl, mcpstdio.ServerInfo{Name: "flow-mcp", Version: version}, logger)

	// Run on stdin/stdout. Serve returns on EOF (client disconnect)
	// or a transport error. Ctrl+C / SIGTERM cancels ctx which stops
	// the sync worker; the stdio loop unwinds when the MCP client
	// closes its end. We deliberately don't force-close stdin on ctx
	// cancellation because the client may still be flushing its last
	// response.
	if err := server.Serve(os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("mcp serve: %w", err)
	}
	return nil
}

// Paths bundles every filesystem location the MCP server needs.
type Paths struct {
	CacheDB string
}

func resolvePaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("home dir: %w", err)
	}
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".local", "share")
	}
	cacheDB := os.Getenv("FLOW_CACHE_DB")
	if cacheDB == "" {
		cacheDB = filepath.Join(xdg, "flow", "cache.db")
	}
	return Paths{CacheDB: cacheDB}, nil
}

func envOrDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
