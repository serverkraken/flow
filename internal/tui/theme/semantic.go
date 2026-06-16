package theme

// Semantic is the consumer-facing view of a Palette: aliases that name
// the *role* a color plays, not the hue. Components read Semantic, not
// the raw hues — so a palette swap shifts the whole UI in lockstep
// without component code changing.
//
// Example: a confirm dialog uses Sem.Warning for the question, not
// Yellow directly; if a future palette redefines "warning" as orange,
// the confirm dialog follows automatically.
type Semantic struct {
	Accent        Color // primary interactive accent
	Active        Color // currently running / live thing
	Success       Color
	Warning       Color // mild approaching state (Endspurt-class)
	Notice        Color // softer than Danger, firmer than Warning — off-pattern but not alarming (Krank-class)
	Danger        Color
	Info          Color // informative without action
	Schedule      Color // fixed scheduled marker — calendar event, day-off Feiertag-class
	Highlight     Color // attention-grabbing, non-actionable mark (also: Urlaub-identity)
	SearchCurrent Color // the focused search hit — brighter than the Warning-toned sibling matches
	Border        Color // panel border / horizontal rule / dim separator
	BorderSubtle  Color // selection-row tint, lighter than Border
	BorderStrong  Color // load-bearing border (modal, focused panel)
}

// Sem returns the semantic alias view of p. The mapping is fixed across
// palettes — every theme answers "what is the danger color?" with its
// own Red. The alias names are the public contract; the hue choices
// behind them are an implementation detail of each Palette.
//
// Spec 2026-05-13-filled-dayoff-dots-supersede added two role tokens
// (Schedule, Notice) for the day-off kind triad so screens can stay
// inside the Sem-only screen-hue lint while still landing on distinct
// hues for Holiday/Vacation/Sick.
func (p Palette) Sem() Semantic {
	return Semantic{
		Accent:        p.Blue,
		Active:        p.Cyan,
		Success:       p.Green,
		Warning:       p.Yellow,
		Notice:        p.Orange,
		Danger:        p.Red,
		Info:          p.Cyan,
		Schedule:      p.Blue,
		Highlight:     p.Purple,
		SearchCurrent: p.Orange,
		Border:        p.BgCode,
		BorderSubtle:  p.BgChip,
		BorderStrong:  p.FgMuted,
	}
}
