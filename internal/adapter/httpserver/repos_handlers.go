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

// ReposServer is the server-side store contract the repos handlers depend
// on. Satisfied by *sqliteserver.Repos (Plan C / Task 6).
type ReposServer interface {
	PullSince(userID string, since int64, limit int) ([]domain.Repo, int64, bool, error)
	Upsert(in domain.Repo, expectedVersion int64) (domain.Repo, error)
	GetByID(userID, id string) (domain.Repo, error)
}

// NewReposPullHandler returns a handler for GET /api/v1/repos.
func NewReposPullHandler(store ReposServer) http.Handler {
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

// NewReposPushHandler returns a handler for PUT /api/v1/repos/{id}.
// Body: JSON-encoded domain.Repo. If-Match: expected version.
// 409 with {"current": <Repo>} on OCC conflict.
func NewReposPushHandler(store ReposServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		id := chi.URLParam(r, "id")
		expected, _ := strconv.ParseInt(r.Header.Get("If-Match"), 10, 64)
		var in domain.Repo
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		in.ID = id
		in.UserID = user.ID
		out, err := store.Upsert(in, expected)
		if errors.Is(err, ports.ErrRepoVersionConflict) {
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
