// Package handlers implements the WebUI HTTP handlers.
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	notestmpl "github.com/serverkraken/flow/internal/webui/templates/notes"
)

// DocumentActionsDeps bundles the write path for documents-backed notes.
type DocumentActionsDeps struct {
	Store flowports.DocumentStore
	Bus   *sse.Broadcaster
}

// NewDocumentPut handles PUT /notes/* — CodeMirror save with If-Match.
// Conflict (412-Semantik) re-renders the edit form with the FRESH server
// body + version and a hint in the title (Spec §8-Analogie: neu laden +
// Hinweis; eine Diff-UI ist bewusst nicht R1).
func NewDocumentPut(d DocumentActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		docPath := docWildcardPath(r, "/edit")
		if docPath == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}
		content := r.PostForm.Get("content")
		version, _ := strconv.ParseInt(strings.TrimSpace(r.PostForm.Get("version")), 10, 64)

		_, err := d.Store.Put(u.ID, docPath, content, "", version)
		if errors.Is(err, flowports.ErrDocumentVersionConflict) {
			current, gerr := d.Store.Get(u.ID, docPath)
			if gerr != nil {
				slog.Error("document put: conflict re-read failed", slog.String("err", gerr.Error()))
				http.Error(w, "internal", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusConflict)
			vm := notestmpl.EditVM{
				ID:      current.Path,
				Title:   docTitle(current.Path, current.Body) + " — Konflikt: Server-Stand neu geladen",
				Content: current.Body,
				Version: current.Version,
			}
			meta := layout.PageMeta{
				Title:       "Notes · Konflikt",
				CurrentPath: "/notes",
				UserLabel:   userLabel(u),
				Spine:       layout.SpineState{},
			}
			_ = layout.Base(meta, notestmpl.Edit(vm)).Render(r.Context(), w)
			return
		}
		if err != nil {
			slog.Error("document put: failed", slog.String("path", docPath), slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		if d.Bus != nil {
			d.Bus.Publish(u.ID, sse.Event{Type: "note.updated", Data: map[string]any{"path": docPath}})
			d.Bus.Changed(u.ID, "documents")
		}
		http.Redirect(w, r, "/notes/"+docPath, http.StatusSeeOther)
	})
}
