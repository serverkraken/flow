// note_actions.go — Plan E · Task 12 (M7).
//
// Browser-side write handlers for the two markdown-editing surfaces:
//
//   - GET /notes/{*}/edit            → kompendium note edit form
//   - PUT /notes/{*}                 → kompendium note save
//   - GET /repos/{key}/note/edit     → repo-note edit form
//   - PUT /repos/{key}/note          → repo-note save (OCC + Lamport)
//
// Kompendium notes are file-backed via kompports.NoteStore — there is
// no Lamport version, so the WebUI is last-write-wins for M7. The
// TODO at the PUT handler tracks adding OCC once kompendium gains
// server sync (Phase 2).
//
// Repo-notes ARE OCC-versioned via sqliteserver.RepoNotes.Upsert. The
// PUT handler renders a two-column conflict overlay (server | dein
// stand) when expectedVersion is stale, mirroring the worktime
// session conflict shape but rendered as a full page (the form is a
// standalone surface, not an HTMX row swap).
//
// All four handlers fail closed with 401 when no user is in context.
// Cross-tenant access returns 404 to avoid leaking row existence.
//
// CSRF: deferred to Phase 2 — single-user hobby surface. Same TODO as
// session_actions.go.

package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	notestmpl "github.com/serverkraken/flow/internal/webui/templates/notes"
	repostmpl "github.com/serverkraken/flow/internal/webui/templates/repos"
)

