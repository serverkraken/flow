package httpserver

import (
	"net/http"
	"strconv"

	"github.com/serverkraken/flow/internal/domain"
)

const maxActivityPageSize = 200

func (s *Server) handleListActivity(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()

	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxActivityPageSize {
			http.Error(w, "bad limit (1..200)", http.StatusBadRequest)
			return
		}
		limit = n
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			http.Error(w, "bad offset", http.StatusBadRequest)
			return
		}
		offset = n
	}

	classes := q["class"]

	var actorRef *string
	if a := q.Get("actor"); a != "" {
		actorRef = &a
	}

	list, _, err := s.ListActivity.Execute(r.Context(), u.ID, classes, actorRef, limit, offset)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []domain.ActivityEntry{}
	}
	writeJSON(w, http.StatusOK, list)
}
