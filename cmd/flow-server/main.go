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
	"sort"
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
	// Cannot fail startup here — see warnOnUnverifiableMachineOwners — so
	// placement relative to FLOW_MIGRATE_ONLY below doesn't matter; left here
	// because it belongs next to the config it inspects.
	warnOnUnverifiableMachineOwners(cfg)
	pool, err := pgstore.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pgstore.Migrate(ctx, pool); err != nil {
		return err
	}
	// The dev database refresh imports the production schema first and then uses
	// the application's embedded goose migrations as the single source of truth.
	// Exit before OIDC/server wiring when only that migration step was requested.
	if os.Getenv("FLOW_MIGRATE_ONLY") == "1" {
		return nil
	}
	verifier, err := oidcverify.New(ctx, oidcverify.VerifierPairs(oidcverify.PairConfig{
		WebIssuer: cfg.OIDCIssuer, WebClient: cfg.OIDCClientID,
		CLIIssuer: cfg.OIDCCliIssuer, CLIClient: cfg.OIDCCliClientID,
		MachineIssuer: cfg.OIDCMachineIssuer, MachineClient: cfg.OIDCMachineClientID,
	}))
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
	nodeLogoStore := pgstore.NewNodeLogoStore(pool)
	nodeBannerStore := pgstore.NewNodeBannerStore(pool)
	nodeHighlightStore := pgstore.NewNodeHighlightStore(pool)
	artifactStore := pgstore.NewArtifactStore(pool)
	clock := systemclock.Clock{}
	ids := uuidgen.Gen{}
	nodeAggregateStore := pgstore.NewNodeAggregateStore(pool, ids)
	documentStore := pgstore.NewDocumentStore(pool, ids)
	tagStore := pgstore.NewTagStore(pool, ids)
	bus := sse.NewBus()
	activityStore := pgstore.NewActivityStore(pool)
	emitter := sse.NewEmitter(bus, activityStore, ids, clock)
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

	// Translate config → adapter. The httpserver package must not import
	// config (adapter → usecase → ports ← domain), so the mapping happens here.
	machines := make(map[string]httpserver.MachineAccount, len(cfg.MachineAccounts))
	for sub, acct := range cfg.MachineAccounts {
		machines[sub] = httpserver.MachineAccount{OwnerSub: acct.OwnerSub, Label: acct.Label}
	}

	srv := &httpserver.Server{
		Ready:    func(ctx context.Context) error { return pool.Ping(ctx) },
		Verifier: verifier,
		Machines: machines,
		Emitter:  emitter,
		Ensure: usecase.EnsureUser{
			Users: userStore,
			IDs:   ids,
			Allow: usecase.AllowList(cfg.AllowedSubs, cfg.AllowedGroups),
		},
		Bus:   bus,
		Clock: clock,
		Dev:   cfg.Dev,
		// CSPEnforce: flipped to enforcing (Soenne Entsch. #8) after the L3
		// Task 9 cross-surface smoke (document/Mermaid, Wissen search,
		// palette, SSE, dialogs, editor preview, projects/cockpit/time)
		// found zero constructs needing script-src beyond 'self'+nonce — see
		// security_headers.go and l3-global-constraints.md Entsch. #8. If a
		// later browser DevTools pass (Soenne-Live-Gate) finds a violation
		// this static+live-curl audit missed, flip back to false.
		CSPEnforce:         true,
		StartSession:       usecase.StartSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock},
		StopSession:        usecase.StopSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock, Loc: time.Local},
		SwitchSession:      usecase.SwitchSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock, Loc: time.Local},
		ListSessions:       usecase.ListSessions{Sessions: sessionStore, Clock: clock},
		CreateNode:         usecase.CreateNode{Nodes: nodeStore, Aggregate: nodeAggregateStore, IDs: ids, Clock: clock},
		CreateBoundNode:    usecase.CreateBoundNode{Nodes: nodeStore, Aggregate: nodeAggregateStore, IDs: ids, Clock: clock},
		ListNodes:          usecase.ListNodes{Nodes: nodeStore},
		UpdateNode:         usecase.UpdateNode{Nodes: nodeStore, Aggregate: nodeAggregateStore, Clock: clock},
		GetNode:            usecase.GetNode{Nodes: nodeStore},
		EditSession:        usecase.EditSession{Sessions: sessionStore, Nodes: nodeStore, Clock: clock, Loc: time.Local},
		DeleteSession:      usecase.DeleteSession{Sessions: sessionStore, Tags: tagStore},
		AddSession:         usecase.AddSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock, Loc: time.Local},
		ListSessionsRange:  usecase.ListSessionsRange{Sessions: sessionStore},
		GetRunningSession:  usecase.GetRunningSession{Sessions: sessionStore},
		ListSessionsPage:   usecase.ListSessionsPage{Sessions: sessionStore},
		BulkAssignNode:     usecase.BulkAssignNode{Sessions: sessionStore, Nodes: nodeStore},
		BulkDeleteSessions: usecase.BulkDeleteSessions{Sessions: sessionStore, Tags: tagStore},
		AddDayOffs:         usecase.AddDayOffs{Store: dayOffStore, Emitter: emitter},
		DeleteDayOff:       usecase.DeleteDayOff{Store: dayOffStore, Emitter: emitter},
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
			Nodes:    nodeStore,
		},
		SetTarget: usecase.SetTargetConfig{Settings: settingsStore},
		BuildExport: usecase.BuildExport{
			Sessions: sessionStore,
			Nodes:    nodeStore,
			Clock:    clock,
			Loc:      time.Local,
		},
		SetNodeRate:            usecase.SetNodeRate{Nodes: nodeStore},
		SetCountsTowardTarget:  usecase.SetCountsTowardTarget{Nodes: nodeStore, Aggregate: nodeAggregateStore, Clock: clock},
		UploadNodeLogo:         usecase.UploadNodeLogo{Nodes: nodeStore, Logos: nodeLogoStore, Aggregate: nodeAggregateStore, Clock: clock},
		GetNodeLogo:            usecase.GetNodeLogo{Logos: nodeLogoStore},
		UploadNodeBanner:       usecase.UploadNodeBanner{Nodes: nodeStore, Banners: nodeBannerStore, Aggregate: nodeAggregateStore, Clock: clock},
		GetNodeBanner:          usecase.GetNodeBanner{Banners: nodeBannerStore},
		AssignHighlight:        usecase.AssignHighlight{Highlights: nodeHighlightStore, IDs: ids, Clock: clock},
		RemoveHighlight:        usecase.RemoveHighlight{Highlights: nodeHighlightStore},
		ListDocumentHighlights: usecase.ListDocumentHighlights{Highlights: nodeHighlightStore},
		ListRecentHighlights:   usecase.ListRecentHighlights{Highlights: nodeHighlightStore},
		ListNewestHighlights:   usecase.ListNewestHighlights{Highlights: nodeHighlightStore},
		UploadArtifact:         usecase.UploadArtifact{Nodes: nodeStore, Artifacts: artifactStore, IDs: ids, Clock: clock, Emitter: emitter},
		RenameArtifact:         usecase.RenameArtifact{Artifacts: artifactStore, Emitter: emitter},
		ListArtifacts:          usecase.ListArtifacts{Nodes: nodeStore, Artifacts: artifactStore},
		DeleteArtifact:         usecase.DeleteArtifact{Artifacts: artifactStore, Emitter: emitter},
		GetArtifact:            usecase.GetArtifact{Artifacts: artifactStore},
		TagTimeReport:          usecase.TagTimeReport{Sessions: sessionStore},
		SetTags:                usecase.SetTags{Tags: tagStore},
		GetTags:                usecase.GetTags{Tags: tagStore},
		NodeTags:               usecase.NodeTags{Nodes: nodeStore, Tags: tagStore},
		DeleteNode:             usecase.DeleteNode{Nodes: nodeStore, Tags: tagStore},
		BindNode:               usecase.BindNode{Bindings: bindingStore, Nodes: nodeStore, IDs: ids, Clock: clock},
		UnbindNode:             usecase.UnbindNode{Bindings: bindingStore},
		ResolveNode:            usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore},
		ResolveEngagement:      usecase.ResolveEngagement{Resolve: usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore}, Nodes: nodeStore},
		NodeAncestors:          usecase.NodeAncestors{Nodes: nodeStore},
		MoveNode:               usecase.MoveNode{Nodes: nodeStore},
		ListNodeBindings:       usecase.ListNodeBindings{Bindings: bindingStore},
		CreateDocument:         usecase.CreateDocument{Docs: documentStore, Aggregate: documentStore, Nodes: nodeStore, IDs: ids, Clock: clock, Notifier: embedWorker, Tags: tagStore},
		ImportDocument:         usecase.ImportDocument{Docs: documentStore, Aggregate: documentStore, Nodes: nodeStore, IDs: ids, Clock: clock, Notifier: embedWorker, Tags: tagStore},
		GetDocument:            usecase.GetDocument{Docs: documentStore},
		ListDocuments:          usecase.ListDocuments{Docs: documentStore},
		ListDocumentsPage:      usecase.NewListDocumentsPage(documentStore),
		ListDocumentLibrary:    usecase.ListDocumentLibrary{Docs: documentStore},
		SearchDocumentLibrary:  usecase.SearchDocumentLibrary{Docs: documentStore, Embedder: embedder, Log: logger},
		UpdateDocument:         usecase.UpdateDocument{Docs: documentStore, Aggregate: documentStore, Clock: clock, Notifier: embedWorker, Tags: tagStore},
		MoveDocument:           usecase.MoveDocument{Docs: documentStore, Nodes: nodeStore, Clock: clock, Notifier: embedWorker},
		DeleteDocument:         usecase.DeleteDocument{Docs: documentStore, Aggregate: documentStore, Tags: tagStore},
		BacklinksDocument:      usecase.Backlinks{Docs: documentStore},
		ListTags:               usecase.ListTags{Tags: tagStore},
		SearchDocuments:        usecase.SearchDocuments{Docs: documentStore, Embedder: embedder, Log: logger},
		RetryEmbedding:         usecase.RetryEmbedding{Docs: documentStore, Notifier: embedWorker},
		GetEmbedStatus:         usecase.GetEmbedStatus{Docs: documentStore},
		StripFrontmatter:       usecase.StripFrontmatter{Docs: documentStore, Clock: clock},
		RedesignDocTypes:       usecase.RedesignDocTypes{Docs: documentStore, Clock: clock},
		AuditDocuments:         usecase.AuditDocuments{Docs: documentStore, Nodes: nodeStore},
		UpsertDocumentByPath:   usecase.UpsertDocumentByPath{Docs: documentStore, Aggregate: documentStore, Nodes: nodeStore, Tags: tagStore, Notifier: embedWorker},
		ComposeContext: usecase.ComposeContext{
			Resolve: usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore},
			Nodes:   nodeStore, Docs: documentStore, Tags: tagStore,
		},
		SetActiveContext: usecase.SetActiveContext{
			Resolve: usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore},
			Nodes:   nodeStore, Docs: documentStore, Aggregate: documentStore, Tags: tagStore,
		},
		SetPinned:          usecase.SetPinned{Docs: documentStore},
		SetContextMode:     usecase.SetContextMode{Docs: documentStore, Curation: documentStore, Clock: clock},
		ReorderContextDocs: usecase.ReorderContextDocs{Docs: documentStore},
		SetArchived:        usecase.SetArchived{Docs: documentStore, Curation: documentStore, Clock: clock},
		BulkCurateDocuments: usecase.BulkCurateDocuments{
			Docs: documentStore, Clock: clock,
		},
		ListArchived:  usecase.ListArchived{Docs: documentStore},
		ListActivity:  usecase.ListActivity{Activities: activityStore},
		ContextBudget: contextBudget(os.Getenv),
		PublicBaseURL: cfg.PublicBaseURL,
		Users:         userStore,
		OIDCAuth:      authn,
		Session:       websession.NewCodec(cfg.SessionSecret, 7*24*time.Hour),
	}

	// tmux status segment — aggregated read-only composer over the worktime
	// readers already wired above (DRY: reuse srv.Stats/GetRunningSession/ListDayOffs).
	srv.WorktimeStatus = usecase.WorktimeStatus{
		Stats:   srv.Stats,
		Running: srv.GetRunningSession,
		DayOffs: srv.ListDayOffs,
		Clock:   clock,
		Loc:     time.Local,
	}
	// stop-picker MRU ranking (exact server support, not a client heuristic).
	srv.NodeMRU = usecase.NodeMRU{Sessions: sessionStore}

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

