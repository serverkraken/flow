package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// fmtExportDur renders a duration as "Hh MMm" (e.g. "2h 05m"), matching the
// domain.fmtDur style used in Markdown export.
func fmtExportDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// exportPageData builds the ExportPageData view model for the given user and
// [from, to] range. projectID "" means all projects.
func (s *Server) exportPageData(ctx context.Context, u domain.User, from, to time.Time) (webui.ExportPageData, error) {
	data, err := s.BuildExport.Execute(ctx, u.ID, from, to, "")
	if err != nil {
		return webui.ExportPageData{}, err
	}

	rows := make([]webui.ExportSummaryRow, 0, len(data.ByProject))
	var grandTotal time.Duration
	amountByCcy := map[string]int64{}

	for _, pt := range data.ByProject {
		amt := "–"
		if pt.Amount != nil {
			amt = pt.Amount.String()
			amountByCcy[pt.Amount.Currency] += pt.Amount.Amount
		}
		rows = append(rows, webui.ExportSummaryRow{
			Project: pt.ProjectName,
			Time:    fmtExportDur(pt.Total),
			Amount:  amt,
		})
		grandTotal += pt.Total
	}

	// Build total amount string: one entry per currency, sorted alphabetically.
	totalAmt := "–"
	if len(amountByCcy) > 0 {
		// Collect and sort currencies for deterministic output.
		ccys := make([]string, 0, len(amountByCcy))
		for c := range amountByCcy {
			ccys = append(ccys, c)
		}
		// Simple sort for small slices: bubble sort to avoid importing sort package
		// only for this. Actually: use a join to keep it short.
		// Use a slice sort inline.
		for i := 0; i < len(ccys); i++ {
			for j := i + 1; j < len(ccys); j++ {
				if ccys[j] < ccys[i] {
					ccys[i], ccys[j] = ccys[j], ccys[i]
				}
			}
		}
		parts := make([]string, 0, len(ccys))
		for _, c := range ccys {
			m := domain.Money{Amount: amountByCcy[c], Currency: c}
			parts = append(parts, m.String())
		}
		totalAmt = ""
		for i, p := range parts {
			if i > 0 {
				totalAmt += " · "
			}
			totalAmt += p
		}
	}

	return webui.ExportPageData{
		User:        u.Username,
		From:        from.Format(dayFmt),
		To:          to.Format(dayFmt),
		Rows:        rows,
		TotalTime:   fmtExportDur(grandTotal),
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
		http.Error(w, "server error", http.StatusInternalServerError)
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
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.ExportFragment(d).Render(r.Context(), w)
}
