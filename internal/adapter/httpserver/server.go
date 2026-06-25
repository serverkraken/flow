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
	CreateProject     usecase.CreateProject
	ListProjects      usecase.ListProjects
	DeleteProject     usecase.DeleteProject
	UpdateProject     usecase.UpdateProject
	GetProject        usecase.GetProject
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
	BuildExport    usecase.BuildExport
	SetProjectRate usecase.SetProjectRate

	// slice 1 bulk ops
	BulkAssignProject  usecase.BulkAssignProject
	BulkDeleteSessions usecase.BulkDeleteSessions

	// project bindings (resolution V0)
	BindProject         usecase.BindProject
	UnbindProject       usecase.UnbindProject
	ResolveProject      usecase.ResolveProject
	ListProjectBindings usecase.ListProjectBindings

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
	mux.Handle("GET /api/v1/sessions", s.auth(http.HandlerFunc(s.handleListSessions)))
	mux.Handle("PATCH /api/v1/sessions/{id}", s.auth(http.HandlerFunc(s.handleEditSession)))
	mux.Handle("DELETE /api/v1/sessions/{id}", s.auth(http.HandlerFunc(s.handleDeleteSession)))
	mux.Handle("POST /api/v1/projects", s.auth(http.HandlerFunc(s.handleCreateProject)))
	mux.Handle("GET /api/v1/projects", s.auth(http.HandlerFunc(s.handleListProjects)))
	mux.Handle("DELETE /api/v1/projects/{id}", s.auth(http.HandlerFunc(s.handleDeleteProject)))
	mux.Handle("GET /api/v1/projects/{id}", s.auth(http.HandlerFunc(s.handleGetProject)))
	mux.Handle("PATCH /api/v1/projects/{id}", s.auth(http.HandlerFunc(s.handleUpdateProject)))

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
	mux.Handle("POST /api/v1/projects/{id}/rate", s.auth(http.HandlerFunc(s.handleSetProjectRate)))

	// project bindings — static paths before {id} wildcard
	mux.Handle("GET /api/v1/projects/resolve", s.auth(http.HandlerFunc(s.handleResolveProject)))
	mux.Handle("GET /api/v1/projects/bindings", s.auth(http.HandlerFunc(s.handleListAllProjectBindings)))
	mux.Handle("DELETE /api/v1/projects/bindings", s.auth(http.HandlerFunc(s.handleUnbindProject)))
	mux.Handle("PUT /api/v1/projects/{id}/bindings", s.auth(http.HandlerFunc(s.handleBindProject)))
	mux.Handle("GET /api/v1/projects/{id}/bindings", s.auth(http.HandlerFunc(s.handleListProjectBindingsByProject)))

	mux.Handle("POST /api/v1/documents", s.auth(http.HandlerFunc(s.handleCreateDocument)))
	mux.Handle("POST /api/v1/documents/import", s.auth(http.HandlerFunc(s.handleImportDocument)))
	mux.Handle("GET /api/v1/documents", s.auth(http.HandlerFunc(s.handleListDocuments)))
	mux.Handle("GET /api/v1/documents/tags", s.auth(http.HandlerFunc(s.handleListTags)))
	mux.Handle("GET /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleGetDocument)))
	mux.Handle("PUT /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleUpdateDocument)))
	mux.Handle("DELETE /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleDeleteDocument)))
	mux.Handle("GET /api/v1/documents/{id}/backlinks", s.auth(http.HandlerFunc(s.handleDocumentBacklinks)))

	// WebUI auth routes (handlers in webauth.go, Task 5)
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)

	// WebUI routes (handlers in webui.go, Task 8)
	mux.Handle("GET /{$}", s.webAuth(http.HandlerFunc(s.handleHeuteHome)))
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

	mux.Handle("GET /export", s.webAuth(http.HandlerFunc(s.handleWebExportHome)))
	mux.Handle("GET /ui/export/preview", s.webAuth(http.HandlerFunc(s.handleWebExportPreview)))

	mux.Handle("GET /wissen", s.webAuth(http.HandlerFunc(s.handleWebWissenHome)))
	mux.Handle("GET /ui/wissen/list", s.webAuth(http.HandlerFunc(s.handleWebWissenList)))
	mux.Handle("GET /wissen/neu", s.webAuth(http.HandlerFunc(s.handleWebEditorNew)))
	mux.Handle("POST /wissen/preview", s.webAuth(http.HandlerFunc(s.handleWebEditorPreview)))
	mux.Handle("POST /wissen", s.webAuth(http.HandlerFunc(s.handleWebEditorCreate)))
	mux.Handle("GET /wissen/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocumentView)))
	mux.Handle("GET /wissen/{id}/bearbeiten", s.webAuth(http.HandlerFunc(s.handleWebEditorEdit)))
	mux.Handle("POST /wissen/{id}", s.webAuth(http.HandlerFunc(s.handleWebEditorUpdate)))
	mux.Handle("POST /wissen/{id}/delete", s.webAuth(http.HandlerFunc(s.handleWebEditorDelete)))
	mux.Handle("POST /wissen/{id}/reembed", s.webAuth(http.HandlerFunc(s.handleWebDocReembed)))

	mux.Handle("GET /projects", s.webAuth(http.HandlerFunc(s.handleWebProjectsHome)))
	mux.Handle("GET /ui/projects/list", s.webAuth(http.HandlerFunc(s.handleWebProjectsList)))
	mux.Handle("GET /projects/new", s.webAuth(http.HandlerFunc(s.handleWebProjectNew)))
	mux.Handle("POST /projects", s.webAuth(http.HandlerFunc(s.handleWebProjectCreate)))
	mux.Handle("GET /projects/{id}", s.webAuth(http.HandlerFunc(s.handleWebProjectView)))
	mux.Handle("GET /projects/{id}/edit", s.webAuth(http.HandlerFunc(s.handleWebProjectEdit)))
	mux.Handle("POST /projects/{id}", s.webAuth(http.HandlerFunc(s.handleWebProjectUpdate)))
	mux.Handle("POST /projects/{id}/status", s.webAuth(http.HandlerFunc(s.handleWebProjectStatus)))
	mux.Handle("POST /projects/{id}/delete", s.webAuth(http.HandlerFunc(s.handleWebProjectDelete)))

	// WebUI design-system showcase (Slice 0 deliverable; handler in webui_styleguide.go).
	mux.Handle("GET /ui", s.webAuth(http.HandlerFunc(s.handleWebStyleguide)))

	mux.Handle("GET /static/", http.StripPrefix("/static/", webui.StaticHandler()))
	return mux
}
