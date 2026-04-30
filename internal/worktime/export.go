package worktime

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Range is a half-open date interval [From, To).
type Range struct {
	From time.Time // inclusive (00:00 of the day in local TZ)
	To   time.Time // exclusive (00:00 of the day in local TZ)
}

// ContainsDate reports whether d (a date-only value) falls inside r.
func (r Range) ContainsDate(d time.Time) bool {
	if !r.From.IsZero() && d.Before(r.From) {
		return false
	}
	if !r.To.IsZero() && !d.Before(r.To) {
		return false
	}
	return true
}

// LoadRange returns all sessions whose Date falls within r, oldest first.
// An empty Range (zero From and To) returns all sessions.
func LoadRange(r Range) ([]Session, error) {
	all, err := loadAllSessions()
	if err != nil {
		return nil, err
	}
	if r.From.IsZero() && r.To.IsZero() {
		return all, nil
	}
	out := make([]Session, 0, len(all))
	for _, s := range all {
		if r.ContainsDate(s.Date) {
			out = append(out, s)
		}
	}
	return out, nil
}

// ExportCSV writes a CSV with header row to w.
// Columns: date, start, stop, elapsed_seconds, tag, note.
func ExportCSV(w io.Writer, r Range) error {
	sessions, err := LoadRange(r)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"date", "start", "stop", "elapsed_seconds", "tag", "note"}); err != nil {
		return err
	}
	for _, s := range sessions {
		row := []string{
			s.Date.Format("2006-01-02"),
			s.Start.Format("15:04"),
			s.Stop.Format("15:04"),
			strconv.FormatInt(int64(s.Elapsed.Seconds()), 10),
			s.Tag,
			s.Note,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ExportJSON writes a JSON array of session objects.
func ExportJSON(w io.Writer, r Range) error {
	sessions, err := LoadRange(r)
	if err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, map[string]any{
			"date":            s.Date.Format("2006-01-02"),
			"start":           s.Start.Format("15:04"),
			"stop":            s.Stop.Format("15:04"),
			"elapsed_seconds": int64(s.Elapsed.Seconds()),
			"tag":             s.Tag,
			"note":            s.Note,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ParseRange resolves a friendly range expression:
//   - ""           → all
//   - "today"      → today only
//   - "week"       → ISO Mon..Sun of the current week
//   - "month"      → first..last of current month
//   - "YYYY-MM"    → that month
//   - "YYYY"       → that year
//   - "FROM..TO"   → both YYYY-MM-DD, inclusive
func ParseRange(now time.Time, expr string) (Range, error) {
	switch expr {
	case "":
		return Range{}, nil
	case "today":
		from := startOfDay(now)
		return Range{From: from, To: from.AddDate(0, 0, 1)}, nil
	case "week":
		wd := int(now.Weekday())
		if wd == 0 {
			wd = 7
		}
		mon := startOfDay(now).AddDate(0, 0, -(wd - 1))
		return Range{From: mon, To: mon.AddDate(0, 0, 7)}, nil
	case "month":
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return Range{From: from, To: from.AddDate(0, 1, 0)}, nil
	}
	if len(expr) == 4 {
		if y, err := strconv.Atoi(expr); err == nil {
			from := time.Date(y, time.January, 1, 0, 0, 0, 0, now.Location())
			return Range{From: from, To: from.AddDate(1, 0, 0)}, nil
		}
	}
	if len(expr) == 7 && expr[4] == '-' {
		if t, err := time.ParseInLocation("2006-01", expr, now.Location()); err == nil {
			return Range{From: t, To: t.AddDate(0, 1, 0)}, nil
		}
	}
	if from, to, ok := splitRange(expr); ok {
		f, err := time.ParseInLocation("2006-01-02", from, now.Location())
		if err != nil {
			return Range{}, fmt.Errorf("ungültiges From: %s", from)
		}
		t, err := time.ParseInLocation("2006-01-02", to, now.Location())
		if err != nil {
			return Range{}, fmt.Errorf("ungültiges To: %s", to)
		}
		return Range{From: f, To: t.AddDate(0, 0, 1)}, nil
	}
	return Range{}, fmt.Errorf("unbekannter Range: %s", expr)
}

func splitRange(s string) (string, string, bool) {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '.' && s[i+1] == '.' {
			return s[:i], s[i+2:], true
		}
	}
	return "", "", false
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// loadAllSessions reads the entire log without filtering.
func loadAllSessions() ([]Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return readSessions(filepath.Join(home, ".tmux", "worktime.log"), nil)
}
