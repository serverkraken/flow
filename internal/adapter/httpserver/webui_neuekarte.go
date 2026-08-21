package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// neueKarteVM baut das Formular: Register des Besitzers als Baum, der
// vorausgewählte Knoten aus ?node=, der Typ aus ?type= (Standard: Projekt-
// Notiz am Register, frei ohne).
func (s *Server) neueKarteVM(r *http.Request, u domain.User, nodeID, typ string) (webui.NeueKarteVM, error) {
	ctx := r.Context()
	if typ == "" {
		typ = string(domain.DocFree)
		if nodeID != "" {
			typ = string(domain.DocProject)
		}
	}
	vm := webui.NeueKarteVM{NodeID: nodeID, Type: typ, Date: s.Clock.Now().Format("2006-01-02"), Types: webui.NeueKarteTypes(ctx)}
	if s.ListNodes.Nodes != nil {
		nodes, err := s.ListNodes.Execute(ctx, u.ID)
		if err != nil {
			return vm, err
		}
		vm.Nodes = webui.NodeSelectOptions(ctx, nodes)
	}
	return vm, nil
}

// handleWebNeueKarteForm serves GET /ui/wissen/neu: das Formular des
// ⌘N-Dialogs, mit dem Register, auf dem man steht.
func (s *Server) handleWebNeueKarteForm(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.neueKarteVM(r, u, r.URL.Query().Get("node"), editorDocumentType(r.URL.Query().Get("type")))
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NeueKarteForm(vm).Render(r.Context(), w)
}

// handleWebNeueKarteCreate serves POST /wissen/schnell: legt die Karte mit
// abgeleitetem Pfad an und öffnet den Editor. Eine Tagesnotiz, die es schon
// gibt, wird geöffnet statt verdoppelt; ein belegter Pfad bekommt eine
// Nummer — der Mensch soll keinen Pfad tippen, also auch keinen reparieren.
func (s *Server) handleWebNeueKarteCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	typ := domain.DocumentType(editorDocumentType(r.FormValue("type")))
	if typ == "" {
		typ = domain.DocFree
	}
	title := strings.TrimSpace(r.FormValue("title"))
	nodeID := strings.TrimSpace(r.FormValue("node"))
	now := s.Clock.Now()
	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		path = webui.DerivedPath(typ, title, now)
	}
	if typ == domain.DocDaily {
		title = now.Format("2006-01-02")
		if s.ListDocuments.Docs != nil {
			if docs, derr := s.ListDocuments.Execute(r.Context(), u.ID, nil, nil); derr == nil {
				for _, d := range docs {
					if d.Type == domain.DocDaily && d.Path == path {
						http.Redirect(w, r, "/wissen/"+d.ID+"/bearbeiten", http.StatusSeeOther)
						return
					}
				}
			}
		}
	}
	if title == "" {
		vm, verr := s.neueKarteVM(r, u, nodeID, string(typ))
		if verr != nil {
			s.webServerError(w, r, verr)
			return
		}
		vm.Err = components.T(r.Context(), "neuekarte.titleMissing")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.NeueKarteForm(vm).Render(r.Context(), w)
		return
	}
	var nodePtr *string
	if nodeID != "" {
		nodePtr = &nodeID
	}
	var date *time.Time
	if typ == domain.DocDaily {
		date = &now
	}
	base := path
	for attempt := 0; attempt < 10; attempt++ {
		doc, err := s.CreateDocument.Execute(r.Context(), u.ID, usecase.CreateDocumentInput{
			Type: typ, NodeID: nodePtr, Path: path, Date: date, Title: title, Body: "",
		})
		switch {
		case err == nil:
			s.emitEvent(r.Context(), domain.Event{Type: domain.EventDocumentCreated, UserID: u.ID, Data: map[string]any{"id": doc.ID, "title": doc.Title}})
			http.Redirect(w, r, "/wissen/"+doc.ID+"/bearbeiten", http.StatusSeeOther)
			return
		case errors.Is(err, ports.ErrDocumentExists):
			path = base + "-" + strconv.Itoa(attempt+2)
			continue
		case errors.Is(err, domain.ErrInvalidDocument):
			vm, verr := s.neueKarteVM(r, u, nodeID, string(typ))
			if verr != nil {
				s.webServerError(w, r, verr)
				return
			}
			vm.Err = components.T(r.Context(), "wissen.metadata.invalid")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = webui.NeueKarteForm(vm).Render(r.Context(), w)
			return
		default:
			s.webServerError(w, r, err)
			return
		}
	}
	s.webServerError(w, r, ports.ErrDocumentExists)
}
