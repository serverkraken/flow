package domain

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ExportData is the rendered shape of a worktime export over [From, To]:
// a per-project aggregate plus the flat detail rows. Built by the export
// use case; serialised by the writers below.
type ExportData struct {
	From, To  time.Time
	ByProject []ProjectTotal
	Sessions  []ExportRow
}

// ProjectTotal is one project's aggregate. Amount = Rate.Mul(Total) when a
// rate is set, else nil.
type ProjectTotal struct {
	ProjectID    string
	ProjectName  string
	Total        time.Duration
	SessionCount int
	Rate         *Money
	Amount       *Money
}

// ExportRow is one booked session in the detail listing.
type ExportRow struct {
	Date        time.Time
	Start       time.Time
	Stop        time.Time
	Elapsed     time.Duration
	ProjectName string
	Tag         string
	Note        string
}

// mdCell escapes a value for use inside a Markdown table cell: pipes would end
// the cell and newlines would break the row.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// fmtDur renders a duration as "Hh MMm" (e.g. "2h 05m").
func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// WriteCSV emits the detail session rows with a header. Pivot-friendly; the
// per-project aggregate is derivable in a spreadsheet (and lives in JSON/MD).
func WriteCSV(w io.Writer, d ExportData) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "start", "stop", "duration_seconds", "project", "tag", "note"})
	for _, r := range d.Sessions {
		_ = cw.Write([]string{
			r.Date.Format("2006-01-02"),
			r.Start.Format("15:04"),
			r.Stop.Format("15:04"),
			strconv.FormatInt(int64(r.Elapsed/time.Second), 10),
			r.ProjectName, r.Tag, r.Note,
		})
	}
	cw.Flush()
	return cw.Error()
}

// WriteJSON emits a structured object with the per-project aggregate and the
// detail rows.
func WriteJSON(w io.Writer, d ExportData) error {
	type projOut struct {
		Project      string `json:"project"`
		TotalSeconds int64  `json:"totalSeconds"`
		SessionCount int    `json:"sessionCount"`
		RateAmount   *int64 `json:"rateAmount,omitempty"`
		RateCurrency string `json:"rateCurrency,omitempty"`
		AmountMinor  *int64 `json:"amountMinor,omitempty"`
	}
	type rowOut struct {
		Date            string `json:"date"`
		Start           string `json:"start"`
		Stop            string `json:"stop"`
		DurationSeconds int64  `json:"durationSeconds"`
		Project         string `json:"project"`
		Tag             string `json:"tag"`
		Note            string `json:"note"`
	}
	out := struct {
		From      string    `json:"from"`
		To        string    `json:"to"`
		ByProject []projOut `json:"byProject"`
		Sessions  []rowOut  `json:"sessions"`
	}{From: d.From.Format("2006-01-02"), To: d.To.Format("2006-01-02")}
	out.ByProject = make([]projOut, 0, len(d.ByProject))
	out.Sessions = make([]rowOut, 0, len(d.Sessions))
	for _, p := range d.ByProject {
		po := projOut{Project: p.ProjectName, TotalSeconds: int64(p.Total / time.Second), SessionCount: p.SessionCount}
		if p.Rate != nil {
			ra := p.Rate.Amount
			po.RateAmount = &ra
			po.RateCurrency = p.Rate.Currency
		}
		if p.Amount != nil {
			am := p.Amount.Amount
			po.AmountMinor = &am
		}
		out.ByProject = append(out.ByProject, po)
	}
	for _, r := range d.Sessions {
		out.Sessions = append(out.Sessions, rowOut{
			Date: r.Date.Format("2006-01-02"), Start: r.Start.Format("15:04"), Stop: r.Stop.Format("15:04"),
			DurationSeconds: int64(r.Elapsed / time.Second), Project: r.ProjectName, Tag: r.Tag, Note: r.Note,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteMarkdown emits a human-readable report: a per-project summary table
// (with Σh and amount), grand totals (hours always; amount per currency), and
// a detail session table.
func WriteMarkdown(w io.Writer, d ExportData) error {
	bw := &mdWriter{w: w}
	bw.printf("# Worktime %s – %s\n\n", d.From.Format("2006-01-02"), d.To.Format("2006-01-02"))
	bw.printf("## Projekte\n\n| Projekt | Zeit | Betrag |\n|---|---|---|\n")
	var grandTotal time.Duration
	amountByCcy := map[string]int64{}
	for _, p := range d.ByProject {
		amt := "–"
		if p.Amount != nil {
			amt = p.Amount.String()
			amountByCcy[p.Amount.Currency] += p.Amount.Amount
		}
		bw.printf("| %s | %s | %s |\n", mdCell(p.ProjectName), fmtDur(p.Total), amt)
		grandTotal += p.Total
	}
	bw.printf("\n**Summe:** %s", fmtDur(grandTotal))
	ccys := make([]string, 0, len(amountByCcy))
	for c := range amountByCcy {
		ccys = append(ccys, c)
	}
	sort.Strings(ccys)
	for _, c := range ccys {
		bw.printf(" · %s", Money{Amount: amountByCcy[c], Currency: c}.String())
	}
	bw.printf("\n\n## Sessions\n\n| Datum | Start | Stop | Dauer | Projekt | Tag | Notiz |\n|---|---|---|---|---|---|---|\n")
	for _, r := range d.Sessions {
		bw.printf("| %s | %s | %s | %s | %s | %s | %s |\n",
			r.Date.Format("2006-01-02"), r.Start.Format("15:04"), r.Stop.Format("15:04"),
			fmtDur(r.Elapsed), mdCell(r.ProjectName), mdCell(r.Tag), mdCell(r.Note))
	}
	return bw.err
}

// mdWriter swallows io errors until the end so the writer code stays flat.
// Named mdWriter (not errWriter) to leave the name available for future helpers.
type mdWriter struct {
	w   io.Writer
	err error
}

func (e *mdWriter) printf(format string, a ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, a...)
}
