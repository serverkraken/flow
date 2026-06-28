// Command flow-server is the flow API + SSE server (composition root).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/serverkraken/flow/internal/adapter/embed"
	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/oidcauth"
	"github.com/serverkraken/flow/internal/adapter/oidcverify"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/systemclock"
	"github.com/serverkraken/flow/internal/adapter/uuidgen"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/config"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("flow-server exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	pool, err := pgstore.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pgstore.Migrate(ctx, pool); err != nil {
		return err
	}
	verifier, err := oidcverify.New(ctx, oidcverify.VerifierPairs(
		cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCCliIssuer, cfg.OIDCCliClientID))
	if err != nil {
		return err
	}
	authn, err := oidcauth.New(ctx, cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.RedirectURL())
	if err != nil {
		return err
	}

	userStore := pgstore.NewUserStore(pool)
	nodeStore := pgstore.NewNodeStore(pool)
	bindingStore := pgstore.NewProjectBindingStore(pool)
	sessionStore := pgstore.NewSessionStore(pool)
	dayOffStore := pgstore.NewDayOffStore(pool)
	settingsStore := pgstore.NewUserSettingsStore(pool)
	feedTokenStore := pgstore.NewFeedTokenStore(pool)
	documentStore := pgstore.NewDocumentStore(pool)
	clock := systemclock.Clock{}
	ids := uuidgen.Gen{}
	tagStore := pgstore.NewTagStore(pool, ids)
	bus := sse.NewBus()
	logger := slog.Default()

	// Semantic-search embedder + background embedding worker. Without Ollama
	// reachable the worker logs WARNs and search degrades to keyword-only.
	ollamaHost := getenvDefault("FLOW_OLLAMA_HOST", "http://localhost:11434")
	embedModel := getenvDefault("FLOW_EMBED_MODEL", "nomic-embed-text")
	embedInterval := getenvDuration("FLOW_EMBED_INTERVAL", 15*time.Second)
	embedBatch := getenvInt("FLOW_EMBED_BATCH", 16)
	embedTimeout := getenvDuration("FLOW_EMBED_TIMEOUT", 60*time.Second)
	embedder := embed.NewOllama(ollamaHost, embedModel, embedTimeout)
	embedWorker := worker.NewEmbedWorker(documentStore, embedder, embedInterval, embedBatch, worker.EmbedPolicy{}, logger)
	var workerWG sync.WaitGroup
	workerWG.Add(1)
	go func() { defer workerWG.Done(); embedWorker.Run(ctx) }()

	srv := &httpserver.Server{
		Ready:    func(ctx context.Context) error { return pool.Ping(ctx) },
		Verifier: verifier,
		Ensure: usecase.EnsureUser{
			Users: userStore,
			IDs:   ids,
			Allow: usecase.AllowList(cfg.AllowedSubs, cfg.AllowedGroups),
		},
		Bus:                bus,
		Clock:              clock,
		Dev:                cfg.Dev,
		StartSession:       usecase.StartSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock},
		StopSession:        usecase.StopSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock, Loc: time.Local},
		ListSessions:       usecase.ListSessions{Sessions: sessionStore, Clock: clock},
		CreateNode:      usecase.CreateNode{Nodes: nodeStore, IDs: ids, Clock: clock},
		ListNodes:       usecase.ListNodes{Nodes: nodeStore},
		UpdateNode:      usecase.UpdateNode{Nodes: nodeStore, Bindings: bindingStore, IDs: ids, Clock: clock},
		GetNode:         usecase.GetNode{Nodes: nodeStore},
		EditSession:        usecase.EditSession{Sessions: sessionStore},
		DeleteSession:      usecase.DeleteSession{Sessions: sessionStore},
		AddSession:         usecase.AddSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock},
		ListSessionsRange:  usecase.ListSessionsRange{Sessions: sessionStore},
		GetRunningSession:  usecase.GetRunningSession{Sessions: sessionStore},
		ListSessionsPage:   usecase.ListSessionsPage{Sessions: sessionStore},
		BulkAssignNode:  usecase.BulkAssignNode{Sessions: sessionStore, Nodes: nodeStore},
		BulkDeleteSessions: usecase.BulkDeleteSessions{Sessions: sessionStore},
		AddDayOffs:         usecase.AddDayOffs{Store: dayOffStore, Bus: bus},
		DeleteDayOff:       usecase.DeleteDayOff{Store: dayOffStore, Bus: bus},
		ListDayOffs:        usecase.ListDayOffs{Store: dayOffStore, Settings: settingsStore, Loc: time.Local},
		GetSettings:        usecase.GetSettings{Settings: settingsStore, Tokens: feedTokenStore},
		SetBundesland:      usecase.SetBundesland{Settings: settingsStore},
		IcsFeed:            usecase.IcsFeed{Tokens: feedTokenStore, Store: dayOffStore, Clock: clock},
		RegenIcsToken:      usecase.RegenerateIcsToken{Tokens: feedTokenStore, Clock: clock},
		Stats: usecase.StatsComputer{
			Sessions: sessionStore,
			Settings: settingsStore,
			DayOffs:  usecase.ListDayOffs{Store: dayOffStore, Settings: settingsStore, Loc: time.Local},
			Clock:    clock,
			Loc:      time.Local,
		},
		SetTarget: usecase.SetTargetConfig{Settings: settingsStore},
		BuildExport: usecase.BuildExport{
			Sessions: sessionStore,
			Nodes: nodeStore,
			Clock:    clock,
			Loc:      time.Local,
		},
		SetNodeRate:      usecase.SetNodeRate{Nodes: nodeStore},
		DeleteNode:       usecase.DeleteNode{Nodes: nodeStore},
		BindNode:          usecase.BindNode{Bindings: bindingStore, Nodes: nodeStore, IDs: ids, Clock: clock},
		UnbindNode:        usecase.UnbindNode{Bindings: bindingStore},
		ResolveNode:       usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore},
		ResolveEngagement: usecase.ResolveEngagement{Resolve: usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore}, Nodes: nodeStore},
		NodeAncestors:     usecase.NodeAncestors{Nodes: nodeStore},
		MoveNode:          usecase.MoveNode{Nodes: nodeStore},
		ListNodeBindings:  usecase.ListNodeBindings{Bindings: bindingStore},
		CreateDocument:      usecase.CreateDocument{Docs: documentStore, IDs: ids, Clock: clock, Notifier: embedWorker, Tags: tagStore},
		ImportDocument:      usecase.ImportDocument{Docs: documentStore, IDs: ids, Clock: clock, Notifier: embedWorker, Tags: tagStore},
		GetDocument:         usecase.GetDocument{Docs: documentStore},
		ListDocuments:       usecase.ListDocuments{Docs: documentStore},
		ListDocumentsPage:   usecase.NewListDocumentsPage(documentStore),
		UpdateDocument:      usecase.UpdateDocument{Docs: documentStore, Clock: clock, Notifier: embedWorker, Tags: tagStore},
		DeleteDocument:      usecase.DeleteDocument{Docs: documentStore, Tags: tagStore},
		BacklinksDocument:   usecase.Backlinks{Docs: documentStore},
		ListTags:            usecase.ListTags{Docs: documentStore},
		SearchDocuments:     usecase.SearchDocuments{Docs: documentStore, Embedder: embedder, Log: logger},
		RetryEmbedding:      usecase.RetryEmbedding{Docs: documentStore, Notifier: embedWorker},
		GetEmbedStatus:      usecase.GetEmbedStatus{Docs: documentStore},
		Users:               userStore,
		OIDCAuth:            authn,
		Session:             websession.NewCodec(cfg.SessionSecret, 7*24*time.Hour),
	}

	httpSrv := &http.Server{Addr: cfg.ListenAddr, Handler: srv.Routes(), ReadHeaderTimeout: 10 * time.Second}
	srvErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr, "dev", cfg.Dev)
		var err error
		if cfg.Dev {
			// Serve TLS in dev so browsers negotiate HTTP/2 (multiplexes the SSE
			// stream + page loads over one connection, dodging the HTTP/1.1
			// per-host connection-starvation). Self-signed; dev only.
			tlsCfg, terr := devTLSConfig(filepath.Join(os.TempDir(), "flow-dev-tls"))
			if terr != nil {
				srvErr <- fmt.Errorf("flow-server: dev TLS: %w", terr)
				return
			}
			httpSrv.TLSConfig = tlsCfg
			err = httpSrv.ListenAndServeTLS("", "")
		} else {
			err = httpSrv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
	}()

	select {
	case err := <-srvErr:
		return fmt.Errorf("flow-server: listen: %w", err)
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	shutErr := httpSrv.Shutdown(shutCtx)
	workerWG.Wait() // let the embed worker observe ctx cancel and exit before the deferred pool.Close
	return shutErr
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
