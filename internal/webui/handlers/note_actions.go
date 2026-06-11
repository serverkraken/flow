// note_actions.go — R1: only the repo-note edit/put handlers remain.
//
// Browser-side write handlers for the repo-note editing surface:
//
//   - GET /repos/{key}/note/edit     → repo-note edit form
//   - PUT /repos/{key}/note          → repo-note save (OCC + Lamport)
//
// Repo-notes are OCC-versioned via pgstore.Documents. The PUT handler
// renders a two-column conflict overlay (server | dein stand) when
// expectedVersion is stale.
//
// Both handlers fail closed with 401 when no user is in context.
// Cross-tenant access returns 404 to avoid leaking row existence.
//
// CSRF: deferred to Phase 2 — single-user hobby surface.

package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	repostmpl "github.com/serverkraken/flow/internal/webui/templates/repos"
)

// NoteActionsDeps bundles the adapters the repo-note edit handlers share.
// Documents is the server-side document store adapter (R1).
type NoteActionsDeps struct {
	Documents ports.DocumentStore
	Clock     ports.Clock

	// Bus broadcasts repo_note.* events to the SSE stream.
	// Optional — nil silently no-ops the publish calls.
	Bus *sse.Broadcaster
}

// publish wraps Bus.Publish with a nil-guard. Mirrors the helper on
// SessionActionsDeps + ProjectActionsDeps so the call sites stay terse.
func (d NoteActionsDeps) publish(userID, eventType string, data any) {
	if d.Bus == nil {
		return
	}
	d.Bus.Publish(userID, sse.Event{Type: eventType, Data: data})
}

// — repo-note edit form (GET /repos/{key}/note/edit) ------------------------

// NewRepoNoteEdit returns the handler for GET /repos/{key}/note/edit.
// chi captures {key} URL-encoded; the handler decodes it and looks up
// the repo + (optional) existing note. When no note exists yet the
// form opens with empty content + version=0, which the handler maps
// to a fresh insert on PUT.
func NewRepoNoteEdit(d NoteActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		canonicalKey, err := decodeRepoKey(r)
		if err != nil || canonicalKey == "" {
			renderReposNoteNotFound(w, r, canonicalKey)
			return
		}

		var (
			content string
			version int64
			isNew   bool
		)
		doc, err := d.Documents.GetByRepoKey(u.ID, canonicalKey)
		switch {
		case err == nil:
			content = doc.Body
			version = doc.Version
		case errors.Is(err, ports.ErrDocumentNotFound):
			isNew = true
		default:
			slog.Error(
				"repo note edit: GetByRepoKey failed",
				slog.String("user_id", u.ID),
				slog.String("canonical_key", canonicalKey),
				slog.String("err", err.Error()),
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		vm := repostmpl.NoteEditVM{
			DisplayName:  repoDisplayName(canonicalKey),
			CanonicalKey: canonicalKey,
			Content:      content,
			Version:      version,
			IsNew:        isNew,
		}
		meta := layout.PageMeta{
			Title:       "Repos · " + vm.DisplayName + " · note",
			CurrentPath: "/repos",
			UserLabel:   userLabelFromContext(r.Context()),
			Spine:       layout.SpineState{},
		}
		if err := layout.Base(meta, repostmpl.EditNote(vm)).Render(r.Context(), w); err != nil {
			slog.Error(
				"repo note edit: render failed",
				slog.String("canonical_key", canonicalKey),
				slog.String("err", err.Error()),
			)
		}
	})
}

// — repo-note PUT (PUT /repos/{key}/note) -----------------------------------

// NewRepoNotePut returns the handler for PUT /repos/{key}/note.
func NewRepoNotePut(d NoteActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		canonicalKey, err := decodeRepoKey(r)
		if err != nil || canonicalKey == "" {
			http.NotFound(w, r)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		content := r.FormValue("content")
		version := parseVersion(r.FormValue("version"))

		saved, err := d.Documents.Put(u.ID, repoNotePathWeb(canonicalKey), content, canonicalKey, version)
		if errors.Is(err, ports.ErrDocumentVersionConflict) {
			renderRepoNoteConflict(w, r, d, u.ID, canonicalKey, content)
			return
		}
		if err != nil {
			slog.Error(
				"repo note put: Put failed",
				slog.String("user_id", u.ID),
				slog.String("canonical_key", canonicalKey),
				slog.String("err", err.Error()),
			)
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}

		d.publish(u.ID, "repo_note.updated", map[string]any{
			"id":            saved.ID,
			"canonical_key": canonicalKey,
		})
		if d.Bus != nil {
			d.Bus.Changed(u.ID, "documents")
		}

		http.Redirect(w, r, repostmpl.NoteHref(canonicalKey), http.StatusSeeOther)
	})
}

// — helpers -----------------------------------------------------------------

// decodeRepoKey reads the chi {key} URL param, URL-decodes it, and
// returns the canonical key. Falls back to splitting the path tail when
// no chi context exists. Mirrors the dispatch shape used by repos.go.
func decodeRepoKey(r *http.Request) (string, error) {
	if escaped := chi.URLParam(r, "key"); escaped != "" {
		return url.PathUnescape(escaped)
	}
	// Direct ServeHTTP fallback — peel "/repos/<key>/note(/edit)?"
	tail := strings.TrimPrefix(r.URL.Path, "/repos/")
	for _, suffix := range []string{"/note/edit", "/note"} {
		if strings.HasSuffix(tail, suffix) {
			tail = strings.TrimSuffix(tail, suffix)
			break
		}
	}
	return url.PathUnescape(tail)
}

// parseVersion folds a form field into the int64 expectedVersion. Empty
// or invalid input → 0 (matches the "first save" branch the PUT handler
// then disambiguates by reading the stored row).
func parseVersion(raw string) int64 {
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// repoNotePathWeb mirrors the API-side path convention (Spec §6).
func repoNotePathWeb(canonicalKey string) string {
	return "repos/" + url.PathEscape(canonicalKey) + ".md"
}

// renderRepoNoteConflict fetches the server's current note state and
// renders the two-column overlay so Soenne picks a branch.
func renderRepoNoteConflict(
	w http.ResponseWriter,
	r *http.Request,
	d NoteActionsDeps,
	userID string,
	canonicalKey string,
	localContent string,
) {
	serverContent := ""
	serverVersion := int64(0)
	if doc, err := d.Documents.GetByRepoKey(userID, canonicalKey); err == nil {
		serverContent = doc.Body
		serverVersion = doc.Version
	} else if !errors.Is(err, ports.ErrDocumentNotFound) {
		slog.Error(
			"repo note put: conflict re-read failed",
			slog.String("user_id", userID),
			slog.String("canonical_key", canonicalKey),
			slog.String("err", err.Error()),
		)
	}
	vm := repostmpl.NoteConflictVM{
		DisplayName:   repoDisplayName(canonicalKey),
		CanonicalKey:  canonicalKey,
		ServerContent: serverContent,
		LocalContent:  localContent,
		ServerVersion: serverVersion,
	}
	meta := layout.PageMeta{
		Title:       "Repos · " + vm.DisplayName + " · Konflikt",
		CurrentPath: "/repos",
		UserLabel:   userLabelFromContext(r.Context()),
		Spine:       layout.SpineState{},
	}
	httpserver.SyncConflicts.WithLabelValues("repo_notes").Inc()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusConflict)
	if err := layout.Base(meta, repostmpl.EditNoteConflict(vm)).Render(r.Context(), w); err != nil {
		slog.Error(
			"repo note conflict: render failed",
			slog.String("err", err.Error()),
		)
	}
}
