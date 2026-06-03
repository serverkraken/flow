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

// ActiveServer is the server-side store contract the active_sessions handlers
// depend on. Satisfied by *sqliteserver.ActiveSessions (Task 22).
//
// Kept local to avoid polluting ports.ActiveSessionStore with server-only
// signatures (Start/Stop with OCC params).
type ActiveServer interface {
	Start(userID, projectID, device string, expectedVersion int64) (domain.ActiveSession, error)
	Stop(userID, projectID string, expectedVersion int64, tag, note string) (domain.Session, error)
	Get(userID, projectID string) (domain.ActiveSession, error)
	ListByUser(userID string) ([]domain.ActiveSession, error)
	PullSince(userID string, since int64) ([]domain.ActiveSession, int64, error)
}

// NewActiveListHandler returns a handler for GET /api/v1/active.
//
// When ?since=N is provided, delegates to PullSince and returns
// {"items": [...], "high_watermark": N}. Otherwise returns {"items": [...]}.
func NewActiveListHandler(store ActiveServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		sinceStr := r.URL.Query().Get("since")

		w.Header().Set("Content-Type", "application/json")

		if sinceStr != "" {
			since, _ := strconv.ParseInt(sinceStr, 10, 64)
			items, hw, err := store.PullSince(user.ID, since)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": items, "high_watermark": hw,
			})
			return
		}

		items, err := store.ListByUser(user.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
}

// NewActiveStartHandler returns a handler for POST /api/v1/active/{project_id}/start.
//
// Body (optional JSON): {"started_on_device": "..."}
// If-Match header: 0 (must not exist) or version (force-takeover).
func NewActiveStartHandler(store ActiveServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		projectID := chi.URLParam(r, "project_id")
		expected, _ := strconv.ParseInt(r.Header.Get("If-Match"), 10, 64)

		var body struct {
			StartedOnDevice string `json:"started_on_device"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		a, err := store.Start(user.ID, projectID, body.StartedOnDevice, expected)
		if errors.Is(err, ports.ErrActiveSessionConflict) {
			cur, _ := store.Get(user.ID, projectID)
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
		_ = json.NewEncoder(w).Encode(a)
	})
}

// NewActiveStopHandler returns a handler for DELETE /api/v1/active/{project_id}.
//
// Body (optional JSON): {"tag": "...", "note": "..."}
// If-Match header: version of the active_sessions row.
func NewActiveStopHandler(store ActiveServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		projectID := chi.URLParam(r, "project_id")
		expected, _ := strconv.ParseInt(r.Header.Get("If-Match"), 10, 64)
		var body struct {
			Tag  string `json:"tag"`
			Note string `json:"note"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		sess, err := store.Stop(user.ID, projectID, expected, body.Tag, body.Note)
		if errors.Is(err, ports.ErrActiveSessionNotFound) {
			http.Error(w, "no active session", http.StatusNotFound)
			return
		}
		if errors.Is(err, ports.ErrActiveSessionConflict) {
			cur, _ := store.Get(user.ID, projectID)
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
		_ = json.NewEncoder(w).Encode(sess)
	})
}
