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

// ProjectsServer is the server-side store contract the projects handlers
// depend on. Satisfied by *sqliteserver.Projects (Task 21).
//
// The server Upsert signature differs from ports.ProjectStore.Upsert
// (which takes no expectedVersion), so this interface is kept local to avoid
// polluting ports with server-only signatures.
type ProjectsServer interface {
	PullSince(userID string, since int64, limit int) ([]domain.Project, int64, bool, error)
	Upsert(in domain.Project, expectedVersion int64) (domain.Project, error)
	GetByID(userID, id string) (domain.Project, error)
}

// NewProjectsPullHandler returns a handler for GET /api/v1/projects.
//
// Query params:
//   - since  (int64, default 0): return only items with version > since
//   - limit  (int, default 200, max 500): maximum items per response
//
// Response: {"items": [...], "high_watermark": N, "has_more": bool}
func NewProjectsPullHandler(store ProjectsServer) http.Handler {
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

// NewProjectsPushHandler returns a handler for PUT /api/v1/projects/{id}.
//
// The request body must be a JSON-encoded domain.Project. The If-Match header
// must contain the client's expected version (0 for new inserts).
//
// On success returns {"id": "...", "version": N}.
// On optimistic-concurrency conflict returns 409 with {"current": <Project>}.
func NewProjectsPushHandler(store ProjectsServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		id := chi.URLParam(r, "id")
		expected, _ := strconv.ParseInt(r.Header.Get("If-Match"), 10, 64)
		var in domain.Project
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		in.ID = id
		in.UserID = user.ID
		out, err := store.Upsert(in, expected)
		if errors.Is(err, ports.ErrProjectVersionConflict) {
			cur, _ := store.GetByID(user.ID, id)
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
