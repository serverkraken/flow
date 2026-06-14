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

	// m1c worktime extras
	AddDayOffs    usecase.AddDayOffs
	DeleteDayOff  usecase.DeleteDayOff
	ListDayOffs   usecase.ListDayOffs
	GetSettings   usecase.GetSettings
	SetBundesland usecase.SetBundesland
	IcsFeed       usecase.IcsFeed
	RegenIcsToken usecase.RegenerateIcsToken

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
	mux.Handle("POST /api/v1/projects", s.auth(http.HandlerFunc(s.handleCreateProject)))
	mux.Handle("GET /api/v1/projects", s.auth(http.HandlerFunc(s.handleListProjects)))

	mux.Handle("GET /api/v1/dayoffs", s.auth(http.HandlerFunc(s.handleListDayOffs)))
	mux.Handle("POST /api/v1/dayoffs", s.auth(http.HandlerFunc(s.handleAddDayOffs)))
	mux.Handle("DELETE /api/v1/dayoffs/{day}", s.auth(http.HandlerFunc(s.handleDeleteDayOff)))

	mux.Handle("GET /api/v1/settings", s.auth(http.HandlerFunc(s.handleGetSettings)))
	mux.Handle("POST /api/v1/settings/bundesland", s.auth(http.HandlerFunc(s.handleSetBundesland)))
	mux.Handle("POST /api/v1/ics-token/regenerate", s.auth(http.HandlerFunc(s.handleRegenIcsToken)))

	// WebUI auth routes (handlers in webauth.go, Task 5)
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)

	// WebUI routes (handlers in webui.go, Task 8)
	mux.Handle("GET /{$}", s.webAuth(http.HandlerFunc(s.handleWebHome)))
	mux.Handle("GET /ui/worktime", s.webAuth(http.HandlerFunc(s.handleWebFragment)))
	mux.Handle("POST /ui/worktime/start", s.webAuth(http.HandlerFunc(s.handleWebStart)))
	mux.Handle("POST /ui/worktime/stop", s.webAuth(http.HandlerFunc(s.handleWebStop)))
	mux.Handle("GET /static/", http.StripPrefix("/static/", webui.StaticHandler()))
	return mux
}
