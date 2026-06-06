package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// RepoNotesServer is the server-side store contract the repo-notes handlers
// depend on. Satisfied by *sqliteserver.RepoNotes (Plan C / Task 7).
type RepoNotesServer interface {
	PullSince(userID string, since int64, limit int) ([]domain.RepoNote, int64, bool, error)
	Upsert(in domain.RepoNote, expectedVersion int64) (domain.RepoNote, error)
	GetByRepo(userID, repoID string) (domain.RepoNote, error)
}

// NewRepoNotesPullHandler returns a handler for GET /api/v1/repo-notes.
func NewRepoNotesPullHandler(store RepoNotesServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		limit := 200
		if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 500 {
			limit = l
		}
		items, hi, hasMore, err := store.PullSince(user.ID, since, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": items, "high_watermark": hi, "has_more": hasMore,
		})
	})
}

// NewRepoNotePushHandler returns a handler for PUT /api/v1/repos/{repo_id}/note.
// Body: JSON-encoded domain.RepoNote. If-Match: expected version.
// 409 with {"current": <RepoNote>} on OCC conflict.
func NewRepoNotePushHandler(store RepoNotesServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		repoID := chi.URLParam(r, "repo_id")
		expected, _ := strconv.ParseInt(r.Header.Get("If-Match"), 10, 64)
		var in domain.RepoNote
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		in.RepoID = repoID
		in.UserID = user.ID
		out, err := store.Upsert(in, expected)
		if errors.Is(err, ports.ErrRepoNoteVersionConflict) {
			SyncConflicts.WithLabelValues("repo_notes").Inc()
			cur, _ := store.GetByRepo(user.ID, repoID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"current": cur})
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": out.ID, "version": out.Version,
		})
	})
}
