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

// SessionsServer is the server-side store contract the sessions handlers
// depend on. Satisfied by *sqliteserver.Sessions (Task 21).
//
// The server Upsert signature differs from ports.SessionStore.Upsert
// (which takes no expectedVersion), so this interface is kept local to avoid
// polluting ports with server-only signatures.
type SessionsServer interface {
	PullSince(userID string, since int64, limit int) ([]domain.Session, int64, bool, error)
	Upsert(in domain.Session, expectedVersion int64) (domain.Session, error)
	GetByID(userID, id string) (domain.Session, error)
}

// NewSessionsPullHandler returns a handler for GET /api/v1/sessions.
//
// Query params:
//   - since  (int64, default 0): return only items with version > since
//   - limit  (int, default 200, max 500): maximum items per response
//
// Response: {"items": [...], "high_watermark": N, "has_more": bool}
func NewSessionsPullHandler(store SessionsServer) http.Handler {
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

// NewSessionsPushHandler returns a handler for PUT /api/v1/sessions/{id}.
//
// The request body must be a JSON-encoded domain.Session. The If-Match header
// must contain the client's expected version (0 for new inserts).
//
// On success returns the upserted Session row.
// On optimistic-concurrency conflict returns 409 with {"current": <Session>}.
func NewSessionsPushHandler(store SessionsServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		id := chi.URLParam(r, "id")
		expected, _ := strconv.ParseInt(r.Header.Get("If-Match"), 10, 64)
		var in domain.Session
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		in.ID = id
		in.UserID = user.ID
		out, err := store.Upsert(in, expected)
		if errors.Is(err, ports.ErrSessionVersionConflict) {
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
		_ = json.NewEncoder(w).Encode(out)
	})
}
