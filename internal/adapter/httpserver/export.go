package httpserver

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// handleExport streams a worktime export in the requested format. authAny so a
// browser <a download> (cookie) and the CLI/TUI (bearer) both work.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	from, to, ok := parseRange(r)
	if !ok {
		http.Error(w, "from/to required (yyyy-mm-dd)", http.StatusBadRequest)
		return
	}
	if to.Before(from) {
		http.Error(w, "to must be >= from", http.StatusBadRequest)
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	data, err := s.BuildExport.Execute(r.Context(), u.ID, from, to, r.URL.Query().Get("project"))
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	fname := fmt.Sprintf("flow-export-%s_%s", from.Format(dayFmt), to.Format(dayFmt))
	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`.csv"`)
		_ = domain.WriteCSV(w, data)
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`.json"`)
		_ = domain.WriteJSON(w, data)
	case "md":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`.md"`)
		_ = domain.WriteMarkdown(w, data)
	default:
		http.Error(w, "format must be csv, json or md", http.StatusBadRequest)
	}
}

type setRateReq struct {
	Amount   *int64 `json:"amount"`
	Currency string `json:"currency"`
}

func (s *Server) handleSetNodeRate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req setRateReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, false) {
		return
	}
	var rate *domain.Money
	if req.Amount != nil {
		rate = &domain.Money{Amount: *req.Amount, Currency: req.Currency}
	}
	err := s.SetNodeRate.Execute(r.Context(), u.ID, r.PathValue("id"), rate)
	switch {
	case errors.Is(err, domain.ErrInvalidRate):
		http.Error(w, "invalid rate", http.StatusBadRequest)
	case errors.Is(err, domain.ErrInvalidNode):
		http.Error(w, "only an engagement may carry a rate", http.StatusBadRequest)
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