// NoteActionsDeps bundles the adapters every note-edit handler shares.
// NoteStore may be nil — the kompendium handlers return 404 in that
// case so the operator sees a deterministic "notebook not configured"
// shape rather than a 500. Documents is the server-side document store
// adapter (R1).
type NoteActionsDeps struct {
	NoteStore kompports.NoteStore
	Documents ports.DocumentStore
	Clock     ports.Clock

	// Bus broadcasts note.* / repo_note.* events to the SSE stream.
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

// — kompendium note edit form (GET /notes/{*}/edit) -------------------------

// NewNoteEdit returns the handler for GET /notes/{*}/edit. The
// wildcard captures everything after /notes/ including the trailing
// /edit segment; the handler strips the suffix and resolves the rest
// as a kompendium ID.
//
// When NoteStore is nil (notebook not configured) the handler returns
// 404 with the standard "not found" body so the gap is visible.
func NewNoteEdit(d NoteActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		if d.NoteStore == nil {
			renderNotesNotFound(w, r, "")
			return
		}

		idStr := noteIDFromEditPath(r)
		if idStr == "" {
			renderNotesNotFound(w, r, "")
			return
		}
		id, err := kompdomain.ParseID(idStr)
		if err != nil {
			renderNotesNotFound(w, r, idStr)
			return
		}
		note, err := d.NoteStore.Get(r.Context(), id)
		if errors.Is(err, kompports.ErrNoteNotFound) {
			renderNotesNotFound(w, r, idStr)
			return
		}
		if err != nil {
			slog.Error(
				"note edit: store.Get failed",
				slog.String("user_id", u.ID),
				slog.String("id", idStr),
				slog.String("err", err.Error()),
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		vm := notestmpl.EditVM{
			ID:      note.ID.String(),
			Title:   notestmpl.TitleOf(note.Meta.Title, note.Body, note.ID.String()),
			Content: string(note.Body),
		}
		meta := layout.PageMeta{
			Title:       "Notes · " + vm.Title + " · bearbeiten",
			CurrentPath: "/notes",
			UserLabel:   userLabelFromContext(r.Context()),
			Spine:       layout.SpineState{SyncState: "ok"},
		}
		if err := layout.Base(meta, notestmpl.Edit(vm)).Render(r.Context(), w); err != nil {
			slog.Error(
				"note edit: render failed",
				slog.String("id", idStr),
				slog.String("err", err.Error()),
			)
		}
	})
}

// — kompendium note PUT (PUT /notes/{*}) ------------------------------------

// NewNotePut returns the handler for PUT /notes/{*}. Reads the
// form-encoded `content` field, writes through NoteStore.Put preserving
// the existing frontmatter, then 303s back to the view page so the
// user lands on a fresh GET (avoids resubmit-on-refresh on the PUT URL).
//
// Last-write-wins for M7 — kompendium notes are file-backed and have
// no version field. Phase 2 will add OCC once kompendium gains server
// sync; the TODO below tracks the surface.
func NewNotePut(d NoteActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if d.NoteStore == nil {
			http.NotFound(w, r)
			return
		}

		idStr := noteIDFromPath(r)
		if idStr == "" {
			http.NotFound(w, r)
			return
		}
		id, err := kompdomain.ParseID(idStr)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		content := r.FormValue("content")

		// Load the existing note so we preserve frontmatter + id.
		existing, err := d.NoteStore.Get(r.Context(), id)
		if errors.Is(err, kompports.ErrNoteNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.Error(
				"note put: store.Get failed",
				slog.String("user_id", u.ID),
				slog.String("id", idStr),
				slog.String("err", err.Error()),
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// TODO Phase 2 — once kompendium notes carry a version field,
		// fold an expectedVersion read into the Put call so concurrent
		// edits from a second device surface as 409 instead of
		// last-write-wins. M7 ships file-backed notes; the WebUI is
		// single-writer in practice.
		existing.Body = []byte(content)
		if err := d.NoteStore.Put(r.Context(), existing); err != nil {
			slog.Error(
				"note put: store.Put failed",
				slog.String("user_id", u.ID),
				slog.String("id", idStr),
				slog.String("err", err.Error()),
			)
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}

		d.publish(u.ID, "note.updated", map[string]any{"id": id.String()})

		http.Redirect(w, r, "/notes/"+id.String(), http.StatusSeeOther)
	})
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

// noteIDFromPath extracts the kompendium ID from a chi wildcard route
// captured under "*". Falls back to stripping the "/notes/" prefix
// when no chi context exists (direct ServeHTTP in tests).
func noteIDFromPath(r *http.Request) string {
	if raw := chi.URLParam(r, "*"); raw != "" {
		if dec, err := url.PathUnescape(raw); err == nil {
			return dec
		}
		return raw
	}
	return strings.TrimPrefix(r.URL.Path, "/notes/")
}

// noteIDFromEditPath does the same as noteIDFromPath but additionally
// strips the trailing "/edit" segment so the kompendium-ID parse sees
// just the note ID. Delegates the actual suffix strip + dispatch-guard
// to stripEditSuffix so the rule stays unit-testable on a plain string.
func noteIDFromEditPath(r *http.Request) string {
	return stripEditSuffix(noteIDFromPath(r))
}

// stripEditSuffix removes the trailing "/edit" segment from a kompendium
// path captured under the chi "*" wildcard.
//
// Reject IDs that would collide with the dispatch convention: a
// kompendium note literally named ".../edit" would be hijacked by the
// GET dispatcher (notesGetDispatch routes "*/edit" → NoteEdit). The
// single-user namespace makes the clash rare in practice, but a
// defensive guard is cheap. Callers treat the empty return as "not
// found".
func stripEditSuffix(raw string) string {
	if !strings.HasSuffix(raw, "/edit") {
		return ""
	}
	stripped := strings.TrimSuffix(raw, "/edit")
	if stripped == "edit" || strings.HasSuffix(stripped, "/edit") {
		return ""
	}
	return stripped
}

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

// newRepoNoteID generates a fresh primary key for a RepoNote insert.
// We use the same google/uuid surface other adapters use; isolated in
// a helper so a future ID strategy change (e.g. ULID) only touches one
// site.
func newRepoNoteID() string {
	return uuid.NewString()
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
		Spine:       layout.SpineState{SyncState: "ok"},
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
