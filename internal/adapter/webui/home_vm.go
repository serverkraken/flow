package webui

import (
	"net/url"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// HomeVM is the view model for the Home landing page — a pure dashboard
// (K3 Task 5/6): saldo tiles, burndown, activity logstream, newest knowledge
// articles. The K1 shell timer widget (sidebar) owns start/stop, so HomeVM
// carries no running-session/timer-hero fields.
type HomeVM struct {
	// Saldo tiles (Heute / Woche / Monat) — fed by homeDataFor when Stats is wired.
	TodaySaldo string
	TodayPos   bool
	TodaySub   string

	WeekSaldo string
	WeekPos   bool
	WeekSub   string

	MonthSaldo string
	MonthPos   bool
	MonthSub   string

	Burndown components.BurndownVM

	// NewestDocs holds the most-recently-updated documents shown in the
	// "Zuletzt im Wissen" section (capped at 5, sorted newest-first).
	NewestDocs []DocRow

	// Logstream — activity feed (Slice 5, Task 9).
	LogEntries []ActivityRowVM // activity rows (capped at 15)
	LogClass   string          // active class filter: "" | "zeit" | "wissen" | "struktur" | "frei"
	LogActor   string          // active actor filter: "" = all
	LogActors  []string        // distinct actor refs from the current result set

	Err string // inline error message (surfaced when stop fails validation)
}

// logQuery returns the current class/actor filter as a URL query string
// (prefixed with "?") so the logstream section can include it in hx-get.
func (vm HomeVM) logQuery() string {
	q := url.Values{}
	if vm.LogClass != "" {
		q.Set("class", vm.LogClass)
	}
	if vm.LogActor != "" {
		q.Set("actor", vm.LogActor)
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// logstreamHref builds the /ui/home/logstream URL with optional class and actor
// query params, properly URL-encoded. Either param may be empty ("" = omit).
func logstreamHref(class, actor string) string {
	v := url.Values{}
	if class != "" {
		v.Set("class", class)
	}
	if actor != "" {
		v.Set("actor", actor)
	}
	if len(v) == 0 {
		return "/ui/home/logstream"
	}
	return "/ui/home/logstream?" + v.Encode()
}
