package webui

import (
	"net/url"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// HomeVM is the view model for the Home landing page.
// Slice 4 adds the timer-hero fields (subset of HeuteVM used by homeHero /
// homeStartCard), keeping the section-link cards below for Tasks 2/3.
type HomeVM struct {
	// Timer-hero fields (mirror of HeuteVM subset used by homeHero/homeStartCard).
	Running     *domain.WorkSession
	RunningBase int    // running session's elapsed seconds at render (data-base seed)
	RunningName string // running session's project name
	RunningHue  string // running session's project hue ("" → blue default)
	RunningTag  string // running session's tag without '#'
	StartedAt   string // running session start time "11:58"

	LoggedDur  string // "5h 12m"
	TargetDur  string // "8h 00m"
	TargetPct  int
	TargetVar  string // hit|over|under|running
	Balance    string // "+2h 18m" / "−1h 05m"
	BalancePos bool

	Nodes   []components.NodePickerItem // bookable engagement nodes for the stop picker
	HasProj bool                        // true when at least one engagement node exists

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
