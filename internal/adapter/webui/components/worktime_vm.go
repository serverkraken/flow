package components

import "strconv"

// itoa is a tiny helper so .templ files can interpolate ints without importing
// strconv (templ files must not add extra imports beyond what they reference).
func itoa(n int) string { return strconv.Itoa(n) }

// SessionRowVM drives a single agenda/list row for a worktime session.
type SessionRowVM struct {
	ID         string
	Title      string // project name or i18n "ohne Projekt"
	Hue        string // project hue; "" → unassigned styling
	Glyph      string // project glyph; "○" for unassigned
	Tag        string // without leading '#'; "" hides chip
	TimeRange  string // "08:30–10:00"
	Duration   string // "1h 30m"
	Unassigned bool
	Running    bool
	Selectable bool // render the row checkbox (bulk mode)
}

// SessionBlockVM drives a positioned block in the day/week time grid.
type SessionBlockVM struct {
	ID         string
	TopPx      int // (start - windowFloor) minutes / 60 * 48
	HeightPx   int // duration minutes / 60 * 48 (min 24)
	Hue        string
	Glyph      string
	Title      string
	TimeRange  string
	Tag        string
	Unassigned bool
	Running    bool
	Size       string // "" | "sm" | "md" (drives detail reveal; see mockup .block-sm/.block-md)
}

// KennzahlenVM drives the week summary metrics panel.
type KennzahlenVM struct {
	AvgPerDay  string // "7h 04m"
	GoalsHit   int    // X
	GoalsTotal int    // 5
	Balance    string // "+2h 18m" / "−1h 05m"
	BalancePos bool
	Dots       []PaceDot
	OnTrack    bool // true → "auf Kurs", false → "Rückstand"
}

// WeekTotalVM drives the WOCHE GESAMT banner.
type WeekTotalVM struct {
	Total   string // "33h 41m"
	Target  string // "40h 00m"
	Pct     int
	Variant string // hit|over|under|running (for the bar)
}

// FuzzyPickerVM drives the project fuzzy picker dropdown.
type FuzzyPickerVM struct {
	ID       string // dom id for the picker container
	Projects []FuzzyProjectVM
	FormID   string // the form whose hidden projectId/newProject fields this writes
}

// FuzzyProjectVM is one selectable project row in the picker.
type FuzzyProjectVM struct {
	ID    string
	Name  string
	Hue   string
	Glyph string
	Rate  string // "95 €/h" or "—"
}

// SelectionBarVM drives the sticky bulk-selection action bar.
type SelectionBarVM struct {
	Picker    FuzzyPickerVM
	AssignURL string // POST target for reassign
	DeleteURL string // POST target for bulk-delete
}

// PaceDot is one day's pace state in the KENNZAHLEN dot strip.
type PaceDot struct{ State string } // behind|ontrack|ahead|running|holiday|off

// SegOption is one tab in a SegToggle segmented control.
type SegOption struct{ Key, LabelKey, Href string }
