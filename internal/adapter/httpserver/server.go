// Package httpserver exposes the REST + SSE API and the WebUI auth flow.
package httpserver

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type Server struct {
	Verifier ports.TokenVerifier
	Ensure   usecase.EnsureUser
	Bus      ports.EventBus
	Emitter  ports.Emitter
	Clock    ports.Clock
	Dev      bool
	Ready    func(context.Context) error // optional DB readiness probe; nil = always ready

	// CSPEnforce switches the Content-Security-Policy header from
	// Report-Only (default, false) to enforcing (Lesesaal L3 Task 9,
	// Soenne Entsch. #8 — flip once the cross-surface smoke shows zero
	// violations).
	CSPEnforce bool

	// worktime usecases
	StartSession      usecase.StartSession
	StopSession       usecase.StopSession
	SwitchSession     usecase.SwitchSession
	ListSessions      usecase.ListSessions
	CreateNode        usecase.CreateNode
	CreateBoundNode   usecase.CreateBoundNode
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

	// tmux status segment — aggregated read-only composer + node MRU
	WorktimeStatus usecase.WorktimeStatus
	NodeMRU        usecase.NodeMRU

	// m1e export
	BuildExport usecase.BuildExport
	SetNodeRate usecase.SetNodeRate

	// cockpit-story slice 1 (Work/Privat)
	SetCountsTowardTarget usecase.SetCountsTowardTarget

	// cockpit-story slice 2 (node logos)
	UploadNodeLogo usecase.UploadNodeLogo
	DeleteNodeLogo usecase.DeleteNodeLogo
	GetNodeLogo    usecase.GetNodeLogo

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
	SetTags  usecase.SetTags
	GetTags  usecase.GetTags
	NodeTags usecase.NodeTags

	// m2a documents
	CreateDocument        usecase.CreateDocument
	ImportDocument        usecase.ImportDocument
	GetDocument           usecase.GetDocument
	ListDocuments         usecase.ListDocuments
	ListDocumentsPage     *usecase.ListDocumentsPage
	ListDocumentLibrary   usecase.ListDocumentLibrary
	SearchDocumentLibrary usecase.SearchDocumentLibrary
	UpdateDocument        usecase.UpdateDocument
	MoveDocument          usecase.MoveDocument
	DeleteDocument        usecase.DeleteDocument
	BacklinksDocument     usecase.Backlinks
	ListTags              usecase.ListTags
	SearchDocuments       usecase.SearchDocuments
	RetryEmbedding        usecase.RetryEmbedding
	GetEmbedStatus        usecase.GetEmbedStatus
	SetPinned             usecase.SetPinned
	SetContextMode        usecase.SetContextMode
	SetArchived           usecase.SetArchived
	ListArchived          usecase.ListArchived

	// activity feed (Task 5)
	ListActivity usecase.ListActivity

	// idempotent upsert by path (B3d Task 7)
	UpsertDocumentByPath usecase.UpsertDocumentByPath

	// maintenance (F1)
	StripFrontmatter usecase.StripFrontmatter
	RedesignDocTypes usecase.RedesignDocTypes
	AuditDocuments   usecase.AuditDocuments

	// B3 context store (B1, B2)
	ComposeContext     usecase.ComposeContext
	SetActiveContext   usecase.SetActiveContext
	ReorderContextDocs usecase.ReorderContextDocs
	ContextBudget      int // default cap when ?cap= absent; 0 → fall back to 12000

	// L6 Task 2: node-scoped artifacts (upload/rename/list/delete/get).
	// Upload/Rename/Delete emit artifact.* themselves (see each usecase's
	// doc comment) — the handlers below do not emit.
	UploadArtifact usecase.UploadArtifact
	RenameArtifact usecase.RenameArtifact
	ListArtifacts  usecase.ListArtifacts
	DeleteArtifact usecase.DeleteArtifact
	GetArtifact    usecase.GetArtifact

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
	mux.Handle("POST /api/v1/nodes/create-bound", s.auth(http.HandlerFunc(s.handleCreateBoundNode)))
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
	mux.Handle("GET /api/v1/worktime/status", s.auth(http.HandlerFunc(s.handleWorktimeStatus)))

	mux.Handle("GET /api/v1/export", s.authAny(http.HandlerFunc(s.handleExport)))
	mux.Handle("POST /api/v1/nodes/{id}/rate", s.auth(http.HandlerFunc(s.handleSetNodeRate)))
	mux.Handle("POST /api/v1/nodes/{id}/move", s.auth(http.HandlerFunc(s.handleMoveNode)))

	// project bindings — static paths before {id} wildcard
	mux.Handle("GET /api/v1/nodes/resolve", s.auth(http.HandlerFunc(s.handleResolveNode)))
	mux.Handle("GET /api/v1/nodes/bindings", s.auth(http.HandlerFunc(s.handleListAllNodeBindings)))
	mux.Handle("GET /api/v1/nodes/mru", s.auth(http.HandlerFunc(s.handleNodeMRU)))
	mux.Handle("DELETE /api/v1/nodes/bindings", s.auth(http.HandlerFunc(s.handleUnbindNode)))
	mux.Handle("GET /api/v1/nodes/resolve-engagement", s.auth(http.HandlerFunc(s.handleResolveEngagement)))
	mux.Handle("PUT /api/v1/nodes/{id}/bindings", s.auth(http.HandlerFunc(s.handleBindNode)))
	mux.Handle("GET /api/v1/nodes/{id}/bindings", s.auth(http.HandlerFunc(s.handleListNodeBindingsByNode)))
	mux.Handle("GET /api/v1/nodes/{id}/stats", s.auth(http.HandlerFunc(s.handleNodeStats)))
	mux.Handle("GET /api/v1/nodes/{id}/ancestors", s.auth(http.HandlerFunc(s.handleNodeAncestors)))
	mux.Handle("GET /api/v1/nodes/{id}/tags", s.auth(http.HandlerFunc(s.handleGetNodeTags)))
	mux.Handle("PUT /api/v1/nodes/{id}/tags", s.auth(http.HandlerFunc(s.handleSetNodeTags)))

	// L6 Task 2: node-scoped artifacts (REST JSON upload/list/delete; the
	// web gallery serve route is registered further down with the other
	// /nodes/{id}/... web routes).
	mux.Handle("POST /api/v1/nodes/{id}/artifacts", s.auth(http.HandlerFunc(s.handleUploadArtifact)))
	mux.Handle("GET /api/v1/nodes/{id}/artifacts", s.auth(http.HandlerFunc(s.handleListArtifacts)))
	mux.Handle("DELETE /api/v1/nodes/{id}/artifacts/{slug}", s.auth(http.HandlerFunc(s.handleDeleteArtifact)))

	// Free (node-less, free-artifacts Task 3) artifact REST verbs — the
	// owner-global counterparts of the /api/v1/nodes/{id}/artifacts trio.
	mux.Handle("POST /api/v1/artifacts", s.auth(http.HandlerFunc(s.handleUploadFreeArtifact)))
	mux.Handle("GET /api/v1/artifacts", s.auth(http.HandlerFunc(s.handleListFreeArtifacts)))
	mux.Handle("DELETE /api/v1/artifacts/{slug}", s.auth(http.HandlerFunc(s.handleDeleteFreeArtifact)))

	mux.Handle("POST /api/v1/documents", s.auth(http.HandlerFunc(s.handleCreateDocument)))
	mux.Handle("POST /api/v1/documents/import", s.auth(http.HandlerFunc(s.handleImportDocument)))
	mux.Handle("PUT /api/v1/documents/by-path", s.authAny(http.HandlerFunc(s.handleUpsertByPath)))
	mux.Handle("GET /api/v1/documents", s.auth(http.HandlerFunc(s.handleListDocuments)))
	mux.Handle("GET /api/v1/documents/tags", s.auth(http.HandlerFunc(s.handleListTags)))
	// archived path registered before the {id} wildcard so the static path wins
	mux.Handle("GET /api/v1/documents/archived", s.auth(http.HandlerFunc(s.handleListArchived)))
	mux.Handle("GET /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleGetDocument)))
	mux.Handle("PUT /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleUpdateDocument)))
	mux.Handle("PATCH /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handlePatchDocument)))
	mux.Handle("POST /api/v1/documents/{id}/move", s.auth(http.HandlerFunc(s.handleMoveDocument)))
	mux.Handle("DELETE /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleDeleteDocument)))
	mux.Handle("GET /api/v1/documents/{id}/backlinks", s.auth(http.HandlerFunc(s.handleDocumentBacklinks)))
	mux.Handle("POST /api/v1/documents/{id}/pin", s.auth(http.HandlerFunc(s.handlePinDocument)))
	mux.Handle("POST /api/v1/documents/{id}/context-mode", s.auth(http.HandlerFunc(s.handleSetContextMode)))
	mux.Handle("POST /api/v1/documents/{id}/archive", s.auth(http.HandlerFunc(s.handleArchiveDocument)))

	mux.Handle("GET /api/v1/activity", s.auth(http.HandlerFunc(s.handleListActivity)))

	// WebUI auth routes (handlers in webauth.go, Task 5)
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)

	// Home landing + timer-hero fragment + start/stop (Slice 4, Task 1)
	mux.Handle("GET /{$}", s.webAuth(http.HandlerFunc(s.handleHomeHome)))
	mux.Handle("GET /ui/home", s.webAuth(http.HandlerFunc(s.handleHomeFragment)))

	// Global shell timer pill (Lesesaal Task 5) — the ONE global home for the
	// running timer, mounted in the topbar as #timer-pill.
	mux.Handle("GET /ui/timer", s.webAuth(http.HandlerFunc(s.handleTimerWidget)))
	mux.Handle("POST /ui/timer/start", s.webAuth(http.HandlerFunc(s.handleTimerStart)))
	mux.Handle("POST /ui/timer/stop", s.webAuth(http.HandlerFunc(s.handleTimerStop)))
	mux.Handle("POST /ui/timer/switch", s.webAuth(http.HandlerFunc(s.handleTimerSwitch)))

	// ⌘K-Palette (Lesesaal Task 6) — fuzzy Sprung zu Knoten/Dokumenten.
	mux.Handle("GET /ui/palette", s.webAuth(http.HandlerFunc(s.handlePalette)))

	// WebUI routes (handlers in webui.go, Task 8)
	mux.Handle("GET /zeit", s.webAuth(http.HandlerFunc(s.handleZeitHome)))
	mux.Handle("GET /ui/worktime", s.webAuth(http.HandlerFunc(s.handleHeuteFragment)))
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

	mux.Handle("GET /einstellungen", s.webAuth(http.HandlerFunc(s.handleWebEinstellungenHome)))
	mux.Handle("POST /ui/einstellungen/target", s.webAuth(http.HandlerFunc(s.handleWebSetTargetEinst)))

	mux.Handle("GET /export", s.webAuth(http.HandlerFunc(s.handleWebExportHome)))
	mux.Handle("GET /ui/export/preview", s.webAuth(http.HandlerFunc(s.handleWebExportPreview)))

	// Free (node-less, free-artifacts Task 2) artifact serve route — the
	// owner-global counterpart of GET /nodes/{id}/artifacts/{slug}.
	mux.Handle("GET /artefakte/{slug}", s.webAuth(http.HandlerFunc(s.handleServeFreeArtifact)))

	mux.Handle("GET /wissen", s.webAuth(http.HandlerFunc(s.handleWebWissenHome)))
	mux.Handle("GET /ui/wissen/list", s.webAuth(http.HandlerFunc(s.handleWebWissenList)))
	// /wissen/typ?type={key} is a query param, not a path segment
	// (/wissen/typ/{type}): Go's http.ServeMux rejects that as ambiguous
	// against the established /wissen/{id}/bearbeiten action route — both
	// are 3-segment patterns with the wildcard/literal swapped, so e.g.
	// "/wissen/typ/bearbeiten" would match either (WissenVM.TypeParam).
	mux.Handle("GET /wissen/typ", s.webAuth(http.HandlerFunc(s.handleWebWissenType)))
	mux.Handle("GET /ui/wissen/list/typ", s.webAuth(http.HandlerFunc(s.handleWebWissenTypeList)))
	// Free (node-less, free-artifacts Task 4) artifact web gallery. The
	// fragment route (grid+form+error, NO AppShell) is grouped with the
	// other /ui/wissen/* fragment routes; the full page + mutation routes
	// are grouped below, registered BEFORE /wissen/{id} so the specific
	// "/wissen/artefakte" segment is unambiguous (Go 1.22 mux prefers the
	// specific literal — no real conflict, just kept clean per the plan).
	mux.Handle("GET /ui/wissen/artefakte", s.webAuth(http.HandlerFunc(s.handleWebWissenArtifactsFragment)))
	// Retired category slugs (Lesesaal L3 Task 7 — Regale nach Typ ersetzen
	// die vier Alt-Kategorien) redirect to their type-shelf successor.
	// /wissen/system has no 1:1 successor (its five legacy types now spread
	// across plan/memory/context/spec) and redirects to the overview.
	mux.Handle("GET /wissen/daily", s.webAuth(s.handleWebWissenRedirect("/wissen/typ?type=daily")))
	mux.Handle("GET /wissen/projekte", s.webAuth(s.handleWebWissenRedirect("/wissen/typ?type=project")))
	mux.Handle("GET /wissen/frei", s.webAuth(s.handleWebWissenRedirect("/wissen/typ?type=free")))
	mux.Handle("GET /wissen/system", s.webAuth(s.handleWebWissenRedirect("/wissen")))
	mux.Handle("GET /wissen/neu", s.webAuth(http.HandlerFunc(s.handleWebEditorNew)))
	mux.Handle("POST /wissen/preview", s.webAuth(http.HandlerFunc(s.handleWebEditorPreview)))
	// L6 Task 6: editor toolbar insert-pickers (Artefakt-Embed / Seiten-Wikilink).
	mux.Handle("GET /ui/editor/artefakte", s.webAuth(http.HandlerFunc(s.handleWebEditorArtefaktePicker)))
	mux.Handle("GET /ui/editor/seiten", s.webAuth(http.HandlerFunc(s.handleWebEditorSeitenPicker)))
	mux.Handle("POST /wissen", s.webAuth(http.HandlerFunc(s.handleWebEditorCreate)))
	// Free (node-less) artifact web gallery page + mutations — registered
	// BEFORE /wissen/{id} (the specific "artefakte" segment wins over the
	// wildcard either way, kept grouped here for readability).
	mux.Handle("GET /wissen/artefakte", s.webAuth(http.HandlerFunc(s.handleWebWissenArtifacts)))
	mux.Handle("POST /wissen/artefakte", s.webAuth(http.HandlerFunc(s.handleWebWissenArtifactUpload)))
	mux.Handle("POST /wissen/artefakte/{slug}/rename", s.webAuth(http.HandlerFunc(s.handleWebWissenArtifactRename)))
	mux.Handle("POST /wissen/artefakte/{slug}/delete", s.webAuth(http.HandlerFunc(s.handleWebWissenArtifactDelete)))
	mux.Handle("GET /wissen/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocumentView)))
	mux.Handle("GET /wissen/{id}/bearbeiten", s.webAuth(http.HandlerFunc(s.handleWebEditorEdit)))
	mux.Handle("POST /wissen/{id}", s.webAuth(http.HandlerFunc(s.handleWebEditorUpdate)))
	mux.Handle("POST /wissen/{id}/delete", s.webAuth(http.HandlerFunc(s.handleWebEditorDelete)))
	mux.Handle("POST /wissen/{id}/reembed", s.webAuth(http.HandlerFunc(s.handleWebDocReembed)))
	mux.Handle("POST /wissen/{id}/pin", s.webAuth(http.HandlerFunc(s.handleWebDocPin)))
	mux.Handle("POST /wissen/{id}/archive", s.webAuth(http.HandlerFunc(s.handleWebDocArchive)))
	mux.Handle("POST /wissen/{id}/mode", s.webAuth(http.HandlerFunc(s.handleWebDocMode)))

	// Kuratieren-Seite (L5 Task 7): Budget-Meter + Rang-Liste mit Höher/
	// Tiefer + Anpinnen, owner-scoped per Knoten-ID. Task 4 adds the
	// Auto/Immer/Nie mode switcher (rang list, Always-Tier, Ausgeblendet).
	mux.Handle("GET /kontext/{id}", s.webAuth(http.HandlerFunc(s.handleWebKontextView)))
	mux.Handle("POST /kontext/{id}/reorder", s.webAuth(http.HandlerFunc(s.handleWebKontextReorder)))
	mux.Handle("POST /kontext/{id}/pin", s.webAuth(http.HandlerFunc(s.handleWebKontextPin)))
	mux.Handle("POST /kontext/{id}/mode", s.webAuth(http.HandlerFunc(s.handleWebKontextMode)))

	mux.Handle("POST /api/v1/maintenance/strip-frontmatter", s.authAny(http.HandlerFunc(s.handleStripFrontmatter)))
	mux.Handle("POST /api/v1/maintenance/redesign-doctypes", s.authAny(http.HandlerFunc(s.handleRedesignDocTypes)))
	mux.Handle("GET /api/v1/maintenance/audit-documents", s.authAny(http.HandlerFunc(s.handleAuditDocuments)))

	mux.Handle("GET /api/v1/context", s.auth(http.HandlerFunc(s.handleGetContext)))
	mux.Handle("PUT /api/v1/context/active", s.auth(http.HandlerFunc(s.handlePutContextActive)))
	mux.Handle("POST /api/v1/context/reorder", s.auth(http.HandlerFunc(s.handleReorderContext)))

	mux.Handle("GET /nodes", s.webAuth(http.HandlerFunc(s.handleWebNodesHome)))
	mux.Handle("GET /ui/nodes/list", s.webAuth(http.HandlerFunc(s.handleWebNodesList)))
	mux.Handle("GET /nodes/new", s.webAuth(http.HandlerFunc(s.handleWebNodeNew)))
	mux.Handle("POST /nodes", s.webAuth(http.HandlerFunc(s.handleWebNodeCreate)))
	mux.Handle("GET /nodes/{id}", s.webAuth(http.HandlerFunc(s.handleWebNodeView)))
	mux.Handle("GET /nodes/{id}/head", s.webAuth(http.HandlerFunc(s.handleWebNodeHead)))
	mux.Handle("GET /nodes/{id}/main", s.webAuth(http.HandlerFunc(s.handleWebNodeMain)))
	mux.Handle("GET /nodes/{id}/rail", s.webAuth(http.HandlerFunc(s.handleWebNodeRail)))
	mux.Handle("GET /nodes/{id}/logo", s.webAuth(http.HandlerFunc(s.handleWebNodeLogo)))
	mux.Handle("GET /nodes/{id}/artifacts/{slug}", s.webAuth(http.HandlerFunc(s.handleServeArtifact)))
	mux.Handle("GET /nodes/{id}/artifacts", s.webAuth(http.HandlerFunc(s.handleWebNodeArtifacts)))
	mux.Handle("POST /nodes/{id}/artifacts", s.webAuth(http.HandlerFunc(s.handleWebNodeArtifactUpload)))
	mux.Handle("POST /nodes/{id}/artifacts/{slug}/rename", s.webAuth(http.HandlerFunc(s.handleWebNodeArtifactRename)))
	mux.Handle("POST /nodes/{id}/artifacts/{slug}/delete", s.webAuth(http.HandlerFunc(s.handleWebNodeArtifactDelete)))
	mux.Handle("POST /nodes/{id}/start", s.webAuth(http.HandlerFunc(s.handleWebNodeStart)))
	mux.Handle("POST /nodes/{id}/stop", s.webAuth(http.HandlerFunc(s.handleWebNodeStop)))
	mux.Handle("POST /nodes/{id}/switch", s.webAuth(http.HandlerFunc(s.handleWebNodeSwitch)))
	mux.Handle("GET /nodes/{id}/edit", s.webAuth(http.HandlerFunc(s.handleWebNodeEdit)))
	mux.Handle("POST /nodes/{id}", s.webAuth(http.HandlerFunc(s.handleWebNodeUpdate)))
	mux.Handle("POST /nodes/{id}/status", s.webAuth(http.HandlerFunc(s.handleWebNodeStatus)))
	mux.Handle("POST /nodes/{id}/delete", s.webAuth(http.HandlerFunc(s.handleWebNodeDelete)))
	mux.Handle("POST /nodes/{id}/move", s.webAuth(http.HandlerFunc(s.handleWebNodeMove)))
	mux.Handle("POST /nodes/{id}/sessions", s.webAuth(http.HandlerFunc(s.handleWebNodeAddSession)))
	mux.Handle("POST /nodes/{id}/sessions/{sid}/edit", s.webAuth(http.HandlerFunc(s.handleWebNodeEditSession)))
	mux.Handle("POST /nodes/{id}/sessions/{sid}/delete", s.webAuth(http.HandlerFunc(s.handleWebNodeDeleteSession)))
	mux.Handle("POST /nodes/{id}/bindings", s.webAuth(http.HandlerFunc(s.handleWebNodeBindRemote)))
	mux.Handle("POST /nodes/{id}/bindings/delete", s.webAuth(http.HandlerFunc(s.handleWebNodeUnbind)))

	// WebUI design-system showcase (Slice 0 deliverable; handler in webui_styleguide.go).
	mux.Handle("GET /ui", s.webAuth(http.HandlerFunc(s.handleWebStyleguide)))

	mux.Handle("GET /static/", http.StripPrefix("/static/", webui.StaticHandler()))
	// Cross the webui → components import direction the one way it's allowed:
	// components can't call webui.AssetVersion() itself (would reverse the
	// dependency), so Routes() hands it the value once here — every
	// components-side AssetURL() call afterwards returns the same
	// fingerprinted URL webui's own AssetURL does (Lesesaal L4 Task 7).
	components.SetAssetVersion(webui.AssetVersion())
	return s.securityHeaders(mux)
}
