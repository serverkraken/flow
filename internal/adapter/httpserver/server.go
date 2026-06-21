// Package httpserver exposes the REST + SSE API and the WebUI auth flow.
package httpserver

import (
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

	// worktime usecases
	StartSession  usecase.StartSession
	StopSession   usecase.StopSession
	ListSessions  usecase.ListSessions
	CreateProject usecase.CreateProject
	ListProjects  usecase.ListProjects
	EditSession   usecase.EditSession
	DeleteSession usecase.DeleteSession
	AddSession        usecase.AddSession
	ListSessionsRange usecase.ListSessionsRange

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
	UpdateDocument    usecase.UpdateDocument
	DeleteDocument    usecase.DeleteDocument
	BacklinksDocument usecase.Backlinks
	ListTags          usecase.ListTags
	SearchDocuments   usecase.SearchDocuments

	// WebUI auth (wired in Task 5)
	OIDCAuth Authenticator
	Session  SessionCodec
	Users    ports.UserStore
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("GET /api/v1/me", s.auth(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/v1/events", s.authAny(http.HandlerFunc(s.handleEvents)))

	mux.Handle("POST /api/v1/sessions", s.auth(http.HandlerFunc(s.handleStartSession)))
	mux.Handle("POST /api/v1/sessions/{id}/stop", s.auth(http.HandlerFunc(s.handleStopSession)))
	mux.Handle("GET /api/v1/sessions", s.auth(http.HandlerFunc(s.handleListSessions)))
	mux.Handle("PATCH /api/v1/sessions/{id}", s.auth(http.HandlerFunc(s.handleEditSession)))
	mux.Handle("DELETE /api/v1/sessions/{id}", s.auth(http.HandlerFunc(s.handleDeleteSession)))
	mux.Handle("POST /api/v1/projects", s.auth(http.HandlerFunc(s.handleCreateProject)))
	mux.Handle("GET /api/v1/projects", s.auth(http.HandlerFunc(s.handleListProjects)))

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
	mux.Handle("GET /{$}", s.webAuth(http.HandlerFunc(s.handleWebHome)))
	mux.Handle("GET /ui/worktime", s.webAuth(http.HandlerFunc(s.handleWebFragment)))
	mux.Handle("POST /ui/worktime/start", s.webAuth(http.HandlerFunc(s.handleWebStart)))
	mux.Handle("POST /ui/worktime/stop", s.webAuth(http.HandlerFunc(s.handleWebStop)))
	mux.Handle("POST /ui/worktime/add", s.webAuth(http.HandlerFunc(s.handleWebAdd)))
	mux.Handle("POST /ui/worktime/edit", s.webAuth(http.HandlerFunc(s.handleWebEdit)))
	mux.Handle("POST /ui/worktime/delete", s.webAuth(http.HandlerFunc(s.handleWebDelete)))

	mux.Handle("GET /dayoffs", s.webAuth(http.HandlerFunc(s.handleWebDayOffHome)))
	mux.Handle("GET /ui/dayoffs", s.webAuth(http.HandlerFunc(s.handleWebDayOffFragment)))
	mux.Handle("POST /ui/dayoffs/add", s.webAuth(http.HandlerFunc(s.handleWebDayOffAdd)))
	mux.Handle("POST /ui/dayoffs/delete", s.webAuth(http.HandlerFunc(s.handleWebDayOffDelete)))
	mux.Handle("POST /ui/dayoffs/regen-token", s.webAuth(http.HandlerFunc(s.handleWebRegenToken)))

	mux.Handle("GET /stats", s.webAuth(http.HandlerFunc(s.handleWebStatsHome)))
	mux.Handle("GET /ui/stats/fragment", s.webAuth(http.HandlerFunc(s.handleWebStatsFragment)))
	mux.Handle("POST /ui/stats/target", s.webAuth(http.HandlerFunc(s.handleWebSetTarget)))

	mux.Handle("GET /export", s.webAuth(http.HandlerFunc(s.handleWebExportHome)))
	mux.Handle("GET /ui/export/preview", s.webAuth(http.HandlerFunc(s.handleWebExportPreview)))

	mux.Handle("GET /docs", s.webAuth(http.HandlerFunc(s.handleWebDocsHome)))
	mux.Handle("GET /ui/docs/list", s.webAuth(http.HandlerFunc(s.handleWebDocsList)))
	mux.Handle("GET /docs/new", s.webAuth(http.HandlerFunc(s.handleWebDocNew)))
	mux.Handle("POST /docs", s.webAuth(http.HandlerFunc(s.handleWebDocCreate)))
	mux.Handle("GET /docs/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocView)))
	mux.Handle("GET /docs/{id}/edit", s.webAuth(http.HandlerFunc(s.handleWebDocEdit)))
	mux.Handle("POST /docs/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocUpdate)))
	mux.Handle("POST /docs/{id}/delete", s.webAuth(http.HandlerFunc(s.handleWebDocDelete)))

	mux.Handle("GET /static/", http.StripPrefix("/static/", webui.StaticHandler()))
	return mux
}
