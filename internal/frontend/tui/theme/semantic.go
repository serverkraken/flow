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
	Accent       string // primary interactive accent
	Active       string // currently running / live thing
	Success      string
	Warning      string
	Danger       string
	Info         string // informative without action
	Highlight    string // attention-grabbing, non-actionable mark
	BorderSubtle string // light divider / panel border
	BorderStrong string // load-bearing border (modal, focused panel)
}

// Sem returns the semantic alias view of p. The mapping is fixed across
// palettes — every theme answers "what is the danger color?" with its
// own Red. The alias names are the public contract; the hue choices
// behind them are an implementation detail of each Palette.
func (p Palette) Sem() Semantic {
	return Semantic{
		Accent:       p.Blue,
		Active:       p.Cyan,
		Success:      p.Green,
		Warning:      p.Yellow,
		Danger:       p.Red,
		Info:         p.Cyan,
		Highlight:    p.Purple,
		BorderSubtle: p.BgChip,
		BorderStrong: p.FgMuted,
	}
}
