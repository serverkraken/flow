package webui

// EinstellungenVM is the view model for the Einstellungen page.
// It holds the Tagesziel editor state: the current default daily target
// (in minutes, as a form string) and Mon–Fri weekday overrides.
// WeekdayTargetVM is shared with StatsVM (defined in stats_vm.go).
type EinstellungenVM struct {
	DefaultTarget string           // minutes as string for the form input
	Weekdays      []WeekdayTargetVM // Mon–Fri override inputs
	Err           string
}
