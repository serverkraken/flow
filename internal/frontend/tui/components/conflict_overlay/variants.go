package conflict_overlay

import (
	"fmt"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// formatSessionEditBody builds the plain-text body for VariantSessionEdit.
// Shows a side-by-side (stacked on narrow screens) summary of the two
// sessions so the user can compare them before resolving.
//
// Example output:
//
//	Lokal:                      Server:
//	  10:00 – 11:30 · 1h30m       10:00 – 11:45 · 1h45m
//	  deep                         deep
//	  morning focus                morning focus (touched on phone)
func formatSessionEditBody(local, server domain.Session) string {
	var sb strings.Builder

	sb.WriteString("Lokal:                      Server:\n")
	fmt.Fprintf(
		&sb, "  %-28s%s\n",
		formatSessionTime(local),
		formatSessionTime(server),
	)
	if local.Tag != "" || server.Tag != "" {
		fmt.Fprintf(
			&sb, "  %-28s%s\n",
			truncate28(local.Tag),
			server.Tag,
		)
	}
	if local.Note != "" || server.Note != "" {
		fmt.Fprintf(
			&sb, "  %-28s%s\n",
			truncate28(local.Note),
			server.Note,
		)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// formatActiveRaceBody builds the plain-text body for VariantActiveRace.
// Shows what is running on the other device.
//
// Example output:
//
//	Auf einem anderen Gerät läuft bereits:
//	  Projekt: flow
//	  Gestartet: 10:23 (vor 7 min)
//	  Gerät: notebook-b
func formatActiveRaceBody(s domain.ActiveSession) string {
	var sb strings.Builder

	sb.WriteString("Auf einem anderen Gerät läuft bereits:\n")
	if s.ProjectID != "" {
		fmt.Fprintf(&sb, "  Projekt: %s\n", s.ProjectID)
	}
	since := time.Since(s.StartedAt)
	fmt.Fprintf(
		&sb, "  Gestartet: %s (vor %s)\n",
		s.StartedAt.Format("15:04"),
		formatDuration(since),
	)
	if s.StartedOnDevice != "" {
		fmt.Fprintf(&sb, "  Gerät: %s\n", s.StartedOnDevice)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// formatSessionTime returns a compact "HH:MM – HH:MM · XhYm" string
// for a session, e.g. "10:00 – 11:30 · 1h30m". Falls back to "—" when
// Start/Stop are zero.
func formatSessionTime(s domain.Session) string {
	if s.Start.IsZero() {
		return "—"
	}
	elapsed := s.Stop.Sub(s.Start)
	if s.Elapsed > 0 {
		elapsed = s.Elapsed
	}
	return fmt.Sprintf(
		"%s – %s · %s",
		s.Start.Format("15:04"),
		s.Stop.Format("15:04"),
		formatDuration(elapsed),
	)
}

// formatDuration converts a duration to a compact German string:
// "1h30m", "45m", "2h". Sub-minute durations return "< 1min".
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	switch {
	case h == 0 && m == 0:
		return "< 1min"
	case h == 0:
		return fmt.Sprintf("%dmin", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh%dmin", h, m)
	}
}

// truncate28 truncates s to 28 characters (the side-by-side column width)
// appending "…" when the original is longer. Used to keep the "Lokal"
// column from overrunning into the "Server" column.
func truncate28(s string) string {
	const maxCols = 27 // leave 1 char for ellipsis
	runes := []rune(s)
	if len(runes) <= maxCols+1 {
		return s
	}
	return string(runes[:maxCols]) + "…"
}
