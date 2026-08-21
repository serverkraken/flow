package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

// Fehlerseiten (Screen 20). Jede Web-Antwort mit 404/500 geht durch diese
// Helfer: ein htmx-Aufruf bekommt die Fläche allein (sie landet in einem
// Fragment-Ziel), eine Navigation die ganze Seite mit Schiene. Die API bleibt
// bei Klartext — Maschinen lesen keine Fehlerseiten.

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, vm webui.ErrorVM) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(vm.Status)
	if r.Header.Get("HX-Request") != "" {
		_ = webui.ErrorBody(vm).Render(r.Context(), w)
		return
	}
	_ = webui.ErrorPage(vm).Render(r.Context(), w)
}

func (s *Server) webUserName(r *http.Request) string {
	if u, ok := userFrom(r.Context()); ok {
		if u.Username != "" {
			return u.Username
		}
		return u.DisplayName
	}
	return ""
}

// webNotFound antwortet 404 mit der Fehlerseite — sagt, was fehlt (Karte,
// Register, Seite), und führt weiter.
func (s *Server) webNotFound(w http.ResponseWriter, r *http.Request) {
	s.renderError(w, r, webui.NotFoundVM(r.URL.Path, s.webUserName(r)))
}

// webServerError antwortet 500 mit der Fehlerseite und protokolliert den
// Fehler unter einer kurzen ID, die auf der Seite steht — die eine Zahl, die
// sich zwischen Mensch und Log austauschen lässt.
func (s *Server) webServerError(w http.ResponseWriter, r *http.Request, err error) {
	id := errorID()
	slog.ErrorContext(r.Context(), "web: server error", "id", id, "path", r.URL.Path, "err", err)
	s.renderError(w, r, webui.ServerErrorVM(r.URL.Path, s.webUserName(r), id))
}

// errorID ist kurz genug zum Abtippen und zufällig genug, um eindeutig zu
// sein: acht Hex-Zeichen, in zwei Blöcken.
func errorID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000-0000"
	}
	h := hex.EncodeToString(b[:])
	return h[:4] + "-" + h[4:]
}

// handleWebNotFound ist der Auffang für unbekannte Adressen: die API bleibt
// Klartext, der Rest bekommt die Seite — angemeldet mit Schiene, sonst ohne.
func (s *Server) handleWebNotFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/static/") {
		http.NotFound(w, r)
		return
	}
	u, ok := s.resolveCookie(r)
	if !ok {
		// Ohne Sitzung keine Hülle und keine Umleitung: eine Umleitung
		// verriete, welche Adressen es gibt.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = webui.ErrorPageBare(webui.NotFoundVM(r.URL.Path, "")).Render(r.Context(), w)
		return
	}
	r = r.WithContext(ctxWithUser(r.Context(), u))
	s.webNotFound(w, r)
}