// warnOnUnverifiableMachineOwners logs a startup WARNING for each machine
// account whose delegated owner does not appear in FLOW_ALLOWED_SUBS by
// subject.
//
// resolveMachine (see internal/adapter/httpserver/machineauth.go) looks the
// delegated owner up with Users.GetBySub, which bypasses the allowlist that
// EnsureUser.Allow applies to human tokens. Removing an owner from
// FLOW_ALLOWED_SUBS alone therefore does NOT disable machine accounts
// delegated to them — revocation has two switches, and only one is obvious.
// This cannot be fixed in the middleware: Allow also consults group claims,
// and the delegation path has no owner token and therefore no groups to
// check.
//
// This can only WARN, never fail startup. usecase.AllowList allows on
// EITHER Username or Subject (internal/usecase/allow.go), and username-keyed
// allowlisting is a first-class, tested deployment shape — but
// config.MachineAccount carries only the owner's OIDC SUBJECT, so a
// username-keyed allowlist entry is structurally invisible here. An absent
// sub is therefore ambiguous: it may mean a revoked owner, or it may simply
// mean the allowlist is keyed by username for an owner who is fully
// allowed. Aborting on an unverifiable condition would lock out that fully
// supported deployment, so the condition below only ever decides whether to
// log, never whether to boot.
//
// The warning only fires when a sub allowlist is configured and no group
// allowlist is in play — the same shape a hard-fail check would have used —
// because that's the only shape in which the mismatch is even plausibly
// meaningful; with groups in play a group-allowed owner can't be checked
// without their token at all, allowlist or not.
func warnOnUnverifiableMachineOwners(cfg config.Config) {
	if len(cfg.AllowedSubs) == 0 || len(cfg.AllowedGroups) > 0 {
		return
	}
	// Range over a map is non-deterministic; sort by machine subject (the
	// map key, unique by construction — see parseMachineAccounts' duplicate
	// check) so repeated runs against identical config log in the same
	// order instead of naming a random offender each restart.
	subs := make([]string, 0, len(cfg.MachineAccounts))
	for sub := range cfg.MachineAccounts {
		subs = append(subs, sub)
	}
	sort.Strings(subs)
	for _, sub := range subs {
		acct := cfg.MachineAccounts[sub]
		if cfg.AllowedSubs[acct.OwnerSub] {
			continue
		}
		slog.Warn(
			"machine account owner not found in FLOW_ALLOWED_SUBS by subject — "+
				"either the owner was revoked and FLOW_MACHINE_ACCOUNTS still delegates to "+
				"them, or FLOW_ALLOWED_SUBS is keyed by username for this owner; revoking an "+
				"owner does not disable machine accounts delegated to them — the entry must "+
				"also be removed from FLOW_MACHINE_ACCOUNTS",
			"label", acct.Label, "owner_sub", acct.OwnerSub)
	}
}

func contextBudget(getenv func(string) string) int {
	if v := getenv("FLOW_CONTEXT_BUDGET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 12000
}
