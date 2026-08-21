package httpserver

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// exportPageData builds the ExportPageData view model for the given user and
// [from, to] range. projectID "" means all projects.
func (s *Server) exportPageData(ctx context.Context, u domain.User, from, to time.Time) (webui.ExportPageData, error) {
	data, err := s.BuildExport.Execute(ctx, u.ID, from, to, "")
	if err != nil {
		return webui.ExportPageData{}, err
	}

	rows := make([]webui.ExportSummaryRow, 0, len(data.ByEngagement))
	var grandTotal time.Duration
	amountByCcy := map[string]int64{}

	for _, pt := range data.ByEngagement {
		amt := "–"
		if pt.Amount != nil {
			amt = pt.Amount.String()
			amountByCcy[pt.Amount.Currency] += pt.Amount.Amount
		}
		rows = append(rows, webui.ExportSummaryRow{
			Project: pt.NodeName,
			Time:    domain.FmtDur(pt.Total),
			Amount:  amt,
		})
		grandTotal += pt.Total
	}

	// Build total amount string: one formatted Money per currency (sorted for
	// deterministic output), joined — matching the per-currency grand total in
	// the Markdown export.
	totalAmt := "–"
	if len(amountByCcy) > 0 {
		ccys := make([]string, 0, len(amountByCcy))
		for c := range amountByCcy {
			ccys = append(ccys, c)
		}
		sort.Strings(ccys)
		parts := make([]string, 0, len(ccys))
		for _, c := range ccys {
			parts = append(parts, domain.Money{Amount: amountByCcy[c], Currency: c}.String())
		}
		totalAmt = strings.Join(parts, " · ")
	}

	return webui.ExportPageData{
		From:        from.Format(dayFmt),
		To:          to.Format(dayFmt),
		Rows:        rows,
		TotalTime:   domain.FmtDur(grandTotal),
		TotalAmount: totalAmt,
	}, nil
}

// exportDefaultRange returns the first and last day of the current month.
func (s *Server) exportDefaultRange() (time.Time, time.Time) {
	now := s.Clock.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return from, to
}

func (s *Server) handleWebExportHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	from, to, ok := parseRange(r)
	if !ok {
		from, to = s.exportDefaultRange()
	}
	d, err := s.exportPageData(r.Context(), u, from, to)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.ExportPage(d).Render(r.Context(), w)
}

func (s *Server) handleWebExportPreview(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	from, to, ok := parseRange(r)
	if !ok {
		from, to = s.exportDefaultRange()
	}
	d, err := s.exportPageData(r.Context(), u, from, to)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.ExportFragment(d).Render(r.Context(), w)
}
