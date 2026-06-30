// Package httpserver exposes the REST + SSE API and the WebUI auth flow.
package httpserver

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type Server struct {
	Verifier ports.TokenVerifier
	Ensure   usecase.EnsureUser
	Bus      ports.EventBus
	Clock    ports.Clock
	Dev      bool
	Ready    func(context.Context) error // optional DB readiness probe; nil = always ready

	// worktime usecases
	StartSession      usecase.StartSession
	StopSession       usecase.StopSession
	ListSessions      usecase.ListSessions
	CreateNode        usecase.CreateNode
	ListNodes         usecase.ListNodes
	DeleteNode        usecase.DeleteNode
	UpdateNode        usecase.UpdateNode
	GetNode           usecase.GetNode
	EditSession       usecase.EditSession
	DeleteSession     usecase.DeleteSession
	AddSession        usecase.AddSession
	ListSessionsRange usecase.ListSessionsRange
	GetRunningSession usecase.GetRunningSession
	ListSessionsPage  usecase.ListSessionsPage

	// m1c worktime extras
	AddDayOffs    usecase.AddDayOffs
	DeleteDayOff  usecase.DeleteDayOff
	ListDayOffs   usecase.ListDayOffs
	GetSettings   usecase.GetSettings
	SetBundesland usecase.SetBundesland
	IcsFeed       usecase.IcsFeed
	RegenIcsToken usecase.RegenerateIcsToken

	// m1d stats
	Stats     usecase.StatsComputer
	SetTarget usecase.SetTargetConfig

	// m1e export
	BuildExport usecase.BuildExport
	SetNodeRate usecase.SetNodeRate

	// slice 1 bulk ops
	BulkAssignNode     usecase.BulkAssignNode
	BulkDeleteSessions usecase.BulkDeleteSessions

	// tag-time report (D2)
	TagTimeReport usecase.TagTimeReport

	// project bindings (resolution V0)
	BindNode          usecase.BindNode
	UnbindNode        usecase.UnbindNode
	ResolveNode       usecase.ResolveNode
	ResolveEngagement usecase.ResolveEngagement
	NodeAncestors     usecase.NodeAncestors
	MoveNode          usecase.MoveNode
	ListNodeBindings  usecase.ListNodeBindings

	// node tagging (C2)
	SetTags usecase.SetTags
	GetTags usecase.GetTags

	// m2a documents
	CreateDocument    usecase.CreateDocument
	ImportDocument    usecase.ImportDocument
	GetDocument       usecase.GetDocument
	ListDocuments     usecase.ListDocuments
	ListDocumentsPage *usecase.ListDocumentsPage
	UpdateDocument    usecase.UpdateDocument
	DeleteDocument    usecase.DeleteDocument
	BacklinksDocument usecase.Backlinks
	ListTags          usecase.ListTags
	SearchDocuments   usecase.SearchDocuments
	RetryEmbedding    usecase.RetryEmbedding
	GetEmbedStatus    usecase.GetEmbedStatus
	SetPinned         usecase.SetPinned
	SetArchived       usecase.SetArchived
	ListArchived      usecase.ListArchived

	// idempotent upsert by path (B3d Task 7)
	UpsertDocumentByPath usecase.UpsertDocumentByPath

	// maintenance (F1)
	StripFrontmatter usecase.StripFrontmatter
	RedesignDocTypes usecase.RedesignDocTypes

	// B3 context store (B1, B2)
	ComposeContext   usecase.ComposeContext
	SetActiveContext usecase.SetActiveContext
	ContextBudget    int // default cap when ?cap= absent; 0 → fall back to 12000

	// WebUI auth (wired in Task 5)
	OIDCAuth Authenticator
	Session  SessionCodec
	Users    ports.UserStore
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.Handle("GET /api/v1/me", s.auth(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/v1/events", s.authAny(http.HandlerFunc(s.handleEvents)))

	mux.Handle("POST /api/v1/sessions", s.auth(http.HandlerFunc(s.handleStartSession)))
	mux.Handle("POST /api/v1/sessions/reassign", s.authAny(http.HandlerFunc(s.handleReassignSessions)))
	mux.Handle("POST /api/v1/sessions/bulk-delete", s.authAny(http.HandlerFunc(s.handleBulkDeleteSessions)))
	mux.Handle("POST /api/v1/sessions/{id}/stop", s.auth(http.HandlerFunc(s.handleStopSession)))
	mux.Handle("GET /api/v1/sessions/tag-times", s.auth(http.HandlerFunc(s.handleTagTimes)))
	mux.Handle("GET /api/v1/sessions", s.auth(http.HandlerFunc(s.handleListSessions)))
	mux.Handle("PATCH /api/v1/sessions/{id}", s.auth(http.HandlerFunc(s.handleEditSession)))
	mux.Handle("DELETE /api/v1/sessions/{id}", s.auth(http.HandlerFunc(s.handleDeleteSession)))
	mux.Handle("POST /api/v1/nodes", s.auth(http.HandlerFunc(s.handleCreateNode)))
	mux.Handle("GET /api/v1/nodes", s.auth(http.HandlerFunc(s.handleListNodes)))
	mux.Handle("DELETE /api/v1/nodes/{id}", s.auth(http.HandlerFunc(s.handleDeleteNode)))
	mux.Handle("GET /api/v1/nodes/{id}", s.auth(http.HandlerFunc(s.handleGetNode)))
	mux.Handle("PATCH /api/v1/nodes/{id}", s.auth(http.HandlerFunc(s.handleUpdateNode)))

	mux.Handle("GET /api/v1/dayoffs", s.auth(http.HandlerFunc(s.handleListDayOffs)))
	mux.Handle("POST /api/v1/dayoffs", s.auth(http.HandlerFunc(s.handleAddDayOffs)))
	mux.Handle("DELETE /api/v1/dayoffs/{day}", s.auth(http.HandlerFunc(s.handleDeleteDayOff)))

	mux.Handle("GET /api/v1/settings", s.auth(http.HandlerFunc(s.handleGetSettings)))
	mux.Handle("POST /api/v1/settings/bundesland", s.auth(http.HandlerFunc(s.handleSetBundesland)))
	mux.Handle("POST /api/v1/settings/target", s.auth(http.HandlerFunc(s.handleSetTarget)))
	mux.Handle("POST /api/v1/ics-token/regenerate", s.auth(http.HandlerFunc(s.handleRegenIcsToken)))
	mux.HandleFunc("GET /ics/{token}", s.handleIcsFeed)

	mux.Handle("GET /api/v1/today", s.auth(http.HandlerFunc(s.handleToday)))
	mux.Handle("GET /api/v1/week", s.auth(http.HandlerFunc(s.handleWeek)))
	mux.Handle("GET /api/v1/stats", s.auth(http.HandlerFunc(s.handleStats)))
	mux.Handle("GET /api/v1/burndown", s.auth(http.HandlerFunc(s.handleBurndown)))

	mux.Handle("GET /api/v1/export", s.authAny(http.HandlerFunc(s.handleExport)))
	mux.Handle("POST /api/v1/nodes/{id}/rate", s.auth(http.HandlerFunc(s.handleSetNodeRate)))
	mux.Handle("POST /api/v1/nodes/{id}/move", s.auth(http.HandlerFunc(s.handleMoveNode)))

	// project bindings — static paths before {id} wildcard
	mux.Handle("GET /api/v1/nodes/resolve", s.auth(http.HandlerFunc(s.handleResolveNode)))
	mux.Handle("GET /api/v1/nodes/bindings", s.auth(http.HandlerFunc(s.handleListAllNodeBindings)))
	mux.Handle("DELETE /api/v1/nodes/bindings", s.auth(http.HandlerFunc(s.handleUnbindNode)))
	mux.Handle("GET /api/v1/nodes/resolve-engagement", s.auth(http.HandlerFunc(s.handleResolveEngagement)))
	mux.Handle("PUT /api/v1/nodes/{id}/bindings", s.auth(http.HandlerFunc(s.handleBindNode)))
	mux.Handle("GET /api/v1/nodes/{id}/bindings", s.auth(http.HandlerFunc(s.handleListNodeBindingsByNode)))
	mux.Handle("GET /api/v1/nodes/{id}/stats", s.auth(http.HandlerFunc(s.handleNodeStats)))
	mux.Handle("GET /api/v1/nodes/{id}/ancestors", s.auth(http.HandlerFunc(s.handleNodeAncestors)))
	mux.Handle("GET /api/v1/nodes/{id}/tags", s.auth(http.HandlerFunc(s.handleGetNodeTags)))
	mux.Handle("PUT /api/v1/nodes/{id}/tags", s.auth(http.HandlerFunc(s.handleSetNodeTags)))

	mux.Handle("POST /api/v1/documents", s.auth(http.HandlerFunc(s.handleCreateDocument)))
	mux.Handle("POST /api/v1/documents/import", s.auth(http.HandlerFunc(s.handleImportDocument)))
	mux.Handle("PUT /api/v1/documents/by-path", s.authAny(http.HandlerFunc(s.handleUpsertByPath)))
	mux.Handle("GET /api/v1/documents", s.auth(http.HandlerFunc(s.handleListDocuments)))
	mux.Handle("GET /api/v1/documents/tags", s.auth(http.HandlerFunc(s.handleListTags)))
	// archived path registered before the {id} wildcard so the static path wins
	mux.Handle("GET /api/v1/documents/archived", s.auth(http.HandlerFunc(s.handleListArchived)))
	mux.Handle("GET /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleGetDocument)))
	mux.Handle("PUT /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleUpdateDocument)))
	mux.Handle("DELETE /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleDeleteDocument)))
	mux.Handle("GET /api/v1/documents/{id}/backlinks", s.auth(http.HandlerFunc(s.handleDocumentBacklinks)))
	mux.Handle("POST /api/v1/documents/{id}/pin", s.auth(http.HandlerFunc(s.handlePinDocument)))
	mux.Handle("POST /api/v1/documents/{id}/archive", s.auth(http.HandlerFunc(s.handleArchiveDocument)))

	// WebUI auth routes (handlers in webauth.go, Task 5)
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)

	// WebUI routes (handlers in webui.go, Task 8)
	mux.Handle("GET /zeit", s.webAuth(http.HandlerFunc(s.handleZeitHome)))
	mux.Handle("GET /ui/worktime", s.webAuth(http.HandlerFunc(s.handleHeuteFragment)))
	mux.Handle("POST /ui/worktime/start", s.webAuth(http.HandlerFunc(s.handleWebStart)))
	mux.Handle("POST /ui/worktime/stop", s.webAuth(http.HandlerFunc(s.handleWebStop)))
	mux.Handle("POST /ui/worktime/add", s.webAuth(http.HandlerFunc(s.handleWebAdd)))
	mux.Handle("POST /ui/worktime/edit", s.webAuth(http.HandlerFunc(s.handleWebEdit)))
	mux.Handle("POST /ui/worktime/delete", s.webAuth(http.HandlerFunc(s.handleWebDelete)))

	// Woche (week) page + fragment (Slice 1, Task 7)
	mux.Handle("GET /woche", s.webAuth(http.HandlerFunc(s.handleWocheHome)))
	mux.Handle("GET /ui/woche/fragment", s.webAuth(http.HandlerFunc(s.handleWocheFragment)))

	// Historie (calendar/month/agenda/list + bulk) page + fragments (Slice 1, Task 8)
	mux.Handle("GET /historie", s.webAuth(http.HandlerFunc(s.handleHistorieHome)))
	mux.Handle("GET /ui/historie/calendar", s.webAuth(http.HandlerFunc(s.handleHistorieCalendarFragment)))
	mux.Handle("GET /ui/historie/list", s.webAuth(http.HandlerFunc(s.handleHistorieListFragment)))
	mux.Handle("POST /ui/historie/reassign", s.webAuth(http.HandlerFunc(s.handleHistorieReassign)))
	mux.Handle("POST /ui/historie/bulk-delete", s.webAuth(http.HandlerFunc(s.handleHistorieBulkDelete)))

	mux.Handle("GET /dayoffs", s.webAuth(http.HandlerFunc(s.handleWebDayOffHome)))
	mux.Handle("GET /ui/dayoffs", s.webAuth(http.HandlerFunc(s.handleWebDayOffFragment)))
	mux.Handle("POST /ui/dayoffs/add", s.webAuth(http.HandlerFunc(s.handleWebDayOffAdd)))
	mux.Handle("POST /ui/dayoffs/delete", s.webAuth(http.HandlerFunc(s.handleWebDayOffDelete)))
	mux.Handle("POST /ui/dayoffs/regen-token", s.webAuth(http.HandlerFunc(s.handleWebRegenToken)))
	mux.Handle("POST /ui/dayoffs/bundesland", s.webAuth(http.HandlerFunc(s.handleWebSetBundesland)))

	mux.Handle("GET /stats", s.webAuth(http.HandlerFunc(s.handleWebStatsHome)))
	mux.Handle("GET /ui/stats/fragment", s.webAuth(http.HandlerFunc(s.handleWebStatsFragment)))
	mux.Handle("POST /ui/stats/target", s.webAuth(http.HandlerFunc(s.handleWebSetTarget)))

	mux.Handle("GET /einstellungen", s.webAuth(http.HandlerFunc(s.handleWebEinstellungenHome)))
	mux.Handle("POST /ui/einstellungen/target", s.webAuth(http.HandlerFunc(s.handleWebSetTargetEinst)))

	mux.Handle("GET /export", s.webAuth(http.HandlerFunc(s.handleWebExportHome)))
	mux.Handle("GET /ui/export/preview", s.webAuth(http.HandlerFunc(s.handleWebExportPreview)))

	mux.Handle("GET /wissen", s.webAuth(http.HandlerFunc(s.handleWebWissenHome)))
	mux.Handle("GET /ui/wissen/list", s.webAuth(http.HandlerFunc(s.handleWebWissenList)))
	mux.Handle("GET /wissen/daily", s.webAuth(http.HandlerFunc(s.handleWebWissenCategory)))
	mux.Handle("GET /wissen/projekte", s.webAuth(http.HandlerFunc(s.handleWebWissenCategory)))
	mux.Handle("GET /wissen/frei", s.webAuth(http.HandlerFunc(s.handleWebWissenCategory)))
	mux.Handle("GET /wissen/system", s.webAuth(http.HandlerFunc(s.handleWebWissenCategory)))
	mux.Handle("GET /ui/wissen/list/{category}", s.webAuth(http.HandlerFunc(s.handleWebWissenCategoryList)))
	mux.Handle("GET /wissen/neu", s.webAuth(http.HandlerFunc(s.handleWebEditorNew)))
	mux.Handle("POST /wissen/preview", s.webAuth(http.HandlerFunc(s.handleWebEditorPreview)))
	mux.Handle("POST /wissen", s.webAuth(http.HandlerFunc(s.handleWebEditorCreate)))
	mux.Handle("GET /wissen/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocumentView)))
	mux.Handle("GET /wissen/{id}/bearbeiten", s.webAuth(http.HandlerFunc(s.handleWebEditorEdit)))
	mux.Handle("POST /wissen/{id}", s.webAuth(http.HandlerFunc(s.handleWebEditorUpdate)))
	mux.Handle("POST /wissen/{id}/delete", s.webAuth(http.HandlerFunc(s.handleWebEditorDelete)))
	mux.Handle("POST /wissen/{id}/reembed", s.webAuth(http.HandlerFunc(s.handleWebDocReembed)))

	mux.Handle("POST /api/v1/maintenance/strip-frontmatter", s.authAny(http.HandlerFunc(s.handleStripFrontmatter)))
	mux.Handle("POST /api/v1/maintenance/redesign-doctypes", s.authAny(http.HandlerFunc(s.handleRedesignDocTypes)))

	mux.Handle("GET /api/v1/context", s.auth(http.HandlerFunc(s.handleGetContext)))
	mux.Handle("PUT /api/v1/context/active", s.auth(http.HandlerFunc(s.handlePutContextActive)))

	mux.Handle("GET /nodes", s.webAuth(http.HandlerFunc(s.handleWebNodesHome)))
	mux.Handle("GET /ui/nodes/list", s.webAuth(http.HandlerFunc(s.handleWebNodesList)))
	mux.Handle("GET /nodes/new", s.webAuth(http.HandlerFunc(s.handleWebNodeNew)))
	mux.Handle("POST /nodes", s.webAuth(http.HandlerFunc(s.handleWebNodeCreate)))
	mux.Handle("GET /nodes/{id}", s.webAuth(http.HandlerFunc(s.handleWebNodeView)))
	mux.Handle("GET /nodes/{id}/edit", s.webAuth(http.HandlerFunc(s.handleWebNodeEdit)))
	mux.Handle("POST /nodes/{id}", s.webAuth(http.HandlerFunc(s.handleWebNodeUpdate)))
	mux.Handle("POST /nodes/{id}/status", s.webAuth(http.HandlerFunc(s.handleWebNodeStatus)))
	mux.Handle("POST /nodes/{id}/delete", s.webAuth(http.HandlerFunc(s.handleWebNodeDelete)))
	mux.Handle("POST /nodes/{id}/move", s.webAuth(http.HandlerFunc(s.handleWebNodeMove)))

	// WebUI design-system showcase (Slice 0 deliverable; handler in webui_styleguide.go).
	mux.Handle("GET /ui", s.webAuth(http.HandlerFunc(s.handleWebStyleguide)))

	mux.Handle("GET /static/", http.StripPrefix("/static/", webui.StaticHandler()))
	return mux
}
