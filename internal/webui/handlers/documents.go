// Package handlers implements the WebUI HTTP handlers.
//
// R1: /notes wird von der documents-Tabelle bedient (Spec §10) — gleiche
// Templates, neue Wahrheit. Drei Handler: Index (Liste + Server-FTS via
// ?q=), View (gerendertes Markdown), Edit (CodeMirror-Form mit Version).
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/markdown"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	notestmpl "github.com/serverkraken/flow/internal/webui/templates/notes"
)

// DocumentsDeps bundles the documents-backed /notes surface dependencies.
type DocumentsDeps struct {
	Store    flowports.DocumentStore
	Markdown *markdown.Renderer
	Clock    flowports.Clock
}

// docWildcardPath extracts the multi-segment document path from /notes/*.
func docWildcardPath(r *http.Request, suffixToStrip string) string {
	raw := chi.URLParam(r, "*")
	if raw == "" {
		raw = strings.TrimPrefix(r.URL.Path, "/notes/")
	}
	p, err := url.PathUnescape(raw)
	if err != nil {
		p = raw
	}
	return strings.TrimSuffix(strings.TrimPrefix(p, "/"), suffixToStrip)
}

// NewDocumentsIndex handles GET /notes (+ ?q= Server-FTS).
func NewDocumentsIndex(d DocumentsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		query := strings.TrimSpace(r.URL.Query().Get("q"))
		entries, err := d.Store.List(u.ID, "", query, 200)
		if err != nil {
			slog.Error("documents index: list failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		vm := buildDocumentsIndexVM(entries, query, d.Clock)
		meta := layout.PageMeta{
			Title:       "Notes",
			CurrentPath: "/notes",
			UserLabel:   userLabel(u),
			Spine:       layout.SpineState{},
		}
		if err := layout.Base(meta, notestmpl.Index(vm)).Render(r.Context(), w); err != nil {
			slog.Error("documents index: render failed", slog.String("err", err.Error()))
		}
	})
}

// NewDocumentView handles GET /notes/* (single document).
func NewDocumentView(d DocumentsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		docPath := docWildcardPath(r, "")
		if docPath == "" {
			http.NotFound(w, r)
			return
		}
		doc, err := d.Store.Get(u.ID, docPath)
		if errors.Is(err, flowports.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.Error("document view: get failed", slog.String("path", docPath), slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		vm, err := buildDocumentViewVM(d, doc)
		if err != nil {
			slog.Error("document view: build failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		meta := layout.PageMeta{
			Title:       "Notes · " + vm.Title,
			CurrentPath: "/notes",
			UserLabel:   userLabel(u),
			Spine:       layout.SpineState{},
		}
		if err := layout.Base(meta, notestmpl.View(vm)).Render(r.Context(), w); err != nil {
			slog.Error("document view: render failed", slog.String("err", err.Error()))
		}
	})
}

// NewDocumentEdit handles GET /notes/*/edit — the CodeMirror form,
// pre-filled with the current body + version (If-Match seed).
func NewDocumentEdit(d DocumentsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		docPath := docWildcardPath(r, "/edit")
		if docPath == "" {
			http.NotFound(w, r)
			return
		}
		doc, err := d.Store.Get(u.ID, docPath)
		if errors.Is(err, flowports.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.Error("document edit: get failed", slog.String("path", docPath), slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		vm := notestmpl.EditVM{
			ID:      doc.Path,
			Title:   docTitle(doc.Path, doc.Body),
			Content: doc.Body,
			Version: doc.Version,
		}
		meta := layout.PageMeta{
			Title:       "Notes · " + vm.Title + " bearbeiten",
			CurrentPath: "/notes",
			UserLabel:   userLabel(u),
			Spine:       layout.SpineState{},
		}
		if err := layout.Base(meta, notestmpl.Edit(vm)).Render(r.Context(), w); err != nil {
			slog.Error("document edit: render failed", slog.String("err", err.Error()))
		}
	})
}

// docTitle derives a display title: first H1 wins, else the file stem.
func docTitle(docPath, body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return strings.TrimSuffix(path.Base(docPath), ".md")
}
