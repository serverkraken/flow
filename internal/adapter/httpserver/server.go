// Package httpserver exposes the REST + SSE API.
package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type Server struct {
	Verifier ports.TokenVerifier
	Ensure   usecase.EnsureUser
	Bus      ports.EventBus
	Dev      bool
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("GET /api/v1/me", s.auth(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/v1/events", s.auth(http.HandlerFunc(s.handleEvents)))
	if s.Dev {
		mux.Handle("POST /api/v1/debug/ping", s.auth(http.HandlerFunc(s.handleDebugPing)))
	}
	return mux
}
