package worktime

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// ExportICS writes RFC 5545 VCALENDAR data covering all day-off entries
// in [from, to] (inclusive) to w. One VEVENT per day-off entry — consecutive
// dates of the same kind/label are NOT collapsed; that simplifies UID
// generation and lets calendars hide individual sick days separately.
//
// All-day events use VALUE=DATE and DTEND that's the day after DTSTART
// (the iCal exclusive-end convention). PRODID is fixed to identify the
// generator; calendars use it for de-duplication when the file is re-imported.
func ExportICS(w io.Writer, from, to time.Time) error {
	entries := ListDayOffs(from, to)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Date.Before(entries[j].Date) })

	now := time.Now().UTC()
	stamp := now.Format("20060102T150405Z")

	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//serverkraken//flow worktime//DE\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	b.WriteString("METHOD:PUBLISH\r\n")
	for _, d := range entries {
		dtstart := d.Date.Format("20060102")
		dtend := d.Date.AddDate(0, 0, 1).Format("20060102")
		summary := d.Kind.LabelDe()
		if d.Label != "" {
			summary = d.Label + " (" + d.Kind.LabelDe() + ")"
		}
		uid := fmt.Sprintf("dayoff-%s-%s@flow.serverkraken", dtstart, d.Kind)
		b.WriteString("BEGIN:VEVENT\r\n")
		fmt.Fprintf(&b, "UID:%s\r\n", uid)
		fmt.Fprintf(&b, "DTSTAMP:%s\r\n", stamp)
		fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", dtstart)
		fmt.Fprintf(&b, "DTEND;VALUE=DATE:%s\r\n", dtend)
		fmt.Fprintf(&b, "SUMMARY:%s\r\n", icalEscape(summary))
		fmt.Fprintf(&b, "CATEGORIES:%s\r\n", icalEscape(d.Kind.LabelDe()))
		// TRANSP:TRANSPARENT means free-busy stays free for holidays/vacation —
		// the calendar still shows the marker but doesn't block scheduling
		// invitations on the parallel work calendar.
		b.WriteString("TRANSP:TRANSPARENT\r\n")
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// icalEscape replaces the characters RFC 5545 §3.3.11 requires escaped in
// TEXT-typed values: backslash, semicolon, comma, newline.
func icalEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`;`, `\;`,
		`,`, `\,`,
		"\n", `\n`,
		"\r", "",
	)
	return r.Replace(s)
}
