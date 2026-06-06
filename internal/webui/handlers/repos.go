// Package handlers — see dashboard.go for the per-handler-Deps
// convention. The repos routes split into two constructors:
//   - NewReposIndex serves /repos (list)
//   - NewRepoNote   serves /repos/{key}/note (single-repo note view)
//
// chi captures the canonical key via {key}; the handler URL-decodes it
// before looking up the Repo. NewRepos is kept as a thin path-dispatch
// wrapper for tests + back-compat.
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/markdown"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	repostmpl "github.com/serverkraken/flow/internal/webui/templates/repos"
)

// ReposDeps bundles exactly the data sources the /repos handler needs.
// Follows the per-handler-Deps convention established by DashboardDeps
// — see its doc comment for the rationale.
//
// Repos + RepoNotes are concrete sqliteserver adapters (their server
// Upsert signatures carry expectedVersion and so don't satisfy the
// client-side ports). Markdown is the HTML renderer reused from
// Task 7. Clock is exposed so tests can pin "now" for relative-time
// rendering on the meta strip.
//
// Phase 2: re-add Devices when we have per-device sync telemetry.
type ReposDeps struct {
	Repos     *sqliteserver.Repos
	RepoNotes *sqliteserver.RepoNotes
	Markdown  *markdown.Renderer
	Clock     flowports.Clock
}

// indexLimit caps the number of repos fetched for the /repos list.
// Server-enforced so a long-tail of archived repos can't blow the
// page render budget. Phase 2: paginate; M6 is single-screen.
const indexLimit = 200

// devicesPlaceholder is the static "Geräte" cell on the note view's
// meta strip — the server doesn't track per-device sync yet (M7+).
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
//
// Falls back to the M6 path-suffix dispatch when no chi route context is
// present (older tests, direct ServeHTTP without a router).
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
			// chi stores raw percent-encoded values; decode here.
			dec, err := url.PathUnescape(escapedKey)
			if err != nil {
				renderReposNoteNotFound(w, r, escapedKey)
				return
			}
			canonicalKey = dec
		} else {
			// Direct ServeHTTP without router — r.URL.Path is already decoded by net/http.
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
// callers. Production code uses the chi-mounted NewReposIndex +
// NewRepoNote pair.
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

// splitNoteTail parses "{escaped-key}/{action}" and returns the two
// segments. Any other shape returns ok=false so the caller 404s.
func splitNoteTail(tail string) (key, action string, ok bool) {
	// We allow the canonical key to contain percent-encoded slashes
	// (url.PathEscape encodes `/` as `%2F`), so we expect exactly one
	// raw `/` separating key and action.
	idx := strings.LastIndex(tail, "/")
	if idx <= 0 || idx == len(tail)-1 {
		return tail, "", false
	}
	return tail[:idx], tail[idx+1:], true
}

// — index —

func renderReposIndex(w http.ResponseWriter, r *http.Request, d ReposDeps, userID string) {
	repos, _, _, err := d.Repos.PullSince(userID, 0, indexLimit)
	if err != nil {
		slog.Error("repos: PullSince failed",
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	vm := buildReposIndexVM(d, userID, repos)

	meta := layout.PageMeta{
		Title:       "Repos",
		CurrentPath: "/repos",
		UserLabel:   userLabelFromContext(r.Context()),
		Spine:       layout.SpineState{SyncState: "ok"},
	}
	if err := layout.Base(meta, repostmpl.Index(vm)).Render(r.Context(), w); err != nil {
		slog.Error("repos: render index failed", slog.String("error", err.Error()))
	}
}

// — single note view —

func renderReposNoteView(w http.ResponseWriter, r *http.Request, d ReposDeps, userID, canonicalKey string) {
	repo, err := d.Repos.GetByCanonicalKey(userID, canonicalKey)
	if errors.Is(err, flowports.ErrRepoNotFound) {
		renderReposNoteNotFound(w, r, canonicalKey)
		return
	}
	if err != nil {
		slog.Error("repos: GetByCanonicalKey failed",
			slog.String("user_id", userID),
			slog.String("canonical_key", canonicalKey),
			slog.String("error", err.Error()),
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	note, noteErr := d.RepoNotes.GetByRepo(userID, repo.ID)
	switch {
	case noteErr == nil:
		// happy path
	case errors.Is(noteErr, flowports.ErrRepoNoteNotFound):
		// render the "noch keine Note" branch — vm.HasNote stays false.
	default:
		slog.Error("repos: GetByRepo failed",
			slog.String("user_id", userID),
			slog.String("repo_id", repo.ID),
			slog.String("error", noteErr.Error()),
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	vm := buildReposNoteVM(d, repo, note, noteErr == nil)

	meta := layout.PageMeta{
		Title:       "Repos · " + vm.DisplayName,
		CurrentPath: "/repos",
		UserLabel:   userLabelFromContext(r.Context()),
		Spine:       layout.SpineState{SyncState: "ok"},
	}
	if err := layout.Base(meta, repostmpl.View(vm)).Render(r.Context(), w); err != nil {
		slog.Error("repos: render view failed",
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
		Spine:       layout.SpineState{SyncState: "ok"},
	}
	if err := layout.Base(meta, repostmpl.ViewNotFound(canonicalKey)).Render(r.Context(), w); err != nil {
		slog.Error("repos: render 404 failed", slog.String("error", err.Error()))
	}
}
