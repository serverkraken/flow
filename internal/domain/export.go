package domain

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
)

// WriteCSV emits sessions as CSV with a header row. Columns:
// date, start, stop, elapsed_seconds, tag, note. Pure — caller supplies
// the slice from a port.
//
// csv.Writer is buffered; per-row Writes don't surface I/O errors at the
// call site. We rely on cw.Error() after Flush to surface them.
func WriteCSV(w io.Writer, sessions []Session) error {
	cw := csv.NewWriter(w)
	cw.Write([]string{"date", "start", "stop", "elapsed_seconds", "tag", "note"}) //nolint:errcheck // surfaced via cw.Error
	for _, s := range sessions {
		cw.Write([]string{ //nolint:errcheck // surfaced via cw.Error
			s.Date.Format("2006-01-02"),
			s.Start.Format("15:04"),
			s.Stop.Format("15:04"),
			strconv.FormatInt(int64(s.Elapsed.Seconds()), 10),
			s.Tag,
			s.Note,
		})
	}
	cw.Flush()
	return cw.Error()
}

// WriteJSON emits sessions as a pretty-printed JSON array. Each element is
// an object with keys: date, start, stop, elapsed_seconds, tag, note.
func WriteJSON(w io.Writer, sessions []Session) error {
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
