// Package handlers — see dashboard.go for the per-handler-Deps
// convention. The notes handler is mounted at /notes and /notes/{id};
// both branches are served by NewNotes() and dispatched on the URL
// path inside the returned http.Handler.
package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	kompusecase "github.com/serverkraken/flow/internal/kompendium/usecase"
	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/markdown"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	notestmpl "github.com/serverkraken/flow/internal/webui/templates/notes"
)

// NotesDeps bundles exactly the data sources the /notes handler needs.
// Follows the per-handler-Deps convention established by DashboardDeps
// — see its doc comment for the rationale.
//
// Store + Lister are pointer-/interface-types and may be nil when the
// operator hasn't set FLOW_NOTEBOOK_ROOT. The handler MUST treat that
// shape as "render the not-configured placeholder" rather than panic.
//
// Markdown is the HTML renderer; it has no notebook dependency, so the
// handler can always build it.
type NotesDeps struct {
	Store    ports.NoteStore
	Lister   *kompusecase.ListNotes
	Markdown *markdown.Renderer
	Clock    flowports.Clock
}

// NewNotes returns the http.Handler mounted at /notes and /notes/{id}.
// The BrowserAuthMiddleware guarantees a domain.User in context; the
// handler fails closed with 401 if it's absent.
//
// Dispatch: an empty path tail (after the /notes prefix) renders the
// index; a non-empty tail is parsed as a note ID and routed to the
// single-note view. The route registration in flow-server (Task 10)
// uses one ServeMux pattern "/notes" plus a separate "/notes/" so the
// trailing-slash split is well-defined.
func NewNotes(d NotesDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		tail := strings.TrimPrefix(r.URL.Path, "/notes")
		tail = strings.TrimPrefix(tail, "/")
		if tail == "" {
			renderNotesIndex(w, r, d)
			return
		}
		renderNotesView(w, r, d, u.ID, tail)
	})
}

// — index —

func renderNotesIndex(w http.ResponseWriter, r *http.Request, d NotesDeps) {
	tab := notestmpl.ParseSubTab(r.URL.Query().Get("type"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	vm := notestmpl.IndexVM{
		ActiveTab:  tab,
		Query:      query,
		Configured: d.Store != nil && d.Lister != nil,
	}

	if vm.Configured {
		entries, err := d.Lister.Execute(r.Context(), kompusecase.ListNotesInput{
			Type: tab.AsNoteType(),
		})
		if err != nil {
			slog.Error("notes: list failed",
				slog.String("tab", string(tab)),
				slog.String("error", err.Error()),
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// rows are the rendered list; filteredCount is the count AFTER
		// type-sub-tab + search-query filtering (NOT the notebook total).
		rows, filteredCount := buildNotesIndexRows(r.Context(), d, entries, query, d.Clock)
		vm.Rows = rows
		vm.TotalLabel = formatNotesTotal(filteredCount)
		vm.EmptyReason = pickEmptyReason(query, filteredCount, len(entries))
	} else {
		vm.TotalLabel = "0 Notes"
	}

	meta := layout.PageMeta{
		Title:       notesTitle(tab),
		CurrentPath: "/notes",
		UserLabel:   userLabelFromContext(r.Context()),
		Spine:       layout.SpineState{SyncState: "ok"},
	}
	if err := layout.Base(meta, notestmpl.Index(vm)).Render(r.Context(), w); err != nil {
		slog.Error("notes: render index failed", slog.String("error", err.Error()))
	}
}

func notesTitle(tab notestmpl.SubTab) string {
	switch tab {
	case notestmpl.TabDaily:
		return "Notes · Daily"
	case notestmpl.TabProject:
		return "Notes · Project"
	case notestmpl.TabFree:
		return "Notes · Frei"
	default:
		return "Notes"
	}
}

// formatNotesTotal returns the count pill label ("0 Notes" / "1 Note"
// / "12 Notes"). German uses "Note" for both singular and plural — the
// label stays compact.
func formatNotesTotal(n int) string {
	if n == 1 {
		return "1 Note"
	}
	return strconv.Itoa(n) + " Notes"
}

// pickEmptyReason maps (query, count, totalCount) → empty-state branch:
//   - "" when there are rows to render
//   - "search" when a query is present and filtered everything out
//   - "tab" when no query but the tab has no rows
//   - "empty" when the underlying notebook is empty
func pickEmptyReason(query string, count, totalEntries int) string {
	if count > 0 {
		return ""
	}
	if query != "" {
		return "search"
	}
	if totalEntries == 0 {
		return "empty"
	}
	return "tab"
}

// — single note view —

func renderNotesView(w http.ResponseWriter, r *http.Request, d NotesDeps, userID, idStr string) {
	if d.Store == nil {
		// No notebook configured → cannot resolve any ID. Treat as
		// 404 with a hint so the operator sees a deterministic shape.
		renderNotesNotFound(w, r, idStr)
		return
	}
	id, err := domain.ParseID(idStr)
	if err != nil {
		renderNotesNotFound(w, r, idStr)
		return
	}
	note, err := d.Store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ports.ErrNoteNotFound) {
			renderNotesNotFound(w, r, idStr)
			return
		}
		slog.Error("notes: store.Get failed",
			slog.String("user_id", userID),
			slog.String("id", idStr),
			slog.String("error", err.Error()),
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	vm, err := buildNotesViewVM(d, note)
	if err != nil {
		slog.Error("notes: build view-model failed",
			slog.String("id", idStr),
			slog.String("error", err.Error()),
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	meta := layout.PageMeta{
		Title:       "Notes · " + vm.Title,
		CurrentPath: "/notes",
		UserLabel:   userLabelFromContext(r.Context()),
		Spine:       layout.SpineState{SyncState: "ok"},
	}
	if err := layout.Base(meta, notestmpl.View(vm)).Render(r.Context(), w); err != nil {
		slog.Error("notes: render view failed",
			slog.String("id", idStr),
			slog.String("error", err.Error()),
		)
	}
}

func renderNotesNotFound(w http.ResponseWriter, r *http.Request, idStr string) {
	w.WriteHeader(http.StatusNotFound)
	meta := layout.PageMeta{
		Title:       "Notes · nicht gefunden",
		CurrentPath: "/notes",
		UserLabel:   userLabelFromContext(r.Context()),
		Spine:       layout.SpineState{SyncState: "ok"},
	}
	if err := layout.Base(meta, notestmpl.ViewNotFound(idStr)).Render(r.Context(), w); err != nil {
		slog.Error("notes: render 404 failed", slog.String("error", err.Error()))
	}
}

// userLabelFromContext returns the nav header label or empty.
// Notes handler doesn't carry domain.User directly into every render
// call, so the helper looks it up from context.
func userLabelFromContext(ctx context.Context) string {
	u, ok := httpserver.UserFromContext(ctx)
	if !ok {
		return ""
	}
	return userLabel(u)
}

