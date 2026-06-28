package httpserver

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/usecase"
)

func (s *Server) handleGetContext(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()
	in := usecase.ContextResolveInput{
		RemoteSlug:   strings.TrimSpace(q.Get("remote")),
		MachineID:    strings.TrimSpace(q.Get("machine")),
		Cwd:          strings.TrimSpace(q.Get("path")),
		NodeOverride: strings.TrimSpace(q.Get("node")),
	}
	budget := s.ContextBudget
	if budget <= 0 {
		budget = 6000
	}
	if v := strings.TrimSpace(q.Get("cap")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			budget = n
		}
	}
	cc, err := s.ComposeContext.Execute(r.Context(), u.ID, in, budget)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cc)
}
