// Package handlers implements the WebUI HTTP handlers.
package handlers

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/format"
	"github.com/serverkraken/flow/internal/webui/markdown"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	repostmpl "github.com/serverkraken/flow/internal/webui/templates/repos"
)

// ReposDeps bundles the documents-backed /repos surface (R1: repo notes
// ARE documents with repo_key set — the repos table is gone).
type ReposDeps struct {
	Documents flowports.DocumentStore
	Markdown  *markdown.Renderer
	Clock     flowports.Clock
}

// indexLimit caps the number of repos fetched for the /repos list.
const indexLimit = 200

// devicesPlaceholder is the static "Geräte" cell on the note view's
// meta strip — the server doesn't track per-device sync yet.
const devicesPlaceholder = "1 / 1 ✓"

// NewReposIndex returns the http.Handler mounted at /repos. The
// BrowserAuthMiddleware guarantees a domain.User in context; the
// handler fails closed with 401 if it's absent.
func NewReposIndex(d ReposDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		renderReposIndex(w, r, d, u.ID)
	})
}

// NewRepoNote returns the http.Handler mounted at /repos/{key}/note.
// chi exposes the {key} segment via URLParam; the handler URL-decodes
// it before looking up the Repo.
func NewRepoNote(d ReposDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		var canonicalKey string
		if escapedKey := chi.URLParam(r, "key"); escapedKey != "" {
			dec, err := url.PathUnescape(escapedKey)
			if err != nil {
				renderReposNoteNotFound(w, r, escapedKey)
				return
			}
			canonicalKey = dec
		} else {
			tail := strings.TrimPrefix(r.URL.Path, "/repos")
			tail = strings.TrimPrefix(tail, "/")
			key, action, ok := splitNoteTail(tail)
			if !ok || action != "note" {
				renderReposNoteNotFound(w, r, key)
				return
			}
			canonicalKey = key
		}
		renderReposNoteView(w, r, d, u.ID, canonicalKey)
	})
}

// NewRepos is a back-compat dispatch wrapper for tests + standalone
// callers.
func NewRepos(d ReposDeps) http.Handler {
	index := NewReposIndex(d)
	note := NewRepoNote(d)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tail := strings.TrimPrefix(r.URL.Path, "/repos")
		tail = strings.TrimPrefix(tail, "/")
		if tail == "" {
			index.ServeHTTP(w, r)
			return
		}
		note.ServeHTTP(w, r)
	})
}

func renderReposIndex(w http.ResponseWriter, r *http.Request, d ReposDeps, userID string) {
	entries, err := d.Documents.List(userID, "repos/", "", indexLimit)
	if err != nil {
		slog.Error(
			"repos: List failed",
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	if d.Clock != nil {
		now = d.Clock.Now()
	}

	var rows []repostmpl.IndexRow
	for _, e := range entries {
		key := repoKeyOfEntry(e)
		rows = append(rows, repostmpl.IndexRow{
			DisplayName: repoDisplayName(key),
			Subtitle:    key + " · note ✓",
			HasNote:     true,
			MetaLeft:    fmt.Sprintf("version %d", e.Version),
			MetaRight:   format.HumanRelativeTime(e.UpdatedAt, now),
			Href:        repostmpl.NoteHref(key),
		})
	}

	vm := repostmpl.IndexVM{
		HasRepos:   len(entries) > 0,
		Rows:       rows,
		TotalLabel: repostmpl.FormatRepoTotal(len(entries), len(entries)),
	}

	meta := layout.PageMeta{
		Title:       "Repos",
		CurrentPath: "/repos",
		UserLabel:   userLabelFromContext(r.Context()),
		Spine:       layout.SpineState{},
	}
	if err := layout.Base(meta, repostmpl.Index(vm)).Render(r.Context(), w); err != nil {
		slog.Error("repos: render index failed", slog.String("error", err.Error()))
	}
}

func renderReposNoteView(w http.ResponseWriter, r *http.Request, d ReposDeps, userID, canonicalKey string) {
	doc, err := d.Documents.GetByRepoKey(userID, canonicalKey)
	var hasNote bool
	var html template.HTML
	modifiedLabel := "—"
	shortHash := "—"
	now := time.Now()
	if d.Clock != nil {
		now = d.Clock.Now()
	}

	if err == nil {
		hasNote = true
		modifiedLabel = format.HumanRelativeTime(doc.UpdatedAt, now)
		if len(doc.ID) >= 7 {
			shortHash = doc.ID[:7]
		} else {
			shortHash = doc.ID
		}
		if d.Markdown != nil && doc.Body != "" {
			html, _ = d.Markdown.Render([]byte(doc.Body))
		}
	} else if errors.Is(err, flowports.ErrDocumentNotFound) {
		// render the "noch keine Note" branch — vm.HasNote stays false.
	} else {
		slog.Error(
			"repos: GetByRepoKey failed",
			slog.String("user_id", userID),
			slog.String("canonical_key", canonicalKey),
			slog.String("error", err.Error()),
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	vm := repostmpl.NoteVM{
		DisplayName:   repoDisplayName(canonicalKey),
		CanonicalKey:  canonicalKey,
		RemoteURL:     "(kein remote — R1: notes only)",
		ShortHash:     shortHash,
		DevicesLabel:  devicesPlaceholder,
		ModifiedLabel: modifiedLabel,
		HasNote:       hasNote,
		HTML:          html,
	}

	meta := layout.PageMeta{
		Title:       "Repos · " + vm.DisplayName,
		CurrentPath: "/repos",
		UserLabel:   userLabelFromContext(r.Context()),
		Spine:       layout.SpineState{},
	}
	if err := layout.Base(meta, repostmpl.View(vm)).Render(r.Context(), w); err != nil {
		slog.Error(
			"repos: render view failed",
			slog.String("canonical_key", canonicalKey),
			slog.String("error", err.Error()),
		)
	}
}

func renderReposNoteNotFound(w http.ResponseWriter, r *http.Request, canonicalKey string) {
	w.WriteHeader(http.StatusNotFound)
	meta := layout.PageMeta{
		Title:       "Repos · nicht gefunden",
		CurrentPath: "/repos",
		UserLabel:   userLabelFromContext(r.Context()),
		Spine:       layout.SpineState{},
	}
	if err := layout.Base(meta, repostmpl.ViewNotFound(canonicalKey)).Render(r.Context(), w); err != nil {
		slog.Error("repos: render 404 failed", slog.String("error", err.Error()))
	}
}

// repoKeyOfEntry prefers the stored repo_key and falls back to decoding
// the path convention repos/<urlescape(key)>.md.
func repoKeyOfEntry(e flowports.DocumentEntry) string {
	if e.RepoKey != "" {
		return e.RepoKey
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(e.Path, "repos/"), ".md")
	if key, err := url.PathUnescape(raw); err == nil {
		return key
	}
	return raw
}

// repoDisplayName shortens "git:github.com/foo/bar" to "foo/bar".
func repoDisplayName(key string) string {
	k := strings.TrimPrefix(key, "git:")
	if i := strings.Index(k, "/"); i > 0 && strings.Contains(k[:i], ".") {
		k = k[i+1:] // Host-Anteil (enthält einen Punkt) abwerfen
	}
	return k
}

// splitNoteTail parses "{escaped-key}/{action}" and returns the two
// segments. Any other shape returns ok=false so the caller 404s.
func splitNoteTail(tail string) (key, action string, ok bool) {
	idx := strings.LastIndex(tail, "/")
	if idx <= 0 || idx == len(tail)-1 {
		return tail, "", false
	}
	return tail[:idx], tail[idx+1:], true
}
