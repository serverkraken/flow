package domain

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// WriteICS writes RFC 5545 VCALENDAR data covering the given day-off
// entries to w, with `now` used as the DTSTAMP value (UTC). One VEVENT per
// day-off entry — consecutive dates of the same kind/label are NOT
// collapsed; that simplifies UID generation and lets calendars hide
// individual sick days separately.
//
// All-day events use VALUE=DATE and a DTEND that's the day after DTSTART
// (the iCal exclusive-end convention). PRODID is fixed to identify the
// generator; calendars use it for de-duplication when the file is re-imported.
func WriteICS(w io.Writer, dayoffs []DayOff, now time.Time) error {
	sorted := make([]DayOff, len(dayoffs))
	copy(sorted, dayoffs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	stamp := now.UTC().Format("20060102T150405Z")

	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//serverkraken//flow worktime//DE\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	b.WriteString("METHOD:PUBLISH\r\n")
	for _, d := range sorted {
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
		fmt.Fprintf(&b, "SUMMARY:%s\r\n", IcalEscape(summary))
		fmt.Fprintf(&b, "CATEGORIES:%s\r\n", IcalEscape(d.Kind.LabelDe()))
		// TRANSP:TRANSPARENT keeps free-busy free for holidays/vacation —
		// the calendar still shows the marker but doesn't block scheduling
		// invitations on the parallel work calendar.
		b.WriteString("TRANSP:TRANSPARENT\r\n")
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	_, err := io.WriteString(w, b.String())
	return err
}
